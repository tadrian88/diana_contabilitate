package contractingestion_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	ci "diana-contabilitate/backend/internal/contractingestion"
	"diana-contabilitate/backend/internal/contractingestion/fixtures"
	"diana-contabilitate/backend/internal/contracts"
)

type memoryStore struct {
	doc                   ci.Document
	source                []byte
	proposal              *ci.Proposal
	confirmed             contracts.Contract
	hash                  string
	failures, completions int
	failureContextError   error
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
func (m *memoryStore) FailExtraction(ctx context.Context, _ string, _ string, _ string, _ time.Time) error {
	m.failures++
	m.failureContextError = ctx.Err()
	m.doc.Status = ci.StatusExtractionFailed
	return nil
}
func (m *memoryStore) RetryExtraction(context.Context, string, string, uint64, ci.Actor, time.Time) error {
	m.doc.Status = ci.StatusUploaded
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
	return ci.ReviewedContract{SupplierName: *p.SupplierName.Value, SupplierCUI: *p.SupplierCUI.Value, Reference: *p.Reference.Value, EffectiveFrom: *p.EffectiveFrom.Value, EffectiveTo: *p.EffectiveTo.Value, TotalValue: *p.TotalValue.Value, Currency: *p.Currency.Value, UnitType: *p.UnitType.Value, PaymentTerms: *p.PaymentTerms.Value}
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
		if !permanent {
			if err = service.Extract(context.Background(), doc.ID); err != nil || store.completions != 1 {
				t.Fatal("transient retry failed")
			}
		}
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
	for _, change := range []func(*ci.ReviewedContract){func(v *ci.ReviewedContract) { v.SupplierCUI = "wrong" }, func(v *ci.ReviewedContract) { v.Currency = "XYZ" }, func(v *ci.ReviewedContract) { v.EffectiveTo = "01.01.2027" }, func(v *ci.ReviewedContract) { v.EffectiveFrom = "2028-01-01" }, func(v *ci.ReviewedContract) { v.TotalValue = "1.12345" }, func(v *ci.ReviewedContract) { v.UnitType = "" }} {
		store := &memoryStore{doc: ci.Document{ID: "doc", ClientID: "client"}}
		service := ci.NewService(store, nil, &availability{}, 0, nil)
		value := reviewed(fixtures.Proposal("romanian"))
		change(&value)
		if _, _, err := service.Confirm(context.Background(), ci.ConfirmCommand{ClientID: "client", DocumentID: "doc", Contract: value, ExpectedDocumentRevision: 1, CommandID: "key", Actor: ci.Actor{AllClients: true}}); !errors.Is(err, apperrors.ErrValidation) {
			t.Fatalf("validation=%v", err)
		}
	}
	store := &memoryStore{doc: ci.Document{ID: "doc", ClientID: "client", BuyerMismatch: true}}
	service := ci.NewService(store, nil, &availability{}, 0, nil)
	if _, _, err := service.Confirm(context.Background(), ci.ConfirmCommand{ClientID: "client", DocumentID: "doc", ExpectedDocumentRevision: 1, CommandID: "key", Actor: ci.Actor{AllClients: true}}); !errors.Is(err, ci.ErrBuyerMismatch) {
		t.Fatal("buyer mismatch ignored")
	}
}
