package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SagaExportAttempt is the immutable generated accounting artifact and its
// operational state. It deliberately does not assert that SAGA imported it.
type SagaExportAttempt struct{ ent.Schema }

func (SagaExportAttempt) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("invoice_id").Immutable(),
		field.String("client_id").Immutable(),
		field.Uint64("invoice_revision").Positive().Immutable(),
		field.String("exporter_version").NotEmpty().Immutable(),
		field.Enum("status").Values("GENERATED", "FAILED").Immutable(),
		field.String("filename").Optional().Nillable().Immutable(),
		field.String("content_type").Optional().Nillable().Immutable(),
		field.Bytes("payload").Optional().Sensitive().Immutable(),
		field.String("payload_sha256").Optional().Nillable().Immutable(),
		field.JSON("classification_snapshot", map[string]string{}).Optional().Immutable(),
		field.Enum("failure_category").Values("DATA_INVALID", "SERIALIZATION_FAILED", "UNSUPPORTED_DOCUMENT_TYPE").Optional().Nillable().Immutable(),
		field.String("safe_error").Optional().Nillable().Immutable(),
		field.Time("started_at").Immutable(),
		field.Time("completed_at").Immutable(),
		field.Time("confirmed_at").Optional().Nillable(),
		field.String("confirmed_by_id").Optional().Nillable(),
		field.String("confirmed_by_display").Optional().Nillable(),
		field.Enum("confirmation_type").Values("HUMAN", "LOCAL_AGENT", "SAGA_API").Optional().Nillable(),
		field.String("confirmation_note").Optional().Nillable(),
	}
}

func (SagaExportAttempt) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("invoice", Invoice.Type).Ref("saga_export_attempts").Field("invoice_id").Unique().Required().Immutable(),
		edge.From("client", AccountingClient.Type).Ref("saga_export_attempts").Field("client_id").Unique().Required().Immutable(),
	}
}

func (SagaExportAttempt) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("invoice_id", "invoice_revision", "exporter_version").Unique(),
		index.Fields("client_id", "completed_at"),
		index.Fields("payload_sha256"),
	}
}
