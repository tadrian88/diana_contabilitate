package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/contracts"
)

// DeleteMistakenContract hides an unused, mistaken upload without destroying
// evidence. Releasing its active-only uniqueness keys permits fresh ingestion
// of the same PDF and reference. Any invoice/match/commercial use blocks it.
func (s *Store) DeleteMistakenContract(ctx context.Context, id string, revision uint64, actorID, actorDisplay string, now time.Time) (bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var clientID, reference, state string
	var currentRevision uint64
	err = tx.QueryRowContext(ctx, `SELECT client_id,reference,lifecycle_state,revision FROM contracts WHERE id=$1 FOR UPDATE`, id).Scan(&clientID, &reference, &state, &currentRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return false, apperrors.ErrNotFound
	}
	if err != nil {
		return false, err
	}
	var activeDocuments bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM contract_source_documents d WHERE (d.confirmed_contract_id=$1 OR d.dossier_id IN (SELECT id FROM contract_dossiers WHERE contract_id=$1)) AND d.lifecycle_state<>'DISCARDED')`, id).Scan(&activeDocuments); err != nil {
		return false, err
	}
	if state == "ARCHIVED" && !activeDocuments {
		return false, nil
	}
	if currentRevision != revision {
		return false, apperrors.ErrConflict
	}
	var used bool
	if err = tx.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM invoice_contract_associations WHERE contract_id=$1)
		OR EXISTS(SELECT 1 FROM contract_match_candidates WHERE contract_id=$1)
		OR EXISTS(SELECT 1 FROM invoice_commercial_validation_runs r JOIN contract_commercial_snapshots s ON s.id=r.snapshot_id WHERE s.contract_id=$1)
		OR EXISTS(SELECT 1 FROM contract_commercial_ledger l JOIN contract_dossiers d ON d.id=l.dossier_id WHERE d.contract_id=$1)`, id).Scan(&used); err != nil {
		return false, err
	}
	if used {
		return false, contracts.ErrContractInUse
	}
	if _, err = tx.ExecContext(ctx, `UPDATE contracts SET lifecycle_state='ARCHIVED',revision=revision+1,updated_at=$2 WHERE id=$1 AND lifecycle_state='ACTIVE'`, id, now); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE contract_dossiers SET status='ARCHIVED',revision=revision+1,updated_at=$2 WHERE contract_id=$1 AND status<>'ARCHIVED'`, id, now); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE contract_source_documents SET lifecycle_state='DISCARDED',revision=revision+1,updated_at=$2 WHERE lifecycle_state<>'DISCARDED' AND (confirmed_contract_id=$1 OR dossier_id IN (SELECT id FROM contract_dossiers WHERE contract_id=$1))`, id, now); err != nil {
		return false, err
	}
	key := "contract-mistaken-delete:" + id
	if _, err = tx.ExecContext(ctx, `INSERT INTO activity_events(id,client_id,aggregate_type,aggregate_id,event_type,occurred_at,actor_kind,actor_id,actor_display,automatic,detail,before_snapshot,after_snapshot,trigger,idempotency_key)
		VALUES($1,$2,'CONTRACT',$3,'CONTRACT_MISTAKEN_UPLOAD_DISCARDED',$4,'USER',$5,$6,false,$7,'{"active":true}'::jsonb,'{"active":false,"sourceDiscarded":true}'::jsonb,'USER_DELETE',$8)`, stableID("evt", key), clientID, id, now, actorID, actorDisplay, "Încărcarea greșită a contractului "+reference+" a fost scoasă din utilizare; dovezile au fost păstrate.", key); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
