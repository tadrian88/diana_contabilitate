package schema

import (
	"encoding/json"
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ContractServiceTerm struct{ ent.Schema }

func (ContractServiceTerm) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(), field.String("contract_id").Immutable(), field.Int("position").Positive().Immutable(),
		field.String("service_description").NotEmpty(),
		field.Enum("pricing_model").Values("FIXED_FEE", "UNIT_RATE", "FIXED_TOTAL"),
		field.String("unit_price").SchemaType(map[string]string{dialect.Postgres: "numeric(20,4)"}).Optional().Nillable(),
		field.String("currency").MinLen(3).MaxLen(3), field.String("unit").Optional(),
		field.Enum("quantity_source").Values("CONTRACT_FIXED_QUANTITY", "INVOICE_REPORTED_QUANTITY", "USER_CONFIRMED_QUANTITY", "EXTERNAL_SOURCE_FUTURE", "UNKNOWN").Default("UNKNOWN"),
		field.String("quantity_value").SchemaType(map[string]string{dialect.Postgres: "numeric(20,4)"}).Optional().Nillable(),
		field.String("quantity_driver").Optional(),
		field.Enum("billing_frequency").Values("MONTHLY", "QUARTERLY", "ANNUAL", "PER_OCCURRENCE", "UNKNOWN").Default("UNKNOWN"),
		field.JSON("source_evidence", json.RawMessage{}).Optional(),
	}
}
func (ContractServiceTerm) Edges() []ent.Edge {
	return []ent.Edge{edge.From("contract", Contract.Type).Ref("service_terms").Field("contract_id").Unique().Required().Immutable()}
}
func (ContractServiceTerm) Indexes() []ent.Index {
	return []ent.Index{index.Fields("contract_id", "position").Unique()}
}
