package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"diana-contabilitate/backend/ent"
	"diana-contabilitate/backend/ent/contractmatchcandidate"
	"diana-contabilitate/backend/ent/invoice"
	"diana-contabilitate/backend/ent/lineclassification"
	entvalidationtask "diana-contabilitate/backend/ent/validationtask"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/audit"
	"diana-contabilitate/backend/internal/validationtasks"
)

func (s *Store) ListValidationTasks(ctx context.Context, filter validationtasks.Filter) ([]validationtasks.InboxItem, error) {
	query := s.Client.ValidationTask.Query().WithInvoice().WithClient().WithContractMatchRun(func(runQuery *ent.ContractMatchRunQuery) {
		runQuery.WithCandidates(func(candidateQuery *ent.ContractMatchCandidateQuery) {
			candidateQuery.Order(ent.Asc(contractmatchcandidate.FieldRank)).WithContract()
		})
	})
	if filter.ClientID != "" {
		query.Where(entvalidationtask.ClientIDEQ(filter.ClientID))
	}
	if filter.InvoiceID != "" {
		query.Where(entvalidationtask.InvoiceIDEQ(filter.InvoiceID))
	}
	if filter.Type != nil {
		query.Where(entvalidationtask.TaskTypeEQ(entvalidationtask.TaskType(*filter.Type)))
	}
	if filter.Status != nil {
		query.Where(entvalidationtask.StatusEQ(entvalidationtask.Status(*filter.Status)))
	}
	rows, err := query.Order(ent.Desc(entvalidationtask.FieldCreatedAt), ent.Desc(entvalidationtask.FieldID)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list validation tasks: %w", err)
	}
	result := make([]validationtasks.InboxItem, 0, len(rows))
	for _, row := range rows {
		invoiceRow, invoiceErr := row.Edges.InvoiceOrErr()
		clientRow, clientErr := row.Edges.ClientOrErr()
		if invoiceErr != nil || clientErr != nil {
			return nil, fmt.Errorf("load validation task context: invoice=%v client=%v", invoiceErr, clientErr)
		}
		task := *validationTaskDomain(row)
		if task.Type == validationtasks.TypeClassification {
			classificationRows, classificationErr := s.Client.LineClassification.Query().
				Where(lineclassification.InvoiceIDEQ(row.InvoiceID), lineclassification.RequiredReviewEQ(true)).
				WithInvoiceLine().WithRuleVersion(func(query *ent.RuleVersionQuery) { query.WithRule() }).
				Order(ent.Asc(lineclassification.FieldInvoiceLineID), ent.Asc(lineclassification.FieldDimension)).All(ctx)
			if classificationErr != nil {
				return nil, classificationErr
			}
			for _, classificationRow := range classificationRows {
				lineRow, edgeErr := classificationRow.Edges.InvoiceLineOrErr()
				if edgeErr != nil {
					return nil, edgeErr
				}
				task.ClassificationItems = append(task.ClassificationItems, lineClassificationDomain(classificationRow, lineRow))
			}
		}
		result = append(result, validationtasks.InboxItem{
			Task: task,
			Invoice: validationtasks.InvoiceSummary{
				ID: invoiceRow.ID, ClientID: invoiceRow.ClientID, SupplierName: invoiceRow.SupplierName,
				SupplierCUI: invoiceRow.SupplierCui, DocumentNumber: invoiceRow.DocumentNumber,
				IssueDate: invoiceRow.IssueDate, TotalAmount: invoiceRow.TotalAmount, Currency: invoiceRow.Currency,
				SPVReference: invoiceRow.SpvReference, PipelineStatus: string(invoiceRow.PipelineStatus), SagaStatus: string(invoiceRow.SagaStatus),
			},
			Client: validationtasks.ClientSummary{ID: clientRow.ID, Name: clientRow.Name, CUI: clientRow.Cui},
		})
	}
	return result, nil
}

func (s *Store) CreateBlockingTask(ctx context.Context, command validationtasks.CreationCommand, definition validationtasks.CreationDefinition, now time.Time) (*validationtasks.Task, bool, error) {
	creationKey := "task-create:" + command.CommandID
	if existing, err := s.taskByCreationKey(ctx, creationKey); err == nil {
		return existing, false, nil
	} else if !errors.Is(err, apperrors.ErrNotFound) {
		return nil, false, err
	}

	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return nil, false, err
	}
	rollback := func(cause error) (*validationtasks.Task, bool, error) {
		_ = tx.Rollback()
		return nil, false, cause
	}
	invoiceRow, err := tx.Invoice.UpdateOneID(command.InvoiceID).
		Where(invoice.PipelineStatusEQ(invoice.PipelineStatus(definition.InvoiceFrom)), invoice.RevisionEQ(command.ExpectedInvoiceRevision)).
		SetPipelineStatus(invoice.PipelineStatus(definition.InvoiceTo)).AddRevision(1).SetUpdatedAt(now).Save(ctx)
	if ent.IsNotFound(err) {
		_ = tx.Rollback()
		if existing, lookupErr := s.taskByCreationKey(ctx, creationKey); lookupErr == nil {
			return existing, false, nil
		}
		exists, lookupErr := s.Client.Invoice.Query().Where(invoice.IDEQ(command.InvoiceID)).Exist(ctx)
		if lookupErr != nil {
			return nil, false, lookupErr
		}
		if !exists {
			return nil, false, apperrors.ErrNotFound
		}
		return nil, false, apperrors.ErrConflict
	}
	if err != nil {
		return rollback(err)
	}
	taskID := command.ID
	if taskID == "" {
		taskID = stableID("task", creationKey)
	}
	create := tx.ValidationTask.Create().SetID(taskID).SetClientID(invoiceRow.ClientID).SetInvoiceID(invoiceRow.ID).
		SetTaskType(entvalidationtask.TaskType(definition.TaskType)).SetStatus(entvalidationtask.StatusOPEN).
		SetTitle(command.Title).SetReason(command.Reason).SetCreatedByKind(entvalidationtask.CreatedByKind(command.Actor)).
		SetCreationKey(creationKey).SetRevision(1).SetCreatedAt(now).SetUpdatedAt(now)
	if command.ContractMatchRunID != "" {
		create.SetContractMatchRunID(command.ContractMatchRunID)
	}
	if command.BlockerCode != nil {
		create.SetBlockerCode(*command.BlockerCode)
	}
	if command.ActorID != "" {
		create.SetCreatedByID(command.ActorID)
	}
	if command.ActorDisplay != "" {
		create.SetCreatedByDisplay(command.ActorDisplay)
	}
	if _, err = create.Save(ctx); err != nil {
		_ = tx.Rollback()
		if IsConstraintError(err) {
			if existing, lookupErr := s.taskByCreationKey(ctx, creationKey); lookupErr == nil {
				return existing, false, nil
			}
			return nil, false, apperrors.ErrConflict
		}
		return nil, false, err
	}
	if err = createAudit(tx, ctx, auditRecord{
		key: creationKey, invoiceID: invoiceRow.ID, taskID: taskID, clientID: invoiceRow.ClientID,
		eventType: "VALIDATION_TASK_CREATED", from: "", to: string(validationtasks.StatusOpen), trigger: "BLOCKING_TASK_CREATED",
		detail: fmt.Sprintf("Blocking %s task created; invoice moved from %s to %s.", definition.TaskType, definition.InvoiceFrom, definition.InvoiceTo),
		actor:  command.Actor, actorID: command.ActorID, actorDisplay: command.ActorDisplay, correlationID: command.CorrelationID, at: now,
	}); err != nil {
		return rollback(err)
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	result, err := s.taskByID(ctx, taskID)
	return result, true, err
}

func (s *Store) RequestMissingContract(ctx context.Context, command validationtasks.RequestMissingContractCommand, now time.Time) (*validationtasks.Task, bool, error) {
	eventKey := "task-request:" + command.CommandID
	if exists, err := s.auditExists(ctx, eventKey); err != nil {
		return nil, false, err
	} else if exists {
		item, getErr := s.taskByID(ctx, command.TaskID)
		return item, false, getErr
	}

	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return nil, false, err
	}
	rollback := func(cause error) (*validationtasks.Task, bool, error) {
		_ = tx.Rollback()
		return nil, false, cause
	}
	taskRow, err := tx.ValidationTask.Query().Where(entvalidationtask.IDEQ(command.TaskID)).Only(ctx)
	if ent.IsNotFound(err) {
		return rollback(apperrors.ErrNotFound)
	}
	if err != nil {
		return rollback(err)
	}
	if taskRow.InvoiceID != command.InvoiceID || taskRow.TaskType != entvalidationtask.TaskTypeMISSING_CONTRACT {
		return rollback(apperrors.ErrValidation)
	}
	if taskRow.Status == entvalidationtask.StatusRESOLVED {
		return rollback(validationtasks.ErrTaskAlreadyResolved)
	}
	if taskRow.Status != entvalidationtask.StatusOPEN || taskRow.Revision != command.ExpectedRevision {
		_ = tx.Rollback()
		if exists, checkErr := s.auditExists(ctx, eventKey); checkErr == nil && exists {
			item, getErr := s.taskByID(ctx, command.TaskID)
			return item, false, getErr
		}
		return nil, false, apperrors.ErrConflict
	}
	invoiceRow, err := tx.Invoice.Query().Where(invoice.IDEQ(command.InvoiceID)).Only(ctx)
	if ent.IsNotFound(err) {
		return rollback(apperrors.ErrNotFound)
	}
	if err != nil {
		return rollback(err)
	}
	if invoiceRow.ClientID != taskRow.ClientID || invoiceRow.PipelineStatus != invoice.PipelineStatusAWAITING_CONTRACT {
		return rollback(apperrors.ErrValidation)
	}
	updated, err := tx.ValidationTask.UpdateOneID(taskRow.ID).
		Where(entvalidationtask.StatusEQ(entvalidationtask.StatusOPEN), entvalidationtask.RevisionEQ(command.ExpectedRevision)).
		SetStatus(entvalidationtask.StatusWAITING).SetWaitingSince(now).AddRevision(1).SetUpdatedAt(now).Save(ctx)
	if ent.IsNotFound(err) {
		_ = tx.Rollback()
		if exists, checkErr := s.auditExists(ctx, eventKey); checkErr == nil && exists {
			item, getErr := s.taskByID(ctx, command.TaskID)
			return item, false, getErr
		}
		return nil, false, apperrors.ErrConflict
	}
	if err != nil {
		return rollback(err)
	}
	if err = createAudit(tx, ctx, auditRecord{
		key: eventKey, invoiceID: invoiceRow.ID, taskID: updated.ID, clientID: invoiceRow.ClientID,
		eventType: "MISSING_CONTRACT_REQUESTED", from: string(validationtasks.StatusOpen), to: string(validationtasks.StatusWaiting),
		trigger: string(validationtasks.TriggerContractRequested), detail: "Contract requested; invoice remains blocked in AWAITING_CONTRACT.",
		actor: audit.ActorUser, actorID: command.ActorID, actorDisplay: command.ActorDisplay, correlationID: command.CorrelationID, at: now,
	}); err != nil {
		return rollback(err)
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	return validationTaskDomain(updated), true, nil
}

func (s *Store) taskByCreationKey(ctx context.Context, key string) (*validationtasks.Task, error) {
	row, err := s.Client.ValidationTask.Query().Where(entvalidationtask.CreationKeyEQ(key)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return validationTaskDomain(row), nil
}

func (s *Store) taskByID(ctx context.Context, id string) (*validationtasks.Task, error) {
	row, err := s.Client.ValidationTask.Query().Where(entvalidationtask.IDEQ(id)).WithContractMatchRun(func(runQuery *ent.ContractMatchRunQuery) {
		runQuery.WithCandidates(func(candidateQuery *ent.ContractMatchCandidateQuery) {
			candidateQuery.Order(ent.Asc(contractmatchcandidate.FieldRank)).WithContract()
		})
	}).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return validationTaskDomain(row), nil
}

func validationTaskDomain(row *ent.ValidationTask) *validationtasks.Task {
	result := &validationtasks.Task{
		ID: row.ID, ClientID: row.ClientID, InvoiceID: row.InvoiceID,
		Type: validationtasks.Type(row.TaskType), Status: validationtasks.Status(row.Status),
		Title: row.Title, Reason: row.Reason, BlockerCode: row.BlockerCode,
		ResolutionMetadata: row.ResolutionMetadata, CreatedByKind: audit.ActorKind(row.CreatedByKind),
		CreatedByID: row.CreatedByID, CreatedByDisplay: row.CreatedByDisplay,
		Revision: row.Revision, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		WaitingSince: row.WaitingSince, ResolvedAt: row.ResolvedAt,
		ContractMatchRunID: row.ContractMatchRunID,
	}
	if run := row.Edges.ContractMatchRun; run != nil {
		for _, candidate := range run.Edges.Candidates {
			contract := candidate.Edges.Contract
			if contract == nil {
				continue
			}
			result.ContractCandidates = append(result.ContractCandidates, validationtasks.ContractCandidate{
				ID: contract.ID, Reference: contract.Reference, SupplierName: contract.SupplierName,
				EffectiveFrom: contract.EffectiveFrom, EffectiveTo: contract.EffectiveTo,
				ValueAmount: contract.TotalValue, Currency: contract.Currency,
				Confidence: candidate.ConfidenceDisplay, Reasons: append([]string(nil), candidate.Reasons...),
				UnitType: contract.UnitType, PaymentTerms: contract.PaymentTerms,
				Recommended: candidate.Recommended, Compatibility: string(candidate.Compatibility),
			})
		}
	}
	return result
}
