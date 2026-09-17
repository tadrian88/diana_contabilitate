package schema

import (
	"entgo.io/ent"
	entsql "entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ContractMatchCandidate struct{ ent.Schema }

func (ContractMatchCandidate) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("client_id").Immutable(),
		field.String("match_run_id").Immutable(),
		field.String("contract_id").Immutable(),
		field.Uint64("contract_revision").Positive().Immutable(),
		field.Int("rank").Positive().Immutable(),
		field.Bool("recommended").Immutable(),
		field.Enum("compatibility").Values("COMPATIBLE", "INCOMPATIBLE").Immutable(),
		field.String("confidence_display").NotEmpty().Immutable(),
		field.JSON("reasons", []string{}).Immutable(),
		field.Time("created_at").Immutable(),
	}
}

func (ContractMatchCandidate) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("client", AccountingClient.Type).Ref("contract_match_candidates").Field("client_id").Unique().Required().Immutable(),
		edge.From("match_run", ContractMatchRun.Type).Ref("candidates").Field("match_run_id").Unique().Required().Immutable(),
		edge.From("contract", Contract.Type).Ref("match_candidates").Field("contract_id").Unique().Required().Immutable(),
	}
}

func (ContractMatchCandidate) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("match_run_id", "rank").Unique(),
		index.Fields("match_run_id", "contract_id").Unique(),
		index.Fields("match_run_id").Unique().Annotations(entsql.IndexWhere("recommended")),
	}
}
