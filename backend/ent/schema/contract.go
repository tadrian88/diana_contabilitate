package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Contract struct{ ent.Schema }

func (Contract) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("client_id").Immutable(),
		field.String("supplier_name").NotEmpty(),
		field.String("supplier_cui").NotEmpty(),
		field.String("normalized_supplier_cui").NotEmpty(),
		field.String("reference").NotEmpty(),
		field.Time("effective_from").SchemaType(map[string]string{dialect.Postgres: "date"}),
		field.Time("effective_to").SchemaType(map[string]string{dialect.Postgres: "date"}),
		field.String("total_value").SchemaType(map[string]string{dialect.Postgres: "numeric(20,4)"}),
		field.String("currency").MinLen(3).MaxLen(3),
		field.String("unit_type").NotEmpty(),
		field.String("payment_terms").NotEmpty(),
		field.String("source_reference").Optional().Nillable(),
		field.String("source_metadata").Optional().Nillable(),
		field.String("source_document_id").Optional().Nillable().Immutable(),
		field.String("extraction_attempt_id").Optional().Nillable().Immutable(),
		field.Uint64("revision").Default(1).Positive(),
		field.Time("created_at").Immutable(),
		field.Time("updated_at"),
	}
}

func (Contract) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("client", AccountingClient.Type).Ref("contracts").Field("client_id").Unique().Required().Immutable(),
		edge.To("match_candidates", ContractMatchCandidate.Type),
		edge.To("invoice_associations", InvoiceContractAssociation.Type),
	}
}

func (Contract) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("client_id", "reference").Unique(),
		index.Fields("id", "client_id").Unique(),
		index.Fields("client_id", "normalized_supplier_cui"),
		index.Fields("source_document_id").Unique(),
		index.Fields("extraction_attempt_id").Unique(),
	}
}
