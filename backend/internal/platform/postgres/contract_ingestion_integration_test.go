//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"diana-contabilitate/backend/ent/contract"
	"diana-contabilitate/backend/ent/contractextractionattempt"
	"diana-contabilitate/backend/ent/contractserviceterm"
	"diana-contabilitate/backend/ent/contractsourcedocument"
	"diana-contabilitate/backend/ent/invoice"
	"diana-contabilitate/backend/ent/outboxentry"
	"diana-contabilitate/backend/ent/validationtask"
	"diana-contabilitate/backend/internal/apperrors"
	ci "diana-contabilitate/backend/internal/contractingestion"
	"diana-contabilitate/backend/internal/contractingestion/fixtures"
	"diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/outbox"
	"diana-contabilitate/backend/internal/validationtasks"
)

type ingestionIntegrationExtractor struct {
	proposal ci.Proposal
	failure  error
}

func (*ingestionIntegrationExtractor) Provider() string { return "DETERMINISTIC_TEST" }
func (*ingestionIntegrationExtractor) Model() string    { return "contract-fixture-v1" }
func (e *ingestionIntegrationExtractor) Extract(context.Context, []byte, string) (ci.ExtractionResult, error) {
	return ci.ExtractionResult{Proposal: e.proposal}, e.failure
}
func contractIngestionFixture(t *testing.T, tc *contractTestContext) (*ci.Service, *contracts.Service, *ingestionIntegrationExtractor) {
	t.Helper()
	client, err := tc.store.Client.AccountingClient.Get(tc.ctx, tc.clientID)
	if err != nil {
		t.Fatal(err)
	}
	p := fixtures.Proposal("romanian")
	p.BuyerCUI.Value = &client.Cui
	p.BuyerCUI.Evidence.Snippet = client.Cui
	extractor := &ingestionIntegrationExtractor{proposal: p}
	matching := contracts.NewService(tc.store, contracts.BaselinePolicy{}, func() time.Time { return tc.now })
	service := ci.NewService(tc.store, extractor, matching, 0, func() time.Time { return tc.now })
	t.Cleanup(func() {
		ids, _ := tc.store.Client.ContractSourceDocument.Query().Where(contractsourcedocument.ClientIDEQ(tc.clientID)).IDs(tc.ctx)
		contractIDs, _ := tc.store.Client.Contract.Query().Where(contract.ClientIDEQ(tc.clientID)).IDs(tc.ctx)
		_, _ = tc.store.Client.OutboxEntry.Delete().Where(outboxentry.AggregateIDIn(append(ids, contractIDs...)...)).Exec(tc.ctx)
		_, _ = tc.store.Client.ContractServiceTerm.Delete().Where(contractserviceterm.ContractIDIn(contractIDs...)).Exec(tc.ctx)
		_, _ = tc.store.Client.ContractExtractionAttempt.Delete().Where(contractextractionattempt.DocumentIDIn(ids...)).Exec(tc.ctx)
		_, _ = tc.store.Client.ContractSourceDocument.Delete().Where(contractsourcedocument.ClientIDEQ(tc.clientID)).Exec(tc.ctx)
	})
	return service, matching, extractor
}
func ingestionUpload(t *testing.T, tc *contractTestContext, s *ci.Service) ci.Document {
	t.Helper()
	doc, _, err := s.Upload(tc.ctx, ci.Upload{ClientID: tc.clientID, Filename: "contract.pdf", ContentType: "application/pdf", Bytes: fixtures.PDF("romanian"), Actor: ci.Actor{ID: "uploader", Display: "Uploader", AllClients: true}})
	if err != nil {
		t.Fatal(err)
	}
	return doc
}
func ingestionReview(t *testing.T, tc *contractTestContext, s *ci.Service, doc ci.Document) ci.ConfirmCommand {
	t.Helper()
	if err := s.Extract(tc.ctx, doc.ID); err != nil {
		t.Fatal(err)
	}
	doc, err := s.Get(tc.ctx, tc.clientID, doc.ID, ci.Actor{AllClients: true})
	if err != nil {
		t.Fatal(err)
	}
	p := doc.LatestAttempt.Proposal
	field := func(value *string) string {
		if value == nil {
			return ""
		}
		return *value
	}
	reviewed := ci.ReviewedContract{SupplierName: field(p.SupplierName.Value), SupplierCUI: field(p.SupplierCUI.Value), BuyerCUI: field(p.BuyerCUI.Value), Reference: field(p.Reference.Value), EffectiveFrom: field(p.EffectiveFrom.Value), EffectiveTo: field(p.EffectiveTo.Value), PeriodType: field(p.PeriodType.Value), TotalValue: field(p.TotalValue.Value), Currency: field(p.Currency.Value), UnitType: field(p.UnitType.Value), PaymentTerms: field(p.PaymentTerms.Value)}
	for _, term := range p.ServiceTerms {
		reviewed.ServiceTerms = append(reviewed.ServiceTerms, ci.ReviewedServiceTerm{ServiceDescription: field(term.ServiceDescription.Value), PricingModel: field(term.PricingModel.Value), UnitPrice: field(term.UnitPrice.Value), Currency: field(term.Currency.Value), Unit: field(term.Unit.Value), QuantitySource: field(term.QuantitySource.Value), QuantityValue: field(term.QuantityValue.Value), QuantityDriver: field(term.QuantityDriver.Value), BillingFrequency: field(term.BillingFrequency.Value), Evidence: term.ServiceDescription.Evidence})
	}
	return ci.ConfirmCommand{ClientID: tc.clientID, DocumentID: doc.ID, ExtractionAttemptID: doc.LatestAttempt.ID, ExpectedDocumentRevision: doc.Revision, CommandID: "confirm-" + doc.ID, Actor: ci.Actor{ID: "reviewer", Display: "Reviewer", AllClients: true}, Contract: reviewed}
}

func TestContractIngestionPersistenceDuplicateAndProvenance(t *testing.T) {
	tc := newContractTestContext(t)
	service, _, _ := contractIngestionFixture(t, tc)
	doc := ingestionUpload(t, tc, service)
	source, err := service.File(tc.ctx, tc.clientID, doc.ID, ci.Actor{AllClients: true})
	if err != nil || string(source.Bytes) != string(fixtures.PDF("romanian")) {
		t.Fatal("source changed")
	}
	duplicate, isDuplicate, err := service.Upload(tc.ctx, ci.Upload{ClientID: tc.clientID, Filename: "different-name.pdf", Bytes: fixtures.PDF("romanian"), Actor: ci.Actor{AllClients: true}})
	if err != nil || !isDuplicate || duplicate.ID != doc.ID {
		t.Fatal("duplicate source")
	}
	count, _ := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(doc.ID), outboxentry.EventTypeEQ(outbox.EventContractExtractionRequested)).Count(tc.ctx)
	if count != 1 {
		t.Fatal("duplicate AI cost")
	}
	command := ingestionReview(t, tc, service, doc)
	command.Contract.Reference = "USER-CORRECTED"
	id, changed, err := service.Confirm(tc.ctx, command)
	if err != nil || !changed {
		t.Fatalf("confirm=%v", err)
	}
	row, err := tc.store.Client.Contract.Get(tc.ctx, id)
	if err != nil || row.Reference != "USER-CORRECTED" || row.SourceDocumentID == nil || *row.SourceDocumentID != doc.ID {
		t.Fatal("authoritative contract/provenance")
	}
	again, changed, err := service.Confirm(tc.ctx, command)
	if err != nil || changed || again != id {
		t.Fatalf("replay=%v", err)
	}
	command.Contract.Reference = "CHANGED-REPLAY"
	if _, _, err = service.Confirm(tc.ctx, command); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("idempotency payload reuse")
	}
	review, err := service.Get(tc.ctx, tc.clientID, doc.ID, ci.Actor{AllClients: true})
	if err != nil || *review.LatestAttempt.Proposal.Reference.Value != "CTR-2026-01" || review.ConfirmedValues.Reference != "USER-CORRECTED" || review.ConfirmedByID == nil || *review.ConfirmedByID != "reviewer" {
		t.Fatal("proposal mutated or reviewer lost")
	}
	count, _ = tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(id), outboxentry.EventTypeEQ(outbox.EventContractAvailable)).Count(tc.ctx)
	if count != 1 {
		t.Fatalf("available events=%d", count)
	}
}

func TestIndefiniteMultipleServiceTermsPersistWithProvenance(t *testing.T) {
	tc := newContractTestContext(t)
	service, _, extractor := contractIngestionFixture(t, tc)
	extractor.proposal = fixtures.Proposal("service-indefinite")
	client, err := tc.store.Client.AccountingClient.Get(tc.ctx, tc.clientID)
	if err != nil {
		t.Fatal(err)
	}
	extractor.proposal.BuyerCUI.Value = &client.Cui
	extractor.proposal.BuyerCUI.Evidence.Snippet = client.Cui
	doc := ingestionUpload(t, tc, service)
	contractID, _, err := service.Confirm(tc.ctx, ingestionReview(t, tc, service, doc))
	if err != nil {
		t.Fatal(err)
	}
	row, err := tc.store.Client.Contract.Query().Where(contract.IDEQ(contractID)).WithServiceTerms().Only(tc.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if row.EffectiveTo != nil || row.PeriodType != contract.PeriodTypeINDEFINITE_TERM || row.HasLegacyTotalValue || len(row.Edges.ServiceTerms) != 2 {
		t.Fatalf("contract period=%s end=%v legacy=%t terms=%d", row.PeriodType, row.EffectiveTo, row.HasLegacyTotalValue, len(row.Edges.ServiceTerms))
	}
	terms := row.Edges.ServiceTerms
	if len(terms[0].SourceEvidence) == 0 || len(terms[1].SourceEvidence) == 0 {
		t.Fatal("service evidence missing")
	}
}
func TestContractIngestionExtractionFailureRetryAndStaleReview(t *testing.T) {
	tc := newContractTestContext(t)
	service, _, extractor := contractIngestionFixture(t, tc)
	doc := ingestionUpload(t, tc, service)
	extractor.failure = ci.ErrExtractionTransient
	if err := service.Extract(tc.ctx, doc.ID); !errors.Is(err, ci.ErrExtractionTransient) {
		t.Fatal("transient category")
	}
	extractor.failure = nil
	command := ingestionReview(t, tc, service, doc)
	if err := service.Retry(tc.ctx, tc.clientID, doc.ID, command.ExpectedDocumentRevision, ci.Actor{AllClients: true}); err != nil {
		t.Fatal(err)
	}
	if err := service.Extract(tc.ctx, doc.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Confirm(tc.ctx, command); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("stale review accepted")
	}
	attempts, _ := tc.store.Client.ContractExtractionAttempt.Query().Where(contractextractionattempt.DocumentIDEQ(doc.ID)).All(tc.ctx)
	if len(attempts) != 3 || attempts[0].ID == attempts[1].ID {
		t.Fatalf("history length=%d", len(attempts))
	}
}
func TestContractIngestionBuyerMismatchAndCrossClient(t *testing.T) {
	tc := newContractTestContext(t)
	service, _, extractor := contractIngestionFixture(t, tc)
	doc := ingestionUpload(t, tc, service)
	wrong := "RO99999999"
	extractor.proposal.BuyerCUI.Value = &wrong
	command := ingestionReview(t, tc, service, doc)
	command.Contract.BuyerCUI = wrong
	if _, _, err := service.Confirm(tc.ctx, command); !errors.Is(err, ci.ErrBuyerMismatch) {
		t.Fatal("buyer mismatch activated")
	}
	client, _ := tc.store.Client.AccountingClient.Get(tc.ctx, tc.clientID)
	command.Contract.BuyerCUI = client.Cui
	if _, _, err := service.Confirm(tc.ctx, command); err != nil {
		t.Fatalf("reviewed buyer correction did not unblock: %v", err)
	}
	actor := ci.Actor{AuthorizedClientIDs: []string{"another-client"}}
	if _, err := service.File(tc.ctx, tc.clientID, doc.ID, actor); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("cross-client file")
	}
	if _, err := service.Get(tc.ctx, tc.clientID, doc.ID, actor); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("cross-client proposal")
	}
}
func TestContractIngestionConcurrentConfirmation(t *testing.T) {
	tc := newContractTestContext(t)
	service, _, _ := contractIngestionFixture(t, tc)
	doc := ingestionUpload(t, tc, service)
	command := ingestionReview(t, tc, service, doc)
	var successes atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			copy := command
			copy.CommandID = fmt.Sprintf("reviewer-%d", i)
			_, changed, err := service.Confirm(tc.ctx, copy)
			if err == nil && changed {
				successes.Add(1)
			} else if err != nil && !errors.Is(err, apperrors.ErrConflict) {
				t.Errorf("confirm=%v", err)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("confirmations=%d", successes.Load())
	}
	count, _ := tc.store.Client.Contract.Query().Where(contract.ClientIDEQ(tc.clientID)).Count(tc.ctx)
	if count != 1 {
		t.Fatalf("contracts=%d", count)
	}
}
func TestContractIngestionMissingContractResumeE2E(t *testing.T) {
	for _, journey := range []string{"unique", "multiple", "irrelevant"} {
		t.Run(journey, func(t *testing.T) {
			tc := newContractTestContext(t)
			service, matching, _ := contractIngestionFixture(t, tc)
			invoiceID := tc.createInvoice(t, "ingestion-"+journey, "RO12345678", "RON")
			if _, _, err := matching.MatchInvoice(tc.ctx, contracts.MatchCommand{InvoiceID: invoiceID, ExpectedRevision: 1, CommandID: "missing:" + invoiceID}); err != nil {
				t.Fatal(err)
			}
			before, err := tc.store.GetInvoice(tc.ctx, invoiceID)
			if err != nil || before.ActiveTask == nil {
				t.Fatal("missing-contract setup")
			}
			if _, _, err = validationtasks.NewService(tc.store, func() time.Time { return tc.now }).RequestMissingContract(tc.ctx, validationtasks.RequestMissingContractCommand{InvoiceID: invoiceID, TaskID: before.ActiveTask.ID, ExpectedRevision: before.ActiveTask.Revision, CommandID: "request:" + invoiceID}); err != nil {
				t.Fatal(err)
			}
			doc := ingestionUpload(t, tc, service)
			command := ingestionReview(t, tc, service, doc)
			if journey == "multiple" {
				tc.createContract(t, "other", "RO12345678", "RON")
			}
			if journey == "irrelevant" {
				command.Contract.SupplierCUI = "RO87654321"
			}
			id, _, err := service.Confirm(tc.ctx, command)
			if err != nil {
				t.Fatal(err)
			}
			event, err := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(id), outboxentry.EventTypeEQ(outbox.EventContractAvailable)).Only(tc.ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = matching.ProcessContractAvailable(tc.ctx, id, event.IdempotencyKey, "ingestion-e2e"); err != nil {
				t.Fatal(err)
			}
			after, err := tc.store.Client.Invoice.Get(tc.ctx, invoiceID)
			if err != nil {
				t.Fatal(err)
			}
			want := invoice.PipelineStatusDEDUPE_CHECKED
			if journey == "multiple" {
				want = invoice.PipelineStatusAWAITING_MATCH_CONFIRM
			}
			if journey == "irrelevant" {
				want = invoice.PipelineStatusAWAITING_CONTRACT
			}
			if after.PipelineStatus != want {
				t.Fatalf("pipeline=%s want=%s", after.PipelineStatus, want)
			}
			task, err := tc.store.Client.ValidationTask.Get(tc.ctx, before.ActiveTask.ID)
			if err != nil {
				t.Fatal(err)
			}
			wantTask := validationtask.StatusRESOLVED
			if journey == "irrelevant" {
				wantTask = validationtask.StatusWAITING
			}
			if task.Status != wantTask {
				t.Fatalf("missing task=%s want=%s", task.Status, wantTask)
			}
		})
	}
}

func TestOneConfirmedContractReevaluatesTwoWaitingInvoices(t *testing.T) {
	tc := newContractTestContext(t)
	service, matching, _ := contractIngestionFixture(t, tc)
	invoiceIDs := []string{tc.createInvoice(t, "shared-a", "RO12345678", "RON"), tc.createInvoice(t, "shared-b", "RO12345678", "RON")}
	for _, invoiceID := range invoiceIDs {
		if _, _, err := matching.MatchInvoice(tc.ctx, contracts.MatchCommand{InvoiceID: invoiceID, ExpectedRevision: 1, CommandID: "missing:" + invoiceID}); err != nil {
			t.Fatal(err)
		}
		item, err := tc.store.GetInvoice(tc.ctx, invoiceID)
		if err != nil || item.ActiveTask == nil {
			t.Fatal("missing-contract setup")
		}
		if _, _, err = validationtasks.NewService(tc.store, func() time.Time { return tc.now }).RequestMissingContract(tc.ctx, validationtasks.RequestMissingContractCommand{InvoiceID: invoiceID, TaskID: item.ActiveTask.ID, ExpectedRevision: item.ActiveTask.Revision, CommandID: "request:" + invoiceID}); err != nil {
			t.Fatal(err)
		}
	}
	doc := ingestionUpload(t, tc, service)
	contractID, _, err := service.Confirm(tc.ctx, ingestionReview(t, tc, service, doc))
	if err != nil {
		t.Fatal(err)
	}
	event, err := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(contractID), outboxentry.EventTypeEQ(outbox.EventContractAvailable)).Only(tc.ctx)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := matching.ProcessContractAvailable(tc.ctx, contractID, event.IdempotencyKey, "shared-contract")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Evaluated != 2 {
		t.Fatalf("evaluated=%d summary=%+v", summary.Evaluated, summary)
	}
	for _, invoiceID := range invoiceIDs {
		row, err := tc.store.Client.Invoice.Get(tc.ctx, invoiceID)
		if err != nil || row.PipelineStatus == invoice.PipelineStatusAWAITING_CONTRACT {
			t.Fatalf("invoice %s was not reevaluated: status=%s err=%v", invoiceID, row.PipelineStatus, err)
		}
	}
}

func TestDiscardUnconfirmedDocumentLeavesInvoiceWaiting(t *testing.T) {
	tc := newContractTestContext(t)
	service, matching, _ := contractIngestionFixture(t, tc)
	invoiceID := tc.createInvoice(t, "discard-waiting", "RO12345678", "RON")
	if _, _, err := matching.MatchInvoice(tc.ctx, contracts.MatchCommand{InvoiceID: invoiceID, ExpectedRevision: 1, CommandID: "missing:" + invoiceID}); err != nil {
		t.Fatal(err)
	}
	before, err := tc.store.GetInvoice(tc.ctx, invoiceID)
	if err != nil || before.ActiveTask == nil {
		t.Fatal("missing-contract setup")
	}
	if _, _, err = validationtasks.NewService(tc.store, func() time.Time { return tc.now }).RequestMissingContract(tc.ctx, validationtasks.RequestMissingContractCommand{InvoiceID: invoiceID, TaskID: before.ActiveTask.ID, ExpectedRevision: before.ActiveTask.Revision, CommandID: "request:" + invoiceID}); err != nil {
		t.Fatal(err)
	}
	doc := ingestionUpload(t, tc, service)
	if err = service.Discard(tc.ctx, tc.clientID, doc.ID, doc.Revision, ci.Actor{AllClients: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Get(tc.ctx, tc.clientID, doc.ID, ci.Actor{AllClients: true}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("discarded document remained active: %v", err)
	}
	after, err := tc.store.GetInvoice(tc.ctx, invoiceID)
	if err != nil || after.PipelineStatus != invoicing.StatusAwaitingContract || after.ActiveTask == nil || after.ActiveTask.Status != validationtasks.StatusWaiting {
		t.Fatalf("waiting workflow changed: %+v err=%v", after, err)
	}
	if err = service.Discard(tc.ctx, "another-client", doc.ID, doc.Revision, ci.Actor{AuthorizedClientIDs: []string{"another-client"}}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("cross-client discard=%v", err)
	}
}
