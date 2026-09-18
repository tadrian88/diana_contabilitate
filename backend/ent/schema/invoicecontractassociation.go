package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type InvoiceContractAssociation struct{ ent.Schema }

func (InvoiceContractAssociation) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("client_id").Immutable(),
		field.String("invoice_id").Immutable(),
		field.String("contract_id").Immutable(),
		field.String("match_run_id").Immutable(),
		field.Enum("association_kind").Values("AUTOMATIC", "HUMAN_CONFIRMED").Immutable(),
		field.String("policy_version").NotEmpty().Immutable(),
		field.String("contract_reference").NotEmpty().Immutable(),
		field.String("supplier_name").NotEmpty().Immutable(),
		field.Time("effective_from").SchemaType(map[string]string{dialect.Postgres: "date"}).Immutable(),
		field.Time("effective_to").SchemaType(map[string]string{dialect.Postgres: "date"}).Optional().Nillable().Immutable(),
		field.Enum("period_type").Values("FIXED_TERM", "INDEFINITE_TERM").Default("FIXED_TERM").Immutable(),
		field.String("total_value").SchemaType(map[string]string{dialect.Postgres: "numeric(20,4)"}).Immutable(),
		field.String("currency").MinLen(3).MaxLen(3).Immutable(),
		field.String("unit_type").NotEmpty().Immutable(),
		field.String("payment_terms").NotEmpty().Immutable(),
		field.Time("associated_at").Immutable(),
		field.String("associated_by_id").Optional().Nillable().Immutable(),
		field.String("associated_by_display").Optional().Nillable().Immutable(),
	}
}

func (InvoiceContractAssociation) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("client", AccountingClient.Type).Ref("invoice_contract_associations").Field("client_id").Unique().Required().Immutable(),
		edge.From("invoice", Invoice.Type).Ref("contract_association").Field("invoice_id").Unique().Required().Immutable(),
		edge.From("contract", Contract.Type).Ref("invoice_associations").Field("contract_id").Unique().Required().Immutable(),
		edge.From("match_run", ContractMatchRun.Type).Ref("invoice_associations").Field("match_run_id").Unique().Required().Immutable(),
	}
}

func (InvoiceContractAssociation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("invoice_id").Unique(),
		index.Fields("contract_id", "associated_at"),
	}
}
