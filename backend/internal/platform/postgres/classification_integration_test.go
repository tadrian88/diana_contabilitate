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
	"diana-contabilitate/backend/ent/classificationrule"
	"diana-contabilitate/backend/ent/invoice"
	"diana-contabilitate/backend/ent/invoiceline"
	"diana-contabilitate/backend/ent/lineclassification"
	"diana-contabilitate/backend/ent/outboxentry"
	"diana-contabilitate/backend/ent/ruleversion"
	"diana-contabilitate/backend/ent/validationtask"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/rules"
	"diana-contabilitate/backend/internal/validationtasks"
)

var classificationTestSequence atomic.Uint64

type module5TestContext struct {
	ctx      context.Context
	store    *Store
	clientID string
	now      time.Time
	invoices []string
	rules    []string
}

// classificationStore keeps each integration test independent from the
// demonstration rules that may already exist in a shared development database.
type classificationStore struct {
	*Store
	ruleIDs map[string]struct{}
}

func (s classificationStore) LoadClassificationInput(ctx context.Context, invoiceID string) (classification.InvoiceContext, error) {
	input, err := s.Store.LoadClassificationInput(ctx, invoiceID)
	if err != nil {
		return classification.InvoiceContext{}, err
	}
	filtered := input.Rules[:0]
	for _, rule := range input.Rules {
		if _, allowed := s.ruleIDs[rule.RuleID]; allowed {
			filtered = append(filtered, rule)
		}
	}
	input.Rules = filtered
	return input, nil
}

func newModule5TestContext(t *testing.T) *module5TestContext {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	store, err := Open(url)
	if err != nil {
		t.Fatal(err)
	}
	sequence := classificationTestSequence.Add(1)
	tc := &module5TestContext{ctx: context.Background(), store: store, clientID: fmt.Sprintf("module5-client-%d", sequence), now: time.Date(2026, 9, 20, 9, int(sequence), 0, 0, time.UTC)}
	if _, err = store.Client.AccountingClient.Create().SetID(tc.clientID).SetName("Module 5 client").SetCui(fmt.Sprintf("RO-M5-%d", sequence)).SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if len(tc.invoices) > 0 {
			_, _ = store.Client.ActivityEvent.Delete().Where(activityevent.InvoiceIDIn(tc.invoices...)).Exec(tc.ctx)
			_, _ = store.Client.OutboxEntry.Delete().Where(outboxentry.AggregateIDIn(tc.invoices...)).Exec(tc.ctx)
			_, _ = store.Client.ValidationTask.Delete().Where(validationtask.InvoiceIDIn(tc.invoices...)).Exec(tc.ctx)
			_, _ = store.Client.LineClassification.Delete().Where(lineclassification.InvoiceIDIn(tc.invoices...)).Exec(tc.ctx)
			_, _ = store.Client.InvoiceLine.Delete().Where(invoiceline.InvoiceIDIn(tc.invoices...)).Exec(tc.ctx)
			_, _ = store.Client.Invoice.Delete().Where(invoice.IDIn(tc.invoices...)).Exec(tc.ctx)
		}
		if len(tc.rules) > 0 {
			_, _ = store.Client.ActivityEvent.Delete().Where(activityevent.AggregateIDIn(tc.rules...)).Exec(tc.ctx)
			_, _ = store.Client.RuleVersion.Delete().Where(ruleversion.RuleIDIn(tc.rules...)).Exec(tc.ctx)
			_, _ = store.Client.ClassificationRule.Delete().Where(classificationrule.IDIn(tc.rules...)).Exec(tc.ctx)
		}
		_ = store.Client.AccountingClient.DeleteOneID(tc.clientID).Exec(tc.ctx)
		_ = store.Close()
	})
	return tc
}

func (tc *module5TestContext) createRule(t *testing.T, suffix string, category classificationrule.Category, kind ruleversion.MatchKind, match *string) string {
	t.Helper()
	id := fmt.Sprintf("m5-rule-%d-%s", classificationTestSequence.Load(), suffix)
	reference := fmt.Sprintf("M5-%d-%s", classificationTestSequence.Load(), suffix)
	if _, err := tc.store.Client.ClassificationRule.Create().SetID(id).SetReference(reference).SetName("Demonstrative " + suffix).SetCategory(category).SetScope(classificationrule.ScopeGLOBAL).SetCreationKey("fixture:" + id).SetRevision(1).SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	create := tc.store.Client.RuleVersion.Create().SetID(id + "-v1").SetRuleID(id).SetVersion(1).SetCriteria("Demonstrative criteria").SetResult("Demo " + suffix).SetExplanation("Demonstrative only").SetLegalBasis(rules.LegalBasisPlaceholder).SetMatchKind(kind).SetEffectiveFrom(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)).SetCreatedByDisplay("Test").SetCommandKey("fixture:" + id + ":v1").SetCreatedAt(tc.now)
	if match != nil {
		create.SetMatchValue(*match)
	}
	if _, err := create.Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	tc.rules = append(tc.rules, id)
	return id
}

func (tc *module5TestContext) baselineRules(t *testing.T) {
	t.Helper()
	value := "serviciu"
	tc.createRule(t, "ACCOUNT", classificationrule.CategoryACCOUNT, ruleversion.MatchKindDESCRIPTION_CONTAINS, &value)
	tc.createRule(t, "VAT", classificationrule.CategoryVAT, ruleversion.MatchKindALWAYS, nil)
	tc.createRule(t, "DEDUCT", classificationrule.CategoryDEDUCTIBILITY, ruleversion.MatchKindDESCRIPTION_CONTAINS, &value)
}

func (tc *module5TestContext) createInvoice(t *testing.T, suffix string, descriptions ...string) string {
	t.Helper()
	id := fmt.Sprintf("m5-invoice-%d-%s", classificationTestSequence.Load(), suffix)
	if _, err := tc.store.Client.Invoice.Create().SetID(id).SetClientID(tc.clientID).SetSupplierName("Demo supplier").SetSupplierCui("RO-M5-SUPPLIER").SetNormalizedSupplierCui("RO-M5-SUPPLIER").SetDocumentNumber("INV-" + suffix).SetNormalizedDocumentNumber("INV-" + suffix).SetIssueDate(tc.now).SetIssueDay(time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)).SetTotalAmount("119.0000").SetCurrency("RON").SetSpvReference("SPV-" + id).SetIngestionSource("TEST").SetExternalDeliveryID("DELIVERY-" + id).SetPipelineStatus(invoice.PipelineStatusLINES_READ).SetSagaStatus(invoice.SagaStatusNOT_READY).SetRevision(1).SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	for index, description := range descriptions {
		position := index + 1
		if _, err := tc.store.Client.InvoiceLine.Create().SetID(fmt.Sprintf("%s-line-%d", id, position)).SetInvoiceID(id).SetPosition(position).SetDescription(description).SetUnit("BUC").SetVatRate("19.0000").SetVatValue("19.0000").SetQuantity("1.0000").SetUnitPrice("100.0000").SetNetValue("100.0000").SetTotalValue("119.0000").Save(tc.ctx); err != nil {
			t.Fatal(err)
		}
	}
	tc.invoices = append(tc.invoices, id)
	return id
}

func (tc *module5TestContext) classificationStore() classificationStore {
	ruleIDs := make(map[string]struct{}, len(tc.rules))
	for _, ruleID := range tc.rules {
		ruleIDs[ruleID] = struct{}{}
	}
	return classificationStore{Store: tc.store, ruleIDs: ruleIDs}
}

func (tc *module5TestContext) process(t *testing.T, invoiceID, key string) {
	t.Helper()
	service := classification.NewService(tc.classificationStore(), classification.BaselinePolicy{}, func() time.Time { return tc.now.Add(time.Minute) })
	if _, changed, err := service.ProcessInvoice(tc.ctx, classification.ProcessCommand{InvoiceID: invoiceID, ExpectedRevision: 1, CommandID: key}); err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
}

func TestAutomaticClassificationPersistsThreeDimensionsAndHistoricalRuleVersion(t *testing.T) {
	tc := newModule5TestContext(t)
	tc.baselineRules(t)
	invoiceID := tc.createInvoice(t, "auto", "Serviciu demonstrativ")
	tc.process(t, invoiceID, "auto")
	item, err := tc.store.GetInvoice(tc.ctx, invoiceID)
	if err != nil || item.PipelineStatus != "READY_FOR_SAGA" || item.SagaStatus != "READY" || len(item.Lines) != 1 || len(item.Lines[0].Classifications) != 3 || item.ActiveTask != nil {
		t.Fatalf("invoice=%+v err=%v", item, err)
	}
	for _, decision := range item.Lines[0].Classifications {
		if decision.Status != classification.ReviewAccepted || decision.Rule == nil || decision.LegalBasis != rules.LegalBasisPlaceholder {
			t.Fatalf("decision=%+v", decision)
		}
	}
	if count, _ := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(invoiceID)).Count(tc.ctx); count != 1 {
		t.Fatalf("outbox=%d", count)
	}
	if count, _ := tc.store.Client.ActivityEvent.Query().Where(activityevent.InvoiceIDEQ(invoiceID), activityevent.EventTypeIn("AUTOMATED_CLASSIFICATION_COMPLETED", "CLASSIFICATION_ROUTED")).Count(tc.ctx); count != 2 {
		t.Fatalf("classification audit=%d", count)
	}
	accountRule := tc.rules[0]
	ruleService := rules.NewService(tc.store, func() time.Time { return tc.now.Add(time.Hour) })
	created, changed, err := ruleService.CreateVersion(tc.ctx, rules.CreateVersionCommand{RuleID: accountRule, ExpectedRevision: 1, Criteria: "New display criteria", Result: "New demo result", EffectiveFrom: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), CommandID: "new-version", ActorDisplay: "Accountant"})
	if err != nil || !changed || len(created.Versions) != 2 {
		t.Fatalf("rule=%+v changed=%v err=%v", created, changed, err)
	}
	item, _ = tc.store.GetInvoice(tc.ctx, invoiceID)
	if item.Lines[0].Classifications[0].Rule.Version != 1 {
		t.Fatalf("historical reference=%+v", item.Lines[0].Classifications[0].Rule)
	}
}

func TestPipelineOutboxDispatchInvokesClassificationProcessor(t *testing.T) {
	tc := newModule5TestContext(t)
	tc.baselineRules(t)
	invoiceID := tc.createInvoice(t, "pipeline", "Serviciu demonstrativ")
	if _, err := tc.store.Client.Invoice.UpdateOneID(invoiceID).SetPipelineStatus(invoice.PipelineStatusHEADER_READ).SetRevision(1).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := tc.store.Client.OutboxEntry.Create().SetID("m5-pipeline-outbox").SetEventType("INVOICE_CONTINUE").SetAggregateType("INVOICE").SetAggregateID(invoiceID).SetPayload([]byte(`{"invoice_id":"` + invoiceID + `"}`)).SetIdempotencyKey("m5-pipeline").SetStatus(outboxentry.StatusPENDING).SetCreatedAt(tc.now).SetAvailableAt(tc.now).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	classifier := classification.NewService(tc.classificationStore(), classification.BaselinePolicy{}, func() time.Time { return tc.now.Add(time.Minute) })
	pipeline := invoicing.NewPipelineService(scopedPipelineStore{Store: tc.store, aggregateID: invoiceID}, invoicing.NewFakeSagaExporter(), func() time.Time { return tc.now.Add(time.Minute) })
	pipeline.SetClassificationProcessor(classifier)
	if count, err := pipeline.DispatchPending(tc.ctx, 1); err != nil || count != 1 {
		t.Fatalf("lines dispatch count=%d err=%v", count, err)
	}
	if count, err := pipeline.DispatchPending(tc.ctx, 1); err != nil || count != 1 {
		t.Fatalf("classification dispatch count=%d err=%v", count, err)
	}
	item, _ := tc.store.GetInvoice(tc.ctx, invoiceID)
	if item.PipelineStatus != invoicing.StatusReadyForSAGA {
		t.Fatalf("status=%s", item.PipelineStatus)
	}
}

func TestClassificationReviewGroupsItemsSupportsPartialCorrectionAndFinalResolution(t *testing.T) {
	tc := newModule5TestContext(t)
	tc.baselineRules(t)
	invoiceID := tc.createInvoice(t, "review", "Necunoscut unu", "Necunoscut doi")
	tc.process(t, invoiceID, "review")
	item, _ := tc.store.GetInvoice(tc.ctx, invoiceID)
	if item.PipelineStatus != "AWAITING_REVIEW" || item.ActiveTask == nil || item.ActiveTask.Type != validationtasks.TypeClassification || len(item.ActiveTask.ClassificationItems) != 4 {
		t.Fatalf("invoice=%+v", item)
	}
	if count, _ := tc.store.Client.ValidationTask.Query().Where(validationtask.InvoiceIDEQ(invoiceID), validationtask.TaskTypeEQ(validationtask.TaskTypeCLASSIFICATION)).Count(tc.ctx); count != 1 {
		t.Fatalf("classification tasks=%d", count)
	}
	ruleCountBefore, _ := tc.store.Client.ClassificationRule.Query().Count(tc.ctx)
	versionCountBefore, _ := tc.store.Client.RuleVersion.Query().Count(tc.ctx)
	service := classification.NewService(tc.store, classification.BaselinePolicy{}, func() time.Time { return tc.now.Add(2 * time.Minute) })
	first := item.ActiveTask.ClassificationItems[0]
	corrected := "Valoare demonstrativă corectată"
	command := classification.ReviewCommand{InvoiceID: invoiceID, TaskID: item.ActiveTask.ID, ClassificationID: first.ID, ExpectedInvoiceRevision: item.Revision, ExpectedTaskRevision: item.ActiveTask.Revision, ExpectedClassificationRevision: first.Revision, CorrectedValue: &corrected, CommandID: "partial", ActorDisplay: "Accountant"}
	if changed, err := service.Review(tc.ctx, command); err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	partial, _ := tc.store.GetInvoice(tc.ctx, invoiceID)
	if partial.PipelineStatus != "AWAITING_REVIEW" || partial.ActiveTask == nil || partial.ActiveTask.Revision != 2 {
		t.Fatalf("partial=%+v", partial)
	}
	if count, _ := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(invoiceID)).Count(tc.ctx); count != 0 {
		t.Fatalf("partial review outbox=%d", count)
	}
	if replay, err := service.Review(tc.ctx, command); err != nil || replay {
		t.Fatalf("replay=%v err=%v", replay, err)
	}
	for {
		current, _ := tc.store.GetInvoice(tc.ctx, invoiceID)
		if current.ActiveTask == nil {
			break
		}
		var pending *classification.Decision
		for index := range current.ActiveTask.ClassificationItems {
			if current.ActiveTask.ClassificationItems[index].Status == classification.ReviewPending {
				pending = &current.ActiveTask.ClassificationItems[index]
				break
			}
		}
		if pending == nil {
			t.Fatal("task open without pending item")
		}
		finalCommand := classification.ReviewCommand{InvoiceID: invoiceID, TaskID: current.ActiveTask.ID, ClassificationID: pending.ID, ExpectedInvoiceRevision: current.Revision, ExpectedTaskRevision: current.ActiveTask.Revision, ExpectedClassificationRevision: pending.Revision, CommandID: "resolve-" + pending.ID, ActorDisplay: "Accountant"}
		if changed, err := service.Review(tc.ctx, finalCommand); err != nil || !changed {
			t.Fatalf("changed=%v err=%v", changed, err)
		}
	}
	final, _ := tc.store.GetInvoice(tc.ctx, invoiceID)
	if final.PipelineStatus != "READY_FOR_SAGA" || final.SagaStatus != "READY" {
		t.Fatalf("final=%+v", final)
	}
	if count, _ := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(invoiceID)).Count(tc.ctx); count != 1 {
		t.Fatalf("final review outbox=%d", count)
	}
	if count, _ := tc.store.Client.ActivityEvent.Query().Where(activityevent.InvoiceIDEQ(invoiceID), activityevent.EventTypeEQ("CLASSIFICATION_CORRECTED")).Count(tc.ctx); count != 1 {
		t.Fatalf("correction audit=%d", count)
	}
	ruleCountAfter, _ := tc.store.Client.ClassificationRule.Query().Count(tc.ctx)
	versionCountAfter, _ := tc.store.Client.RuleVersion.Query().Count(tc.ctx)
	if ruleCountAfter != ruleCountBefore || versionCountAfter != versionCountBefore {
		t.Fatalf("correction learned: rules %d/%d versions %d/%d", ruleCountBefore, ruleCountAfter, versionCountBefore, versionCountAfter)
	}
}

func TestConcurrentFinalClassificationReviewHasOneWinner(t *testing.T) {
	tc := newModule5TestContext(t)
	// Only VAT matches, leaving ACCOUNT and DEDUCTIBILITY pending; resolve one first.
	tc.createRule(t, "VAT", classificationrule.CategoryVAT, ruleversion.MatchKindALWAYS, nil)
	invoiceID := tc.createInvoice(t, "race", "Unknown")
	tc.process(t, invoiceID, "race")
	service := classification.NewService(tc.store, classification.BaselinePolicy{}, nil)
	item, _ := tc.store.GetInvoice(tc.ctx, invoiceID)
	first := item.ActiveTask.ClassificationItems[0]
	if _, err := service.Review(tc.ctx, classification.ReviewCommand{InvoiceID: invoiceID, TaskID: item.ActiveTask.ID, ClassificationID: first.ID, ExpectedInvoiceRevision: item.Revision, ExpectedTaskRevision: 1, ExpectedClassificationRevision: 1, CommandID: "prepare-race", ActorDisplay: "Accountant"}); err != nil {
		t.Fatal(err)
	}
	item, _ = tc.store.GetInvoice(tc.ctx, invoiceID)
	last := item.ActiveTask.ClassificationItems[1]
	var winners atomic.Int32
	results := make(chan error, 2)
	var group sync.WaitGroup
	for _, key := range []string{"race-a", "race-b"} {
		group.Add(1)
		go func(key string) {
			defer group.Done()
			changed, err := service.Review(tc.ctx, classification.ReviewCommand{InvoiceID: invoiceID, TaskID: item.ActiveTask.ID, ClassificationID: last.ID, ExpectedInvoiceRevision: item.Revision, ExpectedTaskRevision: item.ActiveTask.Revision, ExpectedClassificationRevision: last.Revision, CommandID: key, ActorDisplay: key})
			if changed {
				winners.Add(1)
			}
			results <- err
		}(key)
	}
	group.Wait()
	close(results)
	losers := 0
	for err := range results {
		if errors.Is(err, classification.ErrStaleReview) || errors.Is(err, validationtasks.ErrTaskAlreadyResolved) {
			losers++
		} else if err != nil {
			t.Fatal(err)
		}
	}
	if winners.Load() != 1 || losers != 1 {
		t.Fatalf("winners=%d losers=%d", winners.Load(), losers)
	}
	if outbox, _ := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(invoiceID)).Count(tc.ctx); outbox != 1 {
		t.Fatalf("outbox=%d", outbox)
	}
}

func TestConcurrentRuleVersionAndOverrideCreationAreSafe(t *testing.T) {
	tc := newModule5TestContext(t)
	parent := tc.createRule(t, "PARENT", classificationrule.CategoryACCOUNT, ruleversion.MatchKindNO_AUTOMATION, nil)
	service := rules.NewService(tc.store, nil)
	var versionWinners atomic.Int32
	results := make(chan error, 2)
	var group sync.WaitGroup
	for _, key := range []string{"version-a", "version-b"} {
		group.Add(1)
		go func(key string) {
			defer group.Done()
			_, changed, err := service.CreateVersion(tc.ctx, rules.CreateVersionCommand{RuleID: parent, ExpectedRevision: 1, Criteria: key, Result: "demo", EffectiveFrom: tc.now, CommandID: key, ActorDisplay: key})
			if changed {
				versionWinners.Add(1)
			}
			results <- err
		}(key)
	}
	group.Wait()
	close(results)
	versionLosers := 0
	for err := range results {
		if errors.Is(err, rules.ErrRuleVersionStale) {
			versionLosers++
		} else if err != nil {
			t.Fatal(err)
		}
	}
	if versionWinners.Load() != 1 || versionLosers != 1 {
		t.Fatalf("version winners=%d losers=%d", versionWinners.Load(), versionLosers)
	}
	results = make(chan error, 2)
	var overrideWinners atomic.Int32
	group = sync.WaitGroup{}
	tc.rules = append(tc.rules, stableID("rule", parent+":"+tc.clientID))
	for _, key := range []string{"override-a", "override-b"} {
		group.Add(1)
		go func(key string) {
			defer group.Done()
			_, changed, err := service.CreateOverride(tc.ctx, rules.CreateOverrideCommand{ParentRuleID: parent, ClientID: tc.clientID, Criteria: key, Result: "demo", EffectiveFrom: tc.now, CommandID: key, ActorDisplay: key})
			if changed {
				overrideWinners.Add(1)
			}
			results <- err
		}(key)
	}
	group.Wait()
	close(results)
	overrideLosers := 0
	for err := range results {
		if errors.Is(err, rules.ErrOverrideAlreadyExists) || errors.Is(err, apperrors.ErrConflict) {
			overrideLosers++
		} else if err != nil {
			t.Fatal(err)
		}
	}
	if overrideWinners.Load() != 1 || overrideLosers != 1 {
		t.Fatalf("override winners=%d losers=%d", overrideWinners.Load(), overrideLosers)
	}
}

func TestClientOverrideIsScopedAndKeepsIndependentHistory(t *testing.T) {
	tc := newModule5TestContext(t)
	parent := tc.createRule(t, "SCOPED-PARENT", classificationrule.CategoryACCOUNT, ruleversion.MatchKindNO_AUTOMATION, nil)
	otherClient := fmt.Sprintf("module5-override-other-%d", classificationTestSequence.Load())
	if _, err := tc.store.Client.AccountingClient.Create().SetID(otherClient).SetName("Other override client").SetCui("RO-OVERRIDE-" + otherClient).SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tc.store.Client.AccountingClient.DeleteOneID(otherClient).Exec(tc.ctx) })
	service := rules.NewService(tc.store, nil)
	override, changed, err := service.CreateOverride(tc.ctx, rules.CreateOverrideCommand{ParentRuleID: parent, ClientID: tc.clientID, Criteria: "Criteriu override demo", Result: "Rezultat override demo", EffectiveFrom: tc.now, CommandID: "scoped-override", ActorDisplay: "Accountant"})
	if err != nil || !changed || override.Scope != rules.ScopeClientOverride || override.ParentRuleID == nil || *override.ParentRuleID != parent || len(override.Versions) != 1 {
		t.Fatalf("override=%+v changed=%v err=%v", override, changed, err)
	}
	tc.rules = append(tc.rules, override.ID)
	for _, testCase := range []struct {
		clientID       string
		expectOverride bool
	}{
		{clientID: tc.clientID, expectOverride: true},
		{clientID: otherClient, expectOverride: false},
	} {
		items, listErr := service.List(tc.ctx, rules.Filter{ClientID: testCase.clientID})
		if listErr != nil {
			t.Fatal(listErr)
		}
		seenParent, seenOverride := false, false
		for _, item := range items {
			seenParent = seenParent || item.ID == parent
			seenOverride = seenOverride || item.ID == override.ID
		}
		if !seenParent || seenOverride != testCase.expectOverride {
			t.Fatalf("client=%s parent=%v override=%v", testCase.clientID, seenParent, seenOverride)
		}
	}
	global, err := service.Get(tc.ctx, parent)
	if err != nil || global.Scope != rules.ScopeGlobal || len(global.Versions) != 1 {
		t.Fatalf("global=%+v err=%v", global, err)
	}
}

func TestModule5ForeignKeysRejectCrossClientClassificationAndNonGlobalOverrideOrigin(t *testing.T) {
	tc := newModule5TestContext(t)
	parent := tc.createRule(t, "VAT-PARENT", classificationrule.CategoryVAT, ruleversion.MatchKindALWAYS, nil)
	invoiceID := tc.createInvoice(t, "cross-client", "Demo")
	otherClient := fmt.Sprintf("module5-other-client-%d", classificationTestSequence.Load())
	if _, err := tc.store.Client.AccountingClient.Create().SetID(otherClient).SetName("Other").SetCui("RO-OTHER-" + otherClient).SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tc.store.Client.AccountingClient.DeleteOneID(otherClient).Exec(tc.ctx) })
	_, err := tc.store.Client.LineClassification.Create().SetID("cross-client-classification").SetClientID(otherClient).SetInvoiceID(invoiceID).SetInvoiceLineID(invoiceID + "-line-1").SetDimension(lineclassification.DimensionACCOUNT).SetProposedValue("Demo").SetConfidenceDisplay("Demo").SetExplanation("Demo").SetLegalBasis(rules.LegalBasisPlaceholder).SetRequiredReview(true).SetReviewStatus(lineclassification.ReviewStatusPENDING).SetSource(lineclassification.SourceNO_MATCH).SetPolicyVersion(classification.BaselinePolicyVersion).SetRevision(1).SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx)
	if err == nil {
		t.Fatal("cross-client classification should violate composite invoice ownership")
	}
	overrideID := "invalid-parent-override"
	tc.rules = append(tc.rules, overrideID)
	_, err = tc.store.Client.ClassificationRule.Create().SetID(overrideID).SetReference("INVALID-OVERRIDE").SetName("Invalid").SetCategory(classificationrule.CategoryACCOUNT).SetScope(classificationrule.ScopeCLIENT_OVERRIDE).SetClientID(tc.clientID).SetParentRuleID(parent).SetParentScope(classificationrule.ParentScopeGLOBAL).SetCreationKey("invalid-parent-override").SetRevision(1).SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx)
	if err == nil {
		t.Fatal("override category must match its global parent")
	}
}
