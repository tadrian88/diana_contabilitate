package schema

import (
	"encoding/json"
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ContractSourceDocument preserves the immutable PDF independently from the
// authoritative Contract created after human review.
type ContractSourceDocument struct{ ent.Schema }

func (ContractSourceDocument) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("client_id").Immutable(),
		field.String("original_filename").NotEmpty().Immutable(),
		field.String("mime_type").NotEmpty().Immutable(),
		field.Int64("size_bytes").Positive().Immutable(),
		field.String("sha256").NotEmpty().Immutable(),
		field.Bytes("raw_document").Sensitive().Immutable(),
		field.String("uploaded_by_id").Optional().Nillable().Immutable(),
		field.String("uploaded_by_display").Optional().Nillable().Immutable(),
		field.Time("uploaded_at").Immutable(),
		field.Enum("status").Values("UPLOADED", "EXTRACTING", "READY_FOR_REVIEW", "EXTRACTION_FAILED", "CONFIRMED").Default("UPLOADED"),
		field.Enum("lifecycle_state").Values("ACTIVE", "SUPERSEDED", "DISCARDED").Default("ACTIVE"),
		field.String("latest_extraction_id").Optional().Nillable(),
		field.String("confirmed_contract_id").Optional().Nillable(),
		field.String("confirmed_by_id").Optional().Nillable(),
		field.String("confirmed_by_display").Optional().Nillable(),
		field.Time("confirmed_at").Optional().Nillable(),
		field.JSON("confirmed_values", json.RawMessage{}).Optional().Sensitive(),
		field.String("confirmation_key").Optional().Nillable(),
		field.String("confirmation_fingerprint").Optional().Nillable(),
		field.Uint64("revision").Default(1).Positive(),
		field.Time("updated_at"),
	}
}

func (ContractSourceDocument) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("client", AccountingClient.Type).Ref("contract_source_documents").Field("client_id").Unique().Required().Immutable(),
		edge.To("extraction_attempts", ContractExtractionAttempt.Type),
	}
}

func (ContractSourceDocument) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("client_id", "sha256").Unique(),
		index.Fields("client_id", "status"),
		index.Fields("latest_extraction_id"),
		index.Fields("confirmed_contract_id").Unique(),
	}
}
