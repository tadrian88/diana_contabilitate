package schema

import (
	"encoding/json"

	"entgo.io/ent"
	entsql "entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ValidationTask struct{ ent.Schema }

func (ValidationTask) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("client_id").Immutable(),
		field.String("invoice_id").Immutable(),
		field.String("contract_match_run_id").Optional().Nillable().Immutable(),
		field.Enum("task_type").Values("CONTRACT_MATCH", "MISSING_CONTRACT", "CLASSIFICATION").Immutable(),
		field.Enum("status").Values("OPEN", "WAITING", "RESOLVED").Default("OPEN"),
		field.String("title").NotEmpty().Immutable(),
		field.String("reason").NotEmpty().Immutable(),
		field.String("blocker_code").Optional().Nillable().Immutable(),
		field.JSON("resolution_metadata", json.RawMessage{}).Optional(),
		field.Enum("created_by_kind").Values("SYSTEM", "USER", "EXTERNAL").Immutable(),
		field.String("created_by_id").Optional().Nillable().Immutable(),
		field.String("created_by_display").Optional().Nillable().Immutable(),
		field.String("creation_key").NotEmpty().Unique().Immutable(),
		field.Uint64("revision").Default(1).Positive(),
		field.Time("created_at").Immutable(),
		field.Time("updated_at"),
		field.Time("waiting_since").Optional().Nillable(),
		field.Time("resolved_at").Optional().Nillable(),
	}
}

func (ValidationTask) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("client", AccountingClient.Type).Ref("validation_tasks").Field("client_id").Unique().Required().Immutable(),
		edge.From("invoice", Invoice.Type).Ref("validation_tasks").Field("invoice_id").Unique().Required().Immutable(),
		edge.From("contract_match_run", ContractMatchRun.Type).Ref("validation_tasks").Field("contract_match_run_id").Unique().Immutable(),
		edge.To("activity_events", ActivityEvent.Type),
	}
}

func (ValidationTask) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("client_id", "status", "created_at"),
		index.Fields("invoice_id", "created_at"),
		index.Fields("task_type", "status"),
		index.Fields("invoice_id").Unique().Annotations(entsql.IndexWhere("status <> 'RESOLVED'")),
		index.Fields("contract_match_run_id").Unique().Annotations(entsql.IndexWhere("contract_match_run_id IS NOT NULL")),
	}
}
