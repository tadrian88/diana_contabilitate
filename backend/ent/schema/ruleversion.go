package schema

import (
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/rules"
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type RuleVersion struct{ ent.Schema }

func (RuleVersion) Fields() []ent.Field {
	return []ent.Field{
		field.String("model_version").Default("LEGACY_V1").Immutable(),
		field.JSON("domain_rule", &accounting.Rule{}).Optional().Immutable(),
		field.String("id").Immutable(),
		field.Bool("production_eligible").Default(false).Immutable(),
		field.String("rule_pack_version").Default("").Immutable(),
		field.JSON("provenance", &rules.Provenance{}).Optional().Immutable(),
		field.String("rule_id").Immutable(),
		field.Int("version").Positive().Immutable(),
		field.String("criteria").NotEmpty().Immutable(),
		field.String("result").NotEmpty().Immutable(),
		field.String("explanation").NotEmpty().Immutable(),
		field.String("legal_basis").NotEmpty().Immutable(),
		field.Enum("match_kind").Values("DESCRIPTION_CONTAINS", "ALWAYS", "NO_AUTOMATION", "VAT_SOURCE_RATE_EQUALS").Immutable(),
		field.String("match_value").Optional().Nillable().Immutable(),
		field.Time("effective_from").SchemaType(map[string]string{dialect.Postgres: "date"}).Immutable(),
		field.Time("effective_to").SchemaType(map[string]string{dialect.Postgres: "date"}).Optional().Nillable().Immutable(),
		field.String("created_by_id").Optional().Nillable().Immutable(),
		field.String("created_by_display").NotEmpty().Immutable(),
		field.String("command_key").NotEmpty().Unique().Immutable(),
		field.Time("created_at").Immutable(),
	}
}

func (RuleVersion) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("rule", ClassificationRule.Type).Ref("versions").Field("rule_id").Unique().Required().Immutable(),
		edge.To("line_classifications", LineClassification.Type),
	}
}

func (RuleVersion) Indexes() []ent.Index {
	return []ent.Index{index.Fields("rule_id", "version").Unique()}
}
