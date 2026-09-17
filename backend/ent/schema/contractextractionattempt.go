package schema

import (
	"encoding/json"
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ContractExtractionAttempt struct{ ent.Schema }

func (ContractExtractionAttempt) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("document_id").Immutable(),
		field.String("provider").NotEmpty().Immutable(),
		field.String("model").NotEmpty().Immutable(),
		field.String("schema_version").NotEmpty().Immutable(),
		field.String("prompt_version").NotEmpty().Immutable(),
		field.Enum("status").Values("STARTED", "SUCCEEDED", "FAILED"),
		field.JSON("proposal", json.RawMessage{}).Optional().Sensitive(),
		field.String("safe_error_category").Optional().Nillable(),
		field.Int64("input_tokens").Optional().Nillable(),
		field.Int64("output_tokens").Optional().Nillable(),
		field.Time("started_at").Immutable(),
		field.Time("completed_at").Optional().Nillable(),
	}
}

func (ContractExtractionAttempt) Edges() []ent.Edge {
	return []ent.Edge{edge.From("document", ContractSourceDocument.Type).Ref("extraction_attempts").Field("document_id").Unique().Required().Immutable()}
}

func (ContractExtractionAttempt) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("document_id", "started_at"),
		index.Fields("document_id", "model", "schema_version", "prompt_version", "status"),
	}
}
