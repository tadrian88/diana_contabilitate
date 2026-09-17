package postgres

import (
	"context"
	"database/sql"
	"time"

	"diana-contabilitate/backend/internal/outbox"
)

func (s *Store) Claim(ctx context.Context, owner string, limit int, now time.Time, claimTTL time.Duration) ([]outbox.Entry, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `
WITH candidates AS (
    SELECT id
    FROM outbox_entries
    WHERE (status = 'PENDING' AND available_at <= $1)
       OR (status = 'CLAIMED' AND claimed_at <= $2)
    ORDER BY created_at, id
    FOR UPDATE SKIP LOCKED
    LIMIT $3
)
UPDATE outbox_entries AS entry
SET status = 'CLAIMED', claim_owner = $4, claimed_at = $1,
    attempts = entry.attempts + 1, last_error = NULL
FROM candidates
WHERE entry.id = candidates.id
RETURNING entry.id, entry.event_type, entry.aggregate_id,
          entry.idempotency_key, COALESCE(entry.correlation_id, entry.id), entry.attempts`, now, now.Add(-claimTTL), limit, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]outbox.Entry, 0, limit)
	for rows.Next() {
		var entry outbox.Entry
		var attempts int64
		if err = rows.Scan(&entry.ID, &entry.EventType, &entry.AggregateID, &entry.IdempotencyKey, &entry.CorrelationID, &attempts); err != nil {
			return nil, err
		}
		entry.Attempts = uint(attempts)
		entries = append(entries, entry)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *Store) MarkDispatched(ctx context.Context, id, owner string, now time.Time) error {
	result, err := s.DB.ExecContext(ctx, `
UPDATE outbox_entries
SET status = 'DISPATCHED', dispatched_at = $3,
    claim_owner = NULL, claimed_at = NULL, last_error = NULL
WHERE id = $1 AND status = 'CLAIMED' AND claim_owner = $2`, id, owner, now)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return outbox.ErrClaimLost
	}
	return nil
}

func (s *Store) ReleaseClaim(ctx context.Context, id, owner, message string, availableAt time.Time) error {
	if len(message) > 1000 {
		message = message[:1000]
	}
	result, err := s.DB.ExecContext(ctx, `
UPDATE outbox_entries
SET status = 'PENDING', available_at = $4, last_error = $3,
    claim_owner = NULL, claimed_at = NULL
WHERE id = $1 AND status = 'CLAIMED' AND claim_owner = $2`, id, owner, message, availableAt)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return outbox.ErrClaimLost
	}
	return nil
}

func (s *Store) MarkFailed(ctx context.Context, id, owner, message string, now time.Time) error {
	if len(message) > 1000 {
		message = message[:1000]
	}
	result, err := s.DB.ExecContext(ctx, `
UPDATE outbox_entries
SET status = 'FAILED', last_error = $3, processed_at = $4,
    claim_owner = NULL, claimed_at = NULL
WHERE id = $1 AND status = 'CLAIMED' AND claim_owner = $2`, id, owner, message, now)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return outbox.ErrClaimLost
	}
	return nil
}

func (s *Store) Stats(ctx context.Context, now time.Time) (outbox.Stats, error) {
	var count, failed int64
	var oldest sql.NullTime
	err := s.DB.QueryRowContext(ctx, `
SELECT count(*) FILTER (WHERE status IN ('PENDING', 'CLAIMED')),
       count(*) FILTER (WHERE status = 'FAILED'),
       min(created_at) FILTER (WHERE status IN ('PENDING', 'CLAIMED'))
FROM outbox_entries`).Scan(&count, &failed, &oldest)
	if err != nil {
		return outbox.Stats{}, err
	}
	stats := outbox.Stats{PendingCount: count, FailedCount: failed}
	if oldest.Valid && now.After(oldest.Time) {
		stats.OldestPendingAge = now.Sub(oldest.Time)
	}
	return stats, nil
}
