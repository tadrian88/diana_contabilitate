package schema

import (
	"diana-contabilitate/backend/internal/accounting"
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

var pipelineStatuses = []string{
	"DOWNLOADED", "ARCHIVED", "MATCHING", "AWAITING_CONTRACT",
	"AWAITING_MATCH_CONFIRM", "DEDUPE_CHECKED", "HEADER_READ", "LINES_READ",
	"CLASSIFIED", "AWAITING_REVIEW", "READY_FOR_SAGA", "EXPORTING", "EXPORTED", "DUPLICATE",
}

type Invoice struct{ ent.Schema }

func (Invoice) Fields() []ent.Field {
	return []ent.Field{
		field.String("model_version").Default("LEGACY_V1").Immutable(),
		field.JSON("source_facts", &accounting.SourceFacts{}).Optional().Immutable(),
		field.JSON("accounting_snapshot", &accounting.Snapshot{}).Optional(),
		field.String("readiness_reason").Default(""),
		field.String("id").Immutable(),
		field.String("client_id"),
		field.String("supplier_name").NotEmpty(),
		field.String("supplier_cui").Optional().Nillable(),
		field.String("normalized_supplier_cui").Optional().Nillable(),
		field.String("document_number").NotEmpty(),
		field.String("normalized_document_number").NotEmpty(),
		field.Time("issue_date"),
		field.Time("issue_day").SchemaType(map[string]string{dialect.Postgres: "date"}),
		field.Time("due_date").Optional().Nillable(),
		field.String("total_amount").SchemaType(map[string]string{dialect.Postgres: "numeric(20,4)"}),
		field.String("currency").MinLen(3).MaxLen(3),
		field.String("spv_reference").NotEmpty(),
		field.String("ingestion_source").NotEmpty(),
		field.String("external_delivery_id").NotEmpty(),
		field.String("duplicate_of_invoice_id").Optional().Nillable().Immutable(),
		field.Bool("duplicate_amount_matches").Optional().Nillable().Immutable(),
		field.Bool("duplicate_currency_matches").Optional().Nillable().Immutable(),
		field.Enum("document_type").Values("INVOICE", "CREDIT_NOTE").Default("INVOICE").Immutable(),
		field.Enum("pipeline_status").Values(pipelineStatuses...),
		field.Enum("saga_status").Values("NOT_READY", "READY", "EXPORTING", "EXPORTED", "FAILED"),
		field.Uint64("revision").Default(1).Positive(),
		field.Time("created_at").Immutable(),
		field.Time("updated_at"),
	}
}

func (Invoice) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("client", AccountingClient.Type).Ref("invoices").Field("client_id").Unique().Required(),
		edge.To("activity_events", ActivityEvent.Type),
		edge.To("lines", InvoiceLine.Type),
		edge.To("validation_tasks", ValidationTask.Type),
		edge.To("contract_match_runs", ContractMatchRun.Type),
		edge.To("contract_association", InvoiceContractAssociation.Type).Unique(),
		edge.To("line_classifications", LineClassification.Type),
		edge.To("spv_source_document", SPVSourceDocument.Type).Unique(),
		edge.To("saga_export_attempts", SagaExportAttempt.Type),
	}
}

func (Invoice) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("pipeline_status"),
		index.Fields("issue_date"),
		index.Fields("client_id"),
		index.Fields("id", "client_id").Unique(),
		index.Fields("client_id", "spv_reference").Unique(),
		index.Fields("client_id", "normalized_supplier_cui", "normalized_document_number", "issue_day"),
		index.Fields("client_id", "normalized_supplier_cui", "normalized_document_number", "issue_day").
			Unique().
			Annotations(entsql.IndexWhere("duplicate_of_invoice_id IS NULL AND normalized_supplier_cui IS NOT NULL")),
	}
}
