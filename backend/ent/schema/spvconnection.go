package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SPVConnection is one ANAF OAuth authorization scoped to one accounting
// client/CIF. Tokens are encrypted before crossing this persistence boundary.
type SPVConnection struct{ ent.Schema }

func (SPVConnection) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("client_id"),
		field.String("cif").NotEmpty(),
		field.Enum("environment").Values("TEST", "PRODUCTION"),
		field.String("access_token_ciphertext").Sensitive(),
		field.String("refresh_token_ciphertext").Sensitive(),
		field.Time("access_token_expires_at"),
		field.Time("refresh_token_expires_at").Optional().Nillable(),
		field.Enum("status").Values("ACTIVE", "EXPIRED", "REVOKED", "ERROR").Default("ACTIVE"),
		field.Time("connected_at").Optional().Nillable(),
		field.Time("last_successful_sync_at").Optional().Nillable(),
		field.Time("last_sync_started_at").Optional().Nillable(),
		field.Time("last_sync_finished_at").Optional().Nillable(),
		field.Enum("last_sync_status").Values("NEVER", "RUNNING", "SUCCEEDED", "FAILED").Default("NEVER"),
		field.String("last_error").Optional().Nillable(),
		field.Uint64("revision").Default(1).Positive(),
		field.Time("created_at").Immutable(),
		field.Time("updated_at"),
	}
}

func (SPVConnection) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("client", AccountingClient.Type).Ref("spv_connections").Field("client_id").Unique().Required(),
		edge.To("source_documents", SPVSourceDocument.Type),
	}
}

func (SPVConnection) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("client_id").Unique(),
		index.Fields("environment", "cif").Unique(),
		index.Fields("status"),
	}
}
