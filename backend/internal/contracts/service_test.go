package contracts

import (
	"context"
	"errors"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
)

type captureStore struct {
	input           InvoiceContext
	contracts       []Contract
	decision        MatchDecision
	confirmed       ConfirmCommand
	available       AvailableCommand
	blocked         []BlockedInvoice
	resumeCommitted bool
}

type invalidPolicy struct{}

func (invalidPolicy) Version() string { return "INVALID_TEST_POLICY" }
func (invalidPolicy) Evaluate(InvoiceContext, []Contract) (MatchDecision, error) {
	return MatchDecision{Outcome: OutcomeUniqueCompatible, PolicyVersion: "INVALID_TEST_POLICY"}, nil
}

func (s *captureStore) ListContracts(context.Context, Filter) ([]Contract, error) {
	return s.contracts, nil
}
func (s *captureStore) GetContract(context.Context, string) (*Contract, error) { return nil, nil }
func (s *captureStore) ArchiveContract(context.Context, string, uint64, string, string, time.Time) (bool, error) {
	return true, nil
}
func (s *captureStore) DeleteMistakenContract(context.Context, string, uint64, string, string, time.Time) (bool, error) {
	return true, nil
}
func (s *captureStore) ListContractInvoices(context.Context, string) ([]AssociatedInvoice, error) {
	return nil, nil
}
func (s *captureStore) MatchCommandCommitted(context.Context, string) (bool, error) {
	return false, nil
}
func (s *captureStore) LoadMatchingInput(context.Context, string) (InvoiceContext, []Contract, error) {
	return s.input, s.contracts, nil
}
func (s *captureStore) ApplyMatchDecision(_ context.Context, _ MatchCommand, decision MatchDecision, _ time.Time) (bool, error) {
	s.decision = decision
	return true, nil
}
func (s *captureStore) ConfirmContractMatch(_ context.Context, command ConfirmCommand, _ time.Time) (bool, error) {
	s.confirmed = command
	return true, nil
}
func (s *captureStore) RecordContractAvailable(_ context.Context, command AvailableCommand, _ time.Time) (bool, error) {
	s.available = command
	return true, nil
}
func (s *captureStore) ListBlockedInvoicesForContract(context.Context, string, string, int) ([]BlockedInvoice, error) {
	items := s.blocked
	s.blocked = nil
	return items, nil
}
func (s *captureStore) ResumeCommandCommitted(context.Context, string) (bool, error) {
	return s.resumeCommitted, nil
}
func (s *captureStore) ApplyResumeDecision(_ context.Context, _ ResumeCommand, decision MatchDecision, _ time.Time) (bool, error) {
	s.decision = decision
	return true, nil
}

func TestServiceOwnsMatchingPolicyBoundary(t *testing.T) {
	store := &captureStore{input: InvoiceContext{ID: "invoice-1", PipelineStatus: "MATCHING", Revision: 3, IssueDay: time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), Currency: "RON"}, contracts: []Contract{policyContract("contract-1", "CTR-1", "RON")}}
	service := NewService(store, BaselinePolicy{}, func() time.Time { return time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC) })
	decision, changed, err := service.MatchInvoice(context.Background(), MatchCommand{InvoiceID: "invoice-1", ExpectedRevision: 3, CommandID: "match-1"})
	if err != nil || !changed || decision.Outcome != OutcomeUniqueCompatible || store.decision.PolicyVersion != BaselinePolicyVersion {
		t.Fatalf("decision=%+v changed=%v stored=%+v err=%v", decision, changed, store.decision, err)
	}
}

func TestServiceRejectsStaleInvoiceBeforePersistence(t *testing.T) {
	store := &captureStore{input: InvoiceContext{ID: "invoice-1", PipelineStatus: "MATCHING", Revision: 4}}
	service := NewService(store, BaselinePolicy{}, nil)
	_, _, err := service.MatchInvoice(context.Background(), MatchCommand{InvoiceID: "invoice-1", ExpectedRevision: 3, CommandID: "match-1"})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("error=%v", err)
	}
}

func TestConfirmationRequiresBothOptimisticRevisionsAndCandidate(t *testing.T) {
	store := &captureStore{}
	service := NewService(store, nil, nil)
	if _, err := service.Confirm(context.Background(), ConfirmCommand{InvoiceID: "invoice-1", TaskID: "task-1", ContractID: "contract-1", ExpectedInvoiceRevision: 2, ExpectedTaskRevision: 1, CommandID: "confirm-1"}); err != nil {
		t.Fatal(err)
	}
	if store.confirmed.ContractID != "contract-1" || store.confirmed.ExpectedInvoiceRevision != 2 || store.confirmed.ExpectedTaskRevision != 1 {
		t.Fatalf("command=%+v", store.confirmed)
	}
}

func TestServiceRejectsMalformedPolicyOutputBeforePersistence(t *testing.T) {
	store := &captureStore{input: InvoiceContext{ID: "invoice-1", PipelineStatus: "MATCHING", Revision: 1}}
	service := NewService(store, invalidPolicy{}, nil)
	_, changed, err := service.MatchInvoice(context.Background(), MatchCommand{InvoiceID: "invoice-1", ExpectedRevision: 1, CommandID: "invalid-policy"})
	if changed || !errors.Is(err, ErrInvalidMatchDecision) || store.decision.PolicyVersion != "" {
		t.Fatalf("changed=%v decision=%+v error=%v", changed, store.decision, err)
	}
}

func TestContractAvailableIsExplicitDurableBoundary(t *testing.T) {
	store := &captureStore{}
	service := NewService(store, nil, nil)
	changed, err := service.ContractAvailable(context.Background(), AvailableCommand{ContractID: "contract-1", CommandID: "ingestion-1", CorrelationID: "trace-1"})
	if err != nil || !changed || store.available.ContractID != "contract-1" {
		t.Fatalf("changed=%v command=%+v err=%v", changed, store.available, err)
	}
}

func TestResumeBlockedInvoiceReusesBaselinePolicy(t *testing.T) {
	store := &captureStore{
		input:     InvoiceContext{ID: "invoice-1", PipelineStatus: "AWAITING_CONTRACT", Revision: 2, IssueDay: time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), Currency: "RON"},
		contracts: []Contract{policyContract("contract-1", "CTR-1", "RON")},
	}
	service := NewService(store, BaselinePolicy{}, nil)
	decision, outcome, err := service.ResumeBlockedInvoice(context.Background(), ResumeCommand{InvoiceID: "invoice-1", ExpectedRevision: 2, CommandID: "resume-1"})
	if err != nil || outcome != ResumeAutomaticallyAssociated || decision.Outcome != OutcomeUniqueCompatible || store.decision.PolicyVersion != BaselinePolicyVersion {
		t.Fatalf("decision=%+v outcome=%s persisted=%+v err=%v", decision, outcome, store.decision, err)
	}
}

func TestResumeBlockedInvoicePreservesNoMatchAsBusinessOutcome(t *testing.T) {
	store := &captureStore{input: InvoiceContext{ID: "invoice-1", PipelineStatus: "AWAITING_CONTRACT", Revision: 2, IssueDay: time.Now(), Currency: "RON"}}
	service := NewService(store, nil, nil)
	decision, outcome, err := service.ResumeBlockedInvoice(context.Background(), ResumeCommand{InvoiceID: "invoice-1", ExpectedRevision: 2, CommandID: "resume-no-match"})
	if err != nil || outcome != ResumeStillMissing || decision.Outcome != OutcomeNoMatch {
		t.Fatalf("decision=%+v outcome=%s err=%v", decision, outcome, err)
	}
}

func TestResumeBlockedInvoiceStaleStateIsNoop(t *testing.T) {
	store := &captureStore{input: InvoiceContext{ID: "invoice-1", PipelineStatus: "AWAITING_MATCH_CONFIRM", Revision: 3}}
	service := NewService(store, nil, nil)
	_, outcome, err := service.ResumeBlockedInvoice(context.Background(), ResumeCommand{InvoiceID: "invoice-1", ExpectedRevision: 3, CommandID: "stale"})
	if err != nil || outcome != ResumeStaleNoop || store.decision.PolicyVersion != "" {
		t.Fatalf("outcome=%s decision=%+v err=%v", outcome, store.decision, err)
	}
}
