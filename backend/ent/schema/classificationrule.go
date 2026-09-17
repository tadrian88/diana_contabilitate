package schema

import (
	"entgo.io/ent"
	entsql "entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ClassificationRule struct{ ent.Schema }

func (ClassificationRule) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("reference").NotEmpty().Unique().Immutable(),
		field.String("name").NotEmpty().Immutable(),
		field.Enum("category").Values("ACCOUNT", "VAT", "DEDUCTIBILITY", "VAT_TREATMENT", "VAT_DEDUCTIBILITY", "EXPENSE_TAX_TREATMENT").Immutable(),
		field.Enum("scope").Values("GLOBAL", "CLIENT_OVERRIDE").Immutable(),
		field.String("client_id").Optional().Nillable().Immutable(),
		field.String("parent_rule_id").Optional().Nillable().Immutable(),
		field.Enum("parent_scope").Values("GLOBAL").Optional().Nillable().Immutable(),
		field.String("creation_key").NotEmpty().Unique().Immutable(),
		field.Uint64("revision").Default(1).Positive(),
		field.Time("created_at").Immutable(),
		field.Time("updated_at"),
	}
}

func (ClassificationRule) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("client", AccountingClient.Type).Ref("classification_rules").Field("client_id").Unique().Immutable(),
		edge.From("parent", ClassificationRule.Type).Ref("overrides").Field("parent_rule_id").Unique().Immutable(),
		edge.To("overrides", ClassificationRule.Type),
		edge.To("versions", RuleVersion.Type),
	}
}

func (ClassificationRule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("scope", "category"),
		index.Fields("id", "category", "scope").Unique(),
		index.Fields("parent_rule_id", "client_id").Unique().Annotations(entsql.IndexWhere("scope = 'CLIENT_OVERRIDE'")),
	}
}
