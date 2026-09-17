package schema

import (
	"entgo.io/ent"
	entsql "entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SPVOAuthState stores only a SHA-256 digest of the browser OAuth state.
// It is durable across API replicas and deliberately short-lived/single-use.
type SPVOAuthState struct{ ent.Schema }

func (SPVOAuthState) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "spv_oauth_states"}}
}

func (SPVOAuthState) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("state_hash").Immutable().Sensitive(),
		field.String("client_id").Immutable(),
		field.Enum("environment").Values("TEST", "PRODUCTION").Immutable(),
		field.String("return_path").Immutable(),
		field.Time("expires_at").Immutable(),
		field.Time("consumed_at").Optional().Nillable(),
		field.Time("created_at").Immutable(),
	}
}

func (SPVOAuthState) Edges() []ent.Edge {
	return []ent.Edge{edge.From("client", AccountingClient.Type).Ref("spv_oauth_states").Field("client_id").Unique().Required().Immutable()}
}

func (SPVOAuthState) Indexes() []ent.Index {
	return []ent.Index{index.Fields("state_hash").Unique(), index.Fields("expires_at"), index.Fields("client_id")}
}
