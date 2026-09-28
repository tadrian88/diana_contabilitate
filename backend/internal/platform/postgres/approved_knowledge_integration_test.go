//go:build integration

package postgres

import (
	"testing"
	"time"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/classification"
)

func TestApprovedKnowledgeExplicitPromoteReuseAndRevoke(t *testing.T) {
	tc := newModule5TestContext(t)
	facts, lineFacts, _, _ := domainReleaseFixture(t, tc, func(pack *accounting.Pack) {
		for i := range pack.Rules {
			pack.Rules[i].Predicate.SellerItemID = "NON_MATCHING_CHAPTER_E"
		}
	})
	service := classification.NewService(tc.store, classification.DomainPolicy{AllowTestOnly: true}, func() time.Time { return tc.now })
	create := func(label string) string {
		id := domainPersistedInvoice(t, tc, facts, lineFacts, "chapter-e-"+label)
		return id
	}
	first := create("first")
	if _, _, err := service.ProcessInvoice(tc.ctx, classification.ProcessCommand{InvoiceID: first, ExpectedRevision: 1, CommandID: first + ":classify"}); err != nil {
		t.Fatal(err)
	}
	var automaticCount int
	if err := tc.store.DB.QueryRowContext(tc.ctx, `SELECT count(*) FROM approved_accounting_knowledge WHERE client_id=$1 AND dimension IS NOT NULL`, tc.clientID).Scan(&automaticCount); err != nil || automaticCount != 0 {
		t.Fatalf("approval knowledge must start empty: %d %v", automaticCount, err)
	}
	invoice, err := tc.store.GetInvoice(tc.ctx, first)
	if err != nil || invoice.ActiveTask == nil {
		t.Fatalf("missing review: %#v %v", invoice, err)
	}
	var decision classification.Decision
	for _, candidate := range invoice.ActiveTask.ClassificationItems {
		if candidate.Dimension == classification.DimensionVATDeductibility {
			decision = candidate
			break
		}
	}
	value := accounting.Value{Kind: "FULL"}
	if _, err = service.Review(tc.ctx, classification.ReviewCommand{Action: "EDIT", InvoiceID: first, TaskID: invoice.ActiveTask.ID, ClassificationID: decision.ID, ExpectedInvoiceRevision: invoice.Revision, ExpectedTaskRevision: invoice.ActiveTask.Revision, ExpectedClassificationRevision: decision.Revision, TypedValue: &value, Reason: "TEST_ONLY final accountant decision", MappingAction: "NONE", CommandID: first + ":review", ActorID: "accountant", ActorDisplay: "TEST accountant"}); err != nil {
		t.Fatal(err)
	}
	invoice, err = tc.store.GetInvoice(tc.ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	var final classification.Decision
	for _, candidate := range invoice.Lines[0].Classifications {
		if candidate.Dimension == classification.DimensionVATDeductibility {
			final = candidate
			break
		}
	}
	if invoice.CurrentClassificationRunID == nil || !final.HumanReviewed {
		t.Fatalf("decision is not final/current: %#v", final)
	}
	item, created, err := service.PromoteKnowledge(tc.ctx, classification.PromoteKnowledgeCommand{ClientID: tc.clientID, InvoiceID: first, ClassificationID: final.ID, ExpectedClassificationRevision: final.Revision, ExpectedInvoiceRevision: invoice.Revision, ExpectedClassificationRunID: *invoice.CurrentClassificationRunID, CommandID: first + ":promote", ActorID: "accountant", ActorDisplay: "TEST accountant"})
	if err != nil || !created || item.Status != "ACTIVE" {
		t.Fatalf("promotion failed: %#v %v", item, err)
	}
	second := create("second")
	result, _, err := service.ProcessInvoice(tc.ctx, classification.ProcessCommand{InvoiceID: second, ExpectedRevision: 1, CommandID: second + ":classify"})
	if err != nil {
		t.Fatal(err)
	}
	matched := false
	for _, proposal := range result.Proposals {
		if proposal.Dimension == classification.DimensionVATDeductibility {
			matched = proposal.Source == classification.SourceLearnedMapping && proposal.Knowledge != nil
		}
	}
	if !matched {
		t.Fatalf("exact knowledge did not run before AI: %#v", result.Proposals)
	}
	revoked, changed, err := service.RevokeKnowledge(tc.ctx, classification.RevokeKnowledgeCommand{ClientID: tc.clientID, KnowledgeID: item.ID, ExpectedRevision: item.Revision, CommandID: first + ":revoke", ActorID: "accountant", ActorDisplay: "TEST accountant"})
	if err != nil || !changed || revoked.Status != "REVOKED" {
		t.Fatalf("revoke failed: %#v %v", revoked, err)
	}
	third := create("third")
	result, _, err = service.ProcessInvoice(tc.ctx, classification.ProcessCommand{InvoiceID: third, ExpectedRevision: 1, CommandID: third + ":classify"})
	if err != nil {
		t.Fatal(err)
	}
	for _, proposal := range result.Proposals {
		if proposal.Dimension == classification.DimensionVATDeductibility && proposal.Source == classification.SourceLearnedMapping {
			t.Fatal("revoked knowledge matched")
		}
	}
}
