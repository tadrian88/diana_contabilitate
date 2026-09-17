package schema

import (
	"diana-contabilitate/backend/internal/accounting"
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type LineClassification struct{ ent.Schema }

func (LineClassification) Fields() []ent.Field {
	return []ent.Field{
		field.String("model_version").Default("LEGACY_V1").Immutable(),
		field.JSON("proposed_typed_value", &accounting.Value{}).Optional().Immutable(),
		field.JSON("effective_typed_value", &accounting.Value{}).Optional(),
		field.JSON("decision_evidence", &accounting.Evidence{}).Optional().Immutable(),
		field.String("review_reason").Default(""),
		field.String("id").Immutable(),
		field.Time("invoice_date_used").SchemaType(map[string]string{dialect.Postgres: "date"}).Optional().Nillable().Immutable(),
		field.String("client_id").Immutable(),
		field.String("invoice_id").Immutable(),
		field.String("invoice_line_id").Immutable(),
		field.Enum("dimension").Values("ACCOUNT", "VAT", "DEDUCTIBILITY", "VAT_TREATMENT", "VAT_DEDUCTIBILITY", "EXPENSE_TAX_TREATMENT").Immutable(),
		field.String("proposed_value").NotEmpty().Immutable(),
		field.String("effective_value").Optional().Nillable(),
		field.String("confidence_display").NotEmpty().Immutable(),
		field.String("explanation").NotEmpty().Immutable(),
		field.String("legal_basis").NotEmpty().Immutable(),
		field.Bool("required_review").Immutable(),
		field.Enum("review_status").Values("PENDING", "ACCEPTED", "CORRECTED"),
		field.Enum("source").Values("RULE", "NO_MATCH", "AMBIGUOUS").Immutable(),
		field.String("rule_version_id").Optional().Nillable().Immutable(),
		field.String("policy_version").NotEmpty().Immutable(),
		field.String("reviewed_by_id").Optional().Nillable(),
		field.String("reviewed_by_display").Optional().Nillable(),
		field.Time("reviewed_at").Optional().Nillable(),
		field.Uint64("revision").Default(1).Positive(),
		field.Time("created_at").Immutable(),
		field.Time("updated_at"),
	}
}

func (LineClassification) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("client", AccountingClient.Type).Ref("line_classifications").Field("client_id").Unique().Required().Immutable(),
		edge.From("invoice", Invoice.Type).Ref("line_classifications").Field("invoice_id").Unique().Required().Immutable(),
		edge.From("invoice_line", InvoiceLine.Type).Ref("classifications").Field("invoice_line_id").Unique().Required().Immutable(),
		edge.From("rule_version", RuleVersion.Type).Ref("line_classifications").Field("rule_version_id").Unique().Immutable(),
	}
}

func (LineClassification) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("invoice_line_id", "dimension", "model_version").Unique(),
		index.Fields("invoice_id", "review_status"),
	}
}
