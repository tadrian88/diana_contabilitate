//go:build integration

package postgres

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"diana-contabilitate/backend/ent/activityevent"
	"diana-contabilitate/backend/ent/classificationrule"
	"diana-contabilitate/backend/ent/lineclassification"
	"diana-contabilitate/backend/ent/ruleversion"
	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/rules"
	"diana-contabilitate/backend/internal/saga"
	entgo "entgo.io/ent"
)

// This config confirms source rate 19 only, under explicit synthetic assumptions.
// It is not a seeded Romanian fiscal rule or deductibility conclusion.
func (tc *module5TestContext) appendProductionVATFixture(t *testing.T, id string, version int, from time.Time, to *time.Time) string {
	t.Helper()
	versionID := fmt.Sprintf("%s-production-v%d", id, version)
	p := &rules.Provenance{SourceType: "LEGISLATION", SourceTitle: "Codul fiscal", Issuer: "Parlamentul României", LegalInstrument: "Legea 227/2015", Reference: "art. 291; engineering source-confirmation fixture only", SourceURL: "https://legislatie.just.ro/Public/DetaliiDocument/186620", EffectiveFrom: accountingdate.FromTime(from), VerifiedAt: "2026-09-15", VerifiedBy: "Synthetic fixture reviewer", Notes: "TEST ONLY: supplied historical rate and invoice date assumed valid; not a real fiscal rule."}
	create := tc.store.Client.RuleVersion.Create().SetID(versionID).SetRuleID(id).SetVersion(version).SetCriteria("Source rate equals 19").SetResult("19").SetExplanation("Source confirmation only under explicit fixture assumptions").SetLegalBasis("Synthetic fixture: Codul fiscal art. 291").SetMatchKind(ruleversion.MatchKindVAT_SOURCE_RATE_EQUALS).SetMatchValue("19").SetEffectiveFrom(from).SetCreatedByDisplay("Fixture reviewer").SetCommandKey("fixture:" + versionID).SetCreatedAt(tc.now).SetProductionEligible(true).SetRulePackVersion("TEST_ONLY_NO_PRODUCTION_PACK").SetProvenance(p)
	if to != nil {
		create.SetEffectiveTo(*to)
	}
	if _, err := create.Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	return versionID
}

func TestProductionDemoIsolationClassificationTaskLifecycleAndFullPipeline(t *testing.T) {
	tc := newModule5TestContext(t)
	tc.baselineRules(t)
	id := tc.createInvoice(t, "production-review", "Serviciu explicit")
	service := classification.NewService(tc.classificationStore(), nil, nil)
	result, changed, err := service.ProcessInvoice(tc.ctx, classification.ProcessCommand{InvoiceID: id, ExpectedRevision: 1, CommandID: id + ":classify"})
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	for _, p := range result.Proposals {
		if !p.RequiresReview {
			t.Fatalf("demo was accepted %+v", p)
		}
	}
	item, err := tc.store.GetInvoice(tc.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if item.PipelineStatus != invoicing.StatusAwaitingReview || item.ActiveTask == nil || len(item.ActiveTask.ClassificationItems) != 3 {
		t.Fatalf("item=%+v", item)
	}
	values := map[classification.Dimension]string{classification.DimensionAccount: "628.01", classification.DimensionVAT: "19.00", classification.DimensionDeductibility: "SAGA_DEFAULT"}
	// Accountant explicitly approves account policy and TipDeducere. No automatic
	// all-dimensions fiscal fixture is possible until deductibility is approved.
	for _, d := range classification.Dimensions {
		item, err = tc.store.GetInvoice(tc.ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		task := item.ActiveTask
		var decision classification.Decision
		for _, candidate := range task.ClassificationItems {
			if candidate.Dimension == d {
				decision = candidate
			}
		}
		value := values[d]
		if _, err = service.Review(tc.ctx, classification.ReviewCommand{InvoiceID: id, TaskID: task.ID, ClassificationID: decision.ID, ExpectedInvoiceRevision: item.Revision, ExpectedTaskRevision: task.Revision, ExpectedClassificationRevision: decision.Revision, CorrectedValue: &value, CommandID: id + ":review:" + string(d), ActorID: "fixture-accountant", ActorDisplay: "Fixture accountant"}); err != nil {
			t.Fatal(err)
		}
	}
	item, err = tc.store.GetInvoice(tc.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if item.PipelineStatus != invoicing.StatusReadyForSAGA || item.ActiveTask != nil {
		t.Fatal(item.PipelineStatus)
	}
	artifact, err := saga.Generate(item, saga.ClientIdentity{ID: tc.clientID, Name: "Fixture client", CUI: "ROFIXTURE"})
	if err != nil || len(artifact.Payload) == 0 {
		t.Fatalf("artifact=%+v err=%v", artifact, err)
	}
	for _, decision := range item.Lines[0].Classifications {
		if !decision.HumanReviewed || decision.ProposedValue == *decision.EffectiveValue {
			t.Fatalf("manual trace lost %+v", decision)
		}
	}
}

func TestProductionHistoricalProvenanceSurvivesNewVersionAndReplay(t *testing.T) {
	tc := newModule5TestContext(t)
	rule := tc.createRule(t, "VAT", classificationrule.CategoryVAT, ruleversion.MatchKindALWAYS, nil)
	versionID := tc.appendProductionVATFixture(t, rule, 2, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), nil)
	id := tc.createInvoice(t, "production-vat", "Any description")
	service := classification.NewService(tc.classificationStore(), nil, nil)
	cmd := classification.ProcessCommand{InvoiceID: id, ExpectedRevision: 1, CommandID: id + ":classify"}
	if _, _, err := service.ProcessInvoice(tc.ctx, cmd); err != nil {
		t.Fatal(err)
	}
	item, err := tc.store.GetInvoice(tc.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var original classification.Decision
	for _, d := range item.Lines[0].Classifications {
		if d.Dimension == classification.DimensionVAT {
			original = d
		}
	}
	if original.Rule == nil || original.Rule.RuleVersionID != versionID || original.Rule.Provenance == nil || original.InvoiceDateUsed != accountingdate.FromTime(tc.now) || original.HumanReviewed {
		t.Fatalf("decision=%+v", original)
	}
	tc.appendProductionVATFixture(t, rule, 3, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), nil)
	if _, changed, err := service.ProcessInvoice(tc.ctx, cmd); err != nil || changed {
		t.Fatalf("replay changed=%v err=%v", changed, err)
	}
	item, err = tc.store.GetInvoice(tc.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range item.Lines[0].Classifications {
		if d.Dimension == classification.DimensionVAT && (d.Rule.RuleVersionID != original.Rule.RuleVersionID || d.LegalBasis != original.LegalBasis || d.InvoiceDateUsed != original.InvoiceDateUsed || d.Rule.EffectiveFrom != original.Rule.EffectiveFrom) {
			t.Fatalf("history changed %+v", d)
		}
	}
	count, err := tc.store.Client.ActivityEvent.Query().Where(activityevent.InvoiceIDEQ(id), activityevent.EventTypeEQ("AUTOMATED_CLASSIFICATION_COMPLETED")).Count(tc.ctx)
	if err != nil || count != 1 {
		t.Fatalf("events=%d err=%v", count, err)
	}
}

func TestProductionRuleVersionsImmutableAndProvenanceConstraint(t *testing.T) {
	tc := newModule5TestContext(t)
	id := tc.createRule(t, "VAT", classificationrule.CategoryVAT, ruleversion.MatchKindALWAYS, nil)
	if _, err := tc.store.DB.ExecContext(tc.ctx, "UPDATE rule_versions SET result = $1 WHERE rule_id = $2", "21", id); err == nil {
		t.Fatal("historical version rewritten")
	}
	if _, err := tc.store.Client.RuleVersion.Create().SetID(id + "-unsafe").SetRuleID(id).SetVersion(2).SetCriteria("Unsafe").SetResult("21").SetExplanation("Unsafe").SetLegalBasis(rules.LegalBasisPlaceholder).SetMatchKind(ruleversion.MatchKindALWAYS).SetEffectiveFrom(tc.now).SetCreatedByDisplay("Fixture").SetCommandKey(id + ":unsafe").SetCreatedAt(tc.now).SetProductionEligible(true).Save(tc.ctx); err == nil {
		t.Fatal("eligible without evidence")
	}
	tc.appendProductionVATFixture(t, id, 2, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), nil)
	row, err := tc.store.Client.RuleVersion.Query().Where(ruleversion.IDEQ(id + "-v1")).Only(tc.ctx)
	if err != nil || row.ProductionEligible || row.Result != "Demo VAT" {
		t.Fatalf("old row=%+v err=%v", row, err)
	}
}

func TestProductionConcurrentRuleUpdateUsesConsistentSnapshot(t *testing.T) {
	tc := newModule5TestContext(t)
	id := tc.createRule(t, "VAT", classificationrule.CategoryVAT, ruleversion.MatchKindALWAYS, nil)
	first := tc.appendProductionVATFixture(t, id, 2, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), nil)
	invoiceID := tc.createInvoice(t, "snapshot", "Line 1", "Line 2")
	established := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	tc.store.Client.Invoice.Intercept(entgo.InterceptFunc(func(next entgo.Querier) entgo.Querier {
		return entgo.QuerierFunc(func(ctx context.Context, q entgo.Query) (entgo.Value, error) {
			value, err := next.Query(ctx, q)
			once.Do(func() { close(established); <-release })
			return value, err
		})
	}))
	type outcome struct {
		input classification.InvoiceContext
		err   error
	}
	done := make(chan outcome, 1)
	go func() {
		input, err := tc.classificationStore().LoadClassificationInput(tc.ctx, invoiceID)
		done <- outcome{input, err}
	}()
	select {
	case <-established:
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("snapshot was not established")
	}
	// Release even if the writer fails, so cleanup never hangs behind interceptor.
	func() {
		defer close(release)
		tc.appendProductionVATFixture(t, id, 3, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), nil)
	}()
	var loaded outcome
	select {
	case loaded = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("snapshot did not complete")
	}
	if loaded.err != nil {
		t.Fatal(loaded.err)
	}
	if len(loaded.input.Rules) != 2 {
		t.Fatalf("mixed/new snapshot contains %d versions", len(loaded.input.Rules))
	}
	result, err := (classification.ProductionPolicy{}).Evaluate(loaded.input)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range result.Proposals {
		if p.Dimension == classification.DimensionVAT && (p.Rule == nil || p.Rule.RuleVersionID != first) {
			t.Fatalf("mixed version=%+v", p)
		}
	}
}

func TestProductionOverlappingVersionsRouteToReview(t *testing.T) {
	tc := newModule5TestContext(t)
	id := tc.createRule(t, "VAT", classificationrule.CategoryVAT, ruleversion.MatchKindALWAYS, nil)
	for _, version := range []int{2, 3} {
		tc.appendProductionVATFixture(t, id, version, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), nil)
	}
	invoiceID := tc.createInvoice(t, "overlap", "source")
	service := classification.NewService(tc.classificationStore(), nil, nil)
	if _, _, err := service.ProcessInvoice(tc.ctx, classification.ProcessCommand{InvoiceID: invoiceID, ExpectedRevision: 1, CommandID: invoiceID + ":classify"}); err != nil {
		t.Fatal(err)
	}
	row, err := tc.store.Client.LineClassification.Query().Where(lineclassification.InvoiceIDEQ(invoiceID), lineclassification.DimensionEQ(lineclassification.DimensionVAT)).Only(tc.ctx)
	if err != nil || row.ReviewStatus != lineclassification.ReviewStatusPENDING || row.Source != lineclassification.SourceAMBIGUOUS {
		t.Fatalf("row=%+v err=%v", row, err)
	}
}

func TestProductionVerifiedVATAndManualAccountingReachSAGABoundary(t *testing.T) {
	tc := newModule5TestContext(t)
	rule := tc.createRule(t, "pipeline-VAT", classificationrule.CategoryVAT, ruleversion.MatchKindALWAYS, nil)
	version := tc.appendProductionVATFixture(t, rule, 2, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), nil)
	id := tc.createInvoice(t, "verified-pipeline", "Source-rate fixture with accountant-assumed account policy")
	service := classification.NewService(tc.classificationStore(), nil, nil)
	if _, _, err := service.ProcessInvoice(tc.ctx, classification.ProcessCommand{InvoiceID: id, ExpectedRevision: 1, CommandID: id + ":classify"}); err != nil {
		t.Fatal(err)
	}
	for _, dimension := range []classification.Dimension{classification.DimensionAccount, classification.DimensionDeductibility} {
		item, err := tc.store.GetInvoice(tc.ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if item.ActiveTask == nil || len(item.ActiveTask.ClassificationItems) != 2 {
			t.Fatal("expected only two human accounting decisions")
		}
		var decision classification.Decision
		for _, candidate := range item.ActiveTask.ClassificationItems {
			if candidate.Dimension == dimension {
				decision = candidate
			}
		}
		value := "628.01"
		if dimension == classification.DimensionDeductibility {
			value = "SAGA_DEFAULT"
		}
		if _, err = service.Review(tc.ctx, classification.ReviewCommand{InvoiceID: id, TaskID: item.ActiveTask.ID, ClassificationID: decision.ID, ExpectedInvoiceRevision: item.Revision, ExpectedTaskRevision: item.ActiveTask.Revision, ExpectedClassificationRevision: decision.Revision, CorrectedValue: &value, CommandID: id + ":accountant:" + string(dimension), ActorID: "fixture-accountant", ActorDisplay: "Fixture accountant"}); err != nil {
			t.Fatal(err)
		}
	}
	item, err := tc.store.GetInvoice(tc.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if item.PipelineStatus != invoicing.StatusReadyForSAGA {
		t.Fatal(item.PipelineStatus)
	}
	artifact, err := saga.Generate(item, saga.ClientIdentity{ID: tc.clientID, Name: "Fixture client", CUI: "ROFIXTURE"})
	if err != nil || len(artifact.Payload) == 0 {
		t.Fatalf("err=%v", err)
	}
	for _, decision := range item.Lines[0].Classifications {
		if decision.Dimension == classification.DimensionVAT && (decision.Rule == nil || decision.Rule.RuleVersionID != version || decision.HumanReviewed) {
			t.Fatalf("automatic version trace lost %+v", decision)
		}
	}
}
