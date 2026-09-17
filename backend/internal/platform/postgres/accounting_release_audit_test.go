package postgres

import (
	"encoding/json"
	"strings"
	"testing"

	"diana-contabilitate/backend/internal/accountingtest"
)

func TestAccountingApprovalAuditRetainsExactReviewedEvidence(t *testing.T) {
	_, _, profile, pack := accountingtest.Fixture("TEST_ONLY-client")
	for _, configuration := range []any{profile, pack, ReviewedAccountingRule{Rule: pack.Rules[0], ClientID: profile.ClientID, ProfileID: profile.ID, PackID: pack.ID, PackVersion: pack.Version, Approval: pack.Approval}} {
		detail := accountingApprovalDetail(configuration)
		if !json.Valid([]byte(detail)) || !strings.Contains(detail, pack.Approval.Actor) || !strings.Contains(detail, pack.Approval.Evidence[0]) || !strings.Contains(detail, profile.ClientID) {
			t.Fatal("approval lineage lost", detail)
		}
	}
}
