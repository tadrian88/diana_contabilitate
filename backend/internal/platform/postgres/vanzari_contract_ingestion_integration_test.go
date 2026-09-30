//go:build integration

package postgres

import (
	"errors"
	"testing"

	"diana-contabilitate/backend/ent/contract"
	ci "diana-contabilitate/backend/internal/contractingestion"
)

// D-126: the client may be the Locator (supplier) of a confirmed contract; the
// tenant becomes the buyer and the dossier buyer, and an annex is read on the
// same side as its base contract.
func TestLeaseWhereClientIsLocatorIsConfirmedWithSupplierRole(t *testing.T) {
	tc := newContractTestContext(t)
	service, _, extractor := contractIngestionFixture(t, tc)
	client, err := tc.store.Client.AccountingClient.Get(tc.ctx, tc.clientID)
	if err != nil {
		t.Fatal(err)
	}
	tenantName, tenantCUI := "Chiriaș Test SRL", "RO40138380"
	extractor.proposal.SupplierCUI.Value, extractor.proposal.BuyerCUI.Value = &client.Cui, &tenantCUI
	extractor.proposal.BuyerName = extractor.proposal.SupplierName
	extractor.proposal.BuyerName.Value = &tenantName
	doc := ingestionUpload(t, tc, service)
	command := ingestionReview(t, tc, service, doc)
	command.Contract.SupplierName, command.Contract.SupplierCUI = client.Name, client.Cui
	command.Contract.BuyerCUI = tenantCUI

	command.Contract.BuyerName = ""
	if _, _, err = service.Confirm(tc.ctx, command); err == nil || errors.Is(err, ci.ErrBuyerMismatch) {
		t.Fatalf("a lease without the tenant name must be refused for review, err=%v", err)
	}
	command.Contract.BuyerName = tenantName
	id, changed, err := service.Confirm(tc.ctx, command)
	if err != nil || !changed {
		t.Fatalf("confirm lease err=%v", err)
	}
	row, err := tc.store.Client.Contract.Get(tc.ctx, id)
	if err != nil || row.ClientRole != contract.ClientRoleSUPPLIER || row.BuyerName == nil || *row.BuyerName != tenantName || row.NormalizedBuyerCui == nil || *row.NormalizedBuyerCui != "40138380" {
		t.Fatalf("lease contract = %+v err=%v", row, err)
	}
	var dossierBuyer string
	if err = tc.store.DB.QueryRowContext(tc.ctx, `SELECT buyer_cui FROM contract_dossiers WHERE contract_id=$1`, id).Scan(&dossierBuyer); err != nil || dossierBuyer != tenantCUI {
		t.Fatalf("dossier buyer = %q err=%v", dossierBuyer, err)
	}
}
