//go:build integration

package postgres

import (
	"context"
	"time"

	"diana-contabilitate/backend/ent"
	"diana-contabilitate/backend/ent/outboxentry"
	"diana-contabilitate/backend/internal/invoicing"
)

// scopedPipelineStore keeps dispatcher integration tests independent from
// pending work that legitimately belongs to other fixtures or local demo data.
// Production PendingOutbox remains global.
type scopedPipelineStore struct {
	*Store
	aggregateID string
}

func (s scopedPipelineStore) PendingOutbox(ctx context.Context, limit int, now time.Time) ([]invoicing.OutboxEntry, error) {
	rows, err := s.Client.OutboxEntry.Query().Where(
		outboxentry.StatusEQ(outboxentry.StatusPENDING),
		outboxentry.AvailableAtLTE(now),
		outboxentry.AggregateIDEQ(s.aggregateID),
	).Order(ent.Asc(outboxentry.FieldCreatedAt)).Limit(limit).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]invoicing.OutboxEntry, 0, len(rows))
	for _, row := range rows {
		result = append(result, invoicing.OutboxEntry{ID: row.ID, EventType: row.EventType, AggregateID: row.AggregateID, IdempotencyKey: row.IdempotencyKey, Attempts: row.Attempts})
	}
	return result, nil
}
