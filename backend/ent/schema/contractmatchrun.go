package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ContractMatchRun struct{ ent.Schema }

func (ContractMatchRun) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("client_id").Immutable(),
		field.String("invoice_id").Immutable(),
		field.String("policy_version").NotEmpty().Immutable(),
		field.Enum("outcome").Values("UNIQUE_COMPATIBLE", "MULTIPLE_PLAUSIBLE", "UNIQUE_INCOMPATIBLE", "NO_MATCH").Immutable(),
		field.Uint64("invoice_revision").Positive().Immutable(),
		field.String("command_key").NotEmpty().Unique().Immutable(),
		field.Time("created_at").Immutable(),
	}
}

func (ContractMatchRun) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("client", AccountingClient.Type).Ref("contract_match_runs").Field("client_id").Unique().Required().Immutable(),
		edge.From("invoice", Invoice.Type).Ref("contract_match_runs").Field("invoice_id").Unique().Required().Immutable(),
		edge.To("candidates", ContractMatchCandidate.Type),
		edge.To("validation_tasks", ValidationTask.Type),
		edge.To("invoice_associations", InvoiceContractAssociation.Type),
	}
}

func (ContractMatchRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("id", "client_id").Unique(),
		index.Fields("id", "invoice_id", "client_id").Unique(),
		index.Fields("invoice_id", "created_at"),
	}
}
