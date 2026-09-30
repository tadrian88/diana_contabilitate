package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AccountMapping struct{ ent.Schema }

func (AccountMapping) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("client_id").Immutable(),
		// D-129: INCOMING mappings are keyed by supplier, OUTGOING ones by the
		// customer of the issued invoice; normalized_supplier_id holds the
		// normalized counterparty in both cases.
		field.Enum("direction").Values("INCOMING", "OUTGOING").Default("INCOMING").Immutable(),
		field.String("normalized_supplier_id").NotEmpty().Immutable(),
		field.Enum("service_identity_kind").Values("SELLER_ITEM_ID", "STANDARD_ITEM_ID", "NORMALIZED_DESCRIPTION").Immutable(),
		field.String("service_identity_value").NotEmpty().Immutable(),
		field.String("normalizer_version").NotEmpty().Immutable(),
		field.Int("current_version").Positive(),
		field.Enum("status").Values("ACTIVE", "INACTIVE").Default("ACTIVE"),
		field.Uint64("revision").Default(1).Positive(),
		field.Time("created_at").Immutable(),
		field.Time("updated_at"),
	}
}

func (AccountMapping) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("client_id", "direction", "normalized_supplier_id", "service_identity_kind", "service_identity_value", "normalizer_version").Unique(),
		index.Fields("client_id", "direction", "normalized_supplier_id", "status"),
	}
}
