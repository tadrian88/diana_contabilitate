package schema

import (
	"encoding/json"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type OutboxEntry struct{ ent.Schema }

func (OutboxEntry) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("event_type").NotEmpty(),
		field.String("aggregate_type").NotEmpty(),
		field.String("aggregate_id").NotEmpty(),
		field.JSON("payload", json.RawMessage{}),
		field.String("idempotency_key").NotEmpty().Unique(),
		field.String("correlation_id").Optional().Nillable(),
		field.Enum("status").Values("PENDING", "CLAIMED", "DISPATCHED", "FAILED", "PROCESSED").Default("PENDING"),
		field.Uint("attempts").Default(0),
		field.Time("created_at").Immutable(),
		field.Time("available_at"),
		field.String("claim_owner").Optional().Nillable(),
		field.Time("claimed_at").Optional().Nillable(),
		field.Time("dispatched_at").Optional().Nillable(),
		field.String("last_error").Optional().Nillable(),
		field.Time("processed_at").Optional().Nillable(),
	}
}

func (OutboxEntry) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "available_at"),
		index.Fields("status", "claimed_at"),
		index.Fields("aggregate_type", "aggregate_id"),
	}
}
