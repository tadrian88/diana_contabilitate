package schema

import (
	"diana-contabilitate/backend/internal/accounting"
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ClassificationRun struct{ ent.Schema }

func (ClassificationRun) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("client_id").Immutable(),
		field.String("invoice_id").Immutable(),
		field.Uint64("invoice_revision").Positive().Immutable(),
		field.JSON("snapshot", &accounting.Snapshot{}).Immutable(),
		field.String("profile_id").Optional().Nillable().Immutable(),
		field.Int("profile_version").Optional().Nillable().Immutable(),
		field.String("context_fingerprint").NotEmpty().Immutable(),
		field.String("policy_version").NotEmpty().Immutable(),
		field.Enum("status").Values("COMPLETED", "BLOCKED").Immutable(),
		field.String("blocker_code").Optional().Nillable().Immutable(),
		field.String("supersedes_run_id").Optional().Nillable().Immutable(),
		field.String("command_key").NotEmpty().Unique().Immutable(),
		field.String("actor_id").Optional().Nillable().Immutable(),
		field.String("actor_display").NotEmpty().Immutable(),
		field.Time("created_at").Immutable(),
	}
}

func (ClassificationRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("client_id", "invoice_id", "created_at"),
		index.Fields("id", "client_id", "invoice_id").Unique(),
	}
}
