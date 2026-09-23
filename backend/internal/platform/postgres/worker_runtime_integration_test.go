//go:build integration && redis_integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"diana-contabilitate/backend/ent"
	"diana-contabilitate/backend/ent/activityevent"
	entcontract "diana-contabilitate/backend/ent/contract"
	"diana-contabilitate/backend/ent/contractmatchcandidate"
	"diana-contabilitate/backend/ent/contractmatchrun"
	"diana-contabilitate/backend/ent/invoice"
	"diana-contabilitate/backend/ent/invoicecontractassociation"
	"diana-contabilitate/backend/ent/outboxentry"
	"diana-contabilitate/backend/ent/validationtask"
	"diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/outbox"
	"diana-contabilitate/backend/internal/platform/observability"
	"diana-contabilitate/backend/internal/workerruntime"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

type countingContinuationProcessor struct {
	inner             *invoicing.Service
	calls             atomic.Int32
	transientFailures atomic.Int32
}

// scopedRuntimeOutbox keeps this end-to-end integration fixture from claiming
// unrelated pending demo rows in a shared development test database. The
// production SKIP LOCKED claim is exercised independently by outbox tests.
type scopedRuntimeOutbox struct {
	*Store
	aggregateID string
}

func (s scopedRuntimeOutbox) Claim(ctx context.Context, owner string, limit int, now time.Time, _ time.Duration) ([]outbox.Entry, error) {
	rows, err := s.Client.OutboxEntry.Query().Where(
		outboxentry.AggregateIDEQ(s.aggregateID),
		outboxentry.StatusEQ(outboxentry.StatusPENDING),
		outboxentry.AvailableAtLTE(now),
	).Order(ent.Asc(outboxentry.FieldCreatedAt)).Limit(limit).All(ctx)
	if err != nil {
		return nil, err
	}
	entries := make([]outbox.Entry, 0, len(rows))
	for _, row := range rows {
		claimed, updateErr := s.Client.OutboxEntry.UpdateOneID(row.ID).
			Where(outboxentry.StatusEQ(outboxentry.StatusPENDING)).
			SetStatus(outboxentry.StatusCLAIMED).
			SetClaimOwner(owner).
			SetClaimedAt(now).
			AddAttempts(1).
			ClearLastError().
			Save(ctx)
		if ent.IsNotFound(updateErr) {
			continue
		}
		if updateErr != nil {
			return nil, updateErr
		}
		correlationID := claimed.ID
		if claimed.CorrelationID != nil {
			correlationID = *claimed.CorrelationID
		}
		entries = append(entries, outbox.Entry{ID: claimed.ID, EventType: claimed.EventType, AggregateID: claimed.AggregateID, IdempotencyKey: claimed.IdempotencyKey, CorrelationID: correlationID, Attempts: claimed.Attempts})
	}
	return entries, nil
}

func (p *countingContinuationProcessor) ProcessContinuation(ctx context.Context, entry invoicing.OutboxEntry) (invoicing.ContinuationOutcome, error) {
	p.calls.Add(1)
	if p.transientFailures.Add(-1) >= 0 {
		return "", errors.New("simulated transient database boundary failure")
	}
	return p.inner.ProcessContinuation(ctx, entry)
}

func TestOutboxAsynqWorkerCompletesAndDuplicateJobIsIdempotent(t *testing.T) {
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("TEST_REDIS_URL is not set")
	}
	redisOptions, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	if redisOptions.DB == 0 {
		t.Fatal("TEST_REDIS_URL must use an isolated non-zero database")
	}
	redisClient := redis.NewClient(redisOptions)
	if err = redisClient.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = redisClient.FlushDB(context.Background()).Err()
		_ = redisClient.Close()
	})
	asynqOptions, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		t.Fatal(err)
	}

	store, ctx, clientID, _ := pipelineTestStore(t)
	base := time.Date(2002, 1, 1, 0, 0, 0, 0, time.UTC)
	invoiceID := "m7-runtime-" + clientID
	seedPipelineFixture(t, store, ctx, invoiceID, clientID, invoicing.StatusReadyForSAGA, invoicing.SagaReady, base, true)
	pipeline := invoicing.NewPipelineService(store, invoicing.NewFakeSagaExporter(), func() time.Time { return base })
	processor := &countingContinuationProcessor{inner: pipeline}
	processor.transientFailures.Store(1)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	metrics := observability.NewMetrics()

	server := asynq.NewServer(asynqOptions, asynq.Config{Concurrency: 2, Queues: map[string]int{"workflow-integration": 1}, DelayedTaskCheckInterval: 20 * time.Millisecond, RetryDelayFunc: func(int, error, *asynq.Task) time.Duration { return 10 * time.Millisecond }})
	mux := asynq.NewServeMux()
	mux.Handle(workerruntime.ContinueInvoiceTask, workerruntime.NewHandler(processor, logger, metrics))
	if err = server.Start(mux); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Shutdown)
	client := asynq.NewClient(asynqOptions)
	t.Cleanup(func() { _ = client.Close() })
	publisher := workerruntime.NewAsynqPublisher(client, "workflow-integration", 3, time.Minute)
	dispatcher := workerruntime.NewDispatcher(scopedRuntimeOutbox{Store: store, aggregateID: invoiceID}, publisher, workerruntime.DispatcherConfig{Owner: "integration-worker", BatchSize: 10, MaxAttempts: 3, PollInterval: 10 * time.Millisecond, ClaimTTL: time.Second, RetryMin: 10 * time.Millisecond, RetryMax: time.Second}, logger, metrics)

	eventuallyPostgres(t, func() bool {
		_, dispatchErr := dispatcher.DispatchBatch(ctx)
		if dispatchErr != nil {
			t.Fatalf("dispatch: %v", dispatchErr)
		}
		item, getErr := store.GetInvoice(ctx, invoiceID)
		return getErr == nil && item.PipelineStatus == invoicing.StatusExported
	})
	rows, err := store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(invoiceID)).All(ctx)
	if err != nil || len(rows) != 2 {
		t.Fatalf("outbox rows=%d err=%v", len(rows), err)
	}
	for _, row := range rows {
		if row.Status != outboxentry.StatusDISPATCHED {
			t.Fatalf("outbox %s status=%s", row.ID, row.Status)
		}
	}

	first := rows[0]
	beforeCalls := processor.calls.Load()
	if _, err = publisher.Publish(ctx, workerruntime.Job{OutboxID: first.ID, InvoiceID: invoiceID, EventType: first.EventType, IdempotencyKey: first.IdempotencyKey, CorrelationID: "duplicate-test"}); err != nil {
		t.Fatal(err)
	}
	eventuallyPostgres(t, func() bool { return processor.calls.Load() > beforeCalls })
	transitionCount, _ := store.Client.ActivityEvent.Query().Where(activityevent.InvoiceIDEQ(invoiceID), activityevent.EventTypeEQ("INVOICE_PIPELINE_TRANSITION")).Count(ctx)
	correlatedCount, _ := store.Client.ActivityEvent.Query().Where(activityevent.InvoiceIDEQ(invoiceID), activityevent.EventTypeEQ("INVOICE_PIPELINE_TRANSITION"), activityevent.CorrelationIDEQ("out-"+invoiceID)).Count(ctx)
	taskCount, _ := store.Client.ValidationTask.Query().Where(validationtask.InvoiceIDEQ(invoiceID)).Count(ctx)
	item, _ := store.Client.Invoice.Query().Where(invoice.IDEQ(invoiceID)).Only(ctx)
	if processor.calls.Load() < 3 || transitionCount != 2 || correlatedCount != 2 || item.PipelineStatus != invoice.PipelineStatusEXPORTED || taskCount != 0 {
		t.Fatalf("calls=%d transitions=%d correlated=%d status=%s task_count=%d", processor.calls.Load(), transitionCount, correlatedCount, item.PipelineStatus, taskCount)
	}
}

func TestOutboxAsynqWorkerCompletesFullAutomaticPipeline(t *testing.T) {
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("TEST_REDIS_URL is not set")
	}
	redisOptions, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	if redisOptions.DB == 0 {
		t.Fatal("TEST_REDIS_URL must use an isolated non-zero database")
	}
	redisClient := redis.NewClient(redisOptions)
	if err = redisClient.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = redisClient.FlushDB(context.Background()).Err()
		_ = redisClient.Close()
	})
	asynqOptions, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		t.Fatal(err)
	}

	tc := newModule5TestContext(t)
	tc.baselineRules(t)
	invoiceID := fmt.Sprintf("m7-full-runtime-%d", classificationTestSequence.Load())
	contractID := fmt.Sprintf("m7-full-contract-%d", classificationTestSequence.Load())
	if _, err = tc.store.Client.Contract.Create().SetID(contractID).SetClientID(tc.clientID).
		SetSupplierName("Furnizor pipeline test").SetSupplierCui("RO-PIPELINE-SUPPLIER").SetNormalizedSupplierCui("RO-PIPELINE-SUPPLIER").
		SetReference("M7-FULL-AUTO").SetEffectiveFrom(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)).SetEffectiveTo(time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)).
		SetTotalValue("123.4567").SetCurrency("RON").SetUnitType("BUC").SetPaymentTerms("30 zile").SetRevision(1).SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = tc.store.Client.InvoiceContractAssociation.Delete().Where(invoicecontractassociation.InvoiceIDEQ(invoiceID)).Exec(tc.ctx)
		runs, _ := tc.store.Client.ContractMatchRun.Query().Where(contractmatchrun.InvoiceIDEQ(invoiceID)).IDs(tc.ctx)
		if len(runs) > 0 {
			_, _ = tc.store.Client.ContractMatchCandidate.Delete().Where(contractmatchcandidate.MatchRunIDIn(runs...)).Exec(tc.ctx)
		}
		_, _ = tc.store.Client.ContractMatchRun.Delete().Where(contractmatchrun.InvoiceIDEQ(invoiceID)).Exec(tc.ctx)
		_, _ = tc.store.Client.Contract.Delete().Where(entcontract.IDEQ(contractID)).Exec(tc.ctx)
	})

	runtimeNow := time.Now().UTC().Add(-time.Minute)
	classificationStore := tc.classificationStore()
	pipeline := invoicing.NewPipelineService(classificationStore, invoicing.NewFakeSagaExporter(), func() time.Time { return runtimeNow })
	pipeline.SetContractMatchingProcessor(contracts.NewService(tc.store, contracts.BaselinePolicy{}, func() time.Time { return runtimeNow }))
	pipeline.SetClassificationProcessor(classification.NewService(classificationStore, classification.BaselinePolicy{}, func() time.Time { return runtimeNow }))
	pipeline.SetCommercialValidationProcessor(conformingCommercialProcessor{store: classificationStore, clock: func() time.Time { return runtimeNow }})
	if _, created, ingestErr := pipeline.Ingest(tc.ctx, ingestionFixture(invoiceID, tc.clientID, tc.now)); ingestErr != nil || !created {
		t.Fatalf("created=%v err=%v", created, ingestErr)
	}
	tc.invoices = append(tc.invoices, invoiceID)

	metrics := observability.NewMetrics()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := asynq.NewServer(asynqOptions, asynq.Config{Concurrency: 2, Queues: map[string]int{"workflow-full": 1}, DelayedTaskCheckInterval: 20 * time.Millisecond, RetryDelayFunc: func(int, error, *asynq.Task) time.Duration { return 10 * time.Millisecond }})
	mux := asynq.NewServeMux()
	mux.Handle(workerruntime.ContinueInvoiceTask, workerruntime.NewHandler(pipeline, logger, metrics))
	if err = server.Start(mux); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Shutdown)
	client := asynq.NewClient(asynqOptions)
	t.Cleanup(func() { _ = client.Close() })
	publisher := workerruntime.NewAsynqPublisher(client, "workflow-full", 3, time.Minute)
	dispatcher := workerruntime.NewDispatcher(scopedRuntimeOutbox{Store: tc.store, aggregateID: invoiceID}, publisher, workerruntime.DispatcherConfig{Owner: "integration-full-worker", BatchSize: 10, MaxAttempts: 3, PollInterval: 10 * time.Millisecond, ClaimTTL: time.Second, RetryMin: 10 * time.Millisecond, RetryMax: time.Second}, logger, metrics)

	if !eventuallyPostgresWithin(25*time.Second, func() bool {
		if _, dispatchErr := dispatcher.DispatchBatch(tc.ctx); dispatchErr != nil {
			t.Fatalf("dispatch: %v", dispatchErr)
		}
		item, getErr := tc.store.GetInvoice(tc.ctx, invoiceID)
		return getErr == nil && item.PipelineStatus == invoicing.StatusExported
	}) {
		item, getErr := tc.store.GetInvoice(tc.ctx, invoiceID)
		rows, outboxErr := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(invoiceID)).All(tc.ctx)
		outboxStates := make([]string, 0, len(rows))
		for _, row := range rows {
			outboxStates = append(outboxStates, fmt.Sprintf("%s:%s:%v", row.ID, row.Status, row.LastError))
		}
		t.Fatalf("full pipeline did not export within 25s: invoice=%+v invoice_err=%v outbox=%v outbox_err=%v", item, getErr, outboxStates, outboxErr)
	}
	item, err := tc.store.GetInvoice(tc.ctx, invoiceID)
	if err != nil {
		t.Fatal(err)
	}
	associationCount, _ := tc.store.Client.InvoiceContractAssociation.Query().Where(invoicecontractassociation.InvoiceIDEQ(invoiceID)).Count(tc.ctx)
	taskCount, _ := tc.store.Client.ValidationTask.Query().Where(validationtask.InvoiceIDEQ(invoiceID)).Count(tc.ctx)
	if item.SagaStatus != invoicing.SagaExported || associationCount != 1 || taskCount != 0 || len(item.Lines) != 1 || len(item.Lines[0].Classifications) != 3 {
		t.Fatalf("status=%s saga=%s associations=%d tasks=%d lines=%d", item.PipelineStatus, item.SagaStatus, associationCount, taskCount, len(item.Lines))
	}
}

func TestContractAvailableOutboxAsynqAutomaticallyResumesMissingInvoice(t *testing.T) {
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("TEST_REDIS_URL is not set")
	}
	redisOptions, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	if redisOptions.DB == 0 {
		t.Fatal("TEST_REDIS_URL must use an isolated non-zero database")
	}
	redisClient := redis.NewClient(redisOptions)
	if err = redisClient.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = redisClient.FlushDB(context.Background()).Err(); _ = redisClient.Close() })
	asynqOptions, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		t.Fatal(err)
	}

	tc := newContractTestContext(t)
	invoiceID, _ := tc.createMissingContractInvoice(t, "worker-contract-available", "RO-WORKER-AVAILABLE", "RON", true)
	contractID := tc.createContract(t, "WORKER-AVAILABLE", "RO-WORKER-AVAILABLE", "RON")
	contractService := contracts.NewService(tc.store, contracts.BaselinePolicy{}, nil)
	created, err := contractService.ContractAvailable(tc.ctx, contracts.AvailableCommand{ContractID: contractID, CommandID: "worker-contract-available", CorrelationID: "worker-resume-trace"})
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	metrics := observability.NewMetrics()
	server := asynq.NewServer(asynqOptions, asynq.Config{Concurrency: 2, Queues: map[string]int{"contract-available-integration": 1}, DelayedTaskCheckInterval: 20 * time.Millisecond})
	mux := asynq.NewServeMux()
	mux.Handle(workerruntime.ContractAvailableTask, workerruntime.NewContractAvailableHandler(contractService, logger, metrics))
	if err = server.Start(mux); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Shutdown)
	client := asynq.NewClient(asynqOptions)
	t.Cleanup(func() { _ = client.Close() })
	publisher := workerruntime.NewAsynqPublisher(client, "contract-available-integration", 3, time.Minute)
	dispatcher := workerruntime.NewDispatcher(scopedRuntimeOutbox{Store: tc.store, aggregateID: contractID}, publisher, workerruntime.DispatcherConfig{Owner: "contract-available-worker", BatchSize: 10, MaxAttempts: 3, PollInterval: 10 * time.Millisecond, ClaimTTL: time.Second, RetryMin: 10 * time.Millisecond, RetryMax: time.Second}, logger, metrics)

	eventuallyPostgres(t, func() bool {
		if _, dispatchErr := dispatcher.DispatchBatch(tc.ctx); dispatchErr != nil {
			t.Fatalf("dispatch: %v", dispatchErr)
		}
		row, getErr := tc.store.Client.Invoice.Get(tc.ctx, invoiceID)
		return getErr == nil && row.PipelineStatus == invoice.PipelineStatusDEDUPE_CHECKED
	})
	associations, _ := tc.store.Client.InvoiceContractAssociation.Query().Where(invoicecontractassociation.InvoiceIDEQ(invoiceID)).Count(tc.ctx)
	activeTasks, _ := tc.store.Client.ValidationTask.Query().Where(validationtask.InvoiceIDEQ(invoiceID), validationtask.StatusNEQ(validationtask.StatusRESOLVED)).Count(tc.ctx)
	continuations, _ := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(invoiceID), outboxentry.EventTypeEQ(outbox.EventInvoiceContinue)).Count(tc.ctx)
	if associations != 1 || activeTasks != 0 || continuations != 1 {
		t.Fatalf("associations=%d active=%d continuations=%d", associations, activeTasks, continuations)
	}
}

func eventuallyPostgres(t *testing.T, condition func() bool) {
	t.Helper()
	if !eventuallyPostgresWithin(10*time.Second, condition) {
		t.Fatal("eventual worker condition was not met")
	}
}

func eventuallyPostgresWithin(timeout time.Duration, condition func() bool) bool {
	deadline := time.NewTimer(timeout)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for {
		select {
		case <-deadline.C:
			return false
		case <-ticker.C:
			if condition() {
				return true
			}
		}
	}
}
