package contractingestion_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/commercialvalidation"
	ci "diana-contabilitate/backend/internal/contractingestion"
	"diana-contabilitate/backend/internal/contractingestion/fixtures"
	"diana-contabilitate/backend/internal/contracts"
)

func TestCommercialRuleValidationReportsSafeStructuralPath(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*commercialvalidation.Rule)
		path string
	}{
		{"missing ID", func(r *commercialvalidation.Rule) { r.ID = "" }, "commercialClauses[0].rule.id"},
		{"unsupported kind", func(r *commercialvalidation.Rule) { r.Kind = "PRIVATE_CONTRACT_TEXT" }, "commercialClauses[0].rule.kind"},
		{"invalid expression", func(r *commercialvalidation.Rule) { r.Expression.Value = "500 RON" }, "commercialClauses[0].rule.expression"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proposal := fixtures.Proposal("romanian")
			var rule commercialvalidation.Rule
			if err := json.Unmarshal(proposal.CommercialClauses[0].Rule, &rule); err != nil {
				t.Fatal(err)
			}
			tc.edit(&rule)
			proposal.CommercialClauses[0].Rule, _ = json.Marshal(rule)
			var validation *ci.ProposalValidationError
			if err := ci.ValidateProposal(proposal); !errors.As(err, &validation) || validation.Code != "COMMERCIAL_RULE_INVALID" || validation.Path != tc.path {
				t.Fatalf("validation=%v, want path %s", err, tc.path)
			}
		})
	}
}

func TestCommercialRuleValidationRejectsConflictingSupportedKinds(t *testing.T) {
	proposal := fixtures.Proposal("romanian")
	var rule commercialvalidation.Rule
	if err := json.Unmarshal(proposal.CommercialClauses[0].Rule, &rule); err != nil {
		t.Fatal(err)
	}
	rule.Kind = commercialvalidation.RuleUnitRate
	proposal.CommercialClauses[0].Rule, _ = json.Marshal(rule)
	var validation *ci.ProposalValidationError
	if err := ci.ValidateProposal(proposal); !errors.As(err, &validation) || validation.Code != "COMMERCIAL_RULE_KIND_CONFLICT" || validation.Path != "commercialClauses[0].rule.kind" {
		t.Fatalf("conflicting kinds were accepted: %v", err)
	}
}

func TestCommercialClauseWithoutExpressionRemainsReviewableNotExecutable(t *testing.T) {
	proposal := fixtures.Proposal("romanian")
	var rule commercialvalidation.Rule
	if err := json.Unmarshal(proposal.CommercialClauses[0].Rule, &rule); err != nil {
		t.Fatal(err)
	}
	rule.Expression = nil
	proposal.CommercialClauses[0].Rule, _ = json.Marshal(rule)
	if err := ci.ValidateProposal(proposal); err != nil {
		t.Fatalf("narrative clause was discarded before review: %v", err)
	}
	pending := ci.PendingCommercialClauses(proposal)
	if len(pending) != 1 || pending[0].Narrative.Value == nil || *pending[0].Narrative.Value == "" {
		t.Fatalf("pending clause or its narrative was lost: %+v", pending)
	}
}

func TestCommercialClauseRemainsUnconfirmedUntilHumanSelectsRule(t *testing.T) {
	proposal := fixtures.Proposal("romanian")
	var rule commercialvalidation.Rule
	if err := json.Unmarshal(proposal.CommercialClauses[0].Rule, &rule); err != nil {
		t.Fatal(err)
	}
	if pending := ci.UnconfirmedCommercialClauses(proposal, nil); len(pending) != 1 {
		t.Fatalf("AI-normalized rule became authoritative without review: %+v", pending)
	}
	if pending := ci.UnconfirmedCommercialClauses(proposal, []commercialvalidation.Rule{rule}); len(pending) != 0 {
		t.Fatalf("human-selected rule remained pending: %+v", pending)
	}
}

func TestSupplementWithOnlyPendingCommercialClauseMayBeConfirmedPartial(t *testing.T) {
	value := ci.ReviewedContract{DocumentRole: "AMENDMENT", RelatedReference: "102/25.06.2025", BuyerCUI: "RO10000000", Coverage: commercialvalidation.CoveragePartial, PendingCommercialClauses: 1}
	if readiness := ci.ConfirmationReadinessFor(value, "RO10000000"); !readiness.CanConfirm {
		t.Fatalf("narrative-only supplement cannot be retained for review: %+v", readiness.Blockers)
	}
}

type memoryStore struct {
	doc                   ci.Document
	source                []byte
	proposal              *ci.Proposal
	confirmed             contracts.Contract
	hash                  string
	failures, completions int
	failureContextError   error
	failureCategory       string
}

func (m *memoryStore) CreateDocument(_ context.Context, u ci.Upload, id, hash string, now time.Time) (ci.Document, bool, error) {
	if m.hash == hash {
		return m.doc, true, nil
	}
	m.hash = hash
	m.source = append([]byte(nil), u.Bytes...)
	m.doc = ci.Document{ID: id, ClientID: u.ClientID, OriginalFilename: u.Filename, MIMEType: "application/pdf", SizeBytes: int64(len(u.Bytes)), SHA256: hash, Status: ci.StatusUploaded, Revision: 1, UploadedAt: now}
	return m.doc, false, nil
}
func (m *memoryStore) ListDocuments(context.Context, string) ([]ci.Document, error) {
	return []ci.Document{m.doc}, nil
}
func (m *memoryStore) GetDocument(_ context.Context, client, id string) (ci.Document, error) {
	if client != m.doc.ClientID || id != m.doc.ID {
		return ci.Document{}, apperrors.ErrNotFound
	}
	return m.doc, nil
}
func (m *memoryStore) GetSource(context.Context, string) (ci.Source, error) {
	return ci.Source{Document: m.doc, Bytes: m.source}, nil
}
func (m *memoryStore) BeginExtraction(_ context.Context, id, attempt, provider, model string, now time.Time) (ci.Document, bool, error) {
	if m.doc.Status == ci.StatusReadyForReview || m.doc.Status == ci.StatusConfirmed {
		return m.doc, false, nil
	}
	m.doc.Status = ci.StatusExtracting
	m.doc.LatestExtractionID = &attempt
	return m.doc, true, nil
}
func (m *memoryStore) CompleteExtraction(_ context.Context, id, attempt string, result ci.ExtractionResult, now time.Time) error {
	m.completions++
	m.proposal = &result.Proposal
	m.doc.LatestAttempt = &ci.Attempt{ID: attempt, Proposal: m.proposal, Status: "SUCCEEDED"}
	m.doc.Status = ci.StatusReadyForReview
	m.doc.Revision++
	return nil
}
func (m *memoryStore) FailExtraction(ctx context.Context, _ string, _ string, category string, _ time.Time) error {
	m.failures++
	m.failureContextError = ctx.Err()
	m.failureCategory = category
	m.doc.Status = ci.StatusExtractionFailed
	return nil
}
func (m *memoryStore) RetryExtraction(context.Context, string, string, uint64, ci.Actor, time.Time) error {
	m.doc.Status = ci.StatusUploaded
	return nil
}
func (m *memoryStore) DiscardDocument(_ context.Context, client, id string, revision uint64, _ ci.Actor, _ time.Time) error {
	if client != m.doc.ClientID || id != m.doc.ID {
		return apperrors.ErrNotFound
	}
	if m.doc.Status == ci.StatusConfirmed || revision != m.doc.Revision {
		return apperrors.ErrConflict
	}
	m.doc.LifecycleState = "DISCARDED"
	m.doc.Revision++
	return nil
}
func (m *memoryStore) ConfirmDocument(_ context.Context, c ci.ConfirmCommand, value contracts.Contract, now time.Time) (string, bool, error) {
	if m.doc.Status != ci.StatusReadyForReview || m.doc.Revision != c.ExpectedDocumentRevision || m.doc.LatestExtractionID == nil || *m.doc.LatestExtractionID != c.ExtractionAttemptID {
		return "", false, apperrors.ErrConflict
	}
	m.confirmed = value
	m.doc.Status = ci.StatusConfirmed
	return value.ID, true, nil
}

type availability struct{ calls int }

func (a *availability) ContractAvailable(context.Context, contracts.AvailableCommand) (bool, error) {
	a.calls++
	return true, nil
}

type flakyExtractor struct {
	calls     int
	result    ci.Proposal
	permanent bool
}

type proposalExtractor struct{ proposal ci.Proposal }

func (*proposalExtractor) Provider() string { return "FAKE" }
func (*proposalExtractor) Model() string    { return "fake-v1" }
func (e *proposalExtractor) Extract(context.Context, []byte, string) (ci.ExtractionResult, error) {
	return ci.ExtractionResult{Proposal: e.proposal}, nil
}

func (*flakyExtractor) Provider() string { return "FAKE" }
func (*flakyExtractor) Model() string    { return "fake-v1" }
func (f *flakyExtractor) Extract(context.Context, []byte, string) (ci.ExtractionResult, error) {
	f.calls++
	if f.permanent {
		return ci.ExtractionResult{}, ci.ErrExtractionPermanent
	}
	if f.calls == 1 {
		return ci.ExtractionResult{}, ci.ErrExtractionTransient
	}
	return ci.ExtractionResult{Proposal: f.result}, nil
}
func reviewed(p ci.Proposal) ci.ReviewedContract {
	result := ci.ReviewedContract{SupplierName: *p.SupplierName.Value, SupplierCUI: *p.SupplierCUI.Value, Reference: *p.Reference.Value, EffectiveFrom: *p.EffectiveFrom.Value, EffectiveTo: valueOrEmpty(p.EffectiveTo.Value), TotalValue: valueOrEmpty(p.TotalValue.Value), Currency: *p.Currency.Value, UnitType: valueOrEmpty(p.UnitType.Value), PaymentTerms: valueOrEmpty(p.PaymentTerms.Value), BuyerCUI: *p.BuyerCUI.Value, PeriodType: valueOrEmpty(p.PeriodType.Value)}
	for _, term := range p.ServiceTerms {
		result.ServiceTerms = append(result.ServiceTerms, ci.ReviewedServiceTerm{ServiceDescription: valueOrEmpty(term.ServiceDescription.Value), PricingModel: valueOrEmpty(term.PricingModel.Value), UnitPrice: valueOrEmpty(term.UnitPrice.Value), Currency: valueOrEmpty(term.Currency.Value), Unit: valueOrEmpty(term.Unit.Value), QuantitySource: valueOrEmpty(term.QuantitySource.Value), QuantityValue: valueOrEmpty(term.QuantityValue.Value), QuantityDriver: valueOrEmpty(term.QuantityDriver.Value), BillingFrequency: valueOrEmpty(term.BillingFrequency.Value), Evidence: term.ServiceDescription.Evidence})
	}
	return result
}
func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func TestContractDocumentUploadValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		pdf   []byte
		mime  string
		limit int64
		want  error
	}{{"zero", nil, "application/pdf", 0, apperrors.ErrValidation}, {"magic", []byte("not a PDF"), "application/pdf", 0, ci.ErrInvalidPDF}, {"truncated", []byte("%PDF-1.4"), "application/pdf", 0, ci.ErrInvalidPDF}, {"mime", fixtures.PDF("romanian"), "text/html", 0, ci.ErrInvalidPDF}, {"limit", fixtures.PDF("romanian"), "application/pdf", 10, ci.ErrDocumentTooLarge}} {
		t.Run(tc.name, func(t *testing.T) {
			service := ci.NewService(&memoryStore{}, fixtures.Extractor{}, &availability{}, tc.limit, nil)
			_, _, err := service.Upload(context.Background(), ci.Upload{ClientID: "client", Filename: "contract.pdf", ContentType: tc.mime, Bytes: tc.pdf, Actor: ci.Actor{AllClients: true}})
			if !errors.Is(err, tc.want) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
func TestContractDocumentDuplicateAndSafeFilename(t *testing.T) {
	store := &memoryStore{}
	service := ci.NewService(store, fixtures.Extractor{}, &availability{}, 0, nil)
	upload := ci.Upload{ClientID: "client", Filename: "../../contract\r\n.pdf", Bytes: fixtures.PDF("romanian"), Actor: ci.Actor{AllClients: true}}
	doc, duplicate, err := service.Upload(context.Background(), upload)
	if err != nil || duplicate || doc.OriginalFilename != "contract.pdf" || len(doc.SHA256) != 64 {
		t.Fatalf("invalid upload result: %v", err)
	}
	again, duplicate, err := service.Upload(context.Background(), upload)
	if err != nil || !duplicate || again.ID != doc.ID {
		t.Fatal("duplicate not reused")
	}
}
func TestContractExtractionFixturesAndNoAutomaticActivation(t *testing.T) {
	for _, name := range fixtures.Names {
		t.Run(name, func(t *testing.T) {
			store := &memoryStore{}
			available := &availability{}
			service := ci.NewService(store, fixtures.Extractor{}, available, 0, nil)
			doc, _, err := service.Upload(context.Background(), ci.Upload{ClientID: "client", Filename: "contract.pdf", Bytes: fixtures.PDF(name), Actor: ci.Actor{AllClients: true}})
			store.doc.MIMEType = "application/pdf"
			if err != nil {
				t.Fatal(err)
			}
			if err = service.Extract(context.Background(), doc.ID); err != nil {
				t.Fatal(err)
			}
			if store.doc.Status != ci.StatusReadyForReview || available.calls != 0 || store.confirmed.ID != "" {
				t.Fatal("AI must remain a proposal")
			}
			if name == "missing" && store.proposal.Currency.Value != nil {
				t.Fatal("currency hallucination")
			}
			if name == "injection" && *store.proposal.SupplierCUI.Value != "RO12345678" {
				t.Fatal("document instructions changed extraction")
			}
			if err = service.Extract(context.Background(), doc.ID); err != nil || store.completions != 1 {
				t.Fatal("retry duplicated successful extraction")
			}
		})
	}
}
func TestContractExtractionHumanCorrectionPreservesProposal(t *testing.T) {
	store := &memoryStore{}
	available := &availability{}
	service := ci.NewService(store, fixtures.Extractor{}, available, 0, nil)
	doc, _, _ := service.Upload(context.Background(), ci.Upload{ClientID: "client", Filename: "contract.pdf", Bytes: fixtures.PDF("romanian"), Actor: ci.Actor{AllClients: true}})
	store.doc.MIMEType = "application/pdf"
	if err := service.Extract(context.Background(), doc.ID); err != nil {
		t.Fatal(err)
	}
	value := reviewed(*store.proposal)
	value.Reference = "USER-CORRECTED"
	_, changed, err := service.Confirm(context.Background(), ci.ConfirmCommand{ClientID: "client", DocumentID: doc.ID, ExtractionAttemptID: *store.doc.LatestExtractionID, ExpectedDocumentRevision: store.doc.Revision, Contract: value, CommandID: "confirm", Actor: ci.Actor{AllClients: true}})
	if err != nil || !changed || store.confirmed.Reference != "USER-CORRECTED" || *store.proposal.Reference.Value != "CTR-2026-01" || available.calls != 1 {
		t.Fatalf("human boundary error=%v", err)
	}
}

func TestReviewedBuyerCorrectionUsesCanonicalRomanianIdentity(t *testing.T) {
	proposal := fixtures.Proposal("romanian")
	wrong := "RO99999999"
	proposal.BuyerCUI.Value = &wrong
	store := &memoryStore{doc: ci.Document{ID: "doc", ClientID: "client", ClientCUI: "RO21592770", Status: ci.StatusReadyForReview, Revision: 2}, proposal: &proposal}
	attempt := "attempt"
	store.doc.LatestExtractionID = &attempt
	service := ci.NewService(store, nil, &availability{}, 0, nil)
	reviewedValue := reviewed(proposal)
	reviewedValue.BuyerCUI = "ro 21592770"
	if _, _, err := service.Confirm(context.Background(), ci.ConfirmCommand{ClientID: "client", DocumentID: "doc", ExtractionAttemptID: attempt, ExpectedDocumentRevision: 2, Contract: reviewedValue, CommandID: "corrected", Actor: ci.Actor{AllClients: true}}); err != nil {
		t.Fatalf("corrected reviewed value remained blocked: %v", err)
	}
}

func TestIndefiniteServiceContractReadinessAndTerms(t *testing.T) {
	proposal := fixtures.Proposal("service-indefinite")
	reviewedValue := reviewed(proposal)
	readiness := ci.ConfirmationReadinessFor(reviewedValue, "RO10000000")
	if !readiness.CanConfirm || reviewedValue.EffectiveTo != "" {
		t.Fatalf("readiness=%+v end=%q", readiness, reviewedValue.EffectiveTo)
	}
	store := &memoryStore{doc: ci.Document{ID: "doc", ClientID: "client", ClientCUI: "RO10000000", Status: ci.StatusReadyForReview, Revision: 2}}
	attempt := "attempt"
	store.doc.LatestExtractionID = &attempt
	service := ci.NewService(store, nil, &availability{}, 0, nil)
	_, _, err := service.Confirm(context.Background(), ci.ConfirmCommand{ClientID: "client", DocumentID: "doc", ExtractionAttemptID: attempt, ExpectedDocumentRevision: 2, Contract: reviewedValue, CommandID: "indefinite", Actor: ci.Actor{AllClients: true}})
	if err != nil || store.confirmed.EffectiveTo != nil || len(store.confirmed.ServiceTerms) != 2 || store.confirmed.ServiceTerms[1].PricingModel != "UNIT_RATE" || store.confirmed.ServiceTerms[1].Unit != "SALARIAT" {
		t.Fatalf("contract=%+v err=%v", store.confirmed, err)
	}
}

func TestDiscardUnconfirmedDocument(t *testing.T) {
	store := &memoryStore{doc: ci.Document{ID: "doc", ClientID: "client", Status: ci.StatusReadyForReview, Revision: 2}}
	service := ci.NewService(store, nil, nil, 0, nil)
	if err := service.Discard(context.Background(), "client", "doc", 2, ci.Actor{AllClients: true}); err != nil || store.doc.LifecycleState != "DISCARDED" {
		t.Fatalf("discard=%v state=%s", err, store.doc.LifecycleState)
	}
	store.doc.Status = ci.StatusConfirmed
	if err := service.Discard(context.Background(), "client", "doc", 3, ci.Actor{AllClients: true}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("confirmed discard=%v", err)
	}
}
func TestContractExtractionTransientAndPermanentFailure(t *testing.T) {
	for _, permanent := range []bool{false, true} {
		store := &memoryStore{}
		extractor := &flakyExtractor{result: fixtures.Proposal("romanian"), permanent: permanent}
		service := ci.NewService(store, extractor, &availability{}, 0, nil)
		doc, _, err := service.Upload(context.Background(), ci.Upload{ClientID: "client", Filename: "contract.pdf", Bytes: fixtures.PDF("romanian"), Actor: ci.Actor{AllClients: true}})
		if err != nil {
			t.Fatal(err)
		}
		err = service.Extract(context.Background(), doc.ID)
		if err == nil || store.failures != 1 || store.confirmed.ID != "" {
			t.Fatal("unsafe failure")
		}
		failure, ok := ci.ExtractionFailureDetails(err)
		if !ok || failure.Category != store.failureCategory || strings.Contains(err.Error(), "document") {
			t.Fatalf("unsafe categorized failure=%v persisted=%s", err, store.failureCategory)
		}
		if !permanent {
			if err = service.Extract(context.Background(), doc.ID); err != nil || store.completions != 1 {
				t.Fatal("transient retry failed")
			}
		}
	}
}

func TestContractExtractionCarriesSafeProposalValidationDiagnostic(t *testing.T) {
	store := &memoryStore{}
	proposal := fixtures.Proposal("romanian")
	proposal.ServiceTerms[0].Currency.Evidence.Snippet = ""
	service := ci.NewService(store, &proposalExtractor{proposal: proposal}, &availability{}, 0, nil)
	doc, _, err := service.Upload(context.Background(), ci.Upload{ClientID: "client", Filename: "contract.pdf", Bytes: fixtures.PDF("romanian"), Actor: ci.Actor{AllClients: true}})
	if err != nil {
		t.Fatal(err)
	}
	err = service.Extract(context.Background(), doc.ID)
	failure, ok := ci.ExtractionFailureDetails(err)
	if !ok || failure.Category != ci.FailureProposalValidationFailed || failure.Retry != ci.RetryOnce || failure.ValidationCode != "PRESENT_WITHOUT_EVIDENCE" || failure.ValidationPath != "serviceTerms[0].currency" {
		t.Fatalf("failure=%+v error=%v", failure, err)
	}
	if strings.Contains(err.Error(), "RON") || store.failureCategory != ci.FailureProposalValidationFailed {
		t.Fatalf("unsafe or unpersisted failure: %v category=%s", err, store.failureCategory)
	}
}
func TestContractIngestionTenantIsolation(t *testing.T) {
	store := &memoryStore{doc: ci.Document{ID: "doc", ClientID: "client-b"}}
	service := ci.NewService(store, nil, nil, 0, nil)
	actor := ci.Actor{AuthorizedClientIDs: []string{"client-a"}}
	if _, _, err := service.Upload(context.Background(), ci.Upload{ClientID: "client-b", Actor: actor}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("cross-client upload")
	}
	if _, err := service.Get(context.Background(), "client-b", "doc", actor); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("cross-client metadata")
	}
	if _, err := service.File(context.Background(), "client-b", "doc", actor); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("cross-client file")
	}
}

type interruptedExtractor struct{ cancel context.CancelFunc }

func (*interruptedExtractor) Provider() string { return "DETERMINISTIC_TEST" }
func (*interruptedExtractor) Model() string    { return "interrupt-v1" }
func (e *interruptedExtractor) Extract(context.Context, []byte, string) (ci.ExtractionResult, error) {
	e.cancel()
	return ci.ExtractionResult{}, context.Canceled
}
func TestContractExtractionCancellationPersistsFailureAndAllowsRetry(t *testing.T) {
	store := &memoryStore{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := ci.NewService(store, &interruptedExtractor{cancel: cancel}, &availability{}, 0, nil)
	doc, _, err := service.Upload(ctx, ci.Upload{ClientID: "client", Filename: "contract.pdf", Bytes: fixtures.PDF("romanian"), Actor: ci.Actor{AllClients: true}})
	if err != nil {
		t.Fatal(err)
	}
	err = service.Extract(ctx, doc.ID)
	if !errors.Is(err, context.Canceled) || errors.Is(err, ci.ErrExtractionPermanent) || store.failures != 1 || store.failureContextError != nil {
		t.Fatalf("shutdown failure persistence=%v context=%v", err, store.failureContextError)
	}
}
func TestContractExtractionSourceIntegrityGuard(t *testing.T) {
	store := &memoryStore{}
	service := ci.NewService(store, fixtures.Extractor{}, &availability{}, 0, nil)
	doc, _, err := service.Upload(context.Background(), ci.Upload{ClientID: "client", Filename: "contract.pdf", Bytes: fixtures.PDF("romanian"), Actor: ci.Actor{AllClients: true}})
	if err != nil {
		t.Fatal(err)
	}
	store.doc.SHA256 = "corrupted-metadata"
	if err = service.Extract(context.Background(), doc.ID); !errors.Is(err, ci.ErrExtractionPermanent) || store.completions != 0 || store.failures != 1 {
		t.Fatalf("source integrity=%v", err)
	}
}
func TestContractConfirmationRejectsInvalidAndBuyerMismatch(t *testing.T) {
	for _, change := range []func(*ci.ReviewedContract){func(v *ci.ReviewedContract) { v.SupplierCUI = "wrong" }, func(v *ci.ReviewedContract) { v.Currency = "XYZ" }, func(v *ci.ReviewedContract) { v.EffectiveTo = "01.01.2027" }, func(v *ci.ReviewedContract) { v.EffectiveFrom = "2028-01-01" }, func(v *ci.ReviewedContract) { v.TotalValue = "1.12345" }, func(v *ci.ReviewedContract) { v.UnitType = "" }, func(v *ci.ReviewedContract) { v.PaymentTerms = "" }} {
		store := &memoryStore{doc: ci.Document{ID: "doc", ClientID: "client"}}
		service := ci.NewService(store, nil, &availability{}, 0, nil)
		value := reviewed(fixtures.Proposal("romanian"))
		change(&value)
		if _, _, err := service.Confirm(context.Background(), ci.ConfirmCommand{ClientID: "client", DocumentID: "doc", Contract: value, ExpectedDocumentRevision: 1, CommandID: "key", Actor: ci.Actor{AllClients: true}}); !errors.Is(err, apperrors.ErrValidation) {
			t.Fatalf("validation=%v", err)
		}
	}
	store := &memoryStore{doc: ci.Document{ID: "doc", ClientID: "client", ClientCUI: "RO10000000", BuyerMismatch: true}}
	service := ci.NewService(store, nil, &availability{}, 0, nil)
	value := reviewed(fixtures.Proposal("romanian"))
	value.BuyerCUI = "RO99999999"
	if _, _, err := service.Confirm(context.Background(), ci.ConfirmCommand{ClientID: "client", DocumentID: "doc", Contract: value, ExpectedDocumentRevision: 1, CommandID: "key", Actor: ci.Actor{AllClients: true}}); !errors.Is(err, ci.ErrBuyerMismatch) {
		t.Fatal("buyer mismatch ignored")
	}
}

func TestSupplementCanConfirmOnlyCommercialClausesAndExplicitParent(t *testing.T) {
	value := ci.ReviewedContract{DocumentRole: "AMENDMENT", RelatedReference: "102/25.06.2025", BuyerCUI: "RO10000000", Coverage: commercialvalidation.CoverageComplete, CommercialRules: []commercialvalidation.Rule{{ID: "accounting-fee", Kind: commercialvalidation.RuleFixedPrice, Narrative: "Tarif lunar modificat", DateBasis: commercialvalidation.DateInvoiceIssue, Currency: "RON", Expression: &commercialvalidation.Expression{Op: "literal", Value: "700", Scale: 2}, Evidence: []commercialvalidation.Evidence{{DocumentID: "amendment", Snippet: "700 RON lunar"}}, Blocking: true}}}
	readiness := ci.ConfirmationReadinessFor(value, "RO10000000")
	if !readiness.CanConfirm {
		t.Fatalf("supplement blocked: %+v", readiness.Blockers)
	}
	value.RelatedReference = ""
	readiness = ci.ConfirmationReadinessFor(value, "RO10000000")
	if readiness.CanConfirm {
		t.Fatal("supplement without explicit parent accepted")
	}
}
