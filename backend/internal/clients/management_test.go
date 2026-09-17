package clients

import (
	"context"
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/accountingtest"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/platform/requestactor"
	"diana-contabilitate/backend/internal/spv"
	"errors"
	"testing"
	"time"
)

func TestClientMinimumAndIdentifierNormalization(t *testing.T) {
	for _, tc := range []struct {
		raw, country, want string
		invalid            bool
	}{{" RO 12345678 ", "RO", "12345678", false}, {"12345678", "RO", "12345678", false}, {"ro12345678", "RO", "12345678", false}, {"RO-DEMO-1", "RO", "", true}, {"", "RO", "", true}, {"00123", "RO", "", true}, {"12345678901", "RO", "", true}, {"DE123456", "DE", "DE123456", false}, {"<script>", "DE", "", true}} {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := NormalizeIdentifier(tc.raw, tc.country)
			if (err != nil) != tc.invalid || got != tc.want {
				t.Fatalf("got %q,%v", got, err)
			}
		})
	}
	c := Company{Name: " Firmă SRL ", CUI: "RO12345678", Country: "ro", DefaultCurrency: "ron"}
	id, err := c.Normalize()
	if err != nil || id != "12345678" || c.Name != "Firmă SRL" {
		t.Fatalf("%+v %v", c, err)
	}
	c.Name = ""
	if _, err = c.Normalize(); !errors.Is(err, apperrors.ErrValidation) {
		t.Fatal(err)
	}
}
func TestClientIdentityProtectionAndLegacyMetadata(t *testing.T) {
	before := Client{Name: "Demo", CUI: "RO-DEMO-1", NormalizedIdentifier: "RO-DEMO-1", Company: Company{Country: "RO"}}
	c := Company{Name: "Demo nou", CUI: before.CUI, Country: "RO", DefaultCurrency: "RON", Phone: "123"}
	normalized, err := c.NormalizeExisting(before)
	if err != nil || normalized != before.NormalizedIdentifier {
		t.Fatal(err)
	}
	c.CUI = "RO12345678"
	if err = CanChangeIdentity(before, c, "12345678", true); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal(err)
	}
	if err = CanChangeIdentity(before, c, "12345678", false); err != nil {
		t.Fatal(err)
	}
	c.CUI = "invalid"
	if _, err = c.NormalizeExisting(before); err == nil {
		t.Fatal("new invalid identity accepted")
	}
}
func TestClientGrants(t *testing.T) {
	ctx := requestactor.WithActor(context.Background(), requestactor.Actor{ID: "a", AuthorizedClientIDs: []string{"a"}})
	service := NewService(readerStub{clients: []Client{{ID: "a"}, {ID: "b"}}})
	items, err := service.List(ctx)
	if err != nil || len(items) != 1 || items[0].ID != "a" {
		t.Fatalf("%+v %v", items, err)
	}
	if err = Authorize(ctx, "b"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal(err)
	}
	if err = Authorize(context.Background(), "a"); err == nil {
		t.Fatal("missing actor accepted")
	}
}
func TestClientProfileFactsApprovalAndEffectiveDates(t *testing.T) {
	_, _, p, _ := accountingtest.Fixture("a")
	p.TestOnly = false
	if err := ValidateProfile(p); err != nil {
		t.Fatal(err)
	}
	p.Framework = "UNKNOWN"
	p.TaxRegime = "UNKNOWN"
	p.VATRegistration = "UNKNOWN"
	p.DeductionActivity = "UNKNOWN"
	p.CashAccounting = "UNKNOWN"
	p.ProRata = "UNKNOWN"
	if err := ValidateProfile(p); err != nil {
		t.Fatal("UNKNOWN was lost", err)
	}
	p.CashAccounting = ""
	if err := ValidateProfile(p); err == nil {
		t.Fatal("unanswered fact silently defaulted")
	}
}
func TestClientOnboardingIndependentSectionsAndNoFakeGreen(t *testing.T) {
	today := accountingdate.Date("2026-09-15")
	_, _, p, pack := accountingtest.Fixture("a")
	p.TestOnly = false
	c := Client{ID: "a", Name: "Firmă", CUI: "12345678", Status: Onboarding, Company: Company{Country: "RO", DefaultCurrency: "RON"}}
	anaf := spv.ConnectionView{Status: spv.StatusConnected, ConfigurationReady: true}
	cases := []struct {
		name                                           string
		profiles                                       []*accounting.Profile
		view                                           spv.ConnectionView
		enabled                                        bool
		profileStatus, anafStatus, sagaStatus, overall string
	}{
		{"company only", nil, spv.ConnectionView{ConfigurationReady: true}, false, "NOT_STARTED", "NOT_STARTED", "NOT_STARTED", "CONFIGURATION_INCOMPLETE"},
		{"profile ready ANAF missing", []*accounting.Profile{p}, spv.ConnectionView{ConfigurationReady: true}, false, "READY", "NOT_STARTED", "NOT_STARTED", "CONFIGURATION_INCOMPLETE"},
		{"ANAF ready SAGA missing", []*accounting.Profile{p}, anaf, false, "READY", "READY", "NOT_STARTED", "CONFIGURATION_INCOMPLETE"},
		{"core configured no real pack", []*accounting.Profile{p}, anaf, true, "READY", "READY", "READY", "CORE_CONFIGURED_AUTOMATION_PENDING"},
		{"ANAF action required", []*accounting.Profile{p}, spv.ConnectionView{ConfigurationReady: true, Status: spv.StatusNeedsReauthentication}, true, "READY", "ACTION_REQUIRED", "READY", "CONFIGURATION_INCOMPLETE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Derive(c, tc.profiles, []*accounting.Pack{pack}, tc.enabled, tc.view, today)
			if r.Company.Status != "READY" || r.AccountingProfile.Status != tc.profileStatus || r.ANAF.Status != tc.anafStatus || r.Saga.Status != tc.sagaStatus || r.Overall != tc.overall {
				t.Fatalf("%+v", r)
			}
			if r.Classification.Status == "READY" || r.SagaMappingApproved || r.SagaValidation != "VALIDATION_PENDING" {
				t.Fatalf("synthetic pack made green: %+v", r)
			}
		})
	}
	draft := *p
	draft.Approval = accounting.Approval{}
	r := Derive(c, []*accounting.Profile{&draft}, nil, false, anaf, today)
	if r.AccountingProfile.Status != "INCOMPLETE" {
		t.Fatal(r)
	}
	future := *p
	future.EffectiveFrom = "2027-01-01"
	r = Derive(c, []*accounting.Profile{&future}, nil, false, anaf, today)
	if r.AccountingProfile.Status == "READY" || r.CurrentProfileID != "" {
		t.Fatal(r)
	}
	historical := *p
	end := accountingdate.Date("2025-12-31")
	historical.EffectiveFrom = "2025-01-01"
	historical.EffectiveTo = &end
	r = Derive(c, []*accounting.Profile{&historical}, nil, false, anaf, today)
	if r.AccountingProfile.Status == "READY" {
		t.Fatal(r)
	}
	other := *p
	other.ClientID = "b"
	r = Derive(c, []*accounting.Profile{&other}, nil, false, anaf, today)
	if r.AccountingProfile.Status == "READY" {
		t.Fatal("foreign profile accepted")
	}
	r = Derive(c, []*accounting.Profile{p, p}, nil, false, anaf, today)
	if r.AccountingProfile.Status != "ACTION_REQUIRED" {
		t.Fatal(r)
	}
	c.Status = Inactive
	r = Derive(c, []*accounting.Profile{p}, nil, true, anaf, today)
	if r.Overall != "INACTIVE" {
		t.Fatal(r)
	}
	c.Status = Active
	c.Company.DefaultCurrency = "EUR"
	r = Derive(c, nil, nil, true, anaf, today)
	if !r.SagaConfigurationReady {
		t.Fatal("master currency incorrectly used as invoice-export authority")
	}
}
func TestClientReadinessANAFSafeFailure(t *testing.T) {
	c := Client{ID: "a", Name: "Firmă", CUI: "12345678", Company: Company{DefaultCurrency: "RON"}}
	r := Derive(c, nil, nil, false, spv.ConnectionView{}, accountingdate.FromTime(time.Now()))
	r.ApplyANAF(c, spv.ConnectionView{ConfigurationReady: true, Status: spv.StatusConnected, SafeErrorCode: "SYNC_FAILED"})
	if r.ANAF.Status != "ACTION_REQUIRED" {
		t.Fatal(r)
	}
}
