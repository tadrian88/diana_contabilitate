package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SPVSourceDocument is the immutable external delivery and its operational
// processing record. Its status is deliberately separate from Invoice status.
type SPVSourceDocument struct{ ent.Schema }

func (SPVSourceDocument) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("connection_id"),
		field.String("client_id"),
		field.String("external_message_id").NotEmpty(),
		field.String("external_upload_id").Optional().Nillable(),
		field.String("external_request_id").Optional().Nillable(),
		field.String("message_type").Optional().Nillable(),
		field.String("source_created_raw").Optional().Nillable(),
		field.Time("source_created_at").Optional().Nillable(),
		field.Time("discovered_at").Immutable(),
		field.Time("downloaded_at").Optional().Nillable(),
		field.Bytes("raw_document").Optional().Sensitive(),
		field.String("content_type").Optional().Nillable(),
		field.String("content_sha256").Optional().Nillable(),
		field.String("parser_type").Optional().Nillable(),
		field.String("parser_version").Optional().Nillable(),
		field.Enum("processing_status").Values("DISCOVERED", "PROCESSING", "DOWNLOADED", "PROCESSED", "FAILED").Default("DISCOVERED"),
		field.Enum("failure_kind").Values("TRANSIENT", "PERMANENT").Optional().Nillable(),
		field.String("last_error").Optional().Nillable(),
		field.Uint("attempts").Default(0),
		field.Time("available_at"),
		field.String("claim_owner").Optional().Nillable(),
		field.Time("claimed_at").Optional().Nillable(),
		field.String("invoice_id").Optional().Nillable(),
		field.Time("processed_at").Optional().Nillable(),
		field.Time("updated_at"),
	}
}

func (SPVSourceDocument) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("connection", SPVConnection.Type).Ref("source_documents").Field("connection_id").Unique().Required(),
		edge.From("client", AccountingClient.Type).Ref("spv_source_documents").Field("client_id").Unique().Required(),
		edge.From("invoice", Invoice.Type).Ref("spv_source_document").Field("invoice_id").Unique(),
	}
}

func (SPVSourceDocument) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("connection_id", "external_message_id").Unique(),
		index.Fields("client_id"),
		index.Fields("processing_status", "available_at"),
		index.Fields("content_sha256"),
		index.Fields("invoice_id").Unique(),
	}
}
