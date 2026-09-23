//go:build integration

package postgres

import (
	"errors"
	"sync"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/commercialvalidation"
	"diana-contabilitate/backend/internal/invoicing"
)

func TestCommercialValidationLegacyPartialIsIdempotentConcurrentAndClientScoped(t *testing.T) {
	tc := newModule5TestContext(t)
	invoiceID := tc.createInvoice(t, "commercial-legacy", "TEST_ONLY service")
	t.Cleanup(func() {
		_, _ = tc.store.DB.ExecContext(tc.ctx, `DELETE FROM commercial_review_commands WHERE invoice_id=$1`, invoiceID)
		_, _ = tc.store.DB.ExecContext(tc.ctx, `DELETE FROM invoice_commercial_overrides WHERE finding_id IN (SELECT f.id FROM invoice_commercial_findings f JOIN invoice_commercial_validation_runs r ON r.id=f.run_id WHERE r.invoice_id=$1)`, invoiceID)
		_, _ = tc.store.DB.ExecContext(tc.ctx, `DELETE FROM invoice_commercial_findings WHERE run_id IN (SELECT id FROM invoice_commercial_validation_runs WHERE invoice_id=$1)`, invoiceID)
		_, _ = tc.store.DB.ExecContext(tc.ctx, `DELETE FROM invoice_commercial_validation_runs WHERE invoice_id=$1`, invoiceID)
	})
	if _, err := tc.store.DB.ExecContext(tc.ctx, `UPDATE invoices SET pipeline_status='COMMERCIAL_VALIDATING' WHERE id=$1`, invoiceID); err != nil {
		t.Fatal(err)
	}
	service := commercialvalidation.NewService(tc.store, func() time.Time { return tc.now })
	const workers = 4
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := service.ProcessCommercialValidation(tc.ctx, invoiceID, 1, invoiceID+":same-command", "commercial-test"); err != nil && !errors.Is(err, apperrors.ErrConflict) {
				t.Errorf("concurrent validation: %v", err)
			}
		}()
	}
	group.Wait()
	var runCount, taskCount int
	if err := tc.store.DB.QueryRowContext(tc.ctx, `SELECT count(*) FROM invoice_commercial_validation_runs WHERE invoice_id=$1`, invoiceID).Scan(&runCount); err != nil {
		t.Fatal(err)
	}
	if err := tc.store.DB.QueryRowContext(tc.ctx, `SELECT count(*) FROM validation_tasks WHERE invoice_id=$1 AND task_type='COMMERCIAL_REVIEW'`, invoiceID).Scan(&taskCount); err != nil {
		t.Fatal(err)
	}
	if runCount != 1 || taskCount != 1 {
		t.Fatalf("runs=%d tasks=%d", runCount, taskCount)
	}
	item, err := tc.store.GetInvoice(tc.ctx, invoiceID)
	if err != nil || item.PipelineStatus != invoicing.StatusAwaitingCommercialReview || item.Revision != 2 {
		t.Fatalf("invoice=%+v err=%v", item, err)
	}
	run, err := service.Get(tc.ctx, tc.clientID, invoiceID)
	if err != nil || run.Outcome != commercialvalidation.Unverifiable || len(run.Findings) == 0 {
		t.Fatalf("run=%+v err=%v", run, err)
	}
	if _, err := service.Get(tc.ctx, "other-client", invoiceID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("cross-client read returned %v", err)
	}
	if err := service.ProcessCommercialValidation(tc.ctx, invoiceID, 1, invoiceID+":same-command", "commercial-test"); err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	if changed, err := service.Resolve(tc.ctx, commercialvalidation.ReviewResolution{ClientID: tc.clientID, InvoiceID: invoiceID, RunID: run.ID, ExpectedInvoiceRevision: item.Revision, Action: "WAIT_FOR_CORRECTION", CommandID: invoiceID + ":wait"}); err != nil || !changed {
		t.Fatalf("wait changed=%v err=%v", changed, err)
	}
	if changed, err := service.Resolve(tc.ctx, commercialvalidation.ReviewResolution{ClientID: tc.clientID, InvoiceID: invoiceID, RunID: run.ID, ExpectedInvoiceRevision: item.Revision, Action: "RERUN", CommandID: invoiceID + ":rerun"}); err != nil || !changed {
		t.Fatalf("rerun changed=%v err=%v", changed, err)
	}
	item, err = tc.store.GetInvoice(tc.ctx, invoiceID)
	if err != nil || item.PipelineStatus != invoicing.StatusCommercialValidating || item.Revision != 3 {
		t.Fatalf("rerun invoice=%+v err=%v", item, err)
	}
}

func TestCommercialDateFactIsInvoiceScopedProvenancedAndIdempotent(t *testing.T) {
	tc := newModule5TestContext(t)
	invoiceID := tc.createInvoice(t, "commercial-date", "Service")
	service := commercialvalidation.NewService(tc.store, func() time.Time { return tc.now })
	fact := commercialvalidation.InvoiceDateFact{ClientID: tc.clientID, InvoiceID: invoiceID, Kind: "REMITTANCE", Date: time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC), SourceReference: "Confirmare transmitere SPV", ActorID: "reviewer", CommandID: invoiceID + ":remittance"}
	if changed, err := service.PutInvoiceDateFact(tc.ctx, fact); err != nil || !changed {
		t.Fatalf("save date changed=%t err=%v", changed, err)
	}
	if changed, err := service.PutInvoiceDateFact(tc.ctx, fact); err != nil || changed {
		t.Fatalf("idempotent date replay changed=%t err=%v", changed, err)
	}
	fact.CommandID = invoiceID + ":other-client"
	fact.ClientID = "other-client"
	if changed, err := service.PutInvoiceDateFact(tc.ctx, fact); !errors.Is(err, apperrors.ErrNotFound) || changed {
		t.Fatalf("cross-client date write changed=%t err=%v", changed, err)
	}
}
