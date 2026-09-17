//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"diana-contabilitate/backend/ent/activityevent"
	"diana-contabilitate/backend/ent/invoice"
	"diana-contabilitate/backend/ent/outboxentry"
	entvalidationtask "diana-contabilitate/backend/ent/validationtask"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/audit"
	"diana-contabilitate/backend/internal/validationtasks"
)

var taskSequence atomic.Uint64

type taskTestContext struct {
	store    *Store
	ctx      context.Context
	clientID string
	now      time.Time
}

func newTaskTestContext(t *testing.T) taskTestContext {
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
	sequence := taskSequence.Add(1)
	clientID := fmt.Sprintf("task-client-%d", sequence)
	now := time.Date(2026, time.September, 12, 9, int(sequence), 0, 0, time.UTC)
	if _, err = store.Client.AccountingClient.Create().SetID(clientID).SetName("Client task test").SetCui(fmt.Sprintf("RO-TASK-%06d", sequence)).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ids, _ := store.Client.Invoice.Query().Where(invoice.ClientIDEQ(clientID)).IDs(ctx)
		if len(ids) > 0 {
			_, _ = store.Client.ActivityEvent.Delete().Where(activityevent.InvoiceIDIn(ids...)).Exec(ctx)
			_, _ = store.Client.ValidationTask.Delete().Where(entvalidationtask.InvoiceIDIn(ids...)).Exec(ctx)
			_, _ = store.Client.OutboxEntry.Delete().Where(outboxentry.AggregateIDIn(ids...)).Exec(ctx)
			_, _ = store.Client.Invoice.Delete().Where(invoice.IDIn(ids...)).Exec(ctx)
		}
		_ = store.Client.AccountingClient.DeleteOneID(clientID).Exec(ctx)
		_ = store.Close()
	})
	return taskTestContext{store: store, ctx: ctx, clientID: clientID, now: now}
}

func (tc taskTestContext) createInvoice(t *testing.T, suffix string, status invoice.PipelineStatus) string {
	t.Helper()
	id := "task-invoice-" + suffix
	_, err := tc.store.Client.Invoice.Create().SetID(id).SetClientID(tc.clientID).
		SetSupplierName("Furnizor task " + suffix).SetSupplierCui("RO-" + suffix).SetNormalizedSupplierCui("RO-" + suffix).
		SetDocumentNumber("TASK-" + suffix).SetNormalizedDocumentNumber("TASK-" + suffix).
		SetIssueDate(tc.now).SetIssueDay(time.Date(tc.now.Year(), tc.now.Month(), tc.now.Day(), 0, 0, 0, 0, time.UTC)).
		SetTotalAmount("119.0000").SetCurrency("RON").SetSpvReference("SPV-" + suffix).
		SetIngestionSource("TASK_TEST").SetExternalDeliveryID("delivery-" + suffix).
		SetPipelineStatus(status).SetSagaStatus(invoice.SagaStatusNOT_READY).
		SetRevision(1).SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (tc taskTestContext) createTask(t *testing.T, id, invoiceID string, taskType entvalidationtask.TaskType, status entvalidationtask.Status) {
	t.Helper()
	create := tc.store.Client.ValidationTask.Create().SetID(id).SetClientID(tc.clientID).SetInvoiceID(invoiceID).
		SetTaskType(taskType).SetStatus(status).SetTitle("Task " + id).SetReason("Motiv " + id).
		SetCreatedByKind(entvalidationtask.CreatedByKindSYSTEM).SetCreationKey("fixture:" + id).
		SetRevision(1).SetCreatedAt(tc.now).SetUpdatedAt(tc.now)
	if status == entvalidationtask.StatusWAITING {
		create.SetWaitingSince(tc.now)
	}
	if status == entvalidationtask.StatusRESOLVED {
		create.SetResolvedAt(tc.now)
	}
	if _, err := create.Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
}

func creationCommand(invoiceID, commandID string) validationtasks.CreationCommand {
	return validationtasks.CreationCommand{
		InvoiceID: invoiceID, ExpectedInvoiceRevision: 1, Title: "Contract lipsă", Reason: "Nu există contract.",
		CommandID: commandID, Actor: audit.ActorSystem, ActorDisplay: "Sistem test", CorrelationID: commandID,
	}
}

func TestBlockingTaskCreationIsAtomicAndIdempotent(t *testing.T) {
	tc := newTaskTestContext(t)
	invoiceID := tc.createInvoice(t, "atomic-create", invoice.PipelineStatusMATCHING)
	service := validationtasks.NewService(tc.store, func() time.Time { return tc.now })
	command := creationCommand(invoiceID, "atomic-create")
	task, changed, err := service.CreateMissingContractBlock(tc.ctx, command)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	row, _ := tc.store.Client.Invoice.Get(tc.ctx, invoiceID)
	if row.PipelineStatus != invoice.PipelineStatusAWAITING_CONTRACT || row.Revision != 2 || task.Status != validationtasks.StatusOpen {
		t.Fatalf("invoice=%s/%d task=%+v", row.PipelineStatus, row.Revision, task)
	}
	audits, _ := tc.store.Client.ActivityEvent.Query().Where(activityevent.ValidationTaskIDEQ(task.ID), activityevent.EventTypeEQ("VALIDATION_TASK_CREATED")).Count(tc.ctx)
	outbox, _ := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(invoiceID)).Count(tc.ctx)
	if audits != 1 || outbox != 0 {
		t.Fatalf("audits=%d outbox=%d", audits, outbox)
	}
	replayed, changed, err := service.CreateMissingContractBlock(tc.ctx, command)
	if err != nil || changed || replayed.ID != task.ID {
		t.Fatalf("replay=%+v changed=%v err=%v", replayed, changed, err)
	}
}

func TestDatabaseEnforcesOneActiveBlockerAndKeepsResolvedHistory(t *testing.T) {
	tc := newTaskTestContext(t)
	invoiceID := tc.createInvoice(t, "active-index", invoice.PipelineStatusAWAITING_CONTRACT)
	tc.createTask(t, "active-one", invoiceID, entvalidationtask.TaskTypeMISSING_CONTRACT, entvalidationtask.StatusOPEN)
	createSecond := func(id string) error {
		_, err := tc.store.Client.ValidationTask.Create().SetID(id).SetClientID(tc.clientID).SetInvoiceID(invoiceID).
			SetTaskType(entvalidationtask.TaskTypeCONTRACT_MATCH).SetStatus(entvalidationtask.StatusOPEN).
			SetTitle("Second").SetReason("Second active blocker").SetCreatedByKind(entvalidationtask.CreatedByKindSYSTEM).
			SetCreationKey("fixture:" + id).SetRevision(1).SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx)
		return err
	}
	if err := createSecond("active-two-rejected"); err == nil {
		t.Fatal("second active blocker should violate the partial unique index")
	}
	if _, err := tc.store.Client.ValidationTask.UpdateOneID("active-one").SetStatus(entvalidationtask.StatusRESOLVED).SetResolvedAt(tc.now).AddRevision(1).SetUpdatedAt(tc.now).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	if err := createSecond("active-two"); err != nil {
		t.Fatalf("new task after resolved history: %v", err)
	}
	count, _ := tc.store.Client.ValidationTask.Query().Where(entvalidationtask.InvoiceIDEQ(invoiceID)).Count(tc.ctx)
	if count != 2 {
		t.Fatalf("task history count=%d", count)
	}
}

func TestConcurrentBlockingTaskCreationHasAtMostOneWinner(t *testing.T) {
	tc := newTaskTestContext(t)
	invoiceID := tc.createInvoice(t, "concurrent-create", invoice.PipelineStatusMATCHING)
	service := validationtasks.NewService(tc.store, func() time.Time { return tc.now })
	results := make(chan error, 2)
	var winners atomic.Int32
	var group sync.WaitGroup
	for _, key := range []string{"create-a", "create-b"} {
		key := key
		group.Add(1)
		go func() {
			defer group.Done()
			_, changed, err := service.CreateMissingContractBlock(tc.ctx, creationCommand(invoiceID, key))
			if changed {
				winners.Add(1)
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
	if winners.Load() != 1 || conflicts != 1 {
		t.Fatalf("winners=%d conflicts=%d", winners.Load(), conflicts)
	}
	active, _ := tc.store.Client.ValidationTask.Query().Where(entvalidationtask.InvoiceIDEQ(invoiceID), entvalidationtask.StatusNEQ(entvalidationtask.StatusRESOLVED)).Count(tc.ctx)
	if active != 1 {
		t.Fatalf("active tasks=%d", active)
	}
}

func TestConcurrentSameTaskCreationCommandIsIdempotent(t *testing.T) {
	tc := newTaskTestContext(t)
	invoiceID := tc.createInvoice(t, "same-command", invoice.PipelineStatusMATCHING)
	service := validationtasks.NewService(tc.store, func() time.Time { return tc.now })
	results := make(chan error, 2)
	var changedCount atomic.Int32
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, changed, err := service.CreateMissingContractBlock(tc.ctx, creationCommand(invoiceID, "same-command"))
			if changed {
				changedCount.Add(1)
			}
			results <- err
		}()
	}
	group.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	if changedCount.Load() != 1 {
		t.Fatalf("changes=%d, want one committed logical operation", changedCount.Load())
	}
	taskCount, _ := tc.store.Client.ValidationTask.Query().Where(entvalidationtask.InvoiceIDEQ(invoiceID)).Count(tc.ctx)
	auditCount, _ := tc.store.Client.ActivityEvent.Query().Where(activityevent.InvoiceIDEQ(invoiceID), activityevent.EventTypeEQ("VALIDATION_TASK_CREATED")).Count(tc.ctx)
	if taskCount != 1 || auditCount != 1 {
		t.Fatalf("tasks=%d audits=%d", taskCount, auditCount)
	}
}

func TestRequestMissingContractIsAtomicIdempotentAndDoesNotContinuePipeline(t *testing.T) {
	tc := newTaskTestContext(t)
	invoiceID := tc.createInvoice(t, "request", invoice.PipelineStatusAWAITING_CONTRACT)
	tc.createTask(t, "missing-request", invoiceID, entvalidationtask.TaskTypeMISSING_CONTRACT, entvalidationtask.StatusOPEN)
	service := validationtasks.NewService(tc.store, func() time.Time { return tc.now.Add(time.Hour) })
	command := validationtasks.RequestMissingContractCommand{InvoiceID: invoiceID, TaskID: "missing-request", ExpectedRevision: 1, CommandID: "request-once", ActorID: "accountant-1", ActorDisplay: "Contabil test", CorrelationID: "request-correlation"}
	task, changed, err := service.RequestMissingContract(tc.ctx, command)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	invoiceRow, _ := tc.store.Client.Invoice.Get(tc.ctx, invoiceID)
	if task.Status != validationtasks.StatusWaiting || task.Revision != 2 || task.WaitingSince == nil || invoiceRow.PipelineStatus != invoice.PipelineStatusAWAITING_CONTRACT || invoiceRow.Revision != 1 {
		t.Fatalf("task=%+v invoice=%s/%d", task, invoiceRow.PipelineStatus, invoiceRow.Revision)
	}
	audits, _ := tc.store.Client.ActivityEvent.Query().Where(activityevent.ValidationTaskIDEQ(task.ID), activityevent.EventTypeEQ("MISSING_CONTRACT_REQUESTED")).Count(tc.ctx)
	outbox, _ := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(invoiceID)).Count(tc.ctx)
	if audits != 1 || outbox != 0 {
		t.Fatalf("audits=%d outbox=%d", audits, outbox)
	}
	replay, changed, err := service.RequestMissingContract(tc.ctx, command)
	if err != nil || changed || replay.Revision != 2 {
		t.Fatalf("replay=%+v changed=%v err=%v", replay, changed, err)
	}
	stale := command
	stale.CommandID = "stale-request"
	if _, _, err = service.RequestMissingContract(tc.ctx, stale); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("stale error=%v", err)
	}
}

func TestConcurrentContractRequestsProduceOneTransition(t *testing.T) {
	tc := newTaskTestContext(t)
	invoiceID := tc.createInvoice(t, "request-race", invoice.PipelineStatusAWAITING_CONTRACT)
	tc.createTask(t, "missing-race", invoiceID, entvalidationtask.TaskTypeMISSING_CONTRACT, entvalidationtask.StatusOPEN)
	service := validationtasks.NewService(tc.store, func() time.Time { return tc.now.Add(time.Hour) })
	results := make(chan error, 2)
	var changedCount atomic.Int32
	var group sync.WaitGroup
	for _, key := range []string{"accountant-a", "accountant-b"} {
		key := key
		group.Add(1)
		go func() {
			defer group.Done()
			_, changed, err := service.RequestMissingContract(tc.ctx, validationtasks.RequestMissingContractCommand{InvoiceID: invoiceID, TaskID: "missing-race", ExpectedRevision: 1, CommandID: key, ActorID: key})
			if changed {
				changedCount.Add(1)
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
	if changedCount.Load() != 1 || conflicts != 1 {
		t.Fatalf("changes=%d conflicts=%d", changedCount.Load(), conflicts)
	}
	audits, _ := tc.store.Client.ActivityEvent.Query().Where(activityevent.ValidationTaskIDEQ("missing-race"), activityevent.EventTypeEQ("MISSING_CONTRACT_REQUESTED")).Count(tc.ctx)
	if audits != 1 {
		t.Fatalf("audits=%d", audits)
	}
}

func TestRequestMissingContractRejectsWrongTypeStateAndResolvedTask(t *testing.T) {
	tests := []struct {
		name          string
		invoiceStatus invoice.PipelineStatus
		taskType      entvalidationtask.TaskType
		taskStatus    entvalidationtask.Status
		want          error
	}{
		{"wrong type", invoice.PipelineStatusAWAITING_CONTRACT, entvalidationtask.TaskTypeCONTRACT_MATCH, entvalidationtask.StatusOPEN, apperrors.ErrValidation},
		{"wrong invoice state", invoice.PipelineStatusMATCHING, entvalidationtask.TaskTypeMISSING_CONTRACT, entvalidationtask.StatusOPEN, apperrors.ErrValidation},
		{"resolved", invoice.PipelineStatusAWAITING_CONTRACT, entvalidationtask.TaskTypeMISSING_CONTRACT, entvalidationtask.StatusRESOLVED, validationtasks.ErrTaskAlreadyResolved},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tc := newTaskTestContext(t)
			suffix := fmt.Sprintf("invalid-%d", index)
			invoiceID := tc.createInvoice(t, suffix, test.invoiceStatus)
			taskID := "task-" + suffix
			tc.createTask(t, taskID, invoiceID, test.taskType, test.taskStatus)
			service := validationtasks.NewService(tc.store, func() time.Time { return tc.now })
			_, _, err := service.RequestMissingContract(tc.ctx, validationtasks.RequestMissingContractCommand{InvoiceID: invoiceID, TaskID: taskID, ExpectedRevision: 1, CommandID: suffix})
			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}

func TestTaskQueryFiltersClientStatusTypeInvoiceAndKeepsHistory(t *testing.T) {
	tc := newTaskTestContext(t)
	openInvoice := tc.createInvoice(t, "query-open", invoice.PipelineStatusAWAITING_MATCH_CONFIRM)
	resolvedInvoice := tc.createInvoice(t, "query-resolved", invoice.PipelineStatusAWAITING_REVIEW)
	tc.createTask(t, "query-open-task", openInvoice, entvalidationtask.TaskTypeCONTRACT_MATCH, entvalidationtask.StatusOPEN)
	tc.createTask(t, "query-resolved-task", resolvedInvoice, entvalidationtask.TaskTypeCLASSIFICATION, entvalidationtask.StatusRESOLVED)
	service := validationtasks.NewService(tc.store, nil)
	openStatus := validationtasks.StatusOpen
	contractType := validationtasks.TypeContractMatch
	items, err := service.List(tc.ctx, validationtasks.Filter{ClientID: tc.clientID, InvoiceID: openInvoice, Status: &openStatus, Type: &contractType})
	if err != nil || len(items) != 1 || items[0].Task.ID != "query-open-task" || items[0].Invoice.ID != openInvoice || items[0].Client.ID != tc.clientID {
		t.Fatalf("filtered items=%+v err=%v", items, err)
	}
	resolvedStatus := validationtasks.StatusResolved
	history, err := service.List(tc.ctx, validationtasks.Filter{ClientID: tc.clientID, Status: &resolvedStatus})
	if err != nil || len(history) != 1 || history[0].Task.ID != "query-resolved-task" {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	otherClientItems, err := service.List(tc.ctx, validationtasks.Filter{ClientID: "another-client"})
	if err != nil || len(otherClientItems) != 0 {
		t.Fatalf("cross-client items=%+v err=%v", otherClientItems, err)
	}
}
