package classification

import (
	"context"
	"errors"
	"testing"
	"time"
)

type captureStore struct {
	input     InvoiceContext
	result    Result
	review    ReviewCommand
	committed bool
}

func (s *captureStore) ProcessCommandCommitted(context.Context, string) (bool, error) {
	return s.committed, nil
}
func (s *captureStore) LoadClassificationInput(context.Context, string) (InvoiceContext, error) {
	return s.input, nil
}
func (s *captureStore) ApplyClassification(_ context.Context, _ ProcessCommand, result Result, _ time.Time) (bool, error) {
	s.result = result
	return true, nil
}
func (s *captureStore) ReviewClassification(_ context.Context, command ReviewCommand, _ time.Time) (bool, error) {
	s.review = command
	return true, nil
}

func TestServiceOwnsClassificationPolicyBoundaryAndReplay(t *testing.T) {
	store := &captureStore{input: InvoiceContext{ID: "invoice", PipelineStatus: "COMMERCIALLY_VALIDATED", Revision: 4, Lines: []LineContext{{ID: "line", Description: "Serviciu"}}, Rules: baselineRules()}}
	service := NewService(store, BaselinePolicy{}, nil)
	result, changed, err := service.ProcessInvoice(context.Background(), ProcessCommand{InvoiceID: "invoice", ExpectedRevision: 4, CommandID: "classify"})
	if err != nil || !changed || len(result.Proposals) != 3 || store.result.PolicyVersion != BaselinePolicyVersion {
		t.Fatalf("result=%+v changed=%v err=%v", result, changed, err)
	}
	store.committed = true
	if _, changed, err = service.ProcessInvoice(context.Background(), ProcessCommand{InvoiceID: "invoice", ExpectedRevision: 4, CommandID: "classify"}); err != nil || changed {
		t.Fatalf("replay changed=%v err=%v", changed, err)
	}
}

func TestReviewRequiresAllOptimisticRevisionsAndNormalizesCorrection(t *testing.T) {
	store := &captureStore{}
	service := NewService(store, BaselinePolicy{}, nil)
	value := " corrected "
	command := ReviewCommand{InvoiceID: "invoice", TaskID: "task", ClassificationID: "classification", ExpectedInvoiceRevision: 6, ExpectedTaskRevision: 1, ExpectedClassificationRevision: 1, CorrectedValue: &value, CommandID: "review", ActorDisplay: "Contabil"}
	if _, err := service.Review(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	if store.review.CorrectedValue == nil || *store.review.CorrectedValue != "corrected" {
		t.Fatalf("review=%+v", store.review)
	}
	command.ExpectedClassificationRevision = 0
	if _, err := service.Review(context.Background(), command); err == nil {
		t.Fatal("expected validation error")
	}
}

type invalidPolicy struct{}

func (invalidPolicy) Version() string { return "INVALID" }
func (invalidPolicy) Evaluate(InvoiceContext) (Result, error) {
	return Result{PolicyVersion: "INVALID"}, nil
}

func TestMalformedPolicyOutputIsRejectedBeforePersistence(t *testing.T) {
	store := &captureStore{input: InvoiceContext{ID: "invoice", PipelineStatus: "COMMERCIALLY_VALIDATED", Revision: 1, Lines: []LineContext{{ID: "line"}}}}
	_, changed, err := NewService(store, invalidPolicy{}, nil).ProcessInvoice(context.Background(), ProcessCommand{InvoiceID: "invoice", ExpectedRevision: 1, CommandID: "bad"})
	if changed || !errors.Is(err, ErrInvalidPolicyResult) {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
}
