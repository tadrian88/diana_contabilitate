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

	"diana-contabilitate/backend/ent"
	"diana-contabilitate/backend/ent/activityevent"
	entcontract "diana-contabilitate/backend/ent/contract"
	"diana-contabilitate/backend/ent/contractmatchcandidate"
	"diana-contabilitate/backend/ent/contractmatchrun"
	"diana-contabilitate/backend/ent/invoice"
	"diana-contabilitate/backend/ent/invoicecontractassociation"
	"diana-contabilitate/backend/ent/outboxentry"
	"diana-contabilitate/backend/ent/validationtask"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/outbox"
	"diana-contabilitate/backend/internal/validationtasks"
)

var contractTestSequence atomic.Uint64

type contractTestContext struct {
	ctx         context.Context
	store       *Store
	clientID    string
	now         time.Time
	ids         []string
	contractIDs []string
}

func newContractTestContext(t *testing.T) *contractTestContext {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	store, err := Open(url)
	if err != nil {
		t.Fatal(err)
	}
	sequence := contractTestSequence.Add(1)
	tc := &contractTestContext{ctx: context.Background(), store: store, clientID: fmt.Sprintf("contract-client-%d", sequence), now: time.Date(2026, time.September, 15, 9, int(sequence), 0, 0, time.UTC)}
	if _, err = store.Client.AccountingClient.Create().SetID(tc.clientID).SetName("Contract test client").SetCui(fmt.Sprintf("RO-CONTRACT-%06d", sequence)).SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx); err != nil {
		store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = store.Client.ActivityEvent.Delete().Where(activityevent.ClientIDEQ(tc.clientID)).Exec(tc.ctx)
		if len(tc.ids) > 0 {
			_, _ = store.Client.ValidationTask.Delete().Where(validationtask.InvoiceIDIn(tc.ids...)).Exec(tc.ctx)
			_, _ = store.Client.OutboxEntry.Delete().Where(outboxentry.AggregateIDIn(tc.ids...)).Exec(tc.ctx)
			_, _ = store.Client.InvoiceContractAssociation.Delete().Where(invoicecontractassociation.InvoiceIDIn(tc.ids...)).Exec(tc.ctx)
			runs, _ := store.Client.ContractMatchRun.Query().Where(contractmatchrun.InvoiceIDIn(tc.ids...)).IDs(tc.ctx)
			if len(runs) > 0 {
				_, _ = store.Client.ContractMatchCandidate.Delete().Where(contractmatchcandidate.MatchRunIDIn(runs...)).Exec(tc.ctx)
			}
			_, _ = store.Client.ContractMatchRun.Delete().Where(contractmatchrun.InvoiceIDIn(tc.ids...)).Exec(tc.ctx)
			_, _ = store.Client.Invoice.Delete().Where(invoice.IDIn(tc.ids...)).Exec(tc.ctx)
		}
		if len(tc.contractIDs) > 0 {
			_, _ = store.Client.OutboxEntry.Delete().Where(outboxentry.AggregateIDIn(tc.contractIDs...)).Exec(tc.ctx)
		}
		_, _ = store.Client.Contract.Delete().Where(entcontract.ClientIDEQ(tc.clientID)).Exec(tc.ctx)
		_ = store.Client.AccountingClient.DeleteOneID(tc.clientID).Exec(tc.ctx)
		_ = store.Close()
	})
	return tc
}

func (tc *contractTestContext) createInvoice(t *testing.T, suffix, supplierCUI, currency string) string {
	t.Helper()
	id := fmt.Sprintf("contract-invoice-%d-%s", contractTestSequence.Load(), suffix)
	_, err := tc.store.Client.Invoice.Create().SetID(id).SetClientID(tc.clientID).SetSupplierName("Contract supplier").SetSupplierCui(supplierCUI).
		SetNormalizedSupplierCui(supplierCUI).SetDocumentNumber("INV-" + suffix).SetNormalizedDocumentNumber("INV-" + suffix).
		SetIssueDate(tc.now).SetIssueDay(time.Date(tc.now.Year(), tc.now.Month(), tc.now.Day(), 0, 0, 0, 0, time.UTC)).
		SetTotalAmount("100.0000").SetCurrency(currency).SetSpvReference("SPV-" + id).SetIngestionSource("CONTRACT_TEST").
		SetExternalDeliveryID("DELIVERY-" + id).SetPipelineStatus(invoice.PipelineStatusMATCHING).SetSagaStatus(invoice.SagaStatusNOT_READY).
		SetRevision(1).SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx)
	if err != nil {
		t.Fatal(err)
	}
	tc.ids = append(tc.ids, id)
	return id
}

func (tc *contractTestContext) createContract(t *testing.T, suffix, supplierCUI, currency string) string {
	t.Helper()
	id := fmt.Sprintf("contract-%d-%s", contractTestSequence.Load(), suffix)
	_, err := tc.store.Client.Contract.Create().SetID(id).SetClientID(tc.clientID).SetSupplierName("Contract supplier").SetSupplierCui(supplierCUI).
		SetNormalizedSupplierCui(supplierCUI).SetReference("CTR-" + suffix).
		SetEffectiveFrom(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)).SetEffectiveTo(time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)).
		SetTotalValue("100.0000").SetCurrency(currency).SetUnitType("BUC").SetPaymentTerms("30 zile").
		SetRevision(1).SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx)
	if err != nil {
		t.Fatal(err)
	}
	tc.contractIDs = append(tc.contractIDs, id)
	return id
}

func (tc *contractTestContext) match(t *testing.T, invoiceID, key string) (contracts.MatchDecision, bool, error) {
	t.Helper()
	service := contracts.NewService(tc.store, contracts.BaselinePolicy{}, func() time.Time { return tc.now.Add(time.Minute) })
	return service.MatchInvoice(tc.ctx, contracts.MatchCommand{InvoiceID: invoiceID, ExpectedRevision: 1, CommandID: key, CorrelationID: key})
}

func TestContractMatchingPersistsAllFourResultFlows(t *testing.T) {
	tests := []struct {
		name           string
		contracts      []string
		outcome        contracts.MatchOutcome
		pipeline       invoice.PipelineStatus
		taskType       validationtask.TaskType
		association    int
		candidateCount int
		outboxCount    int
	}{
		{"unique compatible", []string{"RON"}, contracts.OutcomeUniqueCompatible, invoice.PipelineStatusDEDUPE_CHECKED, "", 1, 1, 1},
		{"multiple plausible", []string{"RON", "RON"}, contracts.OutcomeMultiplePlausible, invoice.PipelineStatusAWAITING_MATCH_CONFIRM, validationtask.TaskTypeCONTRACT_MATCH, 0, 2, 0},
		{"unique incompatible", []string{"EUR"}, contracts.OutcomeUniqueIncompatible, invoice.PipelineStatusAWAITING_MATCH_CONFIRM, validationtask.TaskTypeCONTRACT_MATCH, 0, 1, 0},
		{"no match", nil, contracts.OutcomeNoMatch, invoice.PipelineStatusAWAITING_CONTRACT, validationtask.TaskTypeMISSING_CONTRACT, 0, 0, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tc := newContractTestContext(t)
			invoiceID := tc.createInvoice(t, test.name, "RO-SUPPLIER", "RON")
			for index, currency := range test.contracts {
				tc.createContract(t, fmt.Sprintf("%c", 'A'+index), "RO-SUPPLIER", currency)
			}
			decision, changed, err := tc.match(t, invoiceID, "match-"+test.name)
			if err != nil || !changed || decision.Outcome != test.outcome {
				t.Fatalf("decision=%+v changed=%v err=%v", decision, changed, err)
			}
			invoiceRow, _ := tc.store.Client.Invoice.Get(tc.ctx, invoiceID)
			associations, _ := tc.store.Client.InvoiceContractAssociation.Query().Where(invoicecontractassociation.InvoiceIDEQ(invoiceID)).Count(tc.ctx)
			candidates, _ := tc.store.Client.ContractMatchCandidate.Query().Where(contractmatchcandidate.HasMatchRunWith(contractmatchrun.InvoiceIDEQ(invoiceID))).Count(tc.ctx)
			outbox, _ := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(invoiceID)).Count(tc.ctx)
			audits, _ := tc.store.Client.ActivityEvent.Query().Where(activityevent.InvoiceIDEQ(invoiceID)).Count(tc.ctx)
			tasks, _ := tc.store.Client.ValidationTask.Query().Where(validationtask.InvoiceIDEQ(invoiceID)).All(tc.ctx)
			if invoiceRow.PipelineStatus != test.pipeline || associations != test.association || candidates != test.candidateCount || outbox != test.outboxCount || audits != 2 {
				t.Fatalf("pipeline=%s associations=%d candidates=%d outbox=%d audits=%d", invoiceRow.PipelineStatus, associations, candidates, outbox, audits)
			}
			if test.taskType == "" && len(tasks) != 0 {
				t.Fatalf("unexpected tasks=%d", len(tasks))
			}
			if test.taskType != "" && (len(tasks) != 1 || tasks[0].TaskType != test.taskType || tasks[0].Status != validationtask.StatusOPEN) {
				t.Fatalf("tasks=%+v", tasks)
			}
			_, replayChanged, replayErr := tc.match(t, invoiceID, "match-"+test.name)
			if replayErr != nil || replayChanged {
				t.Fatalf("replay changed=%v err=%v", replayChanged, replayErr)
			}
			replayedAssociations, _ := tc.store.Client.InvoiceContractAssociation.Query().Where(invoicecontractassociation.InvoiceIDEQ(invoiceID)).Count(tc.ctx)
			replayedCandidates, _ := tc.store.Client.ContractMatchCandidate.Query().Where(contractmatchcandidate.HasMatchRunWith(contractmatchrun.InvoiceIDEQ(invoiceID))).Count(tc.ctx)
			replayedOutbox, _ := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(invoiceID)).Count(tc.ctx)
			replayedAudits, _ := tc.store.Client.ActivityEvent.Query().Where(activityevent.InvoiceIDEQ(invoiceID)).Count(tc.ctx)
			replayedTasks, _ := tc.store.Client.ValidationTask.Query().Where(validationtask.InvoiceIDEQ(invoiceID)).Count(tc.ctx)
			if replayedAssociations != test.association || replayedCandidates != test.candidateCount || replayedOutbox != test.outboxCount || replayedAudits != 2 || replayedTasks != len(tasks) {
				t.Fatalf("replay duplicated state: associations=%d candidates=%d outbox=%d audits=%d tasks=%d", replayedAssociations, replayedCandidates, replayedOutbox, replayedAudits, replayedTasks)
			}
		})
	}
}

func TestPipelineOutboxDispatchInvokesContractMatching(t *testing.T) {
	tc := newContractTestContext(t)
	invoiceID := tc.createInvoice(t, "orchestrated", "RO-ORCHESTRATED", "RON")
	tc.createContract(t, "ORCHESTRATED", "RO-ORCHESTRATED", "RON")
	if _, err := tc.store.Client.Invoice.UpdateOneID(invoiceID).SetPipelineStatus(invoice.PipelineStatusARCHIVED).SetRevision(1).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := tc.store.Client.OutboxEntry.Create().SetID("orchestrated-outbox").SetEventType("INVOICE_CONTINUE").SetAggregateType("INVOICE").SetAggregateID(invoiceID).
		SetPayload([]byte(`{"invoice_id":"` + invoiceID + `"}`)).SetIdempotencyKey("orchestrated").SetStatus(outboxentry.StatusPENDING).
		SetCreatedAt(tc.now).SetAvailableAt(tc.now).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	matching := contracts.NewService(tc.store, nil, func() time.Time { return tc.now.Add(time.Minute) })
	pipeline := invoicing.NewPipelineService(scopedPipelineStore{Store: tc.store, aggregateID: invoiceID}, invoicing.NewFakeSagaExporter(), func() time.Time { return tc.now.Add(time.Minute) })
	pipeline.SetContractMatchingProcessor(matching)
	if count, err := pipeline.DispatchPending(tc.ctx, 1); err != nil || count != 1 {
		t.Fatalf("archive dispatch count=%d err=%v", count, err)
	}
	if count, err := pipeline.DispatchPending(tc.ctx, 1); err != nil || count != 1 {
		t.Fatalf("matching dispatch count=%d err=%v", count, err)
	}
	item, _ := tc.store.GetInvoice(tc.ctx, invoiceID)
	if item.PipelineStatus != invoicing.StatusDedupeChecked || item.ContractAssociation == nil {
		t.Fatalf("invoice=%+v", item)
	}
}

func TestContractConfirmationSupportsRecommendedAndAlternativeCandidates(t *testing.T) {
	for _, selected := range []string{"A", "B"} {
		t.Run(selected, func(t *testing.T) {
			tc := newContractTestContext(t)
			invoiceID := tc.createInvoice(t, "confirm-"+selected, "RO-CONFIRM", "RON")
			contractsBySuffix := map[string]string{
				"A": tc.createContract(t, "A", "RO-CONFIRM", "RON"),
				"B": tc.createContract(t, "B", "RO-CONFIRM", "RON"),
			}
			if _, _, err := tc.match(t, invoiceID, "prepare-confirm"); err != nil {
				t.Fatal(err)
			}
			task, _ := tc.store.Client.ValidationTask.Query().Where(validationtask.InvoiceIDEQ(invoiceID), validationtask.StatusEQ(validationtask.StatusOPEN)).Only(tc.ctx)
			service := contracts.NewService(tc.store, nil, func() time.Time { return tc.now.Add(2 * time.Minute) })
			command := contracts.ConfirmCommand{InvoiceID: invoiceID, TaskID: task.ID, ContractID: contractsBySuffix[selected], ExpectedInvoiceRevision: 2, ExpectedTaskRevision: 1, CommandID: "confirm-" + selected, ActorID: "accountant", ActorDisplay: "Contabil test"}
			changed, err := service.Confirm(tc.ctx, command)
			if err != nil || !changed {
				t.Fatalf("changed=%v err=%v", changed, err)
			}
			invoiceRow, _ := tc.store.Client.Invoice.Get(tc.ctx, invoiceID)
			resolved, _ := tc.store.Client.ValidationTask.Get(tc.ctx, task.ID)
			association, _ := tc.store.Client.InvoiceContractAssociation.Query().Where(invoicecontractassociation.InvoiceIDEQ(invoiceID)).Only(tc.ctx)
			outbox, _ := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(invoiceID)).Count(tc.ctx)
			if invoiceRow.PipelineStatus != invoice.PipelineStatusDEDUPE_CHECKED || resolved.Status != validationtask.StatusRESOLVED || association.ContractID != contractsBySuffix[selected] || outbox != 1 {
				t.Fatalf("invoice=%s task=%s association=%s outbox=%d", invoiceRow.PipelineStatus, resolved.Status, association.ContractID, outbox)
			}
			if selected == "B" {
				auditCount, _ := tc.store.Client.ActivityEvent.Query().Where(activityevent.InvoiceIDEQ(invoiceID), activityevent.EventTypeEQ("CONTRACT_CONFIRMED")).Count(tc.ctx)
				if auditCount != 1 {
					t.Fatalf("alternative audit count=%d", auditCount)
				}
			}
			replayed, replayErr := service.Confirm(tc.ctx, command)
			if replayErr != nil || replayed {
				t.Fatalf("replay changed=%v err=%v", replayed, replayErr)
			}
			associationsAfterReplay, _ := tc.store.Client.InvoiceContractAssociation.Query().Where(invoicecontractassociation.InvoiceIDEQ(invoiceID)).Count(tc.ctx)
			outboxAfterReplay, _ := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(invoiceID)).Count(tc.ctx)
			auditsAfterReplay, _ := tc.store.Client.ActivityEvent.Query().Where(activityevent.InvoiceIDEQ(invoiceID)).Count(tc.ctx)
			if associationsAfterReplay != 1 || outboxAfterReplay != 1 || auditsAfterReplay != 4 {
				t.Fatalf("confirmation replay duplicated state: associations=%d outbox=%d audits=%d", associationsAfterReplay, outboxAfterReplay, auditsAfterReplay)
			}
			differentCommand := command
			differentCommand.CommandID += "-after-resolution"
			if changedAgain, resolvedErr := service.Confirm(tc.ctx, differentCommand); changedAgain || !errors.Is(resolvedErr, validationtasks.ErrTaskAlreadyResolved) {
				t.Fatalf("resolved task confirmation changed=%v err=%v", changedAgain, resolvedErr)
			}
		})
	}
}

func TestContractConfirmationRejectsNonCandidateAndStaleRevisions(t *testing.T) {
	tc := newContractTestContext(t)
	invoiceID := tc.createInvoice(t, "reject", "RO-REJECT", "RON")
	candidate := tc.createContract(t, "A", "RO-REJECT", "RON")
	tc.createContract(t, "B", "RO-REJECT", "RON")
	nonCandidate := tc.createContract(t, "OTHER", "RO-OTHER", "RON")
	if _, _, err := tc.match(t, invoiceID, "prepare-reject"); err != nil {
		t.Fatal(err)
	}
	task, _ := tc.store.Client.ValidationTask.Query().Where(validationtask.InvoiceIDEQ(invoiceID)).Only(tc.ctx)
	service := contracts.NewService(tc.store, nil, nil)
	base := contracts.ConfirmCommand{InvoiceID: invoiceID, TaskID: task.ID, ContractID: candidate, ExpectedInvoiceRevision: 2, ExpectedTaskRevision: 1, CommandID: "reject"}
	notCandidate := base
	notCandidate.ContractID = nonCandidate
	notCandidate.CommandID = "not-candidate"
	if _, err := service.Confirm(tc.ctx, notCandidate); !errors.Is(err, apperrors.ErrValidation) {
		t.Fatalf("non-candidate error=%v", err)
	}
	staleInvoice := base
	staleInvoice.ExpectedInvoiceRevision = 1
	staleInvoice.CommandID = "stale-invoice"
	if _, err := service.Confirm(tc.ctx, staleInvoice); !errors.Is(err, contracts.ErrStaleMatchResult) {
		t.Fatalf("stale invoice error=%v", err)
	}
	staleTask := base
	staleTask.ExpectedTaskRevision = 2
	staleTask.CommandID = "stale-task"
	if _, err := service.Confirm(tc.ctx, staleTask); !errors.Is(err, contracts.ErrStaleMatchResult) {
		t.Fatalf("stale task error=%v", err)
	}
	if _, err := tc.store.Client.Contract.UpdateOneID(candidate).AddRevision(1).SetUpdatedAt(tc.now.Add(time.Hour)).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	staleContract := base
	staleContract.CommandID = "stale-contract"
	if _, err := service.Confirm(tc.ctx, staleContract); !errors.Is(err, contracts.ErrStaleMatchResult) {
		t.Fatalf("stale contract error=%v", err)
	}
}

func TestInvoiceContractAssociationIsHistoricalSnapshot(t *testing.T) {
	tc := newContractTestContext(t)
	invoiceID := tc.createInvoice(t, "snapshot", "RO-SNAPSHOT", "RON")
	contractID := tc.createContract(t, "SNAPSHOT", "RO-SNAPSHOT", "RON")
	if _, _, err := tc.match(t, invoiceID, "snapshot-match"); err != nil {
		t.Fatal(err)
	}
	if _, err := tc.store.Client.Contract.UpdateOneID(contractID).SetReference("CTR-CHANGED").SetTotalValue("999.0000").AddRevision(1).SetUpdatedAt(tc.now.Add(time.Hour)).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	item, err := tc.store.GetInvoice(tc.ctx, invoiceID)
	if err != nil || item.ContractAssociation == nil || item.ContractAssociation.Reference != "CTR-SNAPSHOT" || item.ContractAssociation.Value.Amount.String() != "100.0000" {
		t.Fatalf("association=%+v err=%v", item.ContractAssociation, err)
	}
	invoices, err := tc.store.ListContractInvoices(tc.ctx, contractID)
	if err != nil || len(invoices) != 1 || invoices[0].ID != invoiceID {
		t.Fatalf("invoices=%+v err=%v", invoices, err)
	}
}

func TestContractQueriesRespectClientScopeAndSearch(t *testing.T) {
	tc := newContractTestContext(t)
	wanted := tc.createContract(t, "SEARCHABLE", "RO-SEARCH", "RON")
	tc.createContract(t, "OTHER", "RO-OTHER", "RON")
	items, err := tc.store.ListContracts(tc.ctx, contracts.Filter{ClientID: tc.clientID, Query: "searchable"})
	if err != nil || len(items) != 1 || items[0].ID != wanted {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	outside, err := tc.store.ListContracts(tc.ctx, contracts.Filter{ClientID: "another-client"})
	if err != nil || len(outside) != 0 {
		t.Fatalf("outside=%+v err=%v", outside, err)
	}
}

func TestConcurrentContractMatchingHasOneConsistentOutcome(t *testing.T) {
	tc := newContractTestContext(t)
	invoiceID := tc.createInvoice(t, "match-race", "RO-RACE", "RON")
	tc.createContract(t, "RACE", "RO-RACE", "RON")
	service := contracts.NewService(tc.store, nil, nil)
	var changed atomic.Int32
	results := make(chan error, 2)
	var group sync.WaitGroup
	for _, key := range []string{"worker-a", "worker-b"} {
		group.Add(1)
		go func(key string) {
			defer group.Done()
			_, didChange, err := service.MatchInvoice(tc.ctx, contracts.MatchCommand{InvoiceID: invoiceID, ExpectedRevision: 1, CommandID: key})
			if didChange {
				changed.Add(1)
			}
			results <- err
		}(key)
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
	if changed.Load() != 1 || conflicts != 1 {
		t.Fatalf("changed=%d conflicts=%d", changed.Load(), conflicts)
	}
}

func TestConcurrentContractConfirmationHasOneWinner(t *testing.T) {
	tc := newContractTestContext(t)
	invoiceID := tc.createInvoice(t, "confirm-race", "RO-CONFIRM-RACE", "RON")
	contractID := tc.createContract(t, "A", "RO-CONFIRM-RACE", "RON")
	tc.createContract(t, "B", "RO-CONFIRM-RACE", "RON")
	if _, _, err := tc.match(t, invoiceID, "prepare-race"); err != nil {
		t.Fatal(err)
	}
	task, _ := tc.store.Client.ValidationTask.Query().Where(validationtask.InvoiceIDEQ(invoiceID)).Only(tc.ctx)
	service := contracts.NewService(tc.store, nil, nil)
	var changed atomic.Int32
	results := make(chan error, 2)
	var group sync.WaitGroup
	for _, key := range []string{"accountant-a", "accountant-b"} {
		group.Add(1)
		go func(key string) {
			defer group.Done()
			didChange, err := service.Confirm(tc.ctx, contracts.ConfirmCommand{InvoiceID: invoiceID, TaskID: task.ID, ContractID: contractID, ExpectedInvoiceRevision: 2, ExpectedTaskRevision: 1, CommandID: key, ActorID: key})
			if didChange {
				changed.Add(1)
			}
			results <- err
		}(key)
	}
	group.Wait()
	close(results)
	losers := 0
	for err := range results {
		if errors.Is(err, contracts.ErrStaleMatchResult) || errors.Is(err, validationtasks.ErrTaskAlreadyResolved) || errors.Is(err, apperrors.ErrConflict) {
			losers++
		} else if err != nil {
			t.Fatal(err)
		}
	}
	if changed.Load() != 1 || losers != 1 {
		t.Fatalf("changed=%d losers=%d", changed.Load(), losers)
	}
}

func TestContractForeignKeysRejectCrossClientCandidate(t *testing.T) {
	first := newContractTestContext(t)
	second := newContractTestContext(t)
	invoiceID := first.createInvoice(t, "cross-client", "RO-FIRST", "RON")
	secondContract := second.createContract(t, "SECOND", "RO-SECOND", "RON")
	runID := "cross-client-run"
	if _, err := first.store.Client.ContractMatchRun.Create().SetID(runID).SetClientID(first.clientID).SetInvoiceID(invoiceID).
		SetPolicyVersion(contracts.BaselinePolicyVersion).SetOutcome(contractmatchrun.OutcomeMULTIPLE_PLAUSIBLE).
		SetInvoiceRevision(2).SetCommandKey("cross-client-run").SetCreatedAt(first.now).Save(first.ctx); err != nil {
		t.Fatal(err)
	}
	_, err := first.store.Client.ContractMatchCandidate.Create().SetID("cross-client-candidate").SetClientID(first.clientID).
		SetMatchRunID(runID).SetContractID(secondContract).SetContractRevision(1).SetRank(1).SetRecommended(true).
		SetCompatibility(contractmatchcandidate.CompatibilityCOMPATIBLE).SetConfidenceDisplay("test").SetReasons([]string{"test"}).SetCreatedAt(first.now).Save(first.ctx)
	if err == nil {
		t.Fatal("cross-client candidate should violate the composite foreign key")
	}
	_, err = first.store.Client.InvoiceContractAssociation.Create().SetID("cross-client-association").SetClientID(first.clientID).
		SetInvoiceID(invoiceID).SetContractID(secondContract).SetMatchRunID(runID).
		SetAssociationKind(invoicecontractassociation.AssociationKindAUTOMATIC).SetPolicyVersion(contracts.BaselinePolicyVersion).
		SetContractReference("CTR-SECOND").SetSupplierName("Second client supplier").
		SetEffectiveFrom(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)).SetEffectiveTo(time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)).
		SetTotalValue("100.0000").SetCurrency("RON").SetUnitType("BUC").SetPaymentTerms("30 zile").SetAssociatedAt(first.now).Save(first.ctx)
	if err == nil {
		t.Fatal("cross-client association should violate the composite foreign key")
	}
}

func TestConfirmationOutboxDeliveryMarkIsIdempotent(t *testing.T) {
	tc := newContractTestContext(t)
	invoiceID := tc.createInvoice(t, "outbox", "RO-OUTBOX", "RON")
	tc.createContract(t, "OUTBOX", "RO-OUTBOX", "RON")
	if _, _, err := tc.match(t, invoiceID, "outbox-match"); err != nil {
		t.Fatal(err)
	}
	row, _ := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(invoiceID)).Only(tc.ctx)
	if err := tc.store.MarkOutboxProcessed(tc.ctx, row.ID, tc.now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := tc.store.MarkOutboxProcessed(tc.ctx, row.ID, tc.now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	updated, _ := tc.store.Client.OutboxEntry.Get(tc.ctx, row.ID)
	if updated.Attempts != 1 {
		t.Fatalf("attempts=%d", updated.Attempts)
	}
}

func (tc *contractTestContext) createMissingContractInvoice(t *testing.T, suffix, supplierCUI, currency string, waiting bool) (string, string) {
	t.Helper()
	invoiceID := tc.createInvoice(t, suffix, supplierCUI, currency)
	service := contracts.NewService(tc.store, contracts.BaselinePolicy{}, func() time.Time { return tc.now.Add(time.Minute) })
	decision, changed, err := service.MatchInvoice(tc.ctx, contracts.MatchCommand{InvoiceID: invoiceID, ExpectedRevision: 1, CommandID: "initial-missing-" + suffix, CorrelationID: suffix})
	if err != nil || !changed || decision.Outcome != contracts.OutcomeNoMatch {
		t.Fatalf("prepare missing decision=%+v changed=%v err=%v", decision, changed, err)
	}
	task, err := tc.store.Client.ValidationTask.Query().Where(validationtask.InvoiceIDEQ(invoiceID), validationtask.StatusEQ(validationtask.StatusOPEN)).Only(tc.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if waiting {
		taskService := validationtasks.NewService(tc.store, func() time.Time { return tc.now.Add(2 * time.Minute) })
		if _, changed, err = taskService.RequestMissingContract(tc.ctx, validationtasks.RequestMissingContractCommand{InvoiceID: invoiceID, TaskID: task.ID, ExpectedRevision: 1, CommandID: "request-" + suffix, ActorID: "accountant", ActorDisplay: "Contabil", CorrelationID: suffix}); err != nil || !changed {
			t.Fatalf("request missing contract changed=%v err=%v", changed, err)
		}
	}
	return invoiceID, task.ID
}

func TestMissingContractResumeOutcomeMatrixAndHistory(t *testing.T) {
	tests := []struct {
		name          string
		currencies    []string
		waiting       bool
		wantOutcome   contracts.MatchOutcome
		wantPipeline  invoice.PipelineStatus
		wantTaskType  validationtask.TaskType
		wantAssociate bool
	}{
		{"unique-compatible-waiting", []string{"RON"}, true, contracts.OutcomeUniqueCompatible, invoice.PipelineStatusDEDUPE_CHECKED, "", true},
		{"unique-compatible-open", []string{"RON"}, false, contracts.OutcomeUniqueCompatible, invoice.PipelineStatusDEDUPE_CHECKED, "", true},
		{"multiple-plausible", []string{"RON", "RON"}, true, contracts.OutcomeMultiplePlausible, invoice.PipelineStatusAWAITING_MATCH_CONFIRM, validationtask.TaskTypeCONTRACT_MATCH, false},
		{"unique-incompatible", []string{"EUR"}, true, contracts.OutcomeUniqueIncompatible, invoice.PipelineStatusAWAITING_MATCH_CONFIRM, validationtask.TaskTypeCONTRACT_MATCH, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tc := newContractTestContext(t)
			invoiceID, missingTaskID := tc.createMissingContractInvoice(t, test.name, "RO-RESUME", "RON", test.waiting)
			var triggerID string
			for index, currency := range test.currencies {
				id := tc.createContract(t, fmt.Sprintf("%s-%d", test.name, index), "RO-RESUME", currency)
				if triggerID == "" {
					triggerID = id
				}
			}
			service := contracts.NewService(tc.store, contracts.BaselinePolicy{}, func() time.Time { return tc.now.Add(3 * time.Minute) })
			created, err := service.ContractAvailable(tc.ctx, contracts.AvailableCommand{ContractID: triggerID, CommandID: "available-" + test.name, CorrelationID: test.name})
			if err != nil || !created {
				t.Fatalf("available created=%v err=%v", created, err)
			}
			legacyPipelineRows, err := tc.store.PendingOutbox(tc.ctx, 10, tc.now.Add(4*time.Minute))
			if err != nil || len(legacyPipelineRows) != 0 {
				t.Fatalf("contract event leaked into inline invoice dispatcher: rows=%+v err=%v", legacyPipelineRows, err)
			}
			summary, err := service.ProcessContractAvailable(tc.ctx, triggerID, "contract-available:available-"+test.name+":resume", test.name)
			if err != nil || summary.Evaluated != 1 {
				t.Fatalf("summary=%+v err=%v", summary, err)
			}
			invoiceRow, _ := tc.store.Client.Invoice.Get(tc.ctx, invoiceID)
			missingTask, _ := tc.store.Client.ValidationTask.Get(tc.ctx, missingTaskID)
			run, runErr := tc.store.Client.ContractMatchRun.Query().Where(contractmatchrun.InvoiceIDEQ(invoiceID), contractmatchrun.OutcomeEQ(contractmatchrun.Outcome(test.wantOutcome))).Order(ent.Desc(contractmatchrun.FieldCreatedAt)).First(tc.ctx)
			if runErr != nil || run.PolicyVersion != contracts.BaselinePolicyVersion || invoiceRow.PipelineStatus != test.wantPipeline || missingTask.Status != validationtask.StatusRESOLVED {
				t.Fatalf("pipeline=%s missing=%s run=%+v err=%v", invoiceRow.PipelineStatus, missingTask.Status, run, runErr)
			}
			associationCount, _ := tc.store.Client.InvoiceContractAssociation.Query().Where(invoicecontractassociation.InvoiceIDEQ(invoiceID)).Count(tc.ctx)
			activeTasks, _ := tc.store.Client.ValidationTask.Query().Where(validationtask.InvoiceIDEQ(invoiceID), validationtask.StatusNEQ(validationtask.StatusRESOLVED)).All(tc.ctx)
			if associationCount != boolCount(test.wantAssociate) || len(activeTasks) != boolCount(test.wantTaskType != "") || (len(activeTasks) == 1 && activeTasks[0].TaskType != test.wantTaskType) {
				t.Fatalf("associations=%d activeTasks=%+v", associationCount, activeTasks)
			}
			history, _ := tc.store.Client.ActivityEvent.Query().Where(activityevent.InvoiceIDEQ(invoiceID), activityevent.EventTypeIn("MISSING_CONTRACT_REEVALUATED", "MISSING_CONTRACT_RESOLVED")).Count(tc.ctx)
			if history != 2 {
				t.Fatalf("resume history count=%d", history)
			}
			if test.wantAssociate {
				continuations, _ := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(invoiceID), outboxentry.EventTypeEQ(outbox.EventInvoiceContinue)).Count(tc.ctx)
				if continuations != 1 {
					t.Fatalf("continuation outbox=%d", continuations)
				}
			}
		})
	}
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func TestMissingContractResumeNoMatchPreservesSameWaitingTask(t *testing.T) {
	tc := newContractTestContext(t)
	invoiceID, taskID := tc.createMissingContractInvoice(t, "still-missing", "RO-WANTED", "RON", true)
	service := contracts.NewService(tc.store, nil, func() time.Time { return tc.now.Add(3 * time.Minute) })
	decision, outcome, err := service.ResumeBlockedInvoice(tc.ctx, contracts.ResumeCommand{InvoiceID: invoiceID, ExpectedRevision: 2, CommandID: "irrelevant-arrival", CorrelationID: "still-missing"})
	if err != nil || decision.Outcome != contracts.OutcomeNoMatch || outcome != contracts.ResumeStillMissing {
		t.Fatalf("decision=%+v outcome=%s err=%v", decision, outcome, err)
	}
	invoiceRow, _ := tc.store.Client.Invoice.Get(tc.ctx, invoiceID)
	task, _ := tc.store.Client.ValidationTask.Get(tc.ctx, taskID)
	taskCount, _ := tc.store.Client.ValidationTask.Query().Where(validationtask.InvoiceIDEQ(invoiceID)).Count(tc.ctx)
	if invoiceRow.PipelineStatus != invoice.PipelineStatusAWAITING_CONTRACT || invoiceRow.Revision != 2 || task.Status != validationtask.StatusWAITING || task.Revision != 2 || taskCount != 1 {
		t.Fatalf("invoice=%s/%d task=%s/%d count=%d", invoiceRow.PipelineStatus, invoiceRow.Revision, task.Status, task.Revision, taskCount)
	}
}

func TestContractAvailableSelectionIsClientAndSupplierIsolated(t *testing.T) {
	first := newContractTestContext(t)
	second := newContractTestContext(t)
	firstInvoice, _ := first.createMissingContractInvoice(t, "same-client-right-supplier", "RO-ISOLATED", "RON", true)
	wrongSupplier, _ := first.createMissingContractInvoice(t, "same-client-wrong-supplier", "RO-OTHER", "RON", true)
	crossClient, _ := second.createMissingContractInvoice(t, "cross-client", "RO-ISOLATED", "RON", true)
	contractID := first.createContract(t, "ISOLATED", "RO-ISOLATED", "RON")
	service := contracts.NewService(first.store, nil, nil)
	summary, err := service.ProcessContractAvailable(first.ctx, contractID, "isolated-event", "isolated")
	if err != nil || summary.Evaluated != 1 || summary.AutomaticallyResumed != 1 {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
	for id, want := range map[string]invoice.PipelineStatus{firstInvoice: invoice.PipelineStatusDEDUPE_CHECKED, wrongSupplier: invoice.PipelineStatusAWAITING_CONTRACT, crossClient: invoice.PipelineStatusAWAITING_CONTRACT} {
		row, _ := first.store.Client.Invoice.Get(first.ctx, id)
		if row.PipelineStatus != want {
			t.Fatalf("invoice %s status=%s want=%s", id, row.PipelineStatus, want)
		}
	}
}

func TestOneAvailableContractResumesMultipleBlockedInvoicesIndependently(t *testing.T) {
	tc := newContractTestContext(t)
	first, _ := tc.createMissingContractInvoice(t, "fanout-a", "RO-FANOUT", "RON", true)
	second, _ := tc.createMissingContractInvoice(t, "fanout-b", "RO-FANOUT", "RON", false)
	contractID := tc.createContract(t, "FANOUT", "RO-FANOUT", "RON")
	service := contracts.NewService(tc.store, nil, nil)
	summary, err := service.ProcessContractAvailable(tc.ctx, contractID, "fanout-event", "fanout")
	if err != nil || summary.Evaluated != 2 || summary.AutomaticallyResumed != 2 {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
	for _, invoiceID := range []string{first, second} {
		row, _ := tc.store.Client.Invoice.Get(tc.ctx, invoiceID)
		associationCount, _ := tc.store.Client.InvoiceContractAssociation.Query().Where(invoicecontractassociation.InvoiceIDEQ(invoiceID)).Count(tc.ctx)
		if row.PipelineStatus != invoice.PipelineStatusDEDUPE_CHECKED || associationCount != 1 {
			t.Fatalf("invoice=%s status=%s associations=%d", invoiceID, row.PipelineStatus, associationCount)
		}
	}
}

func TestMissingContractResumePreservesExpiredContractProductDecision(t *testing.T) {
	tc := newContractTestContext(t)
	invoiceID, taskID := tc.createMissingContractInvoice(t, "expired-policy", "RO-EXPIRED", "RON", true)
	contractID := tc.createContract(t, "EXPIRED", "RO-EXPIRED", "RON")
	if _, err := tc.store.Client.Contract.UpdateOneID(contractID).
		SetEffectiveFrom(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)).
		SetEffectiveTo(time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	service := contracts.NewService(tc.store, nil, nil)
	_, err := service.ProcessContractAvailable(tc.ctx, contractID, "expired-event", "expired")
	if !errors.Is(err, contracts.ErrExpiredContractSemantics) {
		t.Fatalf("error=%v", err)
	}
	invoiceRow, _ := tc.store.Client.Invoice.Get(tc.ctx, invoiceID)
	task, _ := tc.store.Client.ValidationTask.Get(tc.ctx, taskID)
	if invoiceRow.PipelineStatus != invoice.PipelineStatusAWAITING_CONTRACT || task.Status != validationtask.StatusWAITING {
		t.Fatalf("invoice=%s task=%s", invoiceRow.PipelineStatus, task.Status)
	}
}

func TestContractAvailableDuplicateAndConcurrentDeliveryConverge(t *testing.T) {
	tc := newContractTestContext(t)
	invoiceID, _ := tc.createMissingContractInvoice(t, "duplicate-event", "RO-DUP-EVENT", "RON", true)
	contractID := tc.createContract(t, "DUP-EVENT", "RO-DUP-EVENT", "RON")
	service := contracts.NewService(tc.store, nil, nil)
	results := make(chan error, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := service.ProcessContractAvailable(tc.ctx, contractID, "same-available-event", "duplicate")
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
	associations, _ := tc.store.Client.InvoiceContractAssociation.Query().Where(invoicecontractassociation.InvoiceIDEQ(invoiceID)).Count(tc.ctx)
	activeTasks, _ := tc.store.Client.ValidationTask.Query().Where(validationtask.InvoiceIDEQ(invoiceID), validationtask.StatusNEQ(validationtask.StatusRESOLVED)).Count(tc.ctx)
	resumeRuns, _ := tc.store.Client.ContractMatchRun.Query().Where(contractmatchrun.InvoiceIDEQ(invoiceID), contractmatchrun.CommandKeyHasPrefix("contract-resume:")).Count(tc.ctx)
	continuations, _ := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(invoiceID), outboxentry.EventTypeEQ(outbox.EventInvoiceContinue)).Count(tc.ctx)
	if associations != 1 || activeTasks != 0 || resumeRuns != 1 || continuations != 1 {
		t.Fatalf("associations=%d active=%d runs=%d continuation=%d", associations, activeTasks, resumeRuns, continuations)
	}
}

func TestTwoAvailableContractsUseAuthoritativeSetAndCreateOneReviewTask(t *testing.T) {
	tc := newContractTestContext(t)
	invoiceID, _ := tc.createMissingContractInvoice(t, "two-contracts", "RO-TWO", "RON", true)
	contractA := tc.createContract(t, "TWO-A", "RO-TWO", "RON")
	contractB := tc.createContract(t, "TWO-B", "RO-TWO", "RON")
	service := contracts.NewService(tc.store, nil, nil)
	var group sync.WaitGroup
	for _, item := range []struct{ id, key string }{{contractA, "available-a"}, {contractB, "available-b"}} {
		group.Add(1)
		go func(contractID, key string) {
			defer group.Done()
			_, _ = service.ProcessContractAvailable(tc.ctx, contractID, key, "two-contracts")
		}(item.id, item.key)
	}
	group.Wait()
	row, _ := tc.store.Client.Invoice.Get(tc.ctx, invoiceID)
	active, _ := tc.store.Client.ValidationTask.Query().Where(validationtask.InvoiceIDEQ(invoiceID), validationtask.StatusEQ(validationtask.StatusOPEN)).WithContractMatchRun(func(query *ent.ContractMatchRunQuery) { query.WithCandidates() }).Only(tc.ctx)
	if row.PipelineStatus != invoice.PipelineStatusAWAITING_MATCH_CONFIRM || active.TaskType != validationtask.TaskTypeCONTRACT_MATCH || len(active.Edges.ContractMatchRun.Edges.Candidates) != 2 {
		t.Fatalf("invoice=%s task=%+v", row.PipelineStatus, active)
	}
}

func TestMissingContractAutomaticResumeUsesExistingPipelineContinuationBoundary(t *testing.T) {
	tc := newContractTestContext(t)
	invoiceID, _ := tc.createMissingContractInvoice(t, "pipeline-continuation", "RO-CONTINUE", "RON", true)
	contractID := tc.createContract(t, "CONTINUE", "RO-CONTINUE", "RON")
	service := contracts.NewService(tc.store, nil, nil)
	if _, err := service.ProcessContractAvailable(tc.ctx, contractID, "continue-event", "continue-trace"); err != nil {
		t.Fatal(err)
	}
	row, err := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(invoiceID), outboxentry.EventTypeEQ(outbox.EventInvoiceContinue)).Only(tc.ctx)
	if err != nil {
		t.Fatal(err)
	}
	pipeline := invoicing.NewPipelineService(tc.store, invoicing.NewFakeSagaExporter(), nil)
	outcome, err := pipeline.ProcessContinuation(tc.ctx, invoicing.OutboxEntry{ID: row.ID, EventType: row.EventType, AggregateID: row.AggregateID, IdempotencyKey: row.IdempotencyKey, CorrelationID: "continue-trace"})
	invoiceRow, _ := tc.store.Client.Invoice.Get(tc.ctx, invoiceID)
	if err != nil || outcome != invoicing.ContinuationProcessed || invoiceRow.PipelineStatus != invoice.PipelineStatusHEADER_READ {
		t.Fatalf("outcome=%s status=%s err=%v", outcome, invoiceRow.PipelineStatus, err)
	}
}
