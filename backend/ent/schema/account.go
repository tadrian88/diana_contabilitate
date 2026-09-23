package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Account is Diana's local, application-configured chart of accounts.
// Postable is a technical selector guard, not a statement of legal applicability.
type Account struct{ ent.Schema }

func (Account) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("code").NotEmpty().Immutable(),
		field.String("name").NotEmpty(),
		field.String("account_type").NotEmpty(),
		field.String("parent_code").Optional().Nillable(),
		field.Int("level").Positive(),
		field.Bool("is_synthetic").Default(false),
		field.Bool("postable").Default(false),
		field.Bool("is_active").Default(true),
	}
}

func (Account) Indexes() []ent.Index {
	return []ent.Index{index.Fields("code").Unique(), index.Fields("is_active", "postable", "code")}
}
