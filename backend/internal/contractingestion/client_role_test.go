package contractingestion_test

import (
	"testing"

	ci "diana-contabilitate/backend/internal/contractingestion"
	"diana-contabilitate/backend/internal/contractingestion/fixtures"
)

func hasBlocker(readiness ci.ConfirmationReadiness, code string) bool {
	for _, blocker := range readiness.Blockers {
		if blocker.Code == code {
			return true
		}
	}
	return false
}

// D-126: a lease where the client is the Locator is confirmed with the client
// as supplier; the buyer (Locatar) needs a name and a valid CUI or CNP.
func TestSaleContractIsConfirmedWithClientAsSupplier(t *testing.T) {
	value := reviewed(fixtures.Proposal("romanian"))
	client := value.BuyerCUI
	value.SupplierName, value.SupplierCUI = "Client Locator SRL", client
	value.BuyerName, value.BuyerCUI = "Chiriaș Test SRL", "RO40138380"
	readiness := ci.ConfirmationReadinessFor(value, client)
	if !readiness.CanConfirm || readiness.ClientRole != "SUPPLIER" {
		t.Fatalf("sale contract readiness = %+v", readiness)
	}
	for _, buyer := range []string{"1800101420010", "43217861"} {
		value.BuyerCUI = buyer
		if readiness = ci.ConfirmationReadinessFor(value, client); !readiness.CanConfirm {
			t.Fatalf("buyer %s refused: %+v", buyer, readiness.Blockers)
		}
	}
	value.BuyerName = ""
	if readiness = ci.ConfirmationReadinessFor(value, client); readiness.CanConfirm || !hasBlocker(readiness, "BUYER_NAME_REQUIRED") {
		t.Fatalf("missing tenant name accepted: %+v", readiness)
	}
	value.BuyerName, value.BuyerCUI = "Chiriaș Test SRL", "not an id"
	if readiness = ci.ConfirmationReadinessFor(value, client); readiness.CanConfirm || !hasBlocker(readiness, "BUYER_ID_INVALID") {
		t.Fatalf("invalid tenant identifier accepted: %+v", readiness)
	}
}

func TestContractWithoutTheClientIsStillRejected(t *testing.T) {
	value := reviewed(fixtures.Proposal("romanian"))
	value.BuyerCUI = "RO99999999"
	readiness := ci.ConfirmationReadinessFor(value, "RO10000000")
	if readiness.CanConfirm || !hasBlocker(readiness, "BUYER_MISMATCH") || readiness.ClientRole != "" {
		t.Fatalf("readiness = %+v", readiness)
	}
	if role, ok := ci.ClientRoleFor(value, "RO10000000"); ok || role != "" {
		t.Fatalf("role=%q ok=%v", role, ok)
	}
}

func TestPurchaseContractKeepsBuyerRole(t *testing.T) {
	value := reviewed(fixtures.Proposal("romanian"))
	readiness := ci.ConfirmationReadinessFor(value, value.BuyerCUI)
	if !readiness.CanConfirm || readiness.ClientRole != "BUYER" {
		t.Fatalf("purchase contract readiness = %+v", readiness)
	}
}
