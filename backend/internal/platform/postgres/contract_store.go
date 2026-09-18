package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"diana-contabilitate/backend/ent"
	"diana-contabilitate/backend/ent/contract"
	"diana-contabilitate/backend/ent/contractmatchcandidate"
	"diana-contabilitate/backend/ent/contractmatchrun"
	"diana-contabilitate/backend/ent/contractserviceterm"
	"diana-contabilitate/backend/ent/invoice"
	"diana-contabilitate/backend/ent/invoicecontractassociation"
	"diana-contabilitate/backend/ent/outboxentry"
	"diana-contabilitate/backend/ent/predicate"
	entvalidationtask "diana-contabilitate/backend/ent/validationtask"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/audit"
	"diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/money"
	"diana-contabilitate/backend/internal/validationtasks"
)

func (s *Store) ListContracts(ctx context.Context, filter contracts.Filter) ([]contracts.Contract, error) {
	query := s.Client.Contract.Query().Where(contract.LifecycleStateEQ(contract.LifecycleStateACTIVE)).WithServiceTerms(func(q *ent.ContractServiceTermQuery) { q.Order(ent.Asc(contractserviceterm.FieldPosition)) })
	if filter.ClientID != "" {
		query.Where(contract.ClientIDEQ(filter.ClientID))
	}
	if value := strings.TrimSpace(filter.Query); value != "" {
		query.Where(contract.Or(contract.ReferenceContainsFold(value), contract.SupplierNameContainsFold(value)))
	}
	rows, err := query.Order(ent.Asc(contract.FieldReference)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list contracts: %w", err)
	}
	result := make([]contracts.Contract, 0, len(rows))
	for _, row := range rows {
		item, convertErr := contractDomain(row)
		if convertErr != nil {
			return nil, convertErr
		}
		result = append(result, *item)
	}
	return result, nil
}

func (s *Store) GetContract(ctx context.Context, id string) (*contracts.Contract, error) {
	row, err := s.Client.Contract.Query().Where(contract.IDEQ(id), contract.LifecycleStateEQ(contract.LifecycleStateACTIVE)).WithServiceTerms(func(q *ent.ContractServiceTermQuery) { q.Order(ent.Asc(contractserviceterm.FieldPosition)) }).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get contract: %w", err)
	}
	return contractDomain(row)
}

func (s *Store) ListContractInvoices(ctx context.Context, contractID string) ([]contracts.AssociatedInvoice, error) {
	exists, err := s.Client.Contract.Query().Where(contract.IDEQ(contractID)).Exist(ctx)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, apperrors.ErrNotFound
	}
	rows, err := s.Client.InvoiceContractAssociation.Query().Where(invoicecontractassociation.ContractIDEQ(contractID)).WithInvoice().Order(ent.Desc(invoicecontractassociation.FieldAssociatedAt)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list contract invoices: %w", err)
	}
	result := make([]contracts.AssociatedInvoice, 0, len(rows))
	for _, association := range rows {
		row, edgeErr := association.Edges.InvoiceOrErr()
		if edgeErr != nil {
			return nil, edgeErr
		}
		amount, parseErr := money.Parse(row.TotalAmount)
		if parseErr != nil {
			return nil, parseErr
		}
		result = append(result, contracts.AssociatedInvoice{
			ID: row.ID, ClientID: row.ClientID, SupplierName: row.SupplierName, DocumentNumber: row.DocumentNumber,
			IssueDate: row.IssueDate, Total: money.Money{Amount: amount, Currency: row.Currency}, SPVReference: row.SpvReference,
			PipelineStatus: string(row.PipelineStatus), SagaStatus: string(row.SagaStatus),
		})
	}
	return result, nil
}

func (s *Store) LoadMatchingInput(ctx context.Context, invoiceID string) (contracts.InvoiceContext, []contracts.Contract, error) {
	row, err := s.Client.Invoice.Query().Where(invoice.IDEQ(invoiceID)).Only(ctx)
	if ent.IsNotFound(err) {
		return contracts.InvoiceContext{}, nil, apperrors.ErrNotFound
	}
	if err != nil {
		return contracts.InvoiceContext{}, nil, err
	}
	input := contracts.InvoiceContext{
		ID: row.ID, ClientID: row.ClientID, NormalizedSupplierCUI: row.NormalizedSupplierCui,
		IssueDay: row.IssueDay, Currency: row.Currency, PipelineStatus: string(row.PipelineStatus), Revision: row.Revision,
	}
	if row.NormalizedSupplierCui == nil {
		return input, nil, nil
	}
	items, err := s.ListContracts(ctx, contracts.Filter{ClientID: row.ClientID})
	if err != nil {
		return contracts.InvoiceContext{}, nil, err
	}
	matched := make([]contracts.Contract, 0)
	for _, item := range items {
		if item.NormalizedSupplierCUI == *row.NormalizedSupplierCui {
			matched = append(matched, item)
		}
	}
	return input, matched, nil
}

func (s *Store) MatchCommandCommitted(ctx context.Context, commandID string) (bool, error) {
	return s.Client.ContractMatchRun.Query().Where(contractmatchrun.CommandKeyEQ("contract-match:" + commandID)).Exist(ctx)
}

func (s *Store) ResumeCommandCommitted(ctx context.Context, commandID string) (bool, error) {
	return s.Client.ContractMatchRun.Query().Where(contractmatchrun.CommandKeyEQ("contract-resume:" + commandID)).Exist(ctx)
}

func (s *Store) RecordContractAvailable(ctx context.Context, command contracts.AvailableCommand, now time.Time) (bool, error) {
	key := "contract-available:" + command.CommandID
	if exists, err := s.auditExists(ctx, key); err != nil {
		return false, err
	} else if exists {
		return false, nil
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return false, err
	}
	rollback := func(cause error) (bool, error) { _ = tx.Rollback(); return false, cause }
	row, err := tx.Contract.Query().Where(contract.IDEQ(command.ContractID)).Only(ctx)
	if ent.IsNotFound(err) {
		return rollback(apperrors.ErrNotFound)
	}
	if err != nil {
		return rollback(err)
	}
	if err = createAudit(tx, ctx, auditRecord{
		key: key, clientID: row.ClientID, aggregateType: "CONTRACT", aggregateID: row.ID,
		eventType: "CONTRACT_AVAILABLE_FOR_MATCHING", trigger: "CONTRACT_AVAILABLE",
		detail: fmt.Sprintf("Contractul %s a devenit disponibil pentru asocierea automată.", row.Reference),
		actor:  audit.ActorSystem, actorDisplay: "Sistem contracte", correlationID: command.CorrelationID, at: now,
	}); err != nil {
		if IsConstraintError(err) {
			_ = tx.Rollback()
			return false, nil
		}
		return rollback(err)
	}
	payload, _ := json.Marshal(map[string]string{"contract_id": row.ID})
	outboxKey := key + ":resume"
	create := tx.OutboxEntry.Create().SetID(stableID("out", outboxKey)).SetEventType("CONTRACT_AVAILABLE").
		SetAggregateType("CONTRACT").SetAggregateID(row.ID).SetPayload(payload).SetIdempotencyKey(outboxKey).
		SetStatus(outboxentry.StatusPENDING).SetCreatedAt(now).SetAvailableAt(now)
	if command.CorrelationID != "" {
		create.SetCorrelationID(command.CorrelationID)
	}
	if _, err = create.Save(ctx); err != nil {
		return rollback(err)
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ListBlockedInvoicesForContract(ctx context.Context, contractID, afterID string, limit int) ([]contracts.BlockedInvoice, error) {
	if limit <= 0 {
		return nil, apperrors.ErrValidation
	}
	available, err := s.Client.Contract.Query().Where(contract.IDEQ(contractID)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	predicates := []predicate.Invoice{
		invoice.ClientIDEQ(available.ClientID),
		invoice.PipelineStatusEQ(invoice.PipelineStatusAWAITING_CONTRACT),
		invoice.NormalizedSupplierCuiEQ(available.NormalizedSupplierCui),
		invoice.HasValidationTasksWith(
			entvalidationtask.TaskTypeEQ(entvalidationtask.TaskTypeMISSING_CONTRACT),
			entvalidationtask.StatusIn(entvalidationtask.StatusOPEN, entvalidationtask.StatusWAITING),
		),
	}
	if afterID != "" {
		predicates = append(predicates, invoice.IDGT(afterID))
	}
	rows, err := s.Client.Invoice.Query().Where(predicates...).Order(ent.Asc(invoice.FieldID)).Limit(limit).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]contracts.BlockedInvoice, 0, len(rows))
	for _, row := range rows {
		result = append(result, contracts.BlockedInvoice{ID: row.ID, Revision: row.Revision})
	}
	return result, nil
}

func (s *Store) ApplyMatchDecision(ctx context.Context, command contracts.MatchCommand, decision contracts.MatchDecision, now time.Time) (bool, error) {
	commandKey := "contract-match:" + command.CommandID
	if exists, err := s.Client.ContractMatchRun.Query().Where(contractmatchrun.CommandKeyEQ(commandKey)).Exist(ctx); err != nil {
		return false, err
	} else if exists {
		return false, nil
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return false, err
	}
	rollback := func(cause error) (bool, error) { _ = tx.Rollback(); return false, cause }
	to := invoice.PipelineStatusAWAITING_MATCH_CONFIRM
	if decision.Outcome == contracts.OutcomeUniqueCompatible {
		to = invoice.PipelineStatusDEDUPE_CHECKED
	} else if decision.Outcome == contracts.OutcomeNoMatch {
		to = invoice.PipelineStatusAWAITING_CONTRACT
	}
	invoiceRow, err := tx.Invoice.UpdateOneID(command.InvoiceID).
		Where(invoice.PipelineStatusEQ(invoice.PipelineStatusMATCHING), invoice.RevisionEQ(command.ExpectedRevision)).
		SetPipelineStatus(to).AddRevision(1).SetUpdatedAt(now).Save(ctx)
	if ent.IsNotFound(err) {
		_ = tx.Rollback()
		if exists, checkErr := s.Client.ContractMatchRun.Query().Where(contractmatchrun.CommandKeyEQ(commandKey)).Exist(ctx); checkErr == nil && exists {
			return false, nil
		}
		return false, apperrors.ErrConflict
	}
	if err != nil {
		return rollback(err)
	}
	runID := stableID("cmr", commandKey)
	_, err = tx.ContractMatchRun.Create().SetID(runID).SetClientID(invoiceRow.ClientID).SetInvoiceID(invoiceRow.ID).
		SetPolicyVersion(decision.PolicyVersion).SetOutcome(contractmatchrun.Outcome(decision.Outcome)).
		SetInvoiceRevision(invoiceRow.Revision).SetCommandKey(commandKey).SetCreatedAt(now).Save(ctx)
	if err != nil {
		if IsConstraintError(err) {
			return rollback(apperrors.ErrConflict)
		}
		return rollback(err)
	}
	contractRows := make(map[string]*ent.Contract, len(decision.Candidates))
	for _, candidate := range decision.Candidates {
		contractRow, queryErr := tx.Contract.Query().Where(contract.IDEQ(candidate.ContractID), contract.ClientIDEQ(invoiceRow.ClientID), contract.RevisionEQ(candidate.ContractRevision)).Only(ctx)
		if ent.IsNotFound(queryErr) {
			return rollback(contracts.ErrStaleMatchResult)
		}
		if queryErr != nil {
			return rollback(queryErr)
		}
		contractRows[candidate.ContractID] = contractRow
		_, err = tx.ContractMatchCandidate.Create().SetID(stableID("cmc", runID+":"+candidate.ContractID)).
			SetClientID(invoiceRow.ClientID).SetMatchRunID(runID).SetContractID(candidate.ContractID).
			SetContractRevision(candidate.ContractRevision).SetRank(candidate.Rank).SetRecommended(candidate.Recommended).
			SetCompatibility(contractmatchcandidate.Compatibility(candidate.Compatibility)).SetConfidenceDisplay(candidate.Confidence).
			SetReasons(candidate.Reasons).SetCreatedAt(now).Save(ctx)
		if err != nil {
			return rollback(err)
		}
	}
	if err = createAudit(tx, ctx, auditRecord{
		key: commandKey + ":executed", invoiceID: invoiceRow.ID, clientID: invoiceRow.ClientID,
		eventType: "CONTRACT_MATCHING_EXECUTED", from: string(invoice.PipelineStatusMATCHING), to: string(to), trigger: string(invoicing.TriggerMatchingDecision),
		detail: fmt.Sprintf("Contract matching produced %s using policy %s.", decision.Outcome, decision.PolicyVersion), actor: audit.ActorSystem,
		actorDisplay: "Sistem matching", correlationID: command.CorrelationID, at: now,
	}); err != nil {
		return rollback(err)
	}

	switch decision.Outcome {
	case contracts.OutcomeUniqueCompatible:
		selected := contractRows[decision.Candidates[0].ContractID]
		if err = createAssociation(ctx, tx, invoiceRow, selected, runID, decision.PolicyVersion, contracts.AssociationAutomatic, "", "Sistem matching", now); err != nil {
			return rollback(err)
		}
		if err = createAudit(tx, ctx, auditRecord{
			key: commandKey + ":associated", invoiceID: invoiceRow.ID, clientID: invoiceRow.ClientID,
			eventType: "CONTRACT_AUTO_ASSOCIATED", from: selected.Reference, to: selected.ID, trigger: "UNIQUE_COMPATIBLE",
			detail: fmt.Sprintf("Contract %s associated automatically; policy=%s; exact supplier CUI, effective period and currency evidence.", selected.Reference, decision.PolicyVersion),
			actor:  audit.ActorSystem, actorDisplay: "Sistem matching", correlationID: command.CorrelationID, at: now,
		}); err != nil {
			return rollback(err)
		}
		if err = createOutbox(tx, ctx, invoiceRow.ID, commandKey+":continue", command.CorrelationID, now); err != nil {
			return rollback(err)
		}
	case contracts.OutcomeMultiplePlausible, contracts.OutcomeUniqueIncompatible:
		reason := "Au fost identificate mai multe contracte candidate; este necesară confirmarea contabilului."
		if decision.Outcome == contracts.OutcomeUniqueIncompatible {
			reason = "Singurul contract identificat are semnale incompatibile și nu poate fi asociat automat."
		}
		taskID := stableID("task", commandKey+":review")
		_, err = tx.ValidationTask.Create().SetID(taskID).SetClientID(invoiceRow.ClientID).SetInvoiceID(invoiceRow.ID).SetContractMatchRunID(runID).
			SetTaskType(entvalidationtask.TaskTypeCONTRACT_MATCH).SetStatus(entvalidationtask.StatusOPEN).
			SetTitle("Confirmă contractul").SetReason(reason).SetCreatedByKind(entvalidationtask.CreatedByKindSYSTEM).
			SetCreatedByDisplay("Sistem matching").SetCreationKey(commandKey + ":review").SetRevision(1).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
		if err != nil {
			if IsConstraintError(err) {
				return rollback(apperrors.ErrConflict)
			}
			return rollback(err)
		}
		if err = createAudit(tx, ctx, auditRecord{
			key: commandKey + ":task", invoiceID: invoiceRow.ID, taskID: taskID, clientID: invoiceRow.ClientID,
			eventType: "VALIDATION_TASK_CREATED", to: string(validationtasks.StatusOpen), trigger: "CONTRACT_MATCH_REVIEW_REQUIRED",
			detail: reason, actor: audit.ActorSystem, actorDisplay: "Sistem matching", correlationID: command.CorrelationID, at: now,
		}); err != nil {
			return rollback(err)
		}
	case contracts.OutcomeNoMatch:
		taskID := stableID("task", commandKey+":missing")
		_, err = tx.ValidationTask.Create().SetID(taskID).SetClientID(invoiceRow.ClientID).SetInvoiceID(invoiceRow.ID).
			SetTaskType(entvalidationtask.TaskTypeMISSING_CONTRACT).SetStatus(entvalidationtask.StatusOPEN).
			SetTitle("Contract lipsă").SetReason("Nu există un contract disponibil pentru asociere.").SetCreatedByKind(entvalidationtask.CreatedByKindSYSTEM).
			SetCreatedByDisplay("Sistem matching").SetCreationKey(commandKey + ":missing").SetRevision(1).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
		if err != nil {
			if IsConstraintError(err) {
				return rollback(apperrors.ErrConflict)
			}
			return rollback(err)
		}
		if err = createAudit(tx, ctx, auditRecord{
			key: commandKey + ":task", invoiceID: invoiceRow.ID, taskID: taskID, clientID: invoiceRow.ClientID,
			eventType: "VALIDATION_TASK_CREATED", to: string(validationtasks.StatusOpen), trigger: "MISSING_CONTRACT",
			detail: "No contract matched the baseline discovery identity; invoice remains blocked.", actor: audit.ActorSystem,
			actorDisplay: "Sistem matching", correlationID: command.CorrelationID, at: now,
		}); err != nil {
			return rollback(err)
		}
	default:
		return rollback(apperrors.ErrValidation)
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ApplyResumeDecision(ctx context.Context, command contracts.ResumeCommand, decision contracts.MatchDecision, now time.Time) (bool, error) {
	commandKey := "contract-resume:" + command.CommandID
	if exists, err := s.Client.ContractMatchRun.Query().Where(contractmatchrun.CommandKeyEQ(commandKey)).Exist(ctx); err != nil {
		return false, err
	} else if exists {
		return false, nil
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return false, err
	}
	rollback := func(cause error) (bool, error) { _ = tx.Rollback(); return false, cause }

	invoiceRow, err := tx.Invoice.Query().Where(invoice.IDEQ(command.InvoiceID)).Only(ctx)
	if ent.IsNotFound(err) {
		return rollback(apperrors.ErrNotFound)
	}
	if err != nil {
		return rollback(err)
	}
	if invoiceRow.PipelineStatus != invoice.PipelineStatusAWAITING_CONTRACT || invoiceRow.Revision != command.ExpectedRevision {
		return rollback(apperrors.ErrConflict)
	}
	missingTask, err := tx.ValidationTask.Query().Where(
		entvalidationtask.InvoiceIDEQ(invoiceRow.ID),
		entvalidationtask.TaskTypeEQ(entvalidationtask.TaskTypeMISSING_CONTRACT),
		entvalidationtask.StatusIn(entvalidationtask.StatusOPEN, entvalidationtask.StatusWAITING),
	).Only(ctx)
	if ent.IsNotFound(err) {
		return rollback(apperrors.ErrConflict)
	}
	if err != nil {
		return rollback(err)
	}
	if missingTask.ClientID != invoiceRow.ClientID {
		return rollback(apperrors.ErrConflict)
	}

	from := invoiceRow.PipelineStatus
	to := invoiceRow.PipelineStatus
	if decision.Outcome == contracts.OutcomeUniqueCompatible {
		to = invoice.PipelineStatusDEDUPE_CHECKED
	} else if decision.Outcome == contracts.OutcomeMultiplePlausible || decision.Outcome == contracts.OutcomeUniqueIncompatible {
		to = invoice.PipelineStatusAWAITING_MATCH_CONFIRM
	}
	if to != from {
		invoiceRow, err = tx.Invoice.UpdateOneID(invoiceRow.ID).
			Where(invoice.PipelineStatusEQ(invoice.PipelineStatusAWAITING_CONTRACT), invoice.RevisionEQ(command.ExpectedRevision)).
			SetPipelineStatus(to).AddRevision(1).SetUpdatedAt(now).Save(ctx)
		if ent.IsNotFound(err) {
			return rollback(apperrors.ErrConflict)
		}
		if err != nil {
			return rollback(err)
		}
	}

	runID := stableID("cmr", commandKey)
	_, err = tx.ContractMatchRun.Create().SetID(runID).SetClientID(invoiceRow.ClientID).SetInvoiceID(invoiceRow.ID).
		SetPolicyVersion(decision.PolicyVersion).SetOutcome(contractmatchrun.Outcome(decision.Outcome)).
		SetInvoiceRevision(invoiceRow.Revision).SetCommandKey(commandKey).SetCreatedAt(now).Save(ctx)
	if err != nil {
		if IsConstraintError(err) {
			_ = tx.Rollback()
			if exists, checkErr := s.Client.ContractMatchRun.Query().Where(contractmatchrun.CommandKeyEQ(commandKey)).Exist(ctx); checkErr == nil && exists {
				return false, nil
			}
			return false, apperrors.ErrConflict
		}
		return rollback(err)
	}
	contractRows := make(map[string]*ent.Contract, len(decision.Candidates))
	for _, candidate := range decision.Candidates {
		contractRow, queryErr := tx.Contract.Query().Where(
			contract.IDEQ(candidate.ContractID), contract.ClientIDEQ(invoiceRow.ClientID), contract.RevisionEQ(candidate.ContractRevision),
		).Only(ctx)
		if ent.IsNotFound(queryErr) {
			return rollback(contracts.ErrStaleMatchResult)
		}
		if queryErr != nil {
			return rollback(queryErr)
		}
		contractRows[candidate.ContractID] = contractRow
		_, err = tx.ContractMatchCandidate.Create().SetID(stableID("cmc", runID+":"+candidate.ContractID)).
			SetClientID(invoiceRow.ClientID).SetMatchRunID(runID).SetContractID(candidate.ContractID).
			SetContractRevision(candidate.ContractRevision).SetRank(candidate.Rank).SetRecommended(candidate.Recommended).
			SetCompatibility(contractmatchcandidate.Compatibility(candidate.Compatibility)).SetConfidenceDisplay(candidate.Confidence).
			SetReasons(candidate.Reasons).SetCreatedAt(now).Save(ctx)
		if err != nil {
			return rollback(err)
		}
	}
	triggerReference := ""
	if command.TriggerContractID != "" {
		trigger, triggerErr := tx.Contract.Query().Where(contract.IDEQ(command.TriggerContractID), contract.ClientIDEQ(invoiceRow.ClientID)).Only(ctx)
		if ent.IsNotFound(triggerErr) {
			return rollback(contracts.ErrStaleMatchResult)
		}
		if triggerErr != nil {
			return rollback(triggerErr)
		}
		triggerReference = trigger.Reference
	}

	resumeDetail := fmt.Sprintf("Factura a fost reevaluată automat: rezultat %s, politica %s.", decision.Outcome, decision.PolicyVersion)
	if triggerReference != "" {
		resumeDetail = fmt.Sprintf("Contractul %s a devenit disponibil. Factura a fost reevaluată automat: rezultat %s, politica %s.", triggerReference, decision.Outcome, decision.PolicyVersion)
	}
	if err = createAudit(tx, ctx, auditRecord{
		key: commandKey + ":reevaluated", invoiceID: invoiceRow.ID, clientID: invoiceRow.ClientID,
		eventType: "MISSING_CONTRACT_REEVALUATED", from: string(from), to: string(to), trigger: "CONTRACT_AVAILABLE",
		detail: resumeDetail,
		actor:  audit.ActorSystem, actorDisplay: "Sistem matching", correlationID: command.CorrelationID, at: now,
	}); err != nil {
		return rollback(err)
	}

	if decision.Outcome == contracts.OutcomeNoMatch {
		if err = createAudit(tx, ctx, auditRecord{
			key: commandKey + ":still-missing", invoiceID: invoiceRow.ID, taskID: missingTask.ID, clientID: invoiceRow.ClientID,
			eventType: "MISSING_CONTRACT_STILL_WAITING", from: string(missingTask.Status), to: string(missingTask.Status), trigger: "NO_MATCH",
			detail: "Contractul disponibil nu corespunde facturii; solicitarea existentă rămâne în așteptare.",
			actor:  audit.ActorSystem, actorDisplay: "Sistem matching", correlationID: command.CorrelationID, at: now,
		}); err != nil {
			return rollback(err)
		}
		if err = tx.Commit(); err != nil {
			return false, err
		}
		return true, nil
	}

	metadata, _ := json.Marshal(map[string]string{"reason": "CONTRACT_BECAME_AVAILABLE", "matchRunId": runID, "outcome": string(decision.Outcome), "policyVersion": decision.PolicyVersion})
	_, err = tx.ValidationTask.UpdateOneID(missingTask.ID).
		Where(entvalidationtask.StatusIn(entvalidationtask.StatusOPEN, entvalidationtask.StatusWAITING), entvalidationtask.RevisionEQ(missingTask.Revision)).
		SetStatus(entvalidationtask.StatusRESOLVED).SetResolvedAt(now).SetResolutionMetadata(metadata).AddRevision(1).SetUpdatedAt(now).Save(ctx)
	if ent.IsNotFound(err) {
		return rollback(apperrors.ErrConflict)
	}
	if err != nil {
		return rollback(err)
	}
	if err = createAudit(tx, ctx, auditRecord{
		key: commandKey + ":resolved", invoiceID: invoiceRow.ID, taskID: missingTask.ID, clientID: invoiceRow.ClientID,
		eventType: "MISSING_CONTRACT_RESOLVED", from: string(missingTask.Status), to: string(validationtasks.StatusResolved), trigger: "CONTRACT_BECAME_AVAILABLE",
		detail: "Contractul lipsă a devenit disponibil; blocajul existent a fost rezolvat automat.",
		actor:  audit.ActorSystem, actorDisplay: "Sistem matching", correlationID: command.CorrelationID, at: now,
	}); err != nil {
		return rollback(err)
	}

	switch decision.Outcome {
	case contracts.OutcomeUniqueCompatible:
		selected := contractRows[decision.Candidates[0].ContractID]
		if err = createAssociation(ctx, tx, invoiceRow, selected, runID, decision.PolicyVersion, contracts.AssociationAutomatic, "", "Sistem matching", now); err != nil {
			if IsConstraintError(err) {
				return rollback(apperrors.ErrConflict)
			}
			return rollback(err)
		}
		if err = createAudit(tx, ctx, auditRecord{
			key: commandKey + ":associated", invoiceID: invoiceRow.ID, clientID: invoiceRow.ClientID,
			eventType: "CONTRACT_AUTO_ASSOCIATED", from: selected.Reference, to: selected.ID, trigger: "UNIQUE_COMPATIBLE",
			detail: fmt.Sprintf("Contractul %s a fost asociat automat folosind politica %s; procesarea facturii continuă.", selected.Reference, decision.PolicyVersion),
			actor:  audit.ActorSystem, actorDisplay: "Sistem matching", correlationID: command.CorrelationID, at: now,
		}); err != nil {
			return rollback(err)
		}
		if err = createOutbox(tx, ctx, invoiceRow.ID, commandKey+":continue", command.CorrelationID, now); err != nil {
			return rollback(err)
		}
	case contracts.OutcomeMultiplePlausible, contracts.OutcomeUniqueIncompatible:
		reason := "Au fost identificate mai multe contracte posibile. Este necesară confirmarea."
		if decision.Outcome == contracts.OutcomeUniqueIncompatible {
			reason = "Contractul identificat are semnale incompatibile. Este necesară confirmarea."
		}
		taskID := stableID("task", commandKey+":review")
		_, err = tx.ValidationTask.Create().SetID(taskID).SetClientID(invoiceRow.ClientID).SetInvoiceID(invoiceRow.ID).SetContractMatchRunID(runID).
			SetTaskType(entvalidationtask.TaskTypeCONTRACT_MATCH).SetStatus(entvalidationtask.StatusOPEN).
			SetTitle("Confirmă contractul").SetReason(reason).SetCreatedByKind(entvalidationtask.CreatedByKindSYSTEM).
			SetCreatedByDisplay("Sistem matching").SetCreationKey(commandKey + ":review").SetRevision(1).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
		if err != nil {
			if IsConstraintError(err) {
				return rollback(apperrors.ErrConflict)
			}
			return rollback(err)
		}
		if err = createAudit(tx, ctx, auditRecord{
			key: commandKey + ":task", invoiceID: invoiceRow.ID, taskID: taskID, clientID: invoiceRow.ClientID,
			eventType: "VALIDATION_TASK_CREATED", to: string(validationtasks.StatusOpen), trigger: "CONTRACT_MATCH_REVIEW_REQUIRED",
			detail: reason, actor: audit.ActorSystem, actorDisplay: "Sistem matching", correlationID: command.CorrelationID, at: now,
		}); err != nil {
			return rollback(err)
		}
	default:
		return rollback(apperrors.ErrValidation)
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ConfirmContractMatch(ctx context.Context, command contracts.ConfirmCommand, now time.Time) (bool, error) {
	eventKey := "contract-confirm:" + command.CommandID
	if exists, err := s.auditExists(ctx, eventKey+":confirmed"); err != nil {
		return false, err
	} else if exists {
		return false, nil
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return false, err
	}
	rollback := func(cause error) (bool, error) { _ = tx.Rollback(); return false, cause }
	taskRow, err := tx.ValidationTask.Query().Where(entvalidationtask.IDEQ(command.TaskID)).WithContractMatchRun().Only(ctx)
	if ent.IsNotFound(err) {
		return rollback(apperrors.ErrNotFound)
	}
	if err != nil {
		return rollback(err)
	}
	if taskRow.InvoiceID != command.InvoiceID || taskRow.TaskType != entvalidationtask.TaskTypeCONTRACT_MATCH || taskRow.ContractMatchRunID == nil {
		return rollback(apperrors.ErrValidation)
	}
	if taskRow.Status == entvalidationtask.StatusRESOLVED {
		return rollback(validationtasks.ErrTaskAlreadyResolved)
	}
	if taskRow.Status != entvalidationtask.StatusOPEN || taskRow.Revision != command.ExpectedTaskRevision {
		return rollback(contracts.ErrStaleMatchResult)
	}
	run := taskRow.Edges.ContractMatchRun
	invoiceRow, err := tx.Invoice.Query().Where(invoice.IDEQ(command.InvoiceID)).Only(ctx)
	if err != nil {
		return rollback(err)
	}
	if invoiceRow.ClientID != taskRow.ClientID || invoiceRow.PipelineStatus != invoice.PipelineStatusAWAITING_MATCH_CONFIRM || invoiceRow.Revision != command.ExpectedInvoiceRevision || run.InvoiceRevision != command.ExpectedInvoiceRevision {
		return rollback(contracts.ErrStaleMatchResult)
	}
	candidate, err := tx.ContractMatchCandidate.Query().Where(contractmatchcandidate.MatchRunIDEQ(run.ID), contractmatchcandidate.ContractIDEQ(command.ContractID)).Only(ctx)
	if ent.IsNotFound(err) {
		return rollback(apperrors.ErrValidation)
	}
	if err != nil {
		return rollback(err)
	}
	selected, err := tx.Contract.Query().Where(
		contract.IDEQ(candidate.ContractID),
		contract.ClientIDEQ(invoiceRow.ClientID),
		contract.RevisionEQ(candidate.ContractRevision),
	).Only(ctx)
	if ent.IsNotFound(err) {
		return rollback(contracts.ErrStaleMatchResult)
	}
	if err != nil {
		return rollback(err)
	}
	updatedInvoice, err := tx.Invoice.UpdateOneID(invoiceRow.ID).
		Where(invoice.PipelineStatusEQ(invoice.PipelineStatusAWAITING_MATCH_CONFIRM), invoice.RevisionEQ(command.ExpectedInvoiceRevision)).
		SetPipelineStatus(invoice.PipelineStatusDEDUPE_CHECKED).AddRevision(1).SetUpdatedAt(now).Save(ctx)
	if ent.IsNotFound(err) {
		_ = tx.Rollback()
		if exists, checkErr := s.auditExists(ctx, eventKey+":confirmed"); checkErr == nil && exists {
			return false, nil
		}
		return false, contracts.ErrStaleMatchResult
	}
	if err != nil {
		return rollback(err)
	}
	if err = createAssociation(ctx, tx, updatedInvoice, selected, run.ID, run.PolicyVersion, contracts.AssociationHuman, command.ActorID, command.ActorDisplay, now); err != nil {
		if IsConstraintError(err) {
			return rollback(apperrors.ErrConflict)
		}
		return rollback(err)
	}
	metadata, _ := json.Marshal(map[string]string{"contractId": selected.ID, "matchRunId": run.ID, "policyVersion": run.PolicyVersion})
	_, err = tx.ValidationTask.UpdateOneID(taskRow.ID).
		Where(entvalidationtask.StatusEQ(entvalidationtask.StatusOPEN), entvalidationtask.RevisionEQ(command.ExpectedTaskRevision)).
		SetStatus(entvalidationtask.StatusRESOLVED).SetResolvedAt(now).SetResolutionMetadata(metadata).AddRevision(1).SetUpdatedAt(now).Save(ctx)
	if ent.IsNotFound(err) {
		return rollback(contracts.ErrStaleMatchResult)
	}
	if err != nil {
		return rollback(err)
	}
	if err = createAudit(tx, ctx, auditRecord{
		key: eventKey + ":resolved", invoiceID: invoiceRow.ID, taskID: taskRow.ID, clientID: invoiceRow.ClientID,
		eventType: "CONTRACT_MATCH_TASK_RESOLVED", from: string(validationtasks.StatusOpen), to: string(validationtasks.StatusResolved), trigger: "CONTRACT_CONFIRMED",
		detail: fmt.Sprintf("Contract match task resolved with candidate %s.", selected.Reference), actor: audit.ActorUser,
		actorID: command.ActorID, actorDisplay: command.ActorDisplay, correlationID: command.CorrelationID, at: now,
	}); err != nil {
		return rollback(err)
	}
	selection := "recommended candidate"
	if !candidate.Recommended {
		selection = "alternative candidate"
	}
	if err = createAudit(tx, ctx, auditRecord{
		key: eventKey + ":confirmed", invoiceID: invoiceRow.ID, clientID: invoiceRow.ClientID,
		eventType: "CONTRACT_CONFIRMED", from: string(invoice.PipelineStatusAWAITING_MATCH_CONFIRM), to: string(invoice.PipelineStatusDEDUPE_CHECKED), trigger: "CONTRACT_CONFIRMED",
		detail: fmt.Sprintf("Human confirmed %s %s using policy %s; pipeline resumed.", selection, selected.Reference, run.PolicyVersion), actor: audit.ActorUser,
		actorID: command.ActorID, actorDisplay: command.ActorDisplay, correlationID: command.CorrelationID, at: now,
	}); err != nil {
		return rollback(err)
	}
	if err = createOutbox(tx, ctx, invoiceRow.ID, eventKey+":continue", command.CorrelationID, now); err != nil {
		return rollback(err)
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func createAssociation(ctx context.Context, tx *ent.Tx, invoiceRow *ent.Invoice, contractRow *ent.Contract, runID, policyVersion string, kind contracts.AssociationKind, actorID, actorDisplay string, now time.Time) error {
	create := tx.InvoiceContractAssociation.Create().SetID(stableID("ica", invoiceRow.ID)).SetClientID(invoiceRow.ClientID).
		SetInvoiceID(invoiceRow.ID).SetContractID(contractRow.ID).SetMatchRunID(runID).
		SetAssociationKind(invoicecontractassociation.AssociationKind(kind)).SetPolicyVersion(policyVersion).
		SetContractReference(contractRow.Reference).SetSupplierName(contractRow.SupplierName).
		SetEffectiveFrom(contractRow.EffectiveFrom).SetNillableEffectiveTo(contractRow.EffectiveTo).SetPeriodType(invoicecontractassociation.PeriodType(contractRow.PeriodType)).
		SetTotalValue(contractRow.TotalValue).SetCurrency(contractRow.Currency).SetUnitType(contractRow.UnitType).
		SetPaymentTerms(contractRow.PaymentTerms).SetAssociatedAt(now)
	if actorID != "" {
		create.SetAssociatedByID(actorID)
	}
	if actorDisplay != "" {
		create.SetAssociatedByDisplay(actorDisplay)
	}
	_, err := create.Save(ctx)
	return err
}

func contractDomain(row *ent.Contract) (*contracts.Contract, error) {
	value, err := money.Parse(row.TotalValue)
	if err != nil {
		return nil, fmt.Errorf("read contract amount: %w", err)
	}
	result := &contracts.Contract{
		ID: row.ID, ClientID: row.ClientID, SupplierName: row.SupplierName, SupplierCUI: row.SupplierCui,
		NormalizedSupplierCUI: row.NormalizedSupplierCui, Reference: row.Reference,
		SourceDocumentID: row.SourceDocumentID, ExtractionAttemptID: row.ExtractionAttemptID,
		EffectiveFrom: row.EffectiveFrom, EffectiveTo: row.EffectiveTo, PeriodType: string(row.PeriodType),
		Value: money.Money{Amount: value, Currency: row.Currency}, UnitType: row.UnitType, PaymentTerms: row.PaymentTerms,
		HasLegacyTotalValue: row.HasLegacyTotalValue,
		SourceReference:     row.SourceReference, SourceMetadata: row.SourceMetadata, Revision: row.Revision,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	for _, serviceRow := range row.Edges.ServiceTerms {
		term := contracts.ServiceTerm{ID: serviceRow.ID, Position: serviceRow.Position, ServiceDescription: serviceRow.ServiceDescription, PricingModel: string(serviceRow.PricingModel), Currency: serviceRow.Currency, Unit: serviceRow.Unit, QuantitySource: string(serviceRow.QuantitySource), QuantityDriver: serviceRow.QuantityDriver, BillingFrequency: string(serviceRow.BillingFrequency), EvidenceJSON: serviceRow.SourceEvidence}
		if serviceRow.UnitPrice != nil {
			parsed, parseErr := money.Parse(*serviceRow.UnitPrice)
			if parseErr != nil {
				return nil, parseErr
			}
			term.UnitPrice = &parsed
		}
		if serviceRow.QuantityValue != nil {
			parsed, parseErr := money.Parse(*serviceRow.QuantityValue)
			if parseErr != nil {
				return nil, parseErr
			}
			term.QuantityValue = &parsed
		}
		result.ServiceTerms = append(result.ServiceTerms, term)
	}
	return result, nil
}
