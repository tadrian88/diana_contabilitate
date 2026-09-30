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

func TestLearnedServiceAliasCanBeRevokedAndLearnedAgain(t *testing.T) {
	tc := newModule5TestContext(t)
	service := commercialvalidation.NewService(tc.store, func() time.Time { return tc.now })
	dossier, _, err := service.CreateDossier(tc.ctx, commercialvalidation.Dossier{ClientID: tc.clientID, SupplierCUI: "RO38920171", BuyerCUI: "RO49678244", PrimaryReference: "BG-2025-117"}, tc.clientID+":alias-dossier")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = tc.store.DB.ExecContext(tc.ctx, `DELETE FROM contract_service_aliases WHERE dossier_id=$1`, dossier.ID)
		_, _ = tc.store.DB.ExecContext(tc.ctx, `DELETE FROM activity_events WHERE aggregate_id=$1`, dossier.ID)
		_, _ = tc.store.DB.ExecContext(tc.ctx, `DELETE FROM contract_dossiers WHERE id=$1`, dossier.ID)
	})
	learn := func(id, serviceID string) error {
		_, err := tc.store.DB.ExecContext(tc.ctx, `INSERT INTO contract_service_aliases(id,client_id,supplier_cui,service_id,normalized_label,confirmed_by_id,confirmed_at,command_key,dossier_id) VALUES($1,$2,'RO38920171',$3,'HOSTING CLOUD BUSINESS 2 VM','reviewer',$4,$5,$6)`, id, tc.clientID, serviceID, tc.now, "commercial-alias:"+id, dossier.ID)
		return err
	}
	if err = learn(dossier.ID+":first", "service-hosting"); err != nil {
		t.Fatal(err)
	}
	if err = learn(dossier.ID+":duplicate", "service-maintenance"); err == nil {
		t.Fatal("an active wording must map to one service only")
	}
	revoke := commercialvalidation.AliasRevocation{ClientID: tc.clientID, AliasID: dossier.ID + ":first", ActorID: "reviewer", CommandID: dossier.ID + ":revoke"}
	if changed, err := service.RevokeAlias(tc.ctx, revoke); err != nil || !changed {
		t.Fatalf("revoke changed=%t err=%v", changed, err)
	}
	if changed, err := service.RevokeAlias(tc.ctx, revoke); err != nil || changed {
		t.Fatalf("revoke replay changed=%t err=%v", changed, err)
	}
	other := revoke
	other.ClientID = "other-client"
	if _, err := service.RevokeAlias(tc.ctx, other); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("cross-client revoke returned %v", err)
	}
	if err = learn(dossier.ID+":second", "service-maintenance"); err != nil {
		t.Fatalf("a revoked wording must be learnable again: %v", err)
	}
	aliases, err := service.ListAliases(tc.ctx, tc.clientID, dossier.ID)
	if err != nil || len(aliases) != 2 || aliases[0].ID != dossier.ID+":second" || aliases[0].RevokedAt != nil || aliases[1].RevokedAt == nil || aliases[1].RevokedBy != "reviewer" {
		t.Fatalf("aliases=%+v err=%v", aliases, err)
	}
}

func TestConfirmedMappingKeepsItsInvoiceWhenTheLearnedWordingIsRevoked(t *testing.T) {
	tc := newModule5TestContext(t)
	service := commercialvalidation.NewService(tc.store, func() time.Time { return tc.now })
	dossier, _, err := service.CreateDossier(tc.ctx, commercialvalidation.Dossier{ClientID: tc.clientID, SupplierCUI: "RO38920171", BuyerCUI: "RO49678244", PrimaryReference: "CWF-0231"}, tc.clientID+":mapping-dossier")
	if err != nil {
		t.Fatal(err)
	}
	invoiceID := tc.createInvoice(t, "mapping-scope", "Servicii curățenie birou")
	t.Cleanup(func() {
		_, _ = tc.store.DB.ExecContext(tc.ctx, `DELETE FROM contract_service_aliases WHERE dossier_id=$1`, dossier.ID)
		_, _ = tc.store.DB.ExecContext(tc.ctx, `DELETE FROM activity_events WHERE aggregate_id=$1`, dossier.ID)
		_, _ = tc.store.DB.ExecContext(tc.ctx, `DELETE FROM contract_dossiers WHERE id=$1`, dossier.ID)
	})
	alias := commercialvalidation.Alias{ID: stableID("service-alias", invoiceID+":map"), ClientID: tc.clientID, InvoiceID: invoiceID, ServiceID: "service-cleaning", NormalizedLabel: "SERVICII CURĂȚENIE BIROU", DossierID: dossier.ID, SupplierCUI: "RO38920171", ReuseForDossier: true, ConfirmedByID: "reviewer", ConfirmedAt: tc.now}
	if changed, err := tc.store.saveAlias(tc.ctx, alias, invoiceID+":map"); err != nil || !changed {
		t.Fatalf("remembered mapping changed=%t err=%v", changed, err)
	}
	if changed, err := tc.store.saveAlias(tc.ctx, alias, invoiceID+":map"); err != nil || changed {
		t.Fatalf("replay changed=%t err=%v", changed, err)
	}
	active := func() (dossierWide, invoiceScoped int) {
		if err := tc.store.DB.QueryRowContext(tc.ctx, `SELECT COUNT(*) FILTER (WHERE invoice_id IS NULL),COUNT(*) FILTER (WHERE invoice_id=$2) FROM contract_service_aliases WHERE dossier_id=$1 AND revoked_at IS NULL`, dossier.ID, invoiceID).Scan(&dossierWide, &invoiceScoped); err != nil {
			t.Fatal(err)
		}
		return dossierWide, invoiceScoped
	}
	if dossierWide, invoiceScoped := active(); dossierWide != 1 || invoiceScoped != 1 {
		t.Fatalf("a remembered mapping is learned and binds its invoice: dossier=%d invoice=%d", dossierWide, invoiceScoped)
	}
	revoke := commercialvalidation.AliasRevocation{ClientID: tc.clientID, AliasID: alias.ID, ActorID: "reviewer", CommandID: invoiceID + ":revoke"}
	if changed, err := service.RevokeAlias(tc.ctx, revoke); err != nil || !changed {
		t.Fatalf("revoke changed=%t err=%v", changed, err)
	}
	if dossierWide, invoiceScoped := active(); dossierWide != 0 || invoiceScoped != 1 {
		t.Fatalf("revoking the learned wording keeps this invoice's mapping: dossier=%d invoice=%d", dossierWide, invoiceScoped)
	}
	again := alias
	again.ID = stableID("service-alias", invoiceID+":map-again")
	if changed, err := tc.store.saveAlias(tc.ctx, again, invoiceID+":map-again"); err != nil || !changed {
		t.Fatalf("learning the wording again next to the kept mapping changed=%t err=%v", changed, err)
	}
	other := again
	other.ID, other.ServiceID = stableID("service-alias", invoiceID+":map-other"), "service-windows"
	if _, err := tc.store.saveAlias(tc.ctx, other, invoiceID+":map-other"); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("an active wording maps to one service only, err=%v", err)
	}
}
