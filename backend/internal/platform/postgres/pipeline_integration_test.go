//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"diana-contabilitate/backend/ent/activityevent"
	"diana-contabilitate/backend/ent/invoice"
	"diana-contabilitate/backend/ent/invoiceline"
	"diana-contabilitate/backend/ent/lineclassification"
	"diana-contabilitate/backend/ent/outboxentry"
	"diana-contabilitate/backend/ent/sagaexportattempt"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/audit"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/money"
)

var integrationSequence atomic.Uint64

func pipelineTestStore(t *testing.T) (*Store, context.Context, string, time.Time) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	store, err := Open(url)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	sequence := integrationSequence.Add(1)
	clientID := fmt.Sprintf("pipeline-client-%d", sequence)
	now := time.Date(2026, time.September, 11, 12, int(sequence), 0, 0, time.UTC)
	_, err = store.Client.AccountingClient.Create().SetID(clientID).SetName("Client pipeline test").SetCui(fmt.Sprintf("RO-PIPELINE-%06d", sequence)).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ids, _ := store.Client.Invoice.Query().Where(invoice.ClientIDEQ(clientID)).IDs(ctx)
		if len(ids) > 0 {
			_, _ = store.Client.SagaExportAttempt.Delete().Where(sagaexportattempt.InvoiceIDIn(ids...)).Exec(ctx)
			_, _ = store.Client.LineClassification.Delete().Where(lineclassification.InvoiceIDIn(ids...)).Exec(ctx)
			_, _ = store.Client.InvoiceLine.Delete().Where(invoiceline.InvoiceIDIn(ids...)).Exec(ctx)
			_, _ = store.Client.ActivityEvent.Delete().Where(activityevent.InvoiceIDIn(ids...)).Exec(ctx)
			_, _ = store.Client.OutboxEntry.Delete().Where(outboxentry.AggregateIDIn(ids...)).Exec(ctx)
			_, _ = store.Client.Invoice.Delete().Where(invoice.IDIn(ids...)).Exec(ctx)
		}
		_ = store.Client.AccountingClient.DeleteOneID(clientID).Exec(ctx)
		_ = store.Close()
	})
	return store, ctx, clientID, now
}

func ingestionFixture(id, clientID string, now time.Time) invoicing.IngestionInput {
	info := "Informație structurată de test"
	cui := "RO-PIPELINE-SUPPLIER"
	return invoicing.IngestionInput{
		ID: id, ClientID: clientID, Source: "TEST_SOURCE", ExternalDeliveryID: "delivery-" + id,
		SupplierName: "Furnizor pipeline test", SupplierCUI: &cui, DocumentNumber: "DOC-" + id, IssueDate: now,
		Total: money.Money{Amount: money.MustParse("123.4567"), Currency: "RON"}, SPVReference: "SOURCE-" + id,
		Lines: []invoicing.Line{{Position: 1, Description: "Serviciu structurat", Unit: "BUC", VATRate: money.MustParse("19.0000"), VATValue: money.MustParse("19.7531"), Quantity: money.MustParse("1.2500"), UnitPrice: money.MustParse("83.1729"), NetValue: money.MustParse("103.9661"), TotalValue: money.MustParse("123.7192"), AdditionalInfo: &info}},
	}
}

func TestIngestionIsIdempotentUnderConcurrencyAndPreservesExactLines(t *testing.T) {
	store, ctx, clientID, now := pipelineTestStore(t)
	service := invoicing.NewPipelineService(store, invoicing.NewFakeSagaExporter(), func() time.Time { return now })
	input := ingestionFixture("concurrent-ingestion", clientID, now)
	const workers = 8
	var created atomic.Int32
	errorsSeen := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			_, wasCreated, err := service.Ingest(ctx, input)
			if wasCreated {
				created.Add(1)
			}
			errorsSeen <- err
		}()
	}
	group.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatal(err)
		}
	}
	if created.Load() != 1 {
		t.Fatalf("created %d invoices, want 1", created.Load())
	}
	item, err := store.GetInvoice(ctx, input.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(item.Lines) != 1 || item.Lines[0].Quantity.String() != "1.2500" || item.Lines[0].UnitPrice.String() != "83.1729" || item.Lines[0].VATValue.String() != "19.7531" || item.Lines[0].NetValue.String() != "103.9661" || item.Lines[0].TotalValue.String() != "123.7192" {
		t.Fatalf("line decimals changed: %+v", item.Lines)
	}
	lineCount, _ := store.Client.InvoiceLine.Query().Where(invoiceline.InvoiceIDEQ(input.ID)).Count(ctx)
	auditCount, _ := store.Client.ActivityEvent.Query().Where(activityevent.InvoiceIDEQ(input.ID)).Count(ctx)
	outboxCount, _ := store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(input.ID)).Count(ctx)
	if lineCount != 1 || auditCount != 1 || outboxCount != 1 {
		t.Fatalf("lines=%d audits=%d outbox=%d", lineCount, auditCount, outboxCount)
	}
}

func TestIngestionSeparatesTechnicalRetryFromTerminalBusinessDuplicate(t *testing.T) {
	store, ctx, clientID, now := pipelineTestStore(t)
	service := invoicing.NewPipelineService(store, invoicing.NewFakeSagaExporter(), func() time.Time { return now })
	cui := "  ro-123  456 "
	canonicalInput := ingestionFixture("canonical-business-document", clientID, now)
	canonicalInput.SupplierCUI = &cui
	canonicalInput.DocumentNumber = " inv-  007 "
	canonicalInput.Total.Amount = money.MustParse("100")
	canonical, created, err := service.Ingest(ctx, canonicalInput)
	if err != nil || !created {
		t.Fatalf("canonical ingestion created=%v err=%v", created, err)
	}

	retryInput := canonicalInput
	retryInput.ID = "must-not-be-created"
	retryInput.ExternalDeliveryID = "different-delivery-metadata"
	retryInput.DocumentNumber = "DIFFERENT-CONTENT-ON-RETRY"
	retry, retryCreated, err := service.Ingest(ctx, retryInput)
	if err != nil || retryCreated || retry.ID != canonical.ID {
		t.Fatalf("technical retry got=%+v created=%v err=%v", retry, retryCreated, err)
	}

	duplicateCUI := "RO-123 456"
	duplicateInput := ingestionFixture("business-duplicate", clientID, now.Add(8*time.Hour))
	duplicateInput.SupplierCUI = &duplicateCUI
	duplicateInput.DocumentNumber = "INV- 007"
	duplicateInput.Total.Amount = money.MustParse("101.0000")
	duplicateInput.Total.Currency = "EUR"
	duplicate, duplicateCreated, err := service.Ingest(ctx, duplicateInput)
	if err != nil || !duplicateCreated {
		t.Fatalf("duplicate ingestion created=%v err=%v", duplicateCreated, err)
	}
	if duplicate.PipelineStatus != invoicing.StatusDuplicate || !duplicate.PipelineStatus.Terminal() || duplicate.SagaStatus != invoicing.SagaNotReady {
		t.Fatalf("duplicate is not terminal: %+v", duplicate)
	}
	if duplicate.DuplicateOfInvoiceID == nil || *duplicate.DuplicateOfInvoiceID != canonical.ID {
		t.Fatalf("duplicate target=%v, want %s", duplicate.DuplicateOfInvoiceID, canonical.ID)
	}
	if duplicate.DuplicateAmountMatches == nil || *duplicate.DuplicateAmountMatches || duplicate.DuplicateCurrencyMatches == nil || *duplicate.DuplicateCurrencyMatches {
		t.Fatalf("supplementary checks amount=%v currency=%v", duplicate.DuplicateAmountMatches, duplicate.DuplicateCurrencyMatches)
	}
	pending, _ := store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(duplicate.ID), outboxentry.StatusEQ(outboxentry.StatusPENDING)).Count(ctx)
	if pending != 0 {
		t.Fatalf("terminal duplicate has %d continuations", pending)
	}
	audits, _ := store.Client.ActivityEvent.Query().Where(activityevent.InvoiceIDEQ(duplicate.ID), activityevent.EventTypeEQ("INVOICE_DUPLICATE_DETECTED")).Count(ctx)
	if audits != 1 {
		t.Fatalf("duplicate audit count=%d", audits)
	}

	matchingInput := ingestionFixture("business-duplicate-matching-signals", clientID, now.Add(10*time.Hour))
	matchingInput.SupplierCUI = &duplicateCUI
	matchingInput.DocumentNumber = "INV- 007"
	matchingInput.Total.Amount = money.MustParse("100.0000")
	matchingInput.Total.Currency = "RON"
	matching, _, err := service.Ingest(ctx, matchingInput)
	if err != nil {
		t.Fatal(err)
	}
	if matching.DuplicateAmountMatches == nil || !*matching.DuplicateAmountMatches || matching.DuplicateCurrencyMatches == nil || !*matching.DuplicateCurrencyMatches {
		t.Fatalf("matching supplementary checks amount=%v currency=%v", matching.DuplicateAmountMatches, matching.DuplicateCurrencyMatches)
	}
}

func TestConcurrentBusinessIdentityCreatesOneCanonicalAndTerminalDuplicates(t *testing.T) {
	store, ctx, clientID, now := pipelineTestStore(t)
	service := invoicing.NewPipelineService(store, invoicing.NewFakeSagaExporter(), func() time.Time { return now })
	cui := "RO-CONCURRENT-IDENTITY"
	const deliveries = 8
	invoiceIDs := make([]string, 0, deliveries)
	for index := range deliveries {
		invoiceIDs = append(invoiceIDs, fmt.Sprintf("business-race-%d", index))
	}
	results := make(chan *invoicing.Invoice, deliveries)
	errorsSeen := make(chan error, deliveries)
	var group sync.WaitGroup
	for index := range deliveries {
		index := index
		group.Add(1)
		go func() {
			defer group.Done()
			input := ingestionFixture(fmt.Sprintf("business-race-%d", index), clientID, now.Add(time.Duration(index)*time.Minute))
			input.SupplierCUI = &cui
			input.DocumentNumber = "RACE-001"
			item, _, err := service.Ingest(ctx, input)
			results <- item
			errorsSeen <- err
		}()
	}
	group.Wait()
	close(results)
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatal(err)
		}
	}
	canonicalCount := 0
	duplicateCount := 0
	for item := range results {
		if item == nil {
			t.Fatal("nil ingestion result")
		}
		switch item.PipelineStatus {
		case invoicing.StatusDownloaded:
			canonicalCount++
		case invoicing.StatusDuplicate:
			duplicateCount++
		default:
			t.Fatalf("unexpected status %s", item.PipelineStatus)
		}
	}
	if canonicalCount != 1 || duplicateCount != deliveries-1 {
		t.Fatalf("canonical=%d duplicates=%d", canonicalCount, duplicateCount)
	}
	pending, _ := store.Client.OutboxEntry.Query().Where(outboxentry.StatusEQ(outboxentry.StatusPENDING), outboxentry.AggregateIDIn(invoiceIDs...)).Count(ctx)
	if pending != 1 {
		t.Fatalf("pending continuations=%d, want 1", pending)
	}
}

func TestDatabaseRejectsDuplicateStatusWithoutCanonicalEvidence(t *testing.T) {
	store, ctx, clientID, now := pipelineTestStore(t)
	_, err := store.Client.Invoice.Create().SetID("orphan-duplicate").SetClientID(clientID).
		SetSupplierName("Furnizor orphan").SetSupplierCui("RO-ORPHAN").SetNormalizedSupplierCui("RO-ORPHAN").
		SetDocumentNumber("ORPHAN-001").SetNormalizedDocumentNumber("ORPHAN-001").
		SetIssueDate(now).SetIssueDay(invoicing.InvoiceIssueDay(now)).SetTotalAmount("10.0000").SetCurrency("RON").
		SetSpvReference("SPV-ORPHAN").SetIngestionSource("FIXTURE").SetExternalDeliveryID("orphan").
		SetPipelineStatus(invoice.PipelineStatusDUPLICATE).SetSagaStatus(invoice.SagaStatusNOT_READY).
		SetRevision(1).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err == nil {
		t.Fatal("expected duplicate evidence constraint violation")
	}
}

func TestConcurrentTransitionHasOneWinnerAndIdempotentReplay(t *testing.T) {
	store, ctx, clientID, now := pipelineTestStore(t)
	service := invoicing.NewPipelineService(store, invoicing.NewFakeSagaExporter(), func() time.Time { return now })
	input := ingestionFixture("concurrent-transition", clientID, now)
	if _, _, err := service.Ingest(ctx, input); err != nil {
		t.Fatal(err)
	}
	commands := []invoicing.TransitionCommand{
		{InvoiceID: input.ID, From: invoicing.StatusDownloaded, To: invoicing.StatusArchived, ExpectedRevision: 1, Trigger: invoicing.TriggerArchiveCompleted, CommandID: "winner-a", Actor: audit.ActorSystem},
		{InvoiceID: input.ID, From: invoicing.StatusDownloaded, To: invoicing.StatusArchived, ExpectedRevision: 1, Trigger: invoicing.TriggerArchiveCompleted, CommandID: "winner-b", Actor: audit.ActorSystem},
	}
	results := make(chan error, 2)
	var winner atomic.Int32
	var group sync.WaitGroup
	for _, command := range commands {
		command := command
		group.Add(1)
		go func() {
			defer group.Done()
			_, changed, err := service.TransitionInternally(ctx, command)
			if changed {
				winner.Add(1)
			}
			results <- err
		}()
	}
	group.Wait()
	close(results)
	conflicts := 0
	for err := range results {
		if errors.Is(err, apperrors.ErrConflict) {
			conflicts++
		} else if err != nil {
			t.Fatal(err)
		}
	}
	if winner.Load() != 1 || conflicts != 1 {
		t.Fatalf("winners=%d conflicts=%d", winner.Load(), conflicts)
	}
	item, _ := store.GetInvoice(ctx, input.ID)
	winning := commands[0]
	if exists, _ := store.auditExists(ctx, "transition:winner-a"); !exists {
		winning = commands[1]
	}
	if _, changed, err := service.TransitionInternally(ctx, winning); err != nil || changed {
		t.Fatalf("idempotent replay changed=%v err=%v", changed, err)
	}
	if item.PipelineStatus != invoicing.StatusArchived || item.Revision != 2 {
		t.Fatalf("unexpected invoice: %+v", item)
	}
}

func TestTransitionRollbackKeepsInvoiceAuditAndOutboxAtomic(t *testing.T) {
	store, ctx, clientID, now := pipelineTestStore(t)
	service := invoicing.NewPipelineService(store, invoicing.NewFakeSagaExporter(), func() time.Time { return now })
	input := ingestionFixture("rollback-transition", clientID, now)
	if _, _, err := service.Ingest(ctx, input); err != nil {
		t.Fatal(err)
	}
	_, _, err := service.TransitionInternally(ctx, invoicing.TransitionCommand{InvoiceID: input.ID, From: invoicing.StatusDownloaded, To: invoicing.StatusArchived, ExpectedRevision: 1, Trigger: invoicing.TriggerArchiveCompleted, CommandID: "rollback", Actor: audit.ActorKind("INVALID")})
	if err == nil {
		t.Fatal("expected audit validation failure")
	}
	item, _ := store.GetInvoice(ctx, input.ID)
	transitionAudits, _ := store.Client.ActivityEvent.Query().Where(activityevent.InvoiceIDEQ(input.ID), activityevent.EventTypeEQ("INVOICE_PIPELINE_TRANSITION")).Count(ctx)
	outboxCount, _ := store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(input.ID)).Count(ctx)
	if item.PipelineStatus != invoicing.StatusDownloaded || item.Revision != 1 || transitionAudits != 0 || outboxCount != 1 {
		t.Fatalf("rollback failed: status=%s revision=%d audits=%d outbox=%d", item.PipelineStatus, item.Revision, transitionAudits, outboxCount)
	}
}

func TestAutomaticProgressionStopsAtMatching(t *testing.T) {
	store, ctx, clientID, now := pipelineTestStore(t)
	service := invoicing.NewPipelineService(store, invoicing.NewFakeSagaExporter(), func() time.Time { return now })
	input := ingestionFixture("automatic-matching", clientID, now)
	if _, _, err := service.Ingest(ctx, input); err != nil {
		t.Fatal(err)
	}
	if err := service.Drain(ctx, 20); err != nil {
		t.Fatal(err)
	}
	item, _ := store.GetInvoice(ctx, input.ID)
	if item.PipelineStatus != invoicing.StatusMatching || item.Revision != 3 {
		t.Fatalf("got %s revision %d", item.PipelineStatus, item.Revision)
	}
	if pending, _ := store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(input.ID), outboxentry.StatusEQ(outboxentry.StatusPENDING)).Count(ctx); pending != 0 {
		t.Fatalf("pending outbox=%d", pending)
	}
}

func TestFakeSagaSuccessFailureAndDuplicateTerminalBehavior(t *testing.T) {
	store, ctx, clientID, now := pipelineTestStore(t)
	exporter := invoicing.NewFakeSagaExporter("saga-failure")
	service := invoicing.NewPipelineService(store, exporter, func() time.Time { return now })
	seedPipelineFixture(t, store, ctx, "saga-success", clientID, invoicing.StatusReadyForSAGA, invoicing.SagaReady, now, true)
	seedPipelineFixture(t, store, ctx, "saga-failure", clientID, invoicing.StatusReadyForSAGA, invoicing.SagaReady, now, true)
	duplicateCUI := "RO-TERMINAL-DUPLICATE"
	canonicalInput := ingestionFixture("duplicate-canonical", clientID, now)
	canonicalInput.SupplierCUI = &duplicateCUI
	canonicalInput.DocumentNumber = "DUPLICATE-TERMINAL-001"
	if _, _, err := service.Ingest(ctx, canonicalInput); err != nil {
		t.Fatal(err)
	}
	duplicateInput := ingestionFixture("duplicate-terminal", clientID, now)
	duplicateInput.SupplierCUI = &duplicateCUI
	duplicateInput.DocumentNumber = "DUPLICATE-TERMINAL-001"
	duplicate, _, err := service.Ingest(ctx, duplicateInput)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Drain(ctx, 20); err != nil {
		t.Fatal(err)
	}
	success, _ := store.GetInvoice(ctx, "saga-success")
	failure, _ := store.GetInvoice(ctx, "saga-failure")
	if success.PipelineStatus != invoicing.StatusExported || success.SagaStatus != invoicing.SagaExported {
		t.Fatalf("success fixture: %+v", success)
	}
	if failure.PipelineStatus != invoicing.StatusExporting || failure.SagaStatus != invoicing.SagaFailed {
		t.Fatalf("failure fixture: %+v", failure)
	}
	if exporter.OperationCount() != 2 {
		t.Fatalf("fake export operations=%d", exporter.OperationCount())
	}
	if !duplicate.PipelineStatus.Terminal() || duplicate.SagaStatus != invoicing.SagaNotReady {
		t.Fatalf("duplicate fixture: %+v", duplicate)
	}
	if _, _, err := service.TransitionInternally(ctx, invoicing.TransitionCommand{InvoiceID: duplicate.ID, From: invoicing.StatusDuplicate, To: invoicing.StatusExporting, ExpectedRevision: duplicate.Revision, Trigger: invoicing.TriggerSagaHandoff, CommandID: "illegal-export", Actor: audit.ActorSystem}); !errors.Is(err, apperrors.ErrValidation) {
		t.Fatalf("terminal transition error=%v", err)
	}
}

func seedPipelineFixture(t *testing.T, store *Store, ctx context.Context, id, clientID string, status invoicing.PipelineStatus, saga invoicing.SagaStatus, now time.Time, continuation bool) {
	t.Helper()
	_, err := store.Client.Invoice.Create().SetID(id).SetClientID(clientID).SetSupplierName("Furnizor fixture").
		SetDocumentNumber("DOC-" + id).SetNormalizedDocumentNumber("DOC-" + id).
		SetIssueDate(now).SetIssueDay(invoicing.InvoiceIssueDay(now)).SetTotalAmount("10.0000").SetCurrency("RON").
		SetSpvReference("REF-" + id).SetIngestionSource("FIXTURE").SetExternalDeliveryID(id).
		SetPipelineStatus(invoice.PipelineStatus(status)).SetSagaStatus(invoice.SagaStatus(saga)).
		SetRevision(1).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if continuation {
		payload, _ := json.Marshal(map[string]string{"invoice_id": id})
		_, err = store.Client.OutboxEntry.Create().SetID("out-" + id).SetEventType("INVOICE_CONTINUE").SetAggregateType("INVOICE").SetAggregateID(id).SetPayload(payload).SetIdempotencyKey("fixture:" + id).SetStatus(outboxentry.StatusPENDING).SetCreatedAt(now).SetAvailableAt(now).Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
	}
}
