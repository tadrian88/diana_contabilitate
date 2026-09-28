package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"diana-contabilitate/backend/ent"
	"diana-contabilitate/backend/ent/invoice"
	"diana-contabilitate/backend/ent/lineclassification"
	"diana-contabilitate/backend/ent/validationtask"
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/audit"
	classificationdomain "diana-contabilitate/backend/internal/classification"
)

func (s *Store) ApproveAllClassifications(ctx context.Context, command classificationdomain.ApproveAllCommand, now time.Time) (bool, error) {
	eventKey := "classification-approve-all:" + command.CommandID
	if exists, err := s.auditExists(ctx, eventKey+":completed"); err != nil || exists {
		return false, err
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return false, err
	}
	rollback := func(cause error) (bool, error) { _ = tx.Rollback(); return false, cause }
	task, err := tx.ValidationTask.Query().Where(validationtask.IDEQ(command.TaskID)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return rollback(apperrors.ErrNotFound)
		}
		return rollback(err)
	}
	item, err := tx.Invoice.Query().Where(invoice.IDEQ(command.InvoiceID)).WithLines().Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return rollback(apperrors.ErrNotFound)
		}
		return rollback(err)
	}
	if task.InvoiceID != item.ID || task.ClientID != item.ClientID || task.TaskType != validationtask.TaskTypeCLASSIFICATION || task.Status != validationtask.StatusOPEN || task.Revision != command.ExpectedTaskRevision || item.Revision != command.ExpectedInvoiceRevision || item.CurrentClassificationRunID == nil || task.ClassificationRunID == nil || *task.ClassificationRunID != *item.CurrentClassificationRunID {
		return rollback(classificationdomain.ErrStaleReview)
	}
	rows, err := tx.LineClassification.Query().Where(lineclassification.ClientIDEQ(item.ClientID), lineclassification.InvoiceIDEQ(item.ID), lineclassification.ClassificationRunIDEQ(*item.CurrentClassificationRunID), lineclassification.ModelVersionEQ(accounting.ModelVersion)).All(ctx)
	if err != nil {
		return rollback(err)
	}
	eligible := map[string]*ent.LineClassification{}
	for _, row := range rows {
		if row.ReviewStatus == lineclassification.ReviewStatusPENDING && row.ProposedTypedValue != nil && row.ProposedTypedValue.Validate(string(row.Dimension)) == nil && len(row.ValidationResults) == 0 {
			eligible[row.ID] = row
		}
	}
	if len(eligible) != len(command.Expected) {
		return rollback(classificationdomain.ErrStaleReview)
	}
	for _, expected := range command.Expected {
		row := eligible[expected.ID]
		if row == nil || row.Revision != expected.Revision {
			return rollback(classificationdomain.ErrStaleReview)
		}
	}
	for _, expected := range command.Expected {
		row := eligible[expected.ID]
		updated, updateErr := tx.LineClassification.UpdateOneID(row.ID).Where(lineclassification.RevisionEQ(expected.Revision), lineclassification.ReviewStatusEQ(lineclassification.ReviewStatusPENDING)).SetReviewStatus(lineclassification.ReviewStatusACCEPTED).SetEffectiveTypedValue(row.ProposedTypedValue).SetEffectiveValue(row.ProposedTypedValue.Text()).SetEffectiveSource("MANUAL").SetReviewReason("Aprobare în grup a propunerilor valide.").SetReviewedAt(now).SetReviewedByDisplay(command.ActorDisplay).AddRevision(1).SetUpdatedAt(now).Save(ctx)
		if ent.IsNotFound(updateErr) {
			return rollback(classificationdomain.ErrStaleReview)
		}
		if updateErr != nil {
			return rollback(updateErr)
		}
		if command.ActorID != "" {
			if _, updateErr = tx.LineClassification.UpdateOneID(updated.ID).SetReviewedByID(command.ActorID).Save(ctx); updateErr != nil {
				return rollback(updateErr)
			}
		}
	}
	refreshed, err := tx.LineClassification.Query().Where(lineclassification.ClassificationRunIDEQ(*item.CurrentClassificationRunID), lineclassification.ModelVersionEQ(accounting.ModelVersion)).All(ctx)
	if err != nil {
		return rollback(err)
	}
	resolution := make([]accounting.ClassificationResolutionItem, 0, len(refreshed))
	for _, row := range refreshed {
		resolution = append(resolution, accounting.ClassificationResolutionItem{Dimension: string(row.Dimension), Effective: row.EffectiveTypedValue, Proposed: row.ProposedTypedValue, Source: string(row.Source), ReviewStatus: string(row.ReviewStatus)})
	}
	complete := accounting.IsAccountingClassificationComplete(resolution, len(item.Edges.Lines)*len(accounting.Dimensions))
	taskUpdate := tx.ValidationTask.UpdateOneID(task.ID).Where(validationtask.RevisionEQ(command.ExpectedTaskRevision), validationtask.StatusEQ(validationtask.StatusOPEN)).AddRevision(1).SetUpdatedAt(now)
	if complete {
		metadata, _ := json.Marshal(map[string]string{"action": "APPROVE_ALL"})
		taskUpdate.SetStatus(validationtask.StatusRESOLVED).SetResolvedAt(now).SetResolutionMetadata(metadata)
	}
	if _, err = taskUpdate.Save(ctx); ent.IsNotFound(err) {
		return rollback(classificationdomain.ErrStaleReview)
	} else if err != nil {
		return rollback(err)
	}
	if complete {
		if _, err = tx.Invoice.UpdateOneID(item.ID).Where(invoice.RevisionEQ(command.ExpectedInvoiceRevision), invoice.PipelineStatusEQ(invoice.PipelineStatusAWAITING_REVIEW)).SetPipelineStatus(invoice.PipelineStatusREADY_FOR_SAGA).SetSagaStatus(invoice.SagaStatusREADY).AddRevision(1).SetUpdatedAt(now).Save(ctx); ent.IsNotFound(err) {
			return rollback(classificationdomain.ErrStaleReview)
		} else if err != nil {
			return rollback(err)
		}
		if err = createOutbox(tx, ctx, item.ID, eventKey+":continue", command.CorrelationID, now); err != nil {
			return rollback(err)
		}
	}
	if err = createAudit(tx, ctx, auditRecord{key: eventKey + ":completed", invoiceID: item.ID, taskID: task.ID, clientID: item.ClientID, eventType: "ACCOUNTING_APPROVE_ALL_COMPLETED", trigger: "APPROVE_ALL", detail: fmt.Sprintf("Approved %d technically valid current-run proposals; complete=%t.", len(eligible), complete), actor: audit.ActorUser, actorID: command.ActorID, actorDisplay: command.ActorDisplay, correlationID: command.CorrelationID, at: now}); err != nil {
		return rollback(err)
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	if s.AccountingWorkflowObserver != nil {
		s.AccountingWorkflowObserver.AccountingApproveAllSucceeded()
	}
	return true, nil
}
