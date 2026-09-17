//go:build integration

package postgres

import (
	"context"
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/clients"
	"diana-contabilitate/backend/internal/platform/httpserver"
	"diana-contabilitate/backend/internal/platform/observability"
	"diana-contabilitate/backend/internal/platform/requestactor"
	"diana-contabilitate/backend/internal/spv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func clientManagementFixture(t *testing.T) (context.Context, *Store, *clients.Service) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	s, err := Open(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	ctx := requestactor.WithActor(context.Background(), requestactor.Actor{ID: "client-management-test", Display: "Contabil test", AllClients: true})
	return ctx, s, clients.NewService(s)
}
func createManagementClient(t *testing.T, ctx context.Context, svc *clients.Service) clients.Detail {
	t.Helper()
	cui := fmt.Sprintf("%010d", 1000000000+time.Now().UnixNano()%8000000000)
	d, err := svc.Command(ctx, "create", clients.Command{Company: clients.Company{Name: "Onboarding integration SRL", CUI: "RO" + cui, Country: "RO", DefaultCurrency: "RON"}, CommandID: "create-" + cui})
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func TestClientManagementPersistenceIdempotencyIdentityConcurrencyAudit(t *testing.T) {
	ctx, s, svc := clientManagementFixture(t)
	d := createManagementClient(t, ctx, svc)
	id := d.Client.ID
	if d.Client.Status != clients.Onboarding || d.Client.Revision != 1 || d.SagaEnabled || d.Onboarding.Classification.Status == "READY" {
		t.Fatalf("%+v", d)
	}
	command := clients.Command{Company: d.Client.Company, CommandID: "retry-create"}
	_, err := svc.Command(ctx, "create", command)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("normalized duplicate accepted", err)
	}
	command.Company.CUI = d.Client.NormalizedIdentifier
	_, err = svc.Command(ctx, "create", command)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("RO/no-RO duplicate accepted", err)
	}
	command = clients.Command{ClientID: id, Company: d.Client.Company, ExpectedRevision: 1, CommandID: "concurrent-update"}
	command.Company.Phone = "0722000000"
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := svc.Command(ctx, "update", command); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	d, err = svc.Detail(ctx, id)
	if err != nil || d.Client.Revision != 2 || d.Client.Company.Phone != "0722000000" {
		t.Fatalf("%+v %v", d, err)
	}
	var count int
	if err = s.DB.QueryRowContext(ctx, `SELECT count(*) FROM activity_events WHERE client_id=$1 AND event_type='CLIENT_UPDATED'`, id).Scan(&count); err != nil || count != 1 {
		t.Fatalf("audit count %d %v", count, err)
	}
	command.CommandID = "stale-update"
	_, err = svc.Command(ctx, "update", command)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("stale revision accepted", err)
	}
	command.CommandID = "concurrent-update"
	command.Company.Phone = "different"
	_, err = svc.Command(ctx, "update", command)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("key payload reuse accepted", err)
	}
	command.CommandID = "identity-before-history"
	command.ExpectedRevision = 2
	command.Company.CUI = fmt.Sprintf("%010d", 1000000000+time.Now().UnixNano()%8000000000)
	d, err = svc.Command(ctx, "update", command)
	if err != nil {
		t.Fatal(err)
	}
	profile := accounting.Profile{EffectiveFrom: "2026-01-01", Framework: "UNKNOWN", TaxRegime: "UNKNOWN", VATRegistration: "UNKNOWN", DeductionActivity: "UNKNOWN", CashAccounting: "UNKNOWN", ProRata: "UNKNOWN"}
	d, err = svc.Command(ctx, "profile", clients.Command{ClientID: id, Profile: &profile, ExpectedRevision: d.Client.Revision, ExpectedProfileVersion: 0, CommandID: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	command.Company = d.Client.Company
	command.Company.CUI = "9876543210"
	command.ExpectedRevision = d.Client.Revision
	command.CommandID = "identity-with-profile"
	_, err = svc.Command(ctx, "update", command)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("dependent identity changed", err)
	}
	d, err = svc.Command(ctx, "lifecycle", clients.Command{ClientID: id, ExpectedRevision: d.Client.Revision, Status: clients.Inactive, CommandID: "deactivate"})
	if err != nil || d.Onboarding.Overall != "INACTIVE" {
		t.Fatalf("%+v %v", d, err)
	}
	active, err := s.ClientOperational(ctx, id)
	if err != nil || active {
		t.Fatal("inactive processing permitted", err)
	}
	d, err = svc.Command(ctx, "lifecycle", clients.Command{ClientID: id, ExpectedRevision: d.Client.Revision, Status: clients.Active, CommandID: "reactivate"})
	if err != nil || d.Client.Status != clients.Active || len(d.History) < 5 {
		t.Fatalf("%+v %v", d, err)
	}
	other := requestactor.WithActor(ctx, requestactor.Actor{ID: "other", AuthorizedClientIDs: []string{"other-client"}})
	if _, err = svc.Detail(other, id); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("cross-client read accepted", err)
	}
	if _, err = svc.Command(other, "saga", clients.Command{ClientID: id, ExpectedRevision: d.Client.Revision, SagaEnabled: true, CommandID: "guessed"}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("cross-client mutation accepted", err)
	}
	// Isolated disposable test DB: immutable non-TEST_ONLY profiles are deliberately retained.
}
func TestClientProfilePersistenceImmutabilityApprovalPeriodAndSagaConfiguration(t *testing.T) {
	ctx, s, svc := clientManagementFixture(t)
	d := createManagementClient(t, ctx, svc)
	id := d.Client.ID
	p := accounting.Profile{EffectiveFrom: "2026-01-01", Framework: "UNKNOWN", TaxRegime: "UNKNOWN", VATRegistration: "UNKNOWN", DeductionActivity: "UNKNOWN", CashAccounting: "UNKNOWN", ProRata: "UNKNOWN"}
	end := accountingdate.Date("2026-12-31")
	p.EffectiveTo = &end
	d, err := svc.Command(ctx, "profile", clients.Command{ClientID: id, Profile: &p, ExpectedRevision: 1, CommandID: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	draftID := d.Profiles[0].ID
	d, err = svc.Command(ctx, "profile", clients.Command{ClientID: id, Profile: &p, Approve: true, Evidence: []string{"verified-client-tax-declaration"}, ExpectedProfileVersion: 1, ExpectedRevision: d.Client.Revision, CommandID: "approve-new-version"})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Profiles) != 2 || d.Profiles[1].Approval.Valid() || !d.Profiles[0].Approval.Valid() {
		t.Fatalf("draft was rewritten %+v", d.Profiles)
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE client_accounting_profiles SET payload=payload WHERE id=$1`, draftID)
	if err == nil {
		t.Fatal("profile update was permitted")
	}
	_, err = svc.Command(ctx, "profile", clients.Command{ClientID: id, Profile: &p, Approve: true, Evidence: []string{"declaration"}, ExpectedProfileVersion: 2, ExpectedRevision: d.Client.Revision, CommandID: "overlap"})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("approved overlap permitted", err)
	}
	p.EffectiveFrom = "2027-01-01"
	p.EffectiveTo = nil
	d, err = svc.Command(ctx, "profile", clients.Command{ClientID: id, Profile: &p, Approve: true, Evidence: []string{"future-declaration"}, ExpectedProfileVersion: 2, ExpectedRevision: d.Client.Revision, CommandID: "future"})
	if err != nil {
		t.Fatal(err)
	}
	d, err = svc.Command(ctx, "saga", clients.Command{ClientID: id, SagaEnabled: true, ExpectedRevision: d.Client.Revision, CommandID: "configure-saga"})
	if err != nil || !d.SagaEnabled || !d.Onboarding.SagaConfigurationReady || d.Onboarding.SagaMappingApproved || d.Onboarding.SagaValidation != "VALIDATION_PENDING" || d.Onboarding.Classification.Status == "READY" {
		t.Fatalf("%+v %v", d, err)
	}
}

type onboardingOAuth struct{}

func (onboardingOAuth) ExchangeToken(context.Context, string, string, string, string) (spv.TokenResponse, error) {
	return spv.TokenResponse{AccessToken: "synthetic-access-secret", RefreshToken: "synthetic-refresh-secret", ExpiresIn: time.Hour, RefreshExpiresIn: time.Hour}, nil
}

type onboardingPublisher struct{ calls int }

func (p *onboardingPublisher) PublishSPVSync(context.Context, string) (string, error) {
	p.calls++
	return "fake-job", nil
}

type onboardingReady struct{}

func (onboardingReady) Ping(context.Context) error { return nil }
func TestClientOnboardingANAFOAuthReplaySyncLifecycleAndSecretIsolation(t *testing.T) {
	ctx, s, svc := clientManagementFixture(t)
	d := createManagementClient(t, ctx, svc)
	cipher, err := spv.NewAESGCMCipher(strings.Repeat("01", 32))
	if err != nil {
		t.Fatal(err)
	}
	publisher := &onboardingPublisher{}
	manager := spv.NewConnectionManager(s, onboardingOAuth{}, cipher, publisher, spv.ConnectionManagerConfig{Enabled: true, Environment: "TEST", AuthorizeURL: "https://fake.example/authorize", OAuthClientID: "fake-client", OAuthClientSecret: "fake-secret", RedirectURI: "https://fake.example/callback", FrontendBaseURL: "https://diana.example"})
	actor := spv.Actor{ID: "test", Display: "Contabil test", CorrelationID: "onboarding-test"}
	first, err := manager.StartOAuthCommand(ctx, d.Client.ID, "authorize", actor)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.StartOAuthCommand(ctx, d.Client.ID, "authorize", actor)
	if err != nil || first != second {
		t.Fatal("OAuth command not idempotent", err)
	}
	parsed, _ := url.Parse(first)
	_, err = manager.Callback(ctx, parsed.Query().Get("state"), "synthetic-code", "", actor)
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.Get(ctx, d.Client.ID)
	if err != nil || view.Status != spv.StatusConnected || view.IdentityValidation != spv.IdentityValidationUnavailable {
		t.Fatalf("%+v %v", view, err)
	}
	if publisher.calls != 0 {
		t.Fatal("duplicate automatic initial sync introduced")
	}
	_, err = manager.RequestSync(ctx, d.Client.ID, "sync", actor)
	if err != nil || publisher.calls != 1 {
		t.Fatal(err)
	}
	handler := httpserver.NewWithSPV(svc, nil, nil, nil, nil, nil, manager, onboardingReady{}, slog.New(slog.NewTextHandler(io.Discard, nil)), observability.NewMetrics())
	for _, route := range []string{"", "/onboarding", "/spv", "/accounting-profiles"} {
		req := httptest.NewRequest("GET", "/api/v1/clients/"+d.Client.ID+route, nil).WithContext(ctx)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		for _, secret := range []string{"synthetic-access-secret", "synthetic-refresh-secret", "accessToken", "refreshToken", "ciphertext", "privateKey", "certificatePassword"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("secret leaked", route, secret)
			}
		}
	}
	d, err = svc.Command(ctx, "lifecycle", clients.Command{ClientID: d.Client.ID, ExpectedRevision: d.Client.Revision, Status: clients.Inactive, CommandID: "deactivate"})
	if err != nil {
		t.Fatal(err)
	}
	connections, err := s.ListActiveConnections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range connections {
		if c.ClientID == d.Client.ID {
			t.Fatal("scheduler polls inactive client")
		}
	}
	if _, err = manager.RequestSync(ctx, d.Client.ID, "inactive-sync", actor); !errors.Is(err, spv.ErrConnectionInactive) {
		t.Fatal("inactive manual sync allowed", err)
	}
	_, err = manager.Disconnect(ctx, d.Client.ID, "disconnect", actor)
	if err != nil {
		t.Fatal(err)
	}
	view, err = manager.Get(ctx, d.Client.ID)
	if err != nil || view.Status != spv.StatusDisabled {
		t.Fatal(err, view)
	}
	d, err = svc.Command(ctx, "lifecycle", clients.Command{ClientID: d.Client.ID, ExpectedRevision: d.Client.Revision, Status: clients.Active, CommandID: "reactivate"})
	if err != nil {
		t.Fatal(err)
	}
	view, _ = manager.Get(ctx, d.Client.ID)
	if view.Status == spv.StatusConnected {
		t.Fatal("reactivation silently reconnected")
	}
	d, err = svc.Detail(ctx, d.Client.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(d.History)
	if strings.Contains(string(raw), "synthetic-access-secret") || strings.Contains(string(raw), "ciphertext") {
		t.Fatal("history leaked secrets")
	}
}

func TestClientManagementCreateDuplicateSubmission(t *testing.T) {
	ctx, s, svc := clientManagementFixture(t)
	cui := fmt.Sprintf("%010d", 1000000000+time.Now().UnixNano()%8000000000)
	command := clients.Command{Company: clients.Company{Name: "Concurrent create SRL", CUI: cui, Country: "RO", DefaultCurrency: "RON"}, CommandID: "duplicate-create-" + cui}
	var wg sync.WaitGroup
	results := make(chan clients.Detail, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); d, err := svc.Command(ctx, "create", command); results <- d; errs <- err }()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	id := ""
	for d := range results {
		if id == "" {
			id = d.Client.ID
		}
		if d.Client.ID != id || d.Client.Revision != 1 {
			t.Fatal("duplicate created independent client", d)
		}
	}
	var count int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM activity_events WHERE client_id=$1 AND event_type='CLIENT_CREATED'`, id).Scan(&count)
	if err != nil || count != 1 {
		t.Fatal("creation not atomic/idempotent", count, err)
	}
}
