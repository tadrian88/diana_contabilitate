package schema

import (
	"diana-contabilitate/backend/internal/accounting"
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type InvoiceLine struct{ ent.Schema }

func (InvoiceLine) Fields() []ent.Field {
	decimal := map[string]string{dialect.Postgres: "numeric(20,4)"}
	return []ent.Field{
		field.JSON("source_facts", &accounting.LineFacts{}).Optional().Immutable(),
		field.String("id").Immutable(),
		field.String("invoice_id"),
		field.Int("position").Positive(),
		field.String("description").NotEmpty(),
		field.String("unit").NotEmpty(),
		field.String("vat_rate").SchemaType(decimal),
		field.String("vat_value").SchemaType(decimal),
		field.String("quantity").SchemaType(decimal),
		field.String("unit_price").SchemaType(decimal),
		field.String("net_value").SchemaType(decimal),
		field.String("total_value").SchemaType(decimal),
		field.String("additional_info").Optional().Nillable(),
	}
}

func (InvoiceLine) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("invoice", Invoice.Type).Ref("lines").Field("invoice_id").Unique().Required(),
		edge.To("classifications", LineClassification.Type),
	}
}

func (InvoiceLine) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("invoice_id", "position").Unique(),
		index.Fields("id", "invoice_id").Unique(),
	}
}
