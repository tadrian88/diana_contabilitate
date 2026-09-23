package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AccountMappingVersion struct{ ent.Schema }

func (AccountMappingVersion) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("mapping_id").Immutable(),
		field.Int("version").Positive().Immutable(),
		field.String("account_code").NotEmpty().Immutable(),
		field.Enum("change_kind").Values("CREATION", "CORRECTION", "POLICY_CHANGE").Immutable(),
		field.String("source_classification_id").NotEmpty().Immutable(),
		field.String("source_invoice_line_id").NotEmpty().Immutable(),
		field.String("raw_description_snapshot").NotEmpty().Immutable(),
		field.String("actor_id").Optional().Nillable().Immutable(),
		field.String("actor_display").NotEmpty().Immutable(),
		field.String("reason").Default("").Immutable(),
		field.Time("created_at").Immutable(),
		field.String("command_key").NotEmpty().Unique().Immutable(),
	}
}

func (AccountMappingVersion) Indexes() []ent.Index {
	return []ent.Index{index.Fields("mapping_id", "version").Unique(), index.Fields("account_code")}
}
