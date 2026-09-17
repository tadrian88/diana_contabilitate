package schema

import (
	"entgo.io/ent"
	entsql "entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AccountingClient struct{ ent.Schema }

func (AccountingClient) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "clients"}}
}

func (AccountingClient) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("name").NotEmpty(),
		field.String("cui").NotEmpty(),
		field.String("display_name").Default(""),
		field.String("registration_number").Default(""),
		field.String("country").Default("RO"),
		field.String("registered_address").Default(""),
		field.String("city").Default(""), field.String("region").Default(""), field.String("postal_code").Default(""),
		field.String("email").Default(""), field.String("phone").Default(""),
		field.String("default_currency").Default("RON"),
		field.String("normalized_identifier").Default(""),
		field.String("lifecycle").Default("ACTIVE"),
		field.Uint64("revision").Default(1),
		field.Time("created_at").Immutable(),
		field.Time("updated_at"),
	}
}

func (AccountingClient) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("invoices", Invoice.Type),
		edge.To("activity_events", ActivityEvent.Type),
		edge.To("validation_tasks", ValidationTask.Type),
		edge.To("contracts", Contract.Type),
		edge.To("contract_match_runs", ContractMatchRun.Type),
		edge.To("contract_match_candidates", ContractMatchCandidate.Type),
		edge.To("invoice_contract_associations", InvoiceContractAssociation.Type),
		edge.To("classification_rules", ClassificationRule.Type),
		edge.To("line_classifications", LineClassification.Type),
		edge.To("spv_connections", SPVConnection.Type),
		edge.To("spv_source_documents", SPVSourceDocument.Type),
		edge.To("spv_oauth_states", SPVOAuthState.Type),
		edge.To("saga_export_attempts", SagaExportAttempt.Type),
		edge.To("contract_source_documents", ContractSourceDocument.Type),
	}
}

func (AccountingClient) Indexes() []ent.Index {
	return []ent.Index{index.Fields("cui").Unique()}
}
