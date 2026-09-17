package schema

import (
	"encoding/json"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ActivityEvent struct{ ent.Schema }

func (ActivityEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("client_id").Optional(),
		field.String("invoice_id").Optional().Nillable(),
		field.String("validation_task_id").Optional().Nillable(),
		field.String("aggregate_type").NotEmpty(),
		field.String("aggregate_id").NotEmpty(),
		field.String("event_type").NotEmpty(),
		field.Time("occurred_at").Immutable(),
		field.Enum("actor_kind").Values("SYSTEM", "USER", "EXTERNAL"),
		field.String("actor_id").Optional().Nillable(),
		field.String("actor_display").Optional().Nillable(),
		field.Bool("automatic"),
		field.String("detail").NotEmpty(),
		field.JSON("before_snapshot", json.RawMessage{}).Optional(),
		field.JSON("after_snapshot", json.RawMessage{}).Optional(),
		field.String("correlation_id").Optional().Nillable(),
		field.String("pipeline_from").Optional().Nillable(),
		field.String("pipeline_to").Optional().Nillable(),
		field.String("trigger").Optional().Nillable(),
		field.String("idempotency_key").Optional().Nillable(),
	}
}

func (ActivityEvent) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("client", AccountingClient.Type).Ref("activity_events").Field("client_id").Unique(),
		edge.From("invoice", Invoice.Type).Ref("activity_events").Field("invoice_id").Unique(),
		edge.From("validation_task", ValidationTask.Type).Ref("activity_events").Field("validation_task_id").Unique(),
	}
}

func (ActivityEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("aggregate_type", "aggregate_id", "occurred_at"),
		index.Fields("occurred_at"),
		index.Fields("client_id"),
		index.Fields("idempotency_key").Unique(),
	}
}
