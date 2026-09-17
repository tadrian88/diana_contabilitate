package saga

import (
	"bytes"
	"encoding/json"
	"testing"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/classification"
)

func TestShadowNoSideEffectsOrPointerAliasing(t *testing.T) {
	item := domainInvoice(t)
	candidate := item.AccountingSnapshot
	before, _ := json.Marshal(item)
	out, err := evaluateShadow(item, domainClient(), candidate, nil, true)
	if err != nil || !out.Readiness.Ready || len(out.Proposals) != 4 || out.MappingVersion == "" || out.MappingCompatibility != "COMPATIBLE" {
		t.Fatal(err, out)
	}
	out.Proposals[0].TypedValue.Account = "999.TEST"
	out.Proposals[0].Evidence.ProfileID = "changed"
	after, _ := json.Marshal(item)
	if !bytes.Equal(before, after) {
		t.Fatal("shadow mutated caller facts, snapshot, decisions, task or status")
	}
	if _, err := EvaluateShadow(item, domainClient(), candidate, nil); err != nil {
		t.Fatal(err)
	}
	production, _ := EvaluateShadow(item, domainClient(), candidate, nil)
	if production.Readiness.Ready {
		t.Fatal("synthetic candidate accepted as production")
	}
}

func TestShadowMissingApprovalConflictAndMixedLines(t *testing.T) {
	item := domainInvoice(t)
	item.AccountingSnapshot.Pack.Approval = accounting.Approval{}
	out, err := evaluateShadow(item, domainClient(), item.AccountingSnapshot, nil, true)
	if err != nil || out.Readiness.Ready {
		t.Fatal(err, out)
	}
	for _, p := range out.Proposals {
		if !p.RequiresReview {
			t.Fatal("unapproved candidate matched")
		}
	}
	item = domainInvoice(t)
	extra := item.Lines[0]
	extra.ID = "unsupported"
	extra.Position = 2
	extra.Description = "unrelated"
	facts := *extra.SourceFacts
	facts.SellerItemID = "unsupported"
	extra.SourceFacts = &facts
	item.Lines = append(item.Lines, extra)
	out, err = evaluateShadow(item, domainClient(), item.AccountingSnapshot, nil, true)
	if err != nil || out.Readiness.Ready || len(out.Proposals) != 8 {
		t.Fatal(err, out)
	}
	for _, p := range out.Proposals {
		if p.RequiresReview != (p.InvoiceLineID == "unsupported") {
			t.Fatal("supported proposal lost or unsupported accepted", p)
		}
	}
	item = domainInvoice(t)
	duplicate := item.AccountingSnapshot.Pack.Rules[0]
	duplicate.VersionID += "-v2"
	duplicate.Version++
	item.AccountingSnapshot.Pack.Rules = append(item.AccountingSnapshot.Pack.Rules, duplicate)
	out, err = evaluateShadow(item, domainClient(), item.AccountingSnapshot, nil, true)
	if err != nil || out.Readiness.Ready || out.Proposals[0].Source != classification.SourceAmbiguous {
		t.Fatal(err, out)
	}
}
