package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"diana-contabilitate/backend/ent"
	"diana-contabilitate/backend/ent/accountingclient"
	"diana-contabilitate/backend/ent/activityevent"
	entinvoice "diana-contabilitate/backend/ent/invoice"
	entsagaattempt "diana-contabilitate/backend/ent/sagaexportattempt"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/audit"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/saga"
)

func (s *Store) LoadExportInput(ctx context.Context, invoiceID string) (*invoicing.Invoice, saga.ClientIdentity, error) {
	item, err := s.GetInvoice(ctx, invoiceID)
	if err != nil {
		return nil, saga.ClientIdentity{}, err
	}
	client, err := s.Client.AccountingClient.Query().Where(accountingclient.IDEQ(item.ClientID)).Only(ctx)
	if err != nil {
		return nil, saga.ClientIdentity{}, fmt.Errorf("load SAGA client identity: %w", err)
	}
	var enabled bool
	err = s.DB.QueryRowContext(ctx, `SELECT enabled FROM client_saga_configurations WHERE client_id=$1`, item.ClientID).Scan(&enabled)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, saga.ClientIdentity{}, err
	}
	// Existing direct fixtures without a configuration row retain legacy export behavior.
	if err == nil && !enabled {
		return nil, saga.ClientIdentity{}, fmt.Errorf("%w: export SAGA neconfigurat pentru client", apperrors.ErrValidation)
	}
	return item, saga.ClientIdentity{ID: client.ID, Name: client.Name, CUI: client.Cui}, nil
}

func (s *Store) FindAttempt(ctx context.Context, invoiceID string, revision uint64, exporterVersion string) (*saga.Attempt, error) {
	row, err := s.Client.SagaExportAttempt.Query().Where(
		entsagaattempt.InvoiceIDEQ(invoiceID),
		entsagaattempt.InvoiceRevisionEQ(revision),
		entsagaattempt.ExporterVersionEQ(exporterVersion),
	).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, saga.ErrAttemptNotFound
	}
	if err != nil {
		return nil, err
	}
	return sagaAttemptDomain(row), nil
}

func (s *Store) LatestGeneratedAttempt(ctx context.Context, invoiceID, clientID string) (*saga.Attempt, error) {
	row, err := s.Client.SagaExportAttempt.Query().Where(
		entsagaattempt.InvoiceIDEQ(invoiceID),
		entsagaattempt.ClientIDEQ(clientID),
		entsagaattempt.StatusEQ(entsagaattempt.StatusGENERATED),
	).Order(ent.Desc(entsagaattempt.FieldCompletedAt)).First(ctx)
	if ent.IsNotFound(err) {
		return nil, saga.ErrAttemptNotFound
	}
	if err != nil {
		return nil, err
	}
	return sagaAttemptDomain(row), nil
}

func (s *Store) GetExportView(ctx context.Context, clientID, invoiceID string) (*saga.ExportView, error) {
	row, err := s.Client.SagaExportAttempt.Query().Where(
		entsagaattempt.InvoiceIDEQ(invoiceID),
		entsagaattempt.ClientIDEQ(clientID),
	).Order(ent.Desc(entsagaattempt.FieldCompletedAt)).First(ctx)
	if ent.IsNotFound(err) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.exportView(ctx, row)
}

func (s *Store) exportView(ctx context.Context, row *ent.SagaExportAttempt) (*saga.ExportView, error) {
	view := &saga.ExportView{
		AttemptID: row.ID, ArtifactStatus: saga.AttemptStatus(row.Status),
		GeneratedAt: row.CompletedAt, ConfirmedAt: row.ConfirmedAt,
		InvoiceRevision: row.InvoiceRevision,
	}
	if row.Filename != nil {
		view.Filename = *row.Filename
	}
	if row.ConfirmedByDisplay != nil {
		view.ConfirmedBy = row.ConfirmedByDisplay
	} else {
		view.ConfirmedBy = row.ConfirmedByID
	}
	if row.ConfirmationType != nil {
		value := saga.ConfirmationType(*row.ConfirmationType)
		view.ConfirmationType = &value
	}
	event, err := s.Client.ActivityEvent.Query().Where(
		activityevent.InvoiceIDEQ(row.InvoiceID),
		activityevent.ClientIDEQ(row.ClientID),
		activityevent.EventTypeEQ("SAGA_EXPORT_DOWNLOADED"),
		activityevent.CorrelationIDEQ(row.ID),
	).Order(ent.Desc(activityevent.FieldOccurredAt)).First(ctx)
	if err == nil {
		view.DownloadedAt = &event.OccurredAt
	} else if !ent.IsNotFound(err) {
		return nil, err
	}
	return view, nil
}

func (s *Store) DownloadExportArtifact(ctx context.Context, clientID, invoiceID string, actor saga.Actor, now time.Time) (*saga.Download, error) {
	row, err := s.Client.SagaExportAttempt.Query().Where(
		entsagaattempt.InvoiceIDEQ(invoiceID), entsagaattempt.ClientIDEQ(clientID),
	).Order(ent.Desc(entsagaattempt.FieldCompletedAt)).First(ctx)
	if ent.IsNotFound(err) {
		return nil, apperrors.ErrNotFound
	}
	if err == nil && row.Status != entsagaattempt.StatusGENERATED {
		return nil, saga.ErrArtifactUnavailable
	}
	if err != nil {
		return nil, err
	}
	eventKey := "saga-download:" + row.ID + ":" + actor.CorrelationID
	if exists, existsErr := s.auditExists(ctx, eventKey); existsErr != nil {
		return nil, existsErr
	} else if exists {
		attempt := sagaAttemptDomain(row)
		return &saga.Download{Artifact: *attempt.Artifact, AttemptID: row.ID}, nil
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	if err = createAudit(tx, ctx, auditRecord{
		key: eventKey, invoiceID: invoiceID, clientID: clientID,
		eventType: "SAGA_EXPORT_DOWNLOADED", from: "GENERATED", to: "DOWNLOADED", trigger: "SAGA_ARTIFACT_DOWNLOAD",
		detail: "Generated SAGA XML artifact downloaded; this does not confirm import into SAGA.",
		actor:  audit.ActorUser, actorID: actor.ID, actorDisplay: actor.Display, correlationID: row.ID, at: now,
	}); err != nil {
		_ = tx.Rollback()
		if IsConstraintError(err) {
			if exists, existsErr := s.auditExists(ctx, eventKey); existsErr == nil && exists {
				attempt := sagaAttemptDomain(row)
				return &saga.Download{Artifact: *attempt.Artifact, AttemptID: row.ID}, nil
			}
		}
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	attempt := sagaAttemptDomain(row)
	return &saga.Download{Artifact: *attempt.Artifact, AttemptID: row.ID}, nil
}

func (s *Store) ConfirmSagaImport(ctx context.Context, command saga.ConfirmCommand, now time.Time) (*invoicing.Invoice, *saga.ExportView, bool, error) {
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return nil, nil, false, err
	}
	rollback := func(cause error) (*invoicing.Invoice, *saga.ExportView, bool, error) {
		_ = tx.Rollback()
		return nil, nil, false, cause
	}
	prior, err := tx.ActivityEvent.Query().Where(activityevent.IdempotencyKeyEQ("saga-confirm:" + command.CommandID)).Only(ctx)
	if err == nil {
		if prior.EventType != "SAGA_IMPORT_CONFIRMED" || prior.ClientID != command.ClientID || prior.InvoiceID == nil || *prior.InvoiceID != command.InvoiceID {
			return rollback(apperrors.ErrConflict)
		}
		_ = tx.Rollback()
		return s.confirmedSagaResult(ctx, command)
	}
	if !ent.IsNotFound(err) {
		return rollback(err)
	}
	attemptRow, err := tx.SagaExportAttempt.Query().Where(
		entsagaattempt.IDEQ(command.AttemptID),
		entsagaattempt.InvoiceIDEQ(command.InvoiceID),
		entsagaattempt.ClientIDEQ(command.ClientID),
	).Only(ctx)
	if ent.IsNotFound(err) {
		return rollback(apperrors.ErrNotFound)
	}
	if err != nil {
		return rollback(err)
	}
	latest, err := tx.SagaExportAttempt.Query().Where(
		entsagaattempt.InvoiceIDEQ(command.InvoiceID), entsagaattempt.ClientIDEQ(command.ClientID),
	).Order(ent.Desc(entsagaattempt.FieldCompletedAt)).First(ctx)
	if err != nil {
		return rollback(err)
	}
	if latest.ID != attemptRow.ID || attemptRow.Status != entsagaattempt.StatusGENERATED {
		return rollback(apperrors.ErrConflict)
	}
	invoiceRow, err := tx.Invoice.Query().Where(entinvoice.IDEQ(command.InvoiceID), entinvoice.ClientIDEQ(command.ClientID)).Only(ctx)
	if ent.IsNotFound(err) {
		return rollback(apperrors.ErrNotFound)
	}
	if err != nil {
		return rollback(err)
	}
	if attemptRow.ConfirmedAt != nil {
		if invoiceRow.PipelineStatus != entinvoice.PipelineStatusEXPORTED {
			return rollback(apperrors.ErrConflict)
		}
		_ = tx.Rollback()
		item, getErr := s.GetInvoice(ctx, command.InvoiceID)
		view, viewErr := s.GetExportView(ctx, command.ClientID, command.InvoiceID)
		if getErr != nil {
			return nil, nil, false, getErr
		}
		return item, view, false, viewErr
	}
	if invoiceRow.PipelineStatus == entinvoice.PipelineStatusEXPORTED {
		_ = tx.Rollback()
		return s.confirmedSagaResult(ctx, command)
	}
	if invoiceRow.PipelineStatus != entinvoice.PipelineStatusEXPORTING {
		return rollback(apperrors.ErrValidation)
	}
	if invoiceRow.Revision != command.ExpectedInvoiceRevision {
		return rollback(apperrors.ErrConflict)
	}
	attemptUpdate := tx.SagaExportAttempt.UpdateOneID(attemptRow.ID).
		Where(entsagaattempt.ConfirmedAtIsNil()).
		SetConfirmedAt(now).SetConfirmedByID(command.Actor.ID).
		SetConfirmationType(entsagaattempt.ConfirmationTypeHUMAN)
	if command.Actor.Display != "" {
		attemptUpdate.SetConfirmedByDisplay(command.Actor.Display)
	}
	if command.Note != "" {
		attemptUpdate.SetConfirmationNote(command.Note)
	}
	if _, err = attemptUpdate.Save(ctx); err != nil {
		if ent.IsNotFound(err) {
			_ = tx.Rollback()
			return s.confirmedSagaResult(ctx, command)
		}
		return rollback(err)
	}
	updated, err := tx.Invoice.UpdateOneID(invoiceRow.ID).Where(
		entinvoice.ClientIDEQ(command.ClientID),
		entinvoice.PipelineStatusEQ(entinvoice.PipelineStatusEXPORTING),
		entinvoice.RevisionEQ(command.ExpectedInvoiceRevision),
	).SetPipelineStatus(entinvoice.PipelineStatusEXPORTED).
		SetSagaStatus(entinvoice.SagaStatusEXPORTED).AddRevision(1).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return rollback(apperrors.ErrConflict)
		}
		return rollback(err)
	}
	if err = createAudit(tx, ctx, auditRecord{
		key: "saga-confirm:" + command.CommandID, invoiceID: updated.ID, clientID: updated.ClientID,
		eventType: "SAGA_IMPORT_CONFIRMED", from: string(invoicing.StatusExporting), to: string(invoicing.StatusExported), trigger: "HUMAN_SAGA_IMPORT_CONFIRMATION",
		detail: "User manually confirmed import of the exact generated artifact into SAGA; no machine acknowledgement was received.",
		actor:  audit.ActorUser, actorID: command.Actor.ID, actorDisplay: command.Actor.Display, correlationID: command.Actor.CorrelationID, at: now,
	}); err != nil {
		return rollback(err)
	}
	if err = tx.Commit(); err != nil {
		return nil, nil, false, err
	}
	item, err := s.GetInvoice(ctx, command.InvoiceID)
	if err != nil {
		return nil, nil, false, err
	}
	view, err := s.GetExportView(ctx, command.ClientID, command.InvoiceID)
	return item, view, true, err
}

func (s *Store) confirmedSagaResult(ctx context.Context, command saga.ConfirmCommand) (*invoicing.Invoice, *saga.ExportView, bool, error) {
	attempt, err := s.Client.SagaExportAttempt.Query().Where(
		entsagaattempt.IDEQ(command.AttemptID), entsagaattempt.InvoiceIDEQ(command.InvoiceID),
		entsagaattempt.ClientIDEQ(command.ClientID), entsagaattempt.ConfirmedAtNotNil(),
	).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil, false, apperrors.ErrConflict
	}
	if err != nil {
		return nil, nil, false, err
	}
	invoiceRow, err := s.Client.Invoice.Query().Where(
		entinvoice.IDEQ(command.InvoiceID), entinvoice.ClientIDEQ(command.ClientID),
		entinvoice.PipelineStatusEQ(entinvoice.PipelineStatusEXPORTED),
	).Only(ctx)
	if ent.IsNotFound(err) || attempt.ID == "" {
		return nil, nil, false, apperrors.ErrConflict
	}
	if err != nil {
		return nil, nil, false, err
	}
	item, err := s.GetInvoice(ctx, invoiceRow.ID)
	if err != nil {
		return nil, nil, false, err
	}
	view, err := s.GetExportView(ctx, command.ClientID, command.InvoiceID)
	return item, view, false, err
}

func (s *Store) SaveAttempt(ctx context.Context, attempt saga.Attempt) (*saga.Attempt, bool, error) {
	if existing, err := s.FindAttempt(ctx, attempt.InvoiceID, attempt.InvoiceRevision, attempt.ExporterVersion); err == nil {
		return existing, false, nil
	} else if !errors.Is(err, saga.ErrAttemptNotFound) {
		return nil, false, err
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return nil, false, err
	}
	rollback := func(cause error) (*saga.Attempt, bool, error) {
		_ = tx.Rollback()
		return nil, false, cause
	}
	create := tx.SagaExportAttempt.Create().
		SetID(attempt.ID).
		SetInvoiceID(attempt.InvoiceID).
		SetClientID(attempt.ClientID).
		SetInvoiceRevision(attempt.InvoiceRevision).
		SetExporterVersion(attempt.ExporterVersion).
		SetStatus(entsagaattempt.Status(attempt.Status)).
		SetStartedAt(attempt.StartedAt).
		SetCompletedAt(attempt.CompletedAt)
	if attempt.Artifact != nil {
		create.SetFilename(attempt.Artifact.Filename).
			SetContentType(attempt.Artifact.ContentType).
			SetPayload(attempt.Artifact.Payload).
			SetPayloadSha256(attempt.Artifact.SHA256).
			SetClassificationSnapshot(attempt.ClassificationSnapshot)
	}
	if attempt.FailureCategory != nil {
		create.SetFailureCategory(entsagaattempt.FailureCategory(*attempt.FailureCategory))
	}
	if attempt.SafeError != nil {
		create.SetSafeError(*attempt.SafeError)
	}
	row, err := create.Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		if IsConstraintError(err) {
			existing, findErr := s.FindAttempt(ctx, attempt.InvoiceID, attempt.InvoiceRevision, attempt.ExporterVersion)
			return existing, false, findErr
		}
		return nil, false, err
	}
	if err = createAudit(tx, ctx, auditRecord{
		key: "saga-export:" + attempt.ID + ":started", invoiceID: attempt.InvoiceID, clientID: attempt.ClientID,
		eventType: "SAGA_EXPORT_STARTED", from: "READY", to: "GENERATING", trigger: "SAGA_EXPORT",
		detail: fmt.Sprintf("SAGA export generation started with exporter %s for invoice revision %d.", attempt.ExporterVersion, attempt.InvoiceRevision),
		actor:  audit.ActorSystem, actorDisplay: "Sistem export SAGA", correlationID: attempt.ID, at: attempt.StartedAt,
	}); err != nil {
		return rollback(err)
	}
	eventType, target, detail := "SAGA_EXPORT_ARTIFACT_GENERATED", "GENERATED", "SAGA import artifact generated and stored; local SAGA import is not acknowledged."
	if attempt.Status == saga.AttemptFailed {
		eventType, target, detail = "SAGA_EXPORT_ARTIFACT_FAILED", "FAILED", "SAGA export generation failed with a permanent, safe error."
	}
	if err = createAudit(tx, ctx, auditRecord{
		key: "saga-export:" + attempt.ID + ":completed", invoiceID: attempt.InvoiceID, clientID: attempt.ClientID,
		eventType: eventType, from: "GENERATING", to: target, trigger: "SAGA_EXPORT",
		detail: detail, actor: audit.ActorSystem, actorDisplay: "Sistem export SAGA", correlationID: attempt.ID, at: attempt.CompletedAt,
	}); err != nil {
		return rollback(err)
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	return sagaAttemptDomain(row), true, nil
}

func sagaAttemptDomain(row *ent.SagaExportAttempt) *saga.Attempt {
	result := &saga.Attempt{
		ID: row.ID, InvoiceID: row.InvoiceID, ClientID: row.ClientID,
		InvoiceRevision: row.InvoiceRevision, ExporterVersion: row.ExporterVersion,
		Status: saga.AttemptStatus(row.Status), ClassificationSnapshot: row.ClassificationSnapshot,
		StartedAt: row.StartedAt, CompletedAt: row.CompletedAt,
		ConfirmedAt: row.ConfirmedAt, ConfirmedByID: row.ConfirmedByID,
		ConfirmedByDisplay: row.ConfirmedByDisplay, ConfirmationNote: row.ConfirmationNote,
	}
	if row.ConfirmationType != nil {
		value := saga.ConfirmationType(*row.ConfirmationType)
		result.ConfirmationType = &value
	}
	if row.Filename != nil && row.ContentType != nil && row.PayloadSha256 != nil {
		result.Artifact = &saga.Artifact{
			Filename: *row.Filename, ContentType: *row.ContentType, Payload: row.Payload,
			SHA256: *row.PayloadSha256, ExporterVersion: row.ExporterVersion,
			ClassificationSnapshot: row.ClassificationSnapshot,
		}
	}
	if row.FailureCategory != nil {
		value := saga.FailureCategory(*row.FailureCategory)
		result.FailureCategory = &value
	}
	result.SafeError = row.SafeError
	return result
}
