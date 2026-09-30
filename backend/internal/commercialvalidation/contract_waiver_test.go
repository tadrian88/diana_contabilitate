package commercialvalidation

import (
	"context"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/money"
)

func waivedInvoice(documentType invoicing.DocumentType) invoicing.Invoice {
	actor := "Contabil Test"
	return invoicing.Invoice{
		ID: "invoice", Revision: 3, DocumentType: documentType, IssueDay: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Total: money.Money{Amount: money.MustParse("2222.92"), Currency: "RON"},
		Lines: []invoicing.Line{{ID: "line-1", Description: "Servicii diverse"}, {ID: "line-2", Description: "Consumabile"}},
		ContractWaiver: &contracts.WaiverSnapshot{
			TaskID: "task", Reason: "Achiziție punctuală fără contract", ActorDisplay: &actor,
			WaivedAt: time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC),
		},
	}
}

func TestWaivedInvoiceWithoutSnapshotIsConformWithOnlyTheWaiverFinding(t *testing.T) {
	input := Input{Invoice: waivedInvoice(invoicing.DocumentTypeInvoice), Snapshot: Snapshot{Version: 1, Coverage: CoveragePartial}, ContractWaived: true}
	run := (Engine{}).Validate(input, time.Now())
	if run.Outcome != Conform || len(run.Findings) != 1 {
		t.Fatalf("outcome=%s findings=%+v", run.Outcome, run.Findings)
	}
	finding := run.Findings[0]
	if finding.Code != "CONTRACT_WAIVED" || finding.Outcome != Conform || finding.Actual != "Achiziție punctuală fără contract" || finding.ActualSource != "Decizia contabilului · Contabil Test · 30.09.2026" {
		t.Fatalf("waiver finding=%+v", finding)
	}
}

func TestWaivedCreditNoteWithoutOriginalStillNeedsReview(t *testing.T) {
	input := Input{Invoice: waivedInvoice(invoicing.DocumentTypeCreditNote), Snapshot: Snapshot{Version: 1, Coverage: CoveragePartial}, ContractWaived: true}
	run := (Engine{}).Validate(input, time.Now())
	if run.Outcome != Unverifiable {
		t.Fatalf("outcome=%s findings=%+v", run.Outcome, run.Findings)
	}
	codes := map[string]bool{}
	for _, finding := range run.Findings {
		codes[finding.Code] = true
	}
	if !codes["CONTRACT_WAIVED"] || !codes["ORIGINAL_INVOICE_UNAVAILABLE"] || codes["SERVICE_LINE_UNCOVERED"] || codes["CONTRACT_COVERAGE_INCOMPLETE"] {
		t.Fatalf("findings=%+v", run.Findings)
	}
}

func TestInvoiceWithoutSnapshotOrWaiverKeepsCoverageFindings(t *testing.T) {
	invoice := waivedInvoice(invoicing.DocumentTypeInvoice)
	invoice.ContractWaiver = nil
	run := (Engine{}).Validate(Input{Invoice: invoice, Snapshot: Snapshot{Version: 1, Coverage: CoveragePartial}}, time.Now())
	uncovered := 0
	coverage := false
	for _, finding := range run.Findings {
		switch finding.Code {
		case "SERVICE_LINE_UNCOVERED":
			uncovered++
		case "CONTRACT_COVERAGE_INCOMPLETE":
			coverage = true
		case "CONTRACT_WAIVED":
			t.Fatalf("waiver finding without a waiver: %+v", finding)
		}
	}
	if run.Outcome != Unverifiable || !coverage || uncovered != 2 {
		t.Fatalf("outcome=%s findings=%+v", run.Outcome, run.Findings)
	}
}

type waiverStore struct {
	Store
	input Input
	err   error
	saved *Run
}

func (s *waiverStore) LoadInput(context.Context, string) (Input, error) { return s.input, s.err }

func (s *waiverStore) SaveRun(_ context.Context, run Run, _ string) (bool, error) {
	s.saved = &run
	return true, nil
}

func TestServiceAppliesWaiverOnlyWhenNoContractSnapshotExists(t *testing.T) {
	invoice := waivedInvoice(invoicing.DocumentTypeInvoice)
	store := &waiverStore{input: Input{Invoice: invoice}, err: ErrNoSnapshot}
	if err := NewService(store, nil).ProcessCommercialValidation(context.Background(), "invoice", invoice.Revision, "command", "corr"); err != nil {
		t.Fatal(err)
	}
	if store.saved == nil || store.saved.Outcome != Conform {
		t.Fatalf("saved=%+v", store.saved)
	}

	// A contract associated after the decision takes precedence: the waiver
	// never masks a real snapshot's coverage.
	withSnapshot := &waiverStore{input: Input{Invoice: invoice, Snapshot: Snapshot{ID: "snapshot", Version: 1, Coverage: CoveragePartial}}}
	if err := NewService(withSnapshot, nil).ProcessCommercialValidation(context.Background(), "invoice", invoice.Revision, "command", "corr"); err != nil {
		t.Fatal(err)
	}
	if withSnapshot.saved == nil || withSnapshot.saved.Outcome != Unverifiable {
		t.Fatalf("saved=%+v", withSnapshot.saved)
	}
	for _, finding := range withSnapshot.saved.Findings {
		if finding.Code == "CONTRACT_WAIVED" {
			t.Fatalf("waiver applied despite snapshot: %+v", withSnapshot.saved.Findings)
		}
	}
}
