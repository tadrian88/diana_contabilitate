package postgres

import (
	"context"
	"database/sql"
	"diana-contabilitate/backend/ent/account"
	"diana-contabilitate/backend/ent/accountingrulepack"
	"diana-contabilitate/backend/ent/accountmapping"
	"diana-contabilitate/backend/ent/accountmappingversion"
	"diana-contabilitate/backend/ent/clientaccountingprofile"
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingdate"
	accountdomain "diana-contabilitate/backend/internal/accounts"
	"diana-contabilitate/backend/internal/money"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"diana-contabilitate/backend/ent"
	"diana-contabilitate/backend/ent/classificationrule"
	"diana-contabilitate/backend/ent/classificationrun"
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
	committed, err := s.auditExists(ctx, "classification:"+commandID+":executed")
	if err != nil || committed {
		return committed, err
	}
	return s.auditExists(ctx, "classification:"+commandID+":blocked")
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
	if row.NormalizedSupplierCui != nil {
		input.NormalizedSupplierID = *row.NormalizedSupplierCui
	}
	if row.ModelVersion == accounting.ModelVersion {
		input.Snapshot = &accounting.Snapshot{}
		profiles, err := tx.ClientAccountingProfile.Query().Where(clientaccountingprofile.ClientIDEQ(row.ClientID)).All(ctx)
		if err != nil {
			return input, err
		}
		allProfiles := make([]*accounting.Profile, 0, len(profiles))
		for _, p := range profiles {
			allProfiles = append(allProfiles, p.Payload)
		}
		selected := accounting.ApplicableProfiles(allProfiles, row.ClientID, input.IssueDate)
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
	if input.NormalizedSupplierID != "" {
		mappingRows, err := tx.AccountMapping.Query().Where(
			accountmapping.ClientIDEQ(row.ClientID),
			accountmapping.NormalizedSupplierIDEQ(input.NormalizedSupplierID),
			accountmapping.StatusEQ(accountmapping.StatusACTIVE),
		).All(ctx)
		if err != nil {
			return classificationdomain.InvoiceContext{}, err
		}
		for _, mappingRow := range mappingRows {
			versionRow, err := tx.AccountMappingVersion.Query().Where(
				accountmappingversion.MappingIDEQ(mappingRow.ID),
				accountmappingversion.VersionEQ(mappingRow.CurrentVersion),
			).Only(ctx)
			if err != nil {
				return classificationdomain.InvoiceContext{}, err
			}
			input.Mappings = append(input.Mappings, classificationdomain.MappingCandidate{MappingReference: classificationdomain.MappingReference{
				MappingID: mappingRow.ID, Version: versionRow.Version, AccountCode: versionRow.AccountCode,
				ServiceIdentityKind: string(mappingRow.ServiceIdentityKind), ServiceIdentityValue: mappingRow.ServiceIdentityValue,
				NormalizerVersion: mappingRow.NormalizerVersion, Revision: mappingRow.Revision,
			}, Status: string(mappingRow.Status)})
		}
	}
	selectableAccountCodes, err := tx.Account.Query().Where(account.IsActiveEQ(true), account.PostableEQ(true)).Select(account.FieldCode).Strings(ctx)
	if err != nil {
		return classificationdomain.InvoiceContext{}, err
	}
	input.SelectableAccounts = make(map[string]bool, len(selectableAccountCodes))
	for _, code := range selectableAccountCodes {
		input.SelectableAccounts[code] = true
	}
	if input.Snapshot != nil && input.Snapshot.Profile != nil && len(input.Snapshot.Profile.AccountCodes) == 0 {
		input.Snapshot.AccountCatalogFingerprint = accountCodesFingerprint(selectableAccountCodes)
	}
	if input.Snapshot != nil && input.Snapshot.Profile != nil && len(input.Snapshot.Profile.AccountCodes) > 0 {
		catalogRows, catalogErr := tx.Account.Query().Where(account.CodeIn(input.Snapshot.Profile.AccountCodes...)).Order(ent.Asc(account.FieldCode)).All(ctx)
		if catalogErr != nil {
			return classificationdomain.InvoiceContext{}, catalogErr
		}
		catalogByCode := make(map[string]*ent.Account, len(catalogRows))
		for _, catalogRow := range catalogRows {
			catalogByCode[catalogRow.Code] = catalogRow
			parent := ""
			if catalogRow.ParentCode != nil {
				parent = *catalogRow.ParentCode
			}
			input.Snapshot.AccountCatalog = append(input.Snapshot.AccountCatalog, accounting.AccountSnapshot{Code: catalogRow.Code, ParentCode: parent, Active: catalogRow.IsActive, Postable: catalogRow.Postable})
		}
		for _, code := range input.Snapshot.Profile.AccountCodes {
			catalogRow := catalogByCode[code]
			var item *accountdomain.Account
			if catalogRow != nil {
				item = &accountdomain.Account{Code: catalogRow.Code, Name: catalogRow.Name, AccountType: catalogRow.AccountType, Synthetic: catalogRow.IsSynthetic, Postable: catalogRow.Postable, Active: catalogRow.IsActive}
			}
			if validationErr := accountdomain.ValidatePostingAccount(item, code, nil); validationErr != nil {
				input.ContextBlocker = "INVALID_ACCOUNTING_PROFILE"
				input.ContextBlockerMessage = validationErr.Error()
				break
			}
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
	if err = s.loadApprovedKnowledgeCandidates(ctx, &input); err != nil {
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
	invoiceBefore, err := tx.Invoice.Query().Where(invoice.IDEQ(command.InvoiceID)).Only(ctx)
	if err != nil {
		return rollback(err)
	}
	runSnapshot := result.Snapshot
	if runSnapshot == nil {
		runSnapshot = &accounting.Snapshot{}
	}
	runID := stableID("classification-run", command.CommandID)
	runCreate := tx.ClassificationRun.Create().SetID(runID).SetClientID(invoiceBefore.ClientID).SetInvoiceID(command.InvoiceID).
		SetInvoiceRevision(command.ExpectedRevision).SetSnapshot(runSnapshot).SetContextFingerprint(runSnapshot.Fingerprint()).
		SetPolicyVersion(result.PolicyVersion).SetStatus(classificationrun.StatusCOMPLETED).SetCommandKey(command.CommandID).
		SetActorDisplay("Sistem clasificare").SetCreatedAt(now)
	if result.Snapshot != nil && result.Snapshot.Profile != nil {
		runCreate.SetProfileID(result.Snapshot.Profile.ID).SetProfileVersion(result.Snapshot.Profile.Version)
	}
	if invoiceBefore.CurrentClassificationRunID != nil {
		runCreate.SetSupersedesRunID(*invoiceBefore.CurrentClassificationRunID)
	}
	if _, err = runCreate.Save(ctx); err != nil {
		_ = tx.Rollback()
		if ent.IsConstraintError(err) {
			committed, checkErr := s.ProcessCommandCommitted(ctx, command.CommandID)
			if checkErr != nil {
				return false, checkErr
			}
			if committed {
				return false, nil
			}
			return false, apperrors.ErrConflict
		}
		return false, err
	}
	invoiceUpdate := tx.Invoice.UpdateOneID(command.InvoiceID)
	// The invoice snapshot mirrors the current run (migration 000034); run
	// history remains immutable in classification_runs.
	if result.ModelVersion == accounting.ModelVersion {
		invoiceUpdate.SetAccountingSnapshot(runSnapshot)
	}
	classified, err := invoiceUpdate.
		Where(invoice.PipelineStatusEQ(invoice.PipelineStatusCOMMERCIALLY_VALIDATED), invoice.RevisionEQ(command.ExpectedRevision)).SetCurrentClassificationRunID(runID).
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
		decisionKey := runID + ":" + proposal.InvoiceLineID + ":" + string(proposal.Dimension)
		if classified.ModelVersion == accounting.ModelVersion {
			decisionKey += ":" + classified.ModelVersion
		}
		create := tx.LineClassification.Create().SetID(stableID("lc", decisionKey)).
			SetClientID(classified.ClientID).SetInvoiceID(classified.ID).SetInvoiceLineID(proposal.InvoiceLineID).
			SetClassificationRunID(runID).
			SetDimension(lineclassification.Dimension(proposal.Dimension)).SetProposedValue(proposal.ProposedValue).
			SetConfidenceDisplay(proposal.Confidence).SetExplanation(proposal.Explanation).SetLegalBasis(proposal.LegalBasis).
			SetRequiredReview(proposal.RequiresReview).
			SetSource(lineclassification.Source(proposal.Source)).SetPolicyVersion(result.PolicyVersion).
			SetRevision(1).SetCreatedAt(now).SetUpdatedAt(now)
		if result.ModelVersion == accounting.ModelVersion {
			create.SetModelVersion(accounting.ModelVersion).SetProposedTypedValue(proposal.TypedValue).SetDecisionEvidence(proposal.Evidence)
			if proposal.Knowledge != nil {
				create.SetProposalProvenance(&accounting.ProposalProvenance{KnowledgeID: proposal.Knowledge.ID, KnowledgeVersion: proposal.Knowledge.Version, SourceInvoiceID: proposal.Knowledge.SourceInvoiceID, SourceLineID: proposal.Knowledge.SourceInvoiceLineID, SourceDecisionID: proposal.Knowledge.SourceClassificationID, PromotedBy: proposal.Knowledge.PromotedBy, PromotedAt: proposal.Knowledge.PromotedAt.UTC().Format(time.RFC3339)})
			}
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
			create.SetReviewStatus(lineclassification.ReviewStatusACCEPTED).SetEffectiveValue(proposal.ProposedValue).SetEffectiveSource(string(proposal.Source))
		}
		if proposal.Rule != nil {
			create.SetRuleVersionID(proposal.Rule.RuleVersionID)
		}
		if proposal.Mapping != nil {
			create.SetAccountMappingID(proposal.Mapping.MappingID).SetAccountMappingVersion(proposal.Mapping.Version)
		}
		if _, err = create.Save(ctx); err != nil {
			return rollback(err)
		}
		if proposal.Dimension == classificationdomain.DimensionAccount && (proposal.Source == classificationdomain.SourceLearnedMapping || proposal.Source == classificationdomain.SourceAmbiguous) {
			auditType := "ACCOUNT_MAPPING_SUGGESTION_PRODUCED"
			detail := "An exact learned ACCOUNT mapping produced a reviewable proposal."
			if proposal.Source == classificationdomain.SourceAmbiguous {
				auditType = "ACCOUNT_MAPPING_CONFLICT_DETECTED"
				detail = "Conflicting ACCOUNT evidence was detected; no account was selected automatically."
			}
			if err = createAudit(tx, ctx, auditRecord{key: eventKey + ":account-mapping:" + proposal.InvoiceLineID, invoiceID: classified.ID, clientID: classified.ClientID, eventType: auditType, trigger: "CLASSIFICATION_DECISION", detail: detail, actor: audit.ActorSystem, actorDisplay: "Sistem clasificare", correlationID: command.CorrelationID, at: now}); err != nil {
				return rollback(err)
			}
		}
		if proposal.Knowledge != nil || proposal.KnowledgeConflict {
			auditType, detail := "APPROVED_KNOWLEDGE_PROPOSAL_PRODUCED", "Exact approved knowledge produced a reviewable proposal before AI."
			if proposal.KnowledgeConflict {
				auditType, detail = "APPROVED_KNOWLEDGE_CONFLICT_DETECTED", "Conflicting approved knowledge was preserved for review; AI was not allowed to choose a winner."
			} else {
				detail += " Knowledge: " + proposal.Knowledge.ID
			}
			if err = createAudit(tx, ctx, auditRecord{key: eventKey + ":approved-knowledge:" + proposal.InvoiceLineID + ":" + string(proposal.Dimension), invoiceID: classified.ID, clientID: classified.ClientID, aggregateType: "APPROVED_KNOWLEDGE", aggregateID: func() string {
				if proposal.Knowledge != nil {
					return proposal.Knowledge.ID
				}
				return classified.ID
			}(), eventType: auditType, trigger: "CLASSIFICATION_DECISION", detail: detail, actor: audit.ActorSystem, actorDisplay: "Sistem clasificare", correlationID: command.CorrelationID, at: now}); err != nil {
				return rollback(err)
			}
		}
	}
	if err = createAudit(tx, ctx, auditRecord{key: eventKey + ":executed", invoiceID: classified.ID, clientID: classified.ClientID, eventType: "AUTOMATED_CLASSIFICATION_COMPLETED", from: string(invoice.PipelineStatusCOMMERCIALLY_VALIDATED), to: string(invoice.PipelineStatusCLASSIFIED), trigger: "CLASSIFICATION_DECISION", detail: fmt.Sprintf("Classification policy %s produced %d line-dimension decisions; immutable rule-version evidence retained.", result.PolicyVersion, len(result.Proposals)), actor: audit.ActorSystem, actorDisplay: "Sistem clasificare", correlationID: command.CorrelationID, at: now}); err != nil {
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
	if result.DeferReviewForAI && pending > 0 {
		// CLASSIFIED is the existing pipeline stage used while the internal AI
		// fallback is pending/running. No human task is exposed until the worker
		// has finished or permanently failed.
		target = invoice.PipelineStatusCLASSIFIED
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
	if (pending > 0 || readinessReason != "") && !result.DeferReviewForAI {
		taskID := stableID("task", eventKey+":review")
		_, err = tx.ValidationTask.Create().SetID(taskID).SetClientID(classified.ClientID).SetInvoiceID(classified.ID).
			SetClassificationRunID(runID).
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

func (s *Store) ApplyClassificationBlock(ctx context.Context, command classificationdomain.ProcessCommand, snapshot *accounting.Snapshot, blockerCode, message string, now time.Time) (bool, error) {
	eventKey := "classification:" + command.CommandID
	if exists, err := s.auditExists(ctx, eventKey+":blocked"); err != nil || exists {
		return false, err
	}
	if snapshot == nil {
		snapshot = &accounting.Snapshot{}
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return false, err
	}
	rollback := func(cause error) (bool, error) { _ = tx.Rollback(); return false, cause }
	before, err := tx.Invoice.Query().Where(invoice.IDEQ(command.InvoiceID)).Only(ctx)
	if err != nil {
		return rollback(err)
	}
	runID := stableID("classification-run", command.CommandID)
	createRun := tx.ClassificationRun.Create().SetID(runID).SetClientID(before.ClientID).SetInvoiceID(before.ID).
		SetInvoiceRevision(command.ExpectedRevision).SetSnapshot(snapshot).SetContextFingerprint(snapshot.Fingerprint()).
		SetPolicyVersion(classificationdomain.DomainPolicyVersion).SetStatus(classificationrun.StatusBLOCKED).
		SetBlockerCode(blockerCode).SetCommandKey(command.CommandID).SetActorDisplay("Sistem clasificare").SetCreatedAt(now)
	if snapshot.Profile != nil {
		createRun.SetProfileID(snapshot.Profile.ID).SetProfileVersion(snapshot.Profile.Version)
	}
	if before.CurrentClassificationRunID != nil {
		createRun.SetSupersedesRunID(*before.CurrentClassificationRunID)
	}
	if _, err = createRun.Save(ctx); err != nil {
		return rollback(err)
	}
	update := tx.Invoice.UpdateOneID(before.ID).
		Where(invoice.PipelineStatusEQ(invoice.PipelineStatusCOMMERCIALLY_VALIDATED), invoice.RevisionEQ(command.ExpectedRevision)).
		SetPipelineStatus(invoice.PipelineStatusAWAITING_REVIEW).SetReadinessReason(blockerCode).
		SetCurrentClassificationRunID(runID).AddRevision(1).SetUpdatedAt(now)
	update.SetAccountingSnapshot(snapshot)
	blocked, err := update.Save(ctx)
	if ent.IsNotFound(err) {
		return rollback(apperrors.ErrConflict)
	}
	if err != nil {
		return rollback(err)
	}
	taskID := stableID("task", eventKey+":review")
	title := "Configurează profilul fiscal"
	if blockerCode == "INVALID_ACCOUNTING_PROFILE" {
		title = "Remediază profilul contabil și fiscal"
	}
	if _, err = tx.ValidationTask.Create().SetID(taskID).SetClientID(blocked.ClientID).SetInvoiceID(blocked.ID).
		SetClassificationRunID(runID).
		SetTaskType(validationtask.TaskTypeCLASSIFICATION).SetStatus(validationtask.StatusOPEN).
		SetTitle(title).SetReason(message).SetBlockerCode(blockerCode).
		SetCreatedByKind(validationtask.CreatedByKindSYSTEM).SetCreatedByDisplay("Sistem clasificare").
		SetCreationKey(eventKey + ":review").SetRevision(1).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		return rollback(err)
	}
	if err = createAudit(tx, ctx, auditRecord{key: eventKey + ":blocked", invoiceID: blocked.ID, taskID: taskID, clientID: blocked.ClientID, eventType: "CLASSIFICATION_BLOCKED", from: string(invoice.PipelineStatusCOMMERCIALLY_VALIDATED), to: string(invoice.PipelineStatusAWAITING_REVIEW), trigger: blockerCode, detail: message, actor: audit.ActorSystem, actorDisplay: "Sistem clasificare", correlationID: command.CorrelationID, at: now}); err != nil {
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
	if classificationRow.InvoiceID != invoiceRow.ID || classificationRow.ClientID != invoiceRow.ClientID || invoiceRow.CurrentClassificationRunID == nil || classificationRow.ClassificationRunID != *invoiceRow.CurrentClassificationRunID {
		return rollback(apperrors.ErrValidation)
	}
	if (classificationRow.ReviewStatus != lineclassification.ReviewStatusPENDING && classificationRow.ModelVersion != accounting.ModelVersion) || classificationRow.Revision != command.ExpectedClassificationRevision {
		return rollback(classificationdomain.ErrStaleReview)
	}
	status := lineclassification.ReviewStatusACCEPTED
	finalValue := classificationRow.ProposedValue
	eventType := "CLASSIFICATION_PROPOSAL_ACCEPTED"
	rejected := command.Action == "REJECT"
	if rejected {
		status = lineclassification.ReviewStatusREJECTED
		finalValue = ""
		eventType = "CLASSIFICATION_PROPOSAL_REJECTED"
	}
	if command.CorrectedValue != nil {
		status = lineclassification.ReviewStatusCORRECTED
		finalValue = *command.CorrectedValue
		eventType = "CLASSIFICATION_CORRECTED"
	}
	typed := classificationRow.ProposedTypedValue
	if classificationRow.ModelVersion == accounting.ModelVersion && !rejected {
		approvesStoredProposal := command.TypedValue == nil || classificationRow.ProposedTypedValue != nil && command.TypedValue.Text() == classificationRow.ProposedTypedValue.Text()
		if command.Action == "APPROVE" && approvesStoredProposal && len(classificationRow.ValidationResults) > 0 {
			return rollback(fmt.Errorf("%w: propunerea invalidă trebuie corectată manual", apperrors.ErrValidation))
		}
		if command.CorrectedValue != nil {
			return rollback(apperrors.ErrValidation)
		}
		if command.TypedValue != nil {
			typed = command.TypedValue
			if classificationRow.ProposedTypedValue != nil && command.TypedValue.Text() == classificationRow.ProposedTypedValue.Text() {
				status = lineclassification.ReviewStatusACCEPTED
				eventType = "CLASSIFICATION_PROPOSAL_ACCEPTED"
			} else {
				status = lineclassification.ReviewStatusCORRECTED
				eventType = "CLASSIFICATION_CORRECTED"
			}
		}
		if typed == nil || typed.Validate(string(classificationRow.Dimension)) != nil || strings.TrimSpace(command.Reason) == "" {
			return rollback(apperrors.ErrValidation)
		}
		finalValue = typed.Text()
		if classificationRow.Dimension == lineclassification.DimensionACCOUNT {
			accountRow, lookupErr := tx.Account.Query().Where(account.CodeEQ(typed.Account)).Only(ctx)
			var item *accountdomain.Account
			if lookupErr == nil {
				item = &accountdomain.Account{Code: accountRow.Code, Name: accountRow.Name, AccountType: accountRow.AccountType, Synthetic: accountRow.IsSynthetic, Postable: accountRow.Postable, Active: accountRow.IsActive}
			} else if !ent.IsNotFound(lookupErr) {
				return rollback(lookupErr)
			}
			run, runErr := tx.ClassificationRun.Get(ctx, classificationRow.ClassificationRunID)
			if runErr != nil {
				return rollback(runErr)
			}
			var allowed func(string) bool
			if run.Snapshot != nil && run.Snapshot.Profile != nil {
				allowed = run.Snapshot.Profile.AccountAllowed
			}
			if validationErr := accountdomain.ValidatePostingAccount(item, typed.Account, allowed); validationErr != nil {
				return rollback(validationErr)
			}
		}
	}
	if !rejected {
		if err := applyAccountMappingAction(ctx, tx, command, invoiceRow, classificationRow, typed, now); err != nil {
			return rollback(err)
		}
	}
	classificationUpdate := tx.LineClassification.UpdateOneID(classificationRow.ID).
		Where(lineclassification.ReviewStatusEQ(classificationRow.ReviewStatus), lineclassification.RevisionEQ(command.ExpectedClassificationRevision)).
		SetReviewStatus(status).SetReviewedAt(now).SetReviewedByDisplay(command.ActorDisplay).AddRevision(1).SetUpdatedAt(now)
	if rejected {
		classificationUpdate.ClearEffectiveValue().ClearEffectiveTypedValue().ClearEffectiveSource().SetReviewReason(command.Reason)
	} else {
		effectiveSource := "MANUAL"
		if status == lineclassification.ReviewStatusACCEPTED && classificationRow.Source == lineclassification.SourceLEARNED_MAPPING {
			effectiveSource = string(classificationdomain.SourceLearnedMapping)
		}
		classificationUpdate.SetEffectiveValue(finalValue).SetEffectiveSource(effectiveSource)
	}
	if classificationRow.ModelVersion == accounting.ModelVersion && !rejected {
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
			Model                 string               `json:"model"`
			Dimension             string               `json:"dimension"`
			Previous              *accounting.Value    `json:"previous"`
			Proposal              *accounting.Value    `json:"proposal"`
			Final                 *accounting.Value    `json:"final"`
			Reason                string               `json:"reason"`
			Evidence              *accounting.Evidence `json:"evidence"`
			MappingAction         string               `json:"mappingAction,omitempty"`
			AccountMappingID      *string              `json:"accountMappingId,omitempty"`
			AccountMappingVersion *int                 `json:"accountMappingVersion,omitempty"`
		}{classificationRow.ModelVersion, string(classificationRow.Dimension), classificationRow.EffectiveTypedValue, classificationRow.ProposedTypedValue, typed, command.Reason, classificationRow.DecisionEvidence, command.MappingAction, classificationRow.AccountMappingID, classificationRow.AccountMappingVersion})
		if err != nil {
			return rollback(err)
		}
		reviewDetail = string(evidence)
	}
	if err = createAudit(tx, ctx, auditRecord{key: eventKey + ":decision", invoiceID: invoiceRow.ID, taskID: taskRow.ID, clientID: invoiceRow.ClientID, eventType: eventType, from: classificationRow.ProposedValue, to: finalValue, trigger: "CLASSIFICATION_REVIEW", detail: reviewDetail, actor: audit.ActorUser, actorID: command.ActorID, actorDisplay: command.ActorDisplay, correlationID: command.CorrelationID, at: now}); err != nil {
		return rollback(err)
	}
	if mappingEvent := accountMappingAuditEvent(command.MappingAction); mappingEvent != "" {
		if err = createAudit(tx, ctx, auditRecord{key: eventKey + ":account-mapping-action", invoiceID: invoiceRow.ID, taskID: taskRow.ID, clientID: invoiceRow.ClientID, eventType: mappingEvent, from: proposedAccount(classificationRow), to: typed.Account, trigger: "CLASSIFICATION_REVIEW", detail: reviewDetail, actor: audit.ActorUser, actorID: command.ActorID, actorDisplay: command.ActorDisplay, correlationID: command.CorrelationID, at: now}); err != nil {
			return rollback(err)
		}
	}
	remaining, err := tx.LineClassification.Query().Where(lineclassification.InvoiceIDEQ(invoiceRow.ID), lineclassification.ClassificationRunIDEQ(classificationRow.ClassificationRunID), lineclassification.ReviewStatusIn(lineclassification.ReviewStatusPENDING, lineclassification.ReviewStatusREJECTED)).Count(ctx)
	if err != nil {
		return rollback(err)
	}
	readyToComplete := remaining == 0
	if classificationRow.ModelVersion == accounting.ModelVersion {
		currentRows, queryErr := tx.LineClassification.Query().Where(lineclassification.InvoiceIDEQ(invoiceRow.ID), lineclassification.ClassificationRunIDEQ(classificationRow.ClassificationRunID), lineclassification.ModelVersionEQ(accounting.ModelVersion)).All(ctx)
		if queryErr != nil {
			return rollback(queryErr)
		}
		resolution := make([]accounting.ClassificationResolutionItem, 0, len(currentRows))
		remaining = 0
		for _, row := range currentRows {
			resolution = append(resolution, accounting.ClassificationResolutionItem{Dimension: string(row.Dimension), Effective: row.EffectiveTypedValue, Proposed: row.ProposedTypedValue, Source: string(row.Source), ReviewStatus: string(row.ReviewStatus)})
			if accounting.ResolveClassification(string(row.Dimension), row.EffectiveTypedValue, row.ProposedTypedValue, string(row.Source), string(row.ReviewStatus)) != accounting.ResolutionFinal {
				remaining++
			}
		}
		lineCount, countErr := tx.InvoiceLine.Query().Where(invoiceline.InvoiceIDEQ(invoiceRow.ID)).Count(ctx)
		if countErr != nil {
			return rollback(countErr)
		}
		readyToComplete = accounting.IsAccountingClassificationComplete(resolution, lineCount*len(accounting.Dimensions))
	}
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

func accountMappingAuditEvent(action string) string {
	switch action {
	case "CREATE":
		return "ACCOUNT_MAPPING_CREATED"
	case "VALIDATE":
		return "ACCOUNT_MAPPING_SUGGESTION_VALIDATED"
	case "OCCURRENCE_ONLY":
		return "ACCOUNT_MAPPING_OCCURRENCE_ONLY"
	case "CORRECT":
		return "ACCOUNT_MAPPING_CORRECTED"
	case "POLICY_CHANGE":
		return "ACCOUNT_MAPPING_POLICY_CHANGED"
	default:
		return ""
	}
}

func proposedAccount(row *ent.LineClassification) string {
	if row.ProposedTypedValue == nil {
		return ""
	}
	return row.ProposedTypedValue.Account
}

func applyAccountMappingAction(ctx context.Context, tx *ent.Tx, command classificationdomain.ReviewCommand, invoiceRow *ent.Invoice, classificationRow *ent.LineClassification, typed *accounting.Value, now time.Time) error {
	if classificationRow.Dimension != lineclassification.DimensionACCOUNT {
		if command.MappingAction != "NONE" {
			return apperrors.ErrValidation
		}
		return nil
	}
	if classificationRow.ModelVersion != accounting.ModelVersion {
		return nil
	}
	if typed == nil || typed.Kind != "ACCOUNT" {
		return apperrors.ErrValidation
	}
	if _, err := tx.Account.Query().Where(account.CodeEQ(typed.Account), account.IsActiveEQ(true), account.PostableEQ(true)).Only(ctx); ent.IsNotFound(err) {
		return apperrors.ErrValidation
	} else if err != nil {
		return err
	}

	proposed := ""
	if classificationRow.ProposedTypedValue != nil {
		proposed = classificationRow.ProposedTypedValue.Account
	}
	changed := proposed != typed.Account
	source := classificationdomain.Source(classificationRow.Source)
	switch source {
	case classificationdomain.SourceNoMatch:
		if command.MappingAction != "OCCURRENCE_ONLY" {
			return apperrors.ErrValidation
		}
	case classificationdomain.SourceLearnedMapping:
		if !changed && command.MappingAction != "VALIDATE" {
			return apperrors.ErrValidation
		}
		if changed && command.MappingAction != "OCCURRENCE_ONLY" && command.MappingAction != "CORRECT" && command.MappingAction != "POLICY_CHANGE" {
			return apperrors.ErrValidation
		}
	case classificationdomain.SourceAmbiguous:
		if command.MappingAction != "OCCURRENCE_ONLY" && command.MappingAction != "CORRECT" && command.MappingAction != "POLICY_CHANGE" {
			return apperrors.ErrValidation
		}
	default:
		if command.MappingAction != "NONE" && command.MappingAction != "OCCURRENCE_ONLY" {
			return apperrors.ErrValidation
		}
	}
	if command.MappingAction == "NONE" || command.MappingAction == "VALIDATE" || command.MappingAction == "OCCURRENCE_ONLY" {
		return nil
	}

	lineRow, err := tx.InvoiceLine.Query().Where(invoiceline.IDEQ(classificationRow.InvoiceLineID)).Only(ctx)
	if err != nil {
		return err
	}
	if invoiceRow.NormalizedSupplierCui == nil || strings.TrimSpace(*invoiceRow.NormalizedSupplierCui) == "" {
		return apperrors.ErrValidation
	}
	if classificationRow.AccountMappingID == nil || classificationRow.AccountMappingVersion == nil || command.ExpectedMappingRevision == 0 {
		return apperrors.ErrValidation
	}
	mappingRow, err := tx.AccountMapping.Query().Where(accountmapping.IDEQ(*classificationRow.AccountMappingID), accountmapping.ClientIDEQ(invoiceRow.ClientID)).Only(ctx)
	if ent.IsNotFound(err) {
		return apperrors.ErrConflict
	}
	if err != nil {
		return err
	}
	if mappingRow.Revision != command.ExpectedMappingRevision || mappingRow.CurrentVersion != *classificationRow.AccountMappingVersion {
		return apperrors.ErrConflict
	}
	nextVersion := mappingRow.CurrentVersion + 1
	updated, err := tx.AccountMapping.UpdateOneID(mappingRow.ID).Where(accountmapping.RevisionEQ(command.ExpectedMappingRevision), accountmapping.CurrentVersionEQ(mappingRow.CurrentVersion)).
		SetCurrentVersion(nextVersion).AddRevision(1).SetUpdatedAt(now).Save(ctx)
	if ent.IsNotFound(err) {
		return apperrors.ErrConflict
	}
	if err != nil {
		return err
	}
	if updated.CurrentVersion != nextVersion {
		return apperrors.ErrConflict
	}
	changeKind := accountmappingversion.ChangeKindCORRECTION
	if command.MappingAction == "POLICY_CHANGE" {
		changeKind = accountmappingversion.ChangeKindPOLICY_CHANGE
	}
	create := tx.AccountMappingVersion.Create().SetID(stableID("account-mapping-version", mappingRow.ID+fmt.Sprintf(":%d", nextVersion))).SetMappingID(mappingRow.ID).SetVersion(nextVersion).
		SetAccountCode(typed.Account).SetChangeKind(changeKind).SetSourceClassificationID(classificationRow.ID).SetSourceInvoiceLineID(lineRow.ID).
		SetRawDescriptionSnapshot(lineRow.Description).SetActorDisplay(command.ActorDisplay).SetReason(command.Reason).SetCreatedAt(now).SetCommandKey("account-mapping:" + command.CommandID)
	if command.ActorID != "" {
		create.SetActorID(command.ActorID)
	}
	_, err = create.Save(ctx)
	return err
}

func accountingDatePointer(value *time.Time) *accountingdate.Date {
	if value == nil {
		return nil
	}
	date := accountingdate.FromTime(*value)
	return &date
}
