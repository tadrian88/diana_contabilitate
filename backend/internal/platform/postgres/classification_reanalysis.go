package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/classification"
)

func (s *Store) PrepareClassificationReanalysis(ctx context.Context, command classification.ReanalysisCommand, now time.Time) (uint64, bool, error) {
	var replayInvoice, replayClient string
	var requested, prepared uint64
	err := s.DB.QueryRowContext(ctx, `SELECT client_id,invoice_id,requested_revision,prepared_revision FROM classification_reanalysis_commands WHERE command_key=$1`, command.CommandID).Scan(&replayClient, &replayInvoice, &requested, &prepared)
	if err == nil {
		if replayClient != command.ClientID || replayInvoice != command.InvoiceID || requested != command.ExpectedRevision {
			return 0, false, apperrors.ErrConflict
		}
		return prepared, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	var clientID, pipeline, saga string
	var revision uint64
	err = tx.QueryRowContext(ctx, `SELECT client_id,pipeline_status,saga_status,revision FROM invoices WHERE id=$1 FOR UPDATE`, command.InvoiceID).Scan(&clientID, &pipeline, &saga, &revision)
	if errors.Is(err, sql.ErrNoRows) || clientID != command.ClientID {
		return 0, false, apperrors.ErrNotFound
	}
	if err != nil {
		return 0, false, err
	}
	if revision != command.ExpectedRevision {
		return 0, false, apperrors.ErrConflict
	}
	if pipeline != "AWAITING_REVIEW" && pipeline != "READY_FOR_SAGA" && pipeline != "CLASSIFIED" {
		return 0, false, fmt.Errorf("%w: factura nu poate fi reanalizată din starea %s", apperrors.ErrConflict, pipeline)
	}
	if saga == "EXPORTING" || saga == "EXPORTED" {
		return 0, false, fmt.Errorf("%w: reanalizarea nu este permisă după pornirea exportului SAGA", apperrors.ErrConflict)
	}
	prepared = revision + 1
	if _, err = tx.ExecContext(ctx, `UPDATE validation_tasks SET status='RESOLVED',resolved_at=$2,resolution_metadata=jsonb_build_object('action','REANALYSIS_SUPERSEDED','commandKey',$3::text),revision=revision+1,updated_at=$2 WHERE invoice_id=$1 AND status IN ('OPEN','WAITING')`, command.InvoiceID, now, command.CommandID); err != nil {
		return 0, false, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE invoices SET pipeline_status='COMMERCIALLY_VALIDATED',saga_status='NOT_READY',readiness_reason='',revision=revision+1,updated_at=$4 WHERE id=$1 AND client_id=$2 AND revision=$3`, command.InvoiceID, command.ClientID, revision, now)
	if err != nil {
		return 0, false, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return 0, false, apperrors.ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO classification_reanalysis_commands(command_key,client_id,invoice_id,requested_revision,prepared_revision,created_at) VALUES($1,$2,$3,$4,$5,$6)`, command.CommandID, command.ClientID, command.InvoiceID, revision, prepared, now); err != nil {
		return 0, false, err
	}
	before, _ := json.Marshal(map[string]any{"revision": revision, "pipelineStatus": pipeline})
	after, _ := json.Marshal(map[string]any{"revision": prepared, "pipelineStatus": "COMMERCIALLY_VALIDATED"})
	eventKey := "classification-reanalysis:" + command.CommandID + ":prepared"
	if _, err = tx.ExecContext(ctx, `INSERT INTO activity_events(id,client_id,invoice_id,aggregate_type,aggregate_id,event_type,occurred_at,actor_kind,actor_id,actor_display,automatic,detail,before_snapshot,after_snapshot,correlation_id,pipeline_from,pipeline_to,trigger,idempotency_key) VALUES($1,$2,$3,'INVOICE',$3,'CLASSIFICATION_REANALYSIS_REQUESTED',$4,'USER',$5,$6,false,'Reanalizare explicită folosind configurația curentă.',$7,$8,$9,$10,'COMMERCIALLY_VALIDATED','USER_REANALYSIS',$11)`, stableID("evt", eventKey), command.ClientID, command.InvoiceID, now, command.ActorID, command.ActorDisplay, before, after, nullText(command.CorrelationID), pipeline, eventKey); err != nil {
		return 0, false, err
	}
	if err = tx.Commit(); err != nil {
		return 0, false, err
	}
	return prepared, true, nil
}
