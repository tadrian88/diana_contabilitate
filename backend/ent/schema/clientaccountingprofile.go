package schema

import (
	"diana-contabilitate/backend/internal/accounting"
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ClientAccountingProfile struct{ ent.Schema }

func (ClientAccountingProfile) Fields() []ent.Field {
	return []ent.Field{field.String("id").Immutable(), field.String("client_id").Immutable(), field.Int("version").Positive().Immutable(), field.JSON("payload", &accounting.Profile{}).Immutable(), field.Time("created_at").Immutable()}
}
func (ClientAccountingProfile) Indexes() []ent.Index {
	return []ent.Index{index.Fields("client_id", "version").Unique()}
}
