package classification

import (
	"context"
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/money"
	"errors"
	"testing"
	"time"
)

type captureStore struct {
	input              InvoiceContext
	result             Result
	review             ReviewCommand
	committed          bool
	blockedCode        string
	reanalysisPrepared bool
}

func TestApprovedKnowledgeIsExactDimensionScopedAndConflictSafe(t *testing.T) {
	profile := &accounting.Profile{ID: "profile", Version: 3}
	base := InvoiceContext{ClientID: "client-a", NormalizedSupplierID: "orange", Currency: "RON", DocumentType: "INVOICE", Snapshot: &accounting.Snapshot{Profile: profile}, Lines: []LineContext{{ID: "line", Description: "Abonament Smart 15", VATRate: money.Amount("21.0000")}}}
	value := accounting.Value{Kind: "FULL"}
	base.Knowledge = []KnowledgeCandidate{{ID: "k1", Version: 1, Dimension: DimensionVATDeductibility, Value: value, NormalizedSupplierID: "orange", ServiceIdentityKind: "NORMALIZED_DESCRIPTION", ServiceIdentityValue: NormalizeDescriptionV1("Abonament Smart 15"), NormalizerVersion: NormalizedDescriptionV1, Currency: "RON", DocumentType: "INVOICE", VATRate: "21", ProfileID: "profile", ProfileVersion: 3, Status: "ACTIVE"}}
	result := func() Result {
		return Result{ModelVersion: accounting.ModelVersion, Proposals: []Proposal{{InvoiceLineID: "line", Dimension: DimensionAccount, Source: SourceNoMatch, RequiresReview: true}, {InvoiceLineID: "line", Dimension: DimensionVATDeductibility, Source: SourceNoMatch, RequiresReview: true}}}
	}
	matched := applyApprovedKnowledge(base, result())
	if matched.Proposals[0].Source != SourceNoMatch || matched.Proposals[1].Source != SourceLearnedMapping || matched.Proposals[1].TypedValue == nil {
		t.Fatalf("dimension leak or missing exact match: %+v", matched.Proposals)
	}
	if (Result{ModelVersion: accounting.ModelVersion, Proposals: []Proposal{matched.Proposals[1]}}).NeedsAI() {
		t.Fatal("exact approved knowledge should keep its resolved dimension out of AI fallback")
	}
	base.Lines[0].Description = "Alt serviciu"
	if got := applyApprovedKnowledge(base, result()); got.Proposals[1].Source != SourceNoMatch {
		t.Fatal("supplier-only match must be impossible")
	}
	base.Lines[0].Description = "Abonament Smart 15"
	base.Lines[0].SourceFacts = &accounting.LineFacts{SellerItemID: "ORANGE-PLAN-15"}
	if got := applyApprovedKnowledge(base, result()); got.Proposals[1].Source != SourceNoMatch {
		t.Fatal("weaker description identity must not override an available exact seller item ID")
	}
	base.Lines[0].SourceFacts = nil
	other := accounting.Value{Kind: "NONE", Reason: "fără drept"}
	base.Knowledge = append(base.Knowledge, KnowledgeCandidate{ID: "k2", Version: 1, Dimension: DimensionVATDeductibility, Value: other, NormalizedSupplierID: "orange", ServiceIdentityKind: "NORMALIZED_DESCRIPTION", ServiceIdentityValue: NormalizeDescriptionV1("Abonament Smart 15"), NormalizerVersion: NormalizedDescriptionV1, Currency: "RON", DocumentType: "INVOICE", VATRate: "21.0000", ProfileID: "profile", ProfileVersion: 3, Status: "ACTIVE"})
	conflict := applyApprovedKnowledge(base, result())
	if conflict.Proposals[1].Source != SourceAmbiguous || !conflict.Proposals[1].KnowledgeConflict || (Result{ModelVersion: accounting.ModelVersion, Proposals: []Proposal{conflict.Proposals[1]}}).NeedsAI() {
		t.Fatalf("conflict was not review-safe: %+v", conflict.Proposals[1])
	}
}

func TestValidateResultAcceptsApprovedKnowledgeAsLearnedEvidence(t *testing.T) {
	input := InvoiceContext{ModelVersion: accounting.ModelVersion, Lines: []LineContext{{ID: "line"}}}
	result := Result{ModelVersion: accounting.ModelVersion, PolicyVersion: ProductionPolicyVersion}
	for _, dimension := range accounting.Dimensions {
		proposal := Proposal{InvoiceLineID: "line", Dimension: Dimension(dimension), ProposedValue: "Necesită decizie", Confidence: "Review", Explanation: "Review contabil", LegalBasis: "Decizie manuală", RequiresReview: true, Source: SourceNoMatch, ModelVersion: accounting.ModelVersion}
		if proposal.Dimension == DimensionVATDeductibility {
			proposal.Source = SourceLearnedMapping
			proposal.ProposedValue = "Deductibilă integral"
			proposal.TypedValue = &accounting.Value{Kind: "FULL"}
			proposal.Knowledge = &KnowledgeReference{ID: "knowledge-1", Version: 1}
		}
		result.Proposals = append(result.Proposals, proposal)
	}
	if err := validateResult(input, result, ProductionPolicyVersion); err != nil {
		t.Fatalf("approved knowledge reference should validate as learned evidence: %v", err)
	}
	for i := range result.Proposals {
		if result.Proposals[i].Source == SourceLearnedMapping {
			result.Proposals[i].Knowledge = nil
		}
	}
	if err := validateResult(input, result, ProductionPolicyVersion); !errors.Is(err, ErrInvalidPolicyResult) {
		t.Fatalf("learned proposal without mapping or knowledge reference was accepted: %v", err)
	}
}

func (s *captureStore) ApplyClassificationBlock(_ context.Context, _ ProcessCommand, _ *accounting.Snapshot, code, _ string, _ time.Time) (bool, error) {
	s.blockedCode = code
	return true, nil
}
func (s *captureStore) PrepareClassificationReanalysis(_ context.Context, command ReanalysisCommand, _ time.Time) (uint64, bool, error) {
	s.reanalysisPrepared = true
	s.input.PipelineStatus = "COMMERCIALLY_VALIDATED"
	s.input.Revision = command.ExpectedRevision + 1
	return s.input.Revision, true, nil
}

func (s *captureStore) ProcessCommandCommitted(context.Context, string) (bool, error) {
	return s.committed, nil
}

func TestMissingFiscalProfileCreatesExplicitBlock(t *testing.T) {
	store := &captureStore{input: InvoiceContext{ID: "invoice", ModelVersion: accounting.ModelVersion, PipelineStatus: "COMMERCIALLY_VALIDATED", Revision: 3, Snapshot: &accounting.Snapshot{}, Lines: []LineContext{{ID: "line"}}}}
	_, changed, err := NewService(store, DomainPolicy{}, nil).ProcessInvoice(context.Background(), ProcessCommand{InvoiceID: "invoice", ExpectedRevision: 3, CommandID: "missing-profile"})
	if err != nil || !changed || store.blockedCode != "MISSING_FISCAL_PROFILE" {
		t.Fatalf("changed=%v blocker=%s err=%v", changed, store.blockedCode, err)
	}
}

func TestExplicitReanalysisPreparesThenRunsExistingClassifier(t *testing.T) {
	store := &captureStore{input: InvoiceContext{ID: "invoice", PipelineStatus: "AWAITING_REVIEW", Revision: 7, Lines: []LineContext{{ID: "line", Description: "Serviciu"}}, Rules: baselineRules()}}
	changed, err := NewService(store, BaselinePolicy{}, nil).Reanalyze(context.Background(), ReanalysisCommand{InvoiceID: "invoice", ClientID: "client", CommandID: "reanalyze", ExpectedRevision: 7, ActorID: "actor", ActorDisplay: "Contabil"})
	if err != nil || !changed || !store.reanalysisPrepared || len(store.result.Proposals) != 3 {
		t.Fatalf("changed=%v prepared=%v proposals=%d err=%v", changed, store.reanalysisPrepared, len(store.result.Proposals), err)
	}
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

func TestResultNeedsAIOnlyForUnresolvedCanonicalDimensions(t *testing.T) {
	result := Result{ModelVersion: accounting.ModelVersion, Proposals: []Proposal{{Dimension: DimensionAccount, RequiresReview: true, Source: SourceNoMatch}}}
	if !result.NeedsAI() {
		t.Fatal("NO_MATCH must request AI")
	}
	result.Proposals[0].Source = SourceAIProposal
	if result.NeedsAI() {
		t.Fatal("existing AI proposal must not request duplicate AI")
	}
	result.Proposals[0].Source = SourceLearnedMapping
	if result.NeedsAI() {
		t.Fatal("exact learned proposal must stay in human review")
	}
}
