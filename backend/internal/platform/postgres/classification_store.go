package postgres

import (
	"context"
	"database/sql"
	"diana-contabilitate/backend/ent/accountingrulepack"
	"diana-contabilitate/backend/ent/clientaccountingprofile"
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/money"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"diana-contabilitate/backend/ent"
	"diana-contabilitate/backend/ent/classificationrule"
	"diana-contabilitate/backend/ent/invoice"
	"diana-contabilitate/backend/ent/invoicecontractassociation"
	"diana-contabilitate/backend/ent/invoiceline"
	"diana-contabilitate/backend/ent/lineclassification"
	"diana-contabilitate/backend/ent/ruleversion"
	"diana-contabilitate/backend/ent/validationtask"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/audit"
	classificationdomain "diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/rules"
	"diana-contabilitate/backend/internal/validationtasks"
)

func (s *Store) ProcessCommandCommitted(ctx context.Context, commandID string) (bool, error) {
	return s.auditExists(ctx, "classification:"+commandID+":executed")
}

func (s *Store) LoadClassificationInput(ctx context.Context, invoiceID string) (classificationdomain.InvoiceContext, error) {
	tx, err := s.Client.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return classificationdomain.InvoiceContext{}, err
	}
	defer tx.Rollback()
	row, err := tx.Invoice.Query().Where(invoice.IDEQ(invoiceID)).WithLines(func(query *ent.InvoiceLineQuery) {
		query.Order(ent.Asc(invoiceline.FieldPosition))
	}).Only(ctx)
	if ent.IsNotFound(err) {
		return classificationdomain.InvoiceContext{}, apperrors.ErrNotFound
	}
	if err != nil {
		return classificationdomain.InvoiceContext{}, err
	}
	input := classificationdomain.InvoiceContext{ModelVersion: row.ModelVersion, SourceFacts: row.SourceFacts, Currency: row.Currency, ID: row.ID, ClientID: row.ClientID, PipelineStatus: string(row.PipelineStatus), Revision: row.Revision, IssueDate: accountingdate.FromTime(row.IssueDate), DocumentType: string(row.DocumentType)}
	for _, line := range row.Edges.Lines {
		input.Lines = append(input.Lines, classificationdomain.LineContext{SourceFacts: line.SourceFacts, ID: line.ID, Position: line.Position, Description: line.Description, VATRate: money.Amount(line.VatRate), VATValue: money.Amount(line.VatValue)})
	}
	if row.SupplierCui != nil {
		input.SupplierID = *row.SupplierCui
	}
	if row.ModelVersion == accounting.ModelVersion {
		input.Snapshot = &accounting.Snapshot{}
		profiles, err := tx.ClientAccountingProfile.Query().Where(clientaccountingprofile.ClientIDEQ(row.ClientID)).All(ctx)
		if err != nil {
			return input, err
		}
		var selected []*accounting.Profile
		for _, p := range profiles {
			if p.Payload.Valid(row.ClientID, input.IssueDate) {
				selected = append(selected, p.Payload)
			}
		}
		if len(selected) == 1 {
			input.Snapshot.Profile = selected[0]
		}
		packs, err := tx.AccountingRulePack.Query().Where(accountingrulepack.ClientIDEQ(row.ClientID)).All(ctx)
		if err != nil {
			return input, err
		}
		var applicable []*accounting.Pack
		for _, p := range packs {
			if p.Payload.Valid(input.Snapshot.Profile, row.ClientID, input.IssueDate, true) {
				applicable = append(applicable, p.Payload)
			}
		}
		if len(applicable) == 1 {
			input.Snapshot.Pack = applicable[0]
		}
		association, err := tx.InvoiceContractAssociation.Query().Where(invoicecontractassociation.InvoiceIDEQ(row.ID)).Only(ctx)
		if err == nil {
			contract, err := tx.Contract.Get(ctx, association.ContractID)
			if err != nil {
				return input, err
			}
			input.Snapshot.ContractID = contract.ID
			input.Snapshot.ContractReference = contract.Reference
			input.Snapshot.ContractRevision = contract.Revision
		} else if !ent.IsNotFound(err) {
			return input, err
		}
	}
	ruleRows, err := tx.ClassificationRule.Query().Where(classificationrule.Or(
		classificationrule.ScopeEQ(classificationrule.ScopeGLOBAL),
		classificationrule.And(classificationrule.ScopeEQ(classificationrule.ScopeCLIENT_OVERRIDE), classificationrule.ClientIDEQ(row.ClientID)),
	)).WithVersions(func(query *ent.RuleVersionQuery) { query.Order(ent.Desc(ruleversion.FieldVersion)) }).All(ctx)
	if err != nil {
		return classificationdomain.InvoiceContext{}, err
	}
	for _, ruleRow := range ruleRows {
		if len(ruleRow.Edges.Versions) == 0 {
			continue
		}
		for _, version := range ruleRow.Edges.Versions {
			input.Rules = append(input.Rules, classificationdomain.RuleCandidate{
				RuleID: ruleRow.ID, RuleVersionID: version.ID, Reference: ruleRow.Reference, Version: version.Version,
				Category: rules.Category(ruleRow.Category), Scope: rules.Scope(ruleRow.Scope), ParentRuleID: ruleRow.ParentRuleID,
				Result: version.Result, Explanation: version.Explanation, LegalBasis: version.LegalBasis,
				MatchKind: rules.MatchKind(version.MatchKind), MatchValue: version.MatchValue,
				ProductionEligible: version.ProductionEligible, RulePackVersion: version.RulePackVersion, Provenance: version.Provenance,
				EffectiveFrom: accountingdate.FromTime(version.EffectiveFrom), EffectiveTo: accountingDatePointer(version.EffectiveTo),
			})
		}
	}
	if err = tx.Commit(); err != nil {
		return classificationdomain.InvoiceContext{}, err
	}
	return input, nil
}

func (s *Store) ApplyClassification(ctx context.Context, command classificationdomain.ProcessCommand, result classificationdomain.Result, now time.Time) (bool, error) {
	eventKey := "classification:" + command.CommandID
	if exists, err := s.auditExists(ctx, eventKey+":executed"); err != nil {
		return false, err
	} else if exists {
		return false, nil
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return false, err
	}
	rollback := func(cause error) (bool, error) { _ = tx.Rollback(); return false, cause }
	invoiceUpdate := tx.Invoice.UpdateOneID(command.InvoiceID)
	if result.ModelVersion == accounting.ModelVersion {
		invoiceUpdate.SetAccountingSnapshot(result.Snapshot)
	}
	classified, err := invoiceUpdate.
		Where(invoice.PipelineStatusEQ(invoice.PipelineStatusLINES_READ), invoice.RevisionEQ(command.ExpectedRevision)).
		SetPipelineStatus(invoice.PipelineStatusCLASSIFIED).AddRevision(1).SetUpdatedAt(now).Save(ctx)
	if ent.IsNotFound(err) {
		_ = tx.Rollback()
		if exists, checkErr := s.auditExists(ctx, eventKey+":executed"); checkErr == nil && exists {
			return false, nil
		}
		return false, apperrors.ErrConflict
	}
	if err != nil {
		return rollback(err)
	}
	if (classified.ModelVersion == accounting.ModelVersion) != (result.ModelVersion == accounting.ModelVersion) {
		return rollback(classificationdomain.ErrInvalidPolicyResult)
	}
	pending := 0
	for _, proposal := range result.Proposals {
		decisionKey := classified.ID + ":" + proposal.InvoiceLineID + ":" + string(proposal.Dimension)
		if classified.ModelVersion == accounting.ModelVersion {
			decisionKey += ":" + classified.ModelVersion
		}
		create := tx.LineClassification.Create().SetID(stableID("lc", decisionKey)).
			SetClientID(classified.ClientID).SetInvoiceID(classified.ID).SetInvoiceLineID(proposal.InvoiceLineID).
			SetDimension(lineclassification.Dimension(proposal.Dimension)).SetProposedValue(proposal.ProposedValue).
			SetConfidenceDisplay(proposal.Confidence).SetExplanation(proposal.Explanation).SetLegalBasis(proposal.LegalBasis).
			SetRequiredReview(proposal.RequiresReview).
			SetSource(lineclassification.Source(proposal.Source)).SetPolicyVersion(result.PolicyVersion).
			SetRevision(1).SetCreatedAt(now).SetUpdatedAt(now)
		if result.ModelVersion == accounting.ModelVersion {
			create.SetModelVersion(accounting.ModelVersion).SetProposedTypedValue(proposal.TypedValue).SetDecisionEvidence(proposal.Evidence)
			if !proposal.RequiresReview {
				create.SetEffectiveTypedValue(proposal.TypedValue)
			}
		}
		if proposal.InvoiceDateUsed.Valid() {
			date, _ := time.Parse("2006-01-02", string(proposal.InvoiceDateUsed))
			create.SetInvoiceDateUsed(date)
		}
		if result.PolicyVersion == classificationdomain.ProductionPolicyVersion && !proposal.RequiresReview && (proposal.Rule == nil || !proposal.Rule.ProductionEligible || !proposal.Rule.Provenance.Valid()) {
			return rollback(classificationdomain.ErrInvalidPolicyResult)
		}
		if proposal.RequiresReview {
			pending++
			create.SetReviewStatus(lineclassification.ReviewStatusPENDING)
		} else {
			create.SetReviewStatus(lineclassification.ReviewStatusACCEPTED).SetEffectiveValue(proposal.ProposedValue)
		}
		if proposal.Rule != nil {
			create.SetRuleVersionID(proposal.Rule.RuleVersionID)
		}
		if _, err = create.Save(ctx); err != nil {
			return rollback(err)
		}
	}
	if err = createAudit(tx, ctx, auditRecord{key: eventKey + ":executed", invoiceID: classified.ID, clientID: classified.ClientID, eventType: "AUTOMATED_CLASSIFICATION_COMPLETED", from: string(invoice.PipelineStatusLINES_READ), to: string(invoice.PipelineStatusCLASSIFIED), trigger: "CLASSIFICATION_DECISION", detail: fmt.Sprintf("Classification policy %s produced %d line-dimension decisions; immutable rule-version evidence retained.", result.PolicyVersion, len(result.Proposals)), actor: audit.ActorSystem, actorDisplay: "Sistem clasificare", correlationID: command.CorrelationID, at: now}); err != nil {
		return rollback(err)
	}
	readinessReason := ""
	if result.ModelVersion == accounting.ModelVersion && pending == 0 {
		ready, err := evaluateAccountingReadiness(ctx, tx, classified.ID, result.Snapshot != nil && result.Snapshot.TestOnly)
		if err != nil {
			return rollback(err)
		}
		if s.AccountingReadinessObserver != nil {
			s.AccountingReadinessObserver.AccountingReadinessEvaluated(ready.Ready)
		}
		if !ready.Ready {
			readinessReason = ready.Reason
		}
	}
	if result.ModelVersion == accounting.ModelVersion && pending == 0 {
		if err := createAudit(tx, ctx, auditRecord{key: eventKey + ":readiness", invoiceID: classified.ID, clientID: classified.ClientID, eventType: "ACCOUNTING_READINESS_EVALUATED", trigger: "AUTO_COMPLETION", detail: fmt.Sprintf("Accounting readiness blocker: %s", readinessReason), actor: audit.ActorSystem, actorDisplay: "Sistem contabil", at: now}); err != nil {
			return rollback(err)
		}
	}
	target := invoice.PipelineStatusREADY_FOR_SAGA
	if pending > 0 || readinessReason != "" {
		target = invoice.PipelineStatusAWAITING_REVIEW
	}
	update := tx.Invoice.UpdateOneID(classified.ID).Where(invoice.PipelineStatusEQ(invoice.PipelineStatusCLASSIFIED), invoice.RevisionEQ(classified.Revision)).SetPipelineStatus(target).SetReadinessReason(readinessReason).AddRevision(1).SetUpdatedAt(now)
	if target == invoice.PipelineStatusREADY_FOR_SAGA {
		update.SetSagaStatus(invoice.SagaStatusREADY)
	}
	finalInvoice, err := update.Save(ctx)
	if err != nil {
		return rollback(err)
	}
	if err = createAudit(tx, ctx, auditRecord{key: eventKey + ":routed", invoiceID: classified.ID, clientID: classified.ClientID, eventType: "CLASSIFICATION_ROUTED", from: string(invoice.PipelineStatusCLASSIFIED), to: string(target), trigger: "CLASSIFICATION_DECISION", detail: fmt.Sprintf("Classification completed with %d pending review items.", pending), actor: audit.ActorSystem, actorDisplay: "Sistem clasificare", correlationID: command.CorrelationID, at: now}); err != nil {
		return rollback(err)
	}
	if pending > 0 || readinessReason != "" {
		taskID := stableID("task", eventKey+":review")
		_, err = tx.ValidationTask.Create().SetID(taskID).SetClientID(classified.ClientID).SetInvoiceID(classified.ID).
			SetTaskType(validationtask.TaskTypeCLASSIFICATION).SetStatus(validationtask.StatusOPEN).
			SetTitle("Revizuiește clasificările incerte").SetReason(fmt.Sprintf("%d dimensiuni necesită decizia contabilului.", pending)).
			SetCreatedByKind(validationtask.CreatedByKindSYSTEM).SetCreatedByDisplay("Sistem clasificare").
			SetBlockerCode(readinessReason).SetCreationKey(eventKey + ":review").SetRevision(1).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
		if err != nil {
			return rollback(err)
		}
		if err = createAudit(tx, ctx, auditRecord{key: eventKey + ":task", invoiceID: classified.ID, taskID: taskID, clientID: classified.ClientID, eventType: "VALIDATION_TASK_CREATED", to: string(validationtasks.StatusOpen), trigger: "CLASSIFICATION_REVIEW_REQUIRED", detail: fmt.Sprintf("One classification task groups %d pending items.", pending), actor: audit.ActorSystem, actorDisplay: "Sistem clasificare", correlationID: command.CorrelationID, at: now}); err != nil {
			return rollback(err)
		}
	} else if err = createOutbox(tx, ctx, finalInvoice.ID, eventKey+":continue", command.CorrelationID, now); err != nil {
		return rollback(err)
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ReviewClassification(ctx context.Context, command classificationdomain.ReviewCommand, now time.Time) (bool, error) {
	eventKey := "classification-review:" + command.CommandID
	if exists, err := s.auditExists(ctx, eventKey+":decision"); err != nil {
		return false, err
	} else if exists {
		return false, nil
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return false, err
	}
	rollback := func(cause error) (bool, error) { _ = tx.Rollback(); return false, cause }
	taskRow, err := tx.ValidationTask.Query().Where(validationtask.IDEQ(command.TaskID)).Only(ctx)
	if ent.IsNotFound(err) {
		return rollback(apperrors.ErrNotFound)
	}
	if err != nil {
		return rollback(err)
	}
	if taskRow.InvoiceID != command.InvoiceID || taskRow.TaskType != validationtask.TaskTypeCLASSIFICATION {
		return rollback(apperrors.ErrValidation)
	}
	if taskRow.Status == validationtask.StatusRESOLVED {
		return rollback(validationtasks.ErrTaskAlreadyResolved)
	}
	if taskRow.Status != validationtask.StatusOPEN || taskRow.Revision != command.ExpectedTaskRevision {
		return rollback(classificationdomain.ErrStaleReview)
	}
	invoiceRow, err := tx.Invoice.Query().Where(invoice.IDEQ(command.InvoiceID)).Only(ctx)
	if ent.IsNotFound(err) {
		return rollback(apperrors.ErrNotFound)
	}
	if err != nil {
		return rollback(err)
	}
	if invoiceRow.ClientID != taskRow.ClientID || invoiceRow.PipelineStatus != invoice.PipelineStatusAWAITING_REVIEW || invoiceRow.Revision != command.ExpectedInvoiceRevision {
		return rollback(classificationdomain.ErrStaleReview)
	}
	classificationRow, err := tx.LineClassification.Query().Where(lineclassification.IDEQ(command.ClassificationID)).Only(ctx)
	if ent.IsNotFound(err) {
		return rollback(apperrors.ErrNotFound)
	}
	if err != nil {
		return rollback(err)
	}
	if classificationRow.InvoiceID != invoiceRow.ID || classificationRow.ClientID != invoiceRow.ClientID {
		return rollback(apperrors.ErrValidation)
	}
	if (classificationRow.ReviewStatus != lineclassification.ReviewStatusPENDING && classificationRow.ModelVersion != accounting.ModelVersion) || classificationRow.Revision != command.ExpectedClassificationRevision {
		return rollback(classificationdomain.ErrStaleReview)
	}
	status := lineclassification.ReviewStatusACCEPTED
	finalValue := classificationRow.ProposedValue
	eventType := "CLASSIFICATION_PROPOSAL_ACCEPTED"
	if command.CorrectedValue != nil {
		status = lineclassification.ReviewStatusCORRECTED
		finalValue = *command.CorrectedValue
		eventType = "CLASSIFICATION_CORRECTED"
	}
	typed := classificationRow.ProposedTypedValue
	if classificationRow.ModelVersion == accounting.ModelVersion {
		if command.CorrectedValue != nil {
			return rollback(apperrors.ErrValidation)
		}
		if command.TypedValue != nil {
			typed = command.TypedValue
			status = lineclassification.ReviewStatusCORRECTED
			eventType = "CLASSIFICATION_CORRECTED"
		}
		if typed == nil || typed.Validate(string(classificationRow.Dimension)) != nil || strings.TrimSpace(command.Reason) == "" {
			return rollback(apperrors.ErrValidation)
		}
		finalValue = typed.Text()
	}
	classificationUpdate := tx.LineClassification.UpdateOneID(classificationRow.ID).
		Where(lineclassification.ReviewStatusEQ(classificationRow.ReviewStatus), lineclassification.RevisionEQ(command.ExpectedClassificationRevision)).
		SetReviewStatus(status).SetEffectiveValue(finalValue).SetReviewedAt(now).SetReviewedByDisplay(command.ActorDisplay).AddRevision(1).SetUpdatedAt(now)
	if classificationRow.ModelVersion == accounting.ModelVersion {
		classificationUpdate.SetEffectiveTypedValue(typed).SetReviewReason(command.Reason)
	}
	if command.ActorID != "" {
		classificationUpdate.SetReviewedByID(command.ActorID)
	}
	updated, err := classificationUpdate.Save(ctx)
	if ent.IsNotFound(err) {
		return rollback(classificationdomain.ErrStaleReview)
	}
	if err != nil {
		return rollback(err)
	}
	reviewDetail := fmt.Sprintf("Human decision for line %s dimension %s; proposal policy=%s rule_version=%s.", classificationRow.InvoiceLineID, classificationRow.Dimension, classificationRow.PolicyVersion, pointerValue(classificationRow.RuleVersionID))
	if classificationRow.ModelVersion == accounting.ModelVersion {
		evidence, err := json.Marshal(struct {
			Model     string               `json:"model"`
			Dimension string               `json:"dimension"`
			Previous  *accounting.Value    `json:"previous"`
			Proposal  *accounting.Value    `json:"proposal"`
			Final     *accounting.Value    `json:"final"`
			Reason    string               `json:"reason"`
			Evidence  *accounting.Evidence `json:"evidence"`
		}{classificationRow.ModelVersion, string(classificationRow.Dimension), classificationRow.EffectiveTypedValue, classificationRow.ProposedTypedValue, typed, command.Reason, classificationRow.DecisionEvidence})
		if err != nil {
			return rollback(err)
		}
		reviewDetail = string(evidence)
	}
	if err = createAudit(tx, ctx, auditRecord{key: eventKey + ":decision", invoiceID: invoiceRow.ID, taskID: taskRow.ID, clientID: invoiceRow.ClientID, eventType: eventType, from: classificationRow.ProposedValue, to: finalValue, trigger: "CLASSIFICATION_REVIEW", detail: reviewDetail, actor: audit.ActorUser, actorID: command.ActorID, actorDisplay: command.ActorDisplay, correlationID: command.CorrelationID, at: now}); err != nil {
		return rollback(err)
	}
	remaining, err := tx.LineClassification.Query().Where(lineclassification.InvoiceIDEQ(invoiceRow.ID), lineclassification.ReviewStatusEQ(lineclassification.ReviewStatusPENDING)).Count(ctx)
	if err != nil {
		return rollback(err)
	}
	readyToComplete := remaining == 0
	reason := ""
	if classificationRow.ModelVersion == accounting.ModelVersion && readyToComplete {
		ready, err := evaluateAccountingReadiness(ctx, tx, invoiceRow.ID, invoiceRow.AccountingSnapshot != nil && invoiceRow.AccountingSnapshot.TestOnly)
		if err != nil {
			return rollback(err)
		}
		readyToComplete = ready.Ready
		reason = ready.Reason
		if s.AccountingReadinessObserver != nil {
			s.AccountingReadinessObserver.AccountingReadinessEvaluated(ready.Ready)
		}
		if _, err := tx.Invoice.UpdateOneID(invoiceRow.ID).Where(invoice.RevisionEQ(command.ExpectedInvoiceRevision)).SetReadinessReason(reason).Save(ctx); err != nil {
			return rollback(err)
		}
	}
	if classificationRow.ModelVersion == accounting.ModelVersion && remaining == 0 {
		if err := createAudit(tx, ctx, auditRecord{key: eventKey + ":readiness", invoiceID: invoiceRow.ID, taskID: taskRow.ID, clientID: invoiceRow.ClientID, eventType: "ACCOUNTING_READINESS_EVALUATED", trigger: "HUMAN_COMPLETION", detail: fmt.Sprintf("Accounting readiness blocker: %s", reason), actor: audit.ActorUser, actorDisplay: command.ActorDisplay, at: now}); err != nil {
			return rollback(err)
		}
	}
	taskUpdate := tx.ValidationTask.UpdateOneID(taskRow.ID).Where(validationtask.StatusEQ(validationtask.StatusOPEN), validationtask.RevisionEQ(command.ExpectedTaskRevision)).AddRevision(1).SetUpdatedAt(now)
	if readyToComplete {
		metadata, _ := json.Marshal(map[string]string{"finalClassificationId": updated.ID})
		taskUpdate.SetStatus(validationtask.StatusRESOLVED).SetResolvedAt(now).SetResolutionMetadata(metadata)
	}
	if reason != "" {
		metadata, _ := json.Marshal(map[string]string{"readinessBlocker": reason})
		taskUpdate.SetResolutionMetadata(metadata)
	}
	if _, err = taskUpdate.Save(ctx); ent.IsNotFound(err) {
		return rollback(classificationdomain.ErrStaleReview)
	} else if err != nil {
		return rollback(err)
	}
	if readyToComplete {
		if _, err = tx.Invoice.UpdateOneID(invoiceRow.ID).Where(invoice.PipelineStatusEQ(invoice.PipelineStatusAWAITING_REVIEW), invoice.RevisionEQ(command.ExpectedInvoiceRevision)).SetPipelineStatus(invoice.PipelineStatusREADY_FOR_SAGA).SetSagaStatus(invoice.SagaStatusREADY).AddRevision(1).SetUpdatedAt(now).Save(ctx); ent.IsNotFound(err) {
			return rollback(classificationdomain.ErrStaleReview)
		} else if err != nil {
			return rollback(err)
		}
		if err = createAudit(tx, ctx, auditRecord{key: eventKey + ":resolved", invoiceID: invoiceRow.ID, taskID: taskRow.ID, clientID: invoiceRow.ClientID, eventType: "CLASSIFICATION_TASK_RESOLVED", from: string(validationtasks.StatusOpen), to: string(validationtasks.StatusResolved), trigger: "FINAL_CLASSIFICATION_REVIEW", detail: "All pending classification items were resolved.", actor: audit.ActorUser, actorID: command.ActorID, actorDisplay: command.ActorDisplay, correlationID: command.CorrelationID, at: now}); err != nil {
			return rollback(err)
		}
		if err = createAudit(tx, ctx, auditRecord{key: eventKey + ":resumed", invoiceID: invoiceRow.ID, clientID: invoiceRow.ClientID, eventType: "INVOICE_REVIEW_COMPLETED", from: string(invoice.PipelineStatusAWAITING_REVIEW), to: string(invoice.PipelineStatusREADY_FOR_SAGA), trigger: "REVIEW_COMPLETED", detail: "Final classification review completed; invoice resumed.", actor: audit.ActorUser, actorID: command.ActorID, actorDisplay: command.ActorDisplay, correlationID: command.CorrelationID, at: now}); err != nil {
			return rollback(err)
		}
		if err = createOutbox(tx, ctx, invoiceRow.ID, eventKey+":continue", command.CorrelationID, now); err != nil {
			return rollback(err)
		}
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func accountingDatePointer(value *time.Time) *accountingdate.Date {
	if value == nil {
		return nil
	}
	date := accountingdate.FromTime(*value)
	return &date
}
