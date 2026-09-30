//go:build integration

package postgres

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"diana-contabilitate/backend/ent/activityevent"
	"diana-contabilitate/backend/ent/invoice"
	"diana-contabilitate/backend/ent/invoicecontractassociation"
	"diana-contabilitate/backend/ent/outboxentry"
	entvalidationtask "diana-contabilitate/backend/ent/validationtask"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/commercialvalidation"
	"diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/validationtasks"
)

func waiverCommand(invoiceID, taskID, commandID string, revision uint64) validationtasks.ContinueWithoutContractCommand {
	return validationtasks.ContinueWithoutContractCommand{
		InvoiceID: invoiceID, TaskID: taskID, ExpectedRevision: revision, Reason: "Achiziție punctuală fără contract",
		CommandID: commandID, ActorID: "accountant-1", ActorDisplay: "Contabil test", CorrelationID: commandID,
	}
}

func TestContinueWithoutContractIsAtomicAndIdempotentFromOpenOrWaiting(t *testing.T) {
	for _, status := range []entvalidationtask.Status{entvalidationtask.StatusOPEN, entvalidationtask.StatusWAITING} {
		t.Run(string(status), func(t *testing.T) {
			tc := newTaskTestContext(t)
			suffix := "waiver-" + string(status)
			invoiceID := tc.createInvoice(t, suffix, invoice.PipelineStatusAWAITING_CONTRACT)
			taskID := "missing-" + suffix
			tc.createTask(t, taskID, invoiceID, entvalidationtask.TaskTypeMISSING_CONTRACT, status)
			service := validationtasks.NewService(tc.store, func() time.Time { return tc.now.Add(time.Hour) })
			command := waiverCommand(invoiceID, taskID, "waive-once-"+suffix, 1)

			task, changed, err := service.ContinueWithoutContract(tc.ctx, command)
			if err != nil || !changed {
				t.Fatalf("changed=%v err=%v", changed, err)
			}
			var metadata map[string]string
			_ = json.Unmarshal(task.ResolutionMetadata, &metadata)
			if task.Status != validationtasks.StatusResolved || task.Revision != 2 || task.ResolvedAt == nil || metadata["reason"] != "CONTRACT_WAIVED" || metadata["note"] != command.Reason {
				t.Fatalf("task=%+v metadata=%v", task, metadata)
			}
			invoiceRow, _ := tc.store.Client.Invoice.Get(tc.ctx, invoiceID)
			if invoiceRow.PipelineStatus != invoice.PipelineStatusDEDUPE_CHECKED || invoiceRow.Revision != 2 {
				t.Fatalf("invoice=%s/%d", invoiceRow.PipelineStatus, invoiceRow.Revision)
			}
			taskAudits, _ := tc.store.Client.ActivityEvent.Query().Where(activityevent.ValidationTaskIDEQ(taskID), activityevent.EventTypeEQ("MISSING_CONTRACT_WAIVED")).Count(tc.ctx)
			invoiceAudits, _ := tc.store.Client.ActivityEvent.Query().Where(activityevent.InvoiceIDEQ(invoiceID), activityevent.EventTypeEQ("CONTRACT_WAIVED")).Count(tc.ctx)
			outbox, _ := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(invoiceID)).Count(tc.ctx)
			associated, _ := tc.store.Client.InvoiceContractAssociation.Query().Where(invoicecontractassociation.InvoiceIDEQ(invoiceID)).Exist(tc.ctx)
			if taskAudits != 1 || invoiceAudits != 1 || outbox != 1 || associated {
				t.Fatalf("taskAudits=%d invoiceAudits=%d outbox=%d associated=%v", taskAudits, invoiceAudits, outbox, associated)
			}

			detail, err := tc.store.GetInvoice(tc.ctx, invoiceID)
			if err != nil || detail.ContractWaiver == nil || detail.ContractWaiver.Reason != command.Reason || detail.ContractWaiver.ActorDisplay == nil || *detail.ContractWaiver.ActorDisplay != "Contabil test" || detail.ContractAssociation != nil {
				t.Fatalf("detail waiver=%+v association=%+v err=%v", detail.ContractWaiver, detail.ContractAssociation, err)
			}

			replay, changed, err := service.ContinueWithoutContract(tc.ctx, command)
			if err != nil || changed || replay.Revision != 2 {
				t.Fatalf("replay=%+v changed=%v err=%v", replay, changed, err)
			}
			again := command
			again.CommandID = "another-key-" + suffix
			again.ExpectedRevision = 2
			if _, _, err = service.ContinueWithoutContract(tc.ctx, again); !errors.Is(err, validationtasks.ErrTaskAlreadyResolved) {
				t.Fatalf("second waiver error=%v", err)
			}
			if outbox, _ = tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(invoiceID)).Count(tc.ctx); outbox != 1 {
				t.Fatalf("replays enqueued work: outbox=%d", outbox)
			}
		})
	}
}

func TestContinueWithoutContractRejectsWrongTypeStateAndStaleRevision(t *testing.T) {
	tests := []struct {
		name          string
		invoiceStatus invoice.PipelineStatus
		taskType      entvalidationtask.TaskType
		revision      uint64
		want          error
	}{
		{"wrong type", invoice.PipelineStatusAWAITING_MATCH_CONFIRM, entvalidationtask.TaskTypeCONTRACT_MATCH, 1, apperrors.ErrValidation},
		{"wrong invoice state", invoice.PipelineStatusMATCHING, entvalidationtask.TaskTypeMISSING_CONTRACT, 1, apperrors.ErrValidation},
		{"stale revision", invoice.PipelineStatusAWAITING_CONTRACT, entvalidationtask.TaskTypeMISSING_CONTRACT, 7, apperrors.ErrConflict},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tc := newTaskTestContext(t)
			suffix := fmt.Sprintf("waiver-invalid-%d", index)
			invoiceID := tc.createInvoice(t, suffix, test.invoiceStatus)
			taskID := "task-" + suffix
			tc.createTask(t, taskID, invoiceID, test.taskType, entvalidationtask.StatusOPEN)
			service := validationtasks.NewService(tc.store, func() time.Time { return tc.now })
			if _, _, err := service.ContinueWithoutContract(tc.ctx, waiverCommand(invoiceID, taskID, suffix, test.revision)); !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
			invoiceRow, _ := tc.store.Client.Invoice.Get(tc.ctx, invoiceID)
			if invoiceRow.PipelineStatus != test.invoiceStatus || invoiceRow.Revision != 1 {
				t.Fatalf("rejected waiver mutated invoice: %s/%d", invoiceRow.PipelineStatus, invoiceRow.Revision)
			}
		})
	}
}

func TestConcurrentWaiversProduceOneTransition(t *testing.T) {
	tc := newTaskTestContext(t)
	invoiceID := tc.createInvoice(t, "waiver-race", invoice.PipelineStatusAWAITING_CONTRACT)
	tc.createTask(t, "missing-waiver-race", invoiceID, entvalidationtask.TaskTypeMISSING_CONTRACT, entvalidationtask.StatusOPEN)
	service := validationtasks.NewService(tc.store, func() time.Time { return tc.now.Add(time.Hour) })
	results := make(chan error, 2)
	var changedCount atomic.Int32
	var group sync.WaitGroup
	for _, key := range []string{"accountant-a", "accountant-b"} {
		group.Add(1)
		go func() {
			defer group.Done()
			_, changed, err := service.ContinueWithoutContract(tc.ctx, waiverCommand(invoiceID, "missing-waiver-race", key, 1))
			if changed {
				changedCount.Add(1)
			}
			results <- err
		}()
	}
	group.Wait()
	close(results)
	losers := 0
	for err := range results {
		if errors.Is(err, apperrors.ErrConflict) || errors.Is(err, validationtasks.ErrTaskAlreadyResolved) {
			losers++
		} else if err != nil {
			t.Fatal(err)
		}
	}
	if changedCount.Load() != 1 || losers != 1 {
		t.Fatalf("changes=%d losers=%d", changedCount.Load(), losers)
	}
	outbox, _ := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(invoiceID)).Count(tc.ctx)
	if outbox != 1 {
		t.Fatalf("outbox=%d", outbox)
	}
}

func TestWaivedInvoiceIsNeverResumedByALaterContract(t *testing.T) {
	tc := newTaskTestContext(t)
	invoiceID := tc.createInvoice(t, "waiver-resume", invoice.PipelineStatusAWAITING_CONTRACT)
	tc.createTask(t, "missing-waiver-resume", invoiceID, entvalidationtask.TaskTypeMISSING_CONTRACT, entvalidationtask.StatusOPEN)
	service := validationtasks.NewService(tc.store, func() time.Time { return tc.now.Add(time.Hour) })
	if _, _, err := service.ContinueWithoutContract(tc.ctx, waiverCommand(invoiceID, "missing-waiver-resume", "waive-resume", 1)); err != nil {
		t.Fatal(err)
	}
	decision := contracts.MatchDecision{Outcome: contracts.OutcomeNoMatch, PolicyVersion: "test"}
	if _, err := tc.store.ApplyResumeDecision(tc.ctx, contracts.ResumeCommand{InvoiceID: invoiceID, ExpectedRevision: 2, CommandID: "resume-after-waiver"}, decision, tc.now.Add(2*time.Hour)); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("resume after waiver error=%v", err)
	}
}

func TestWaivedInvoicePassesCommercialValidationWithTheWaiverFinding(t *testing.T) {
	tc := newTaskTestContext(t)
	invoiceID := tc.createInvoice(t, "waiver-commercial", invoice.PipelineStatusAWAITING_CONTRACT)
	t.Cleanup(func() {
		_, _ = tc.store.DB.ExecContext(tc.ctx, `DELETE FROM invoice_commercial_findings WHERE run_id IN (SELECT id FROM invoice_commercial_validation_runs WHERE invoice_id=$1)`, invoiceID)
		_, _ = tc.store.DB.ExecContext(tc.ctx, `DELETE FROM invoice_commercial_validation_runs WHERE invoice_id=$1`, invoiceID)
	})
	tc.createTask(t, "missing-waiver-commercial", invoiceID, entvalidationtask.TaskTypeMISSING_CONTRACT, entvalidationtask.StatusWAITING)
	service := validationtasks.NewService(tc.store, func() time.Time { return tc.now.Add(time.Hour) })
	if _, _, err := service.ContinueWithoutContract(tc.ctx, waiverCommand(invoiceID, "missing-waiver-commercial", "waive-commercial", 1)); err != nil {
		t.Fatal(err)
	}
	// Fixture shortcut: the pipeline moves DEDUPE_CHECKED -> HEADER_READ ->
	// LINES_READ -> COMMERCIAL_VALIDATING without touching contract state.
	if _, err := tc.store.Client.Invoice.UpdateOneID(invoiceID).SetPipelineStatus(invoice.PipelineStatusCOMMERCIAL_VALIDATING).SetRevision(5).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	commercial := commercialvalidation.NewService(tc.store, func() time.Time { return tc.now.Add(2 * time.Hour) })
	if err := commercial.ProcessCommercialValidation(tc.ctx, invoiceID, 5, "commercial-after-waiver", "corr"); err != nil {
		t.Fatal(err)
	}
	invoiceRow, _ := tc.store.Client.Invoice.Get(tc.ctx, invoiceID)
	if invoiceRow.PipelineStatus != invoice.PipelineStatusCOMMERCIALLY_VALIDATED {
		t.Fatalf("status=%s", invoiceRow.PipelineStatus)
	}
	var code, outcome, actual string
	if err := tc.store.DB.QueryRowContext(tc.ctx, `SELECT f.code,f.outcome,COALESCE(f.actual_value,'') FROM invoice_commercial_findings f JOIN invoice_commercial_validation_runs r ON r.id=f.run_id WHERE r.invoice_id=$1`, invoiceID).Scan(&code, &outcome, &actual); err != nil {
		t.Fatal(err)
	}
	if code != "CONTRACT_WAIVED" || outcome != "CONFORM" || actual != "Achiziție punctuală fără contract" {
		t.Fatalf("finding=%s/%s/%s", code, outcome, actual)
	}
}
