package config

import (
	"strings"
	"testing"
	"time"
)

func TestEnabledSPVReportsMissingVariableNamesWithoutValues(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://example.invalid/diana")
	t.Setenv("SPV_ENABLED", "true")
	t.Setenv("SPV_OAUTH_CLIENT_ID", "")
	t.Setenv("SPV_OAUTH_CLIENT_SECRET", "")
	t.Setenv("SPV_TOKEN_ENCRYPTION_KEY", strings.Repeat("ab", 32))
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "SPV_OAUTH_CLIENT_ID") || !strings.Contains(err.Error(), "SPV_OAUTH_CLIENT_SECRET") {
		t.Fatalf("expected safe missing-name diagnostic, got %v", err)
	}
}

func TestAPIInProcessDispatcherDefaultsOffAndCanBeEnabledForLegacyDevelopment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://example.invalid/diana")
	t.Setenv("PIPELINE_DISPATCH_ENABLED", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PipelineDispatchEnabled {
		t.Fatal("the production API must not own asynchronous workflow progression")
	}
	t.Setenv("PIPELINE_DISPATCH_ENABLED", "true")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.PipelineDispatchEnabled {
		t.Fatal("legacy development/test mode must remain explicitly available")
	}
}

func TestWorkerRuntimeDefaultsAreBounded(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://example.invalid/diana")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WorkerConcurrency < 1 || cfg.WorkerMaxRetry < 1 || cfg.DispatcherPollInterval <= 0 || cfg.DispatcherClaimTTL <= cfg.DispatcherPollInterval || cfg.DispatcherRetryMax < cfg.DispatcherRetryMin {
		t.Fatalf("unsafe defaults: %+v", cfg)
	}
}

func TestSPVOAuthUXSecurityDefaultsAreBounded(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://example.invalid/diana")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SPVOAuthStateTTL <= 0 || cfg.SPVOAuthStateTTL > time.Hour || cfg.SPVAuthorizeURL == "" || cfg.SPVOAuthRedirectURI == "" || cfg.FrontendBaseURL == "" {
		t.Fatalf("unsafe OAuth UX defaults: %+v", cfg)
	}
}

func TestProductionCannotSilentlyUseFakeSAGA(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://example.invalid/diana")
	t.Setenv("SPV_ENABLED", "false")
	t.Setenv("APP_ENV", "production")
	t.Setenv("SAGA_MODE", "fake")
	if _, err := Load(); err == nil {
		t.Fatal("production must reject the fake SAGA adapter")
	}
	t.Setenv("SAGA_MODE", "file")
	if cfg, err := Load(); err != nil || cfg.SagaMode != "file" {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
}

func TestCloudTestUsesProductionAdapterBoundaries(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://example.invalid/diana")
	t.Setenv("APP_ENV", "cloud-test")
	t.Setenv("SAGA_MODE", "fake")
	if _, err := Load(); err == nil {
		t.Fatal("cloud-test must reject the fake SAGA adapter")
	}
	t.Setenv("SAGA_MODE", "file")
	t.Setenv("SPV_ENABLED", "true")
	t.Setenv("SPV_OAUTH_CLIENT_ID", "fixture-id")
	t.Setenv("SPV_OAUTH_CLIENT_SECRET", "fixture-secret")
	t.Setenv("SPV_TOKEN_ENCRYPTION_KEY", strings.Repeat("ab", 32))
	t.Setenv("SPV_API_BASE_URL", "http://fakeanaf.invalid")
	if _, err := Load(); err == nil {
		t.Fatal("cloud-test must reject non-official ANAF endpoints")
	}
}

func TestLocalRealFailsClosed(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://example.invalid/diana")
	t.Setenv("SPV_API_BASE_URL", "https://api.anaf.ro/prod/FCTEL/rest")
	t.Setenv("SPV_TOKEN_URL", "https://logincert.anaf.ro/anaf-oauth2/v1/token")
	t.Setenv("SPV_AUTHORIZE_URL", "https://logincert.anaf.ro/anaf-oauth2/v1/authorize")
	t.Setenv("APP_ENV", "local-real")
	t.Setenv("SAGA_MODE", "file")
	t.Setenv("SPV_ENABLED", "true")
	t.Setenv("SPV_ENVIRONMENT", "PRODUCTION")
	t.Setenv("SPV_OAUTH_CLIENT_ID", "fixture-id")
	t.Setenv("SPV_OAUTH_CLIENT_SECRET", "fixture-secret")
	t.Setenv("SPV_TOKEN_ENCRYPTION_KEY", strings.Repeat("ab", 32))
	t.Setenv("PIPELINE_DISPATCH_ENABLED", "false")
	t.Setenv("CONTRACT_EXTRACTOR_MODE", "gemini")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ key, value string }{
		{"APP_ENV", "local-rela"}, {"SAGA_MODE", "fake"}, {"SPV_ENABLED", "false"},
		{"SPV_ENVIRONMENT", "typo"}, {"SPV_TOKEN_ENCRYPTION_KEY", strings.Repeat("z", 64)},
		{"SPV_TOKEN_ENCRYPTION_KEY", ""}, {"SPV_OAUTH_CLIENT_SECRET", ""},
		{"SPV_API_BASE_URL", "http://fakeanaf:8090"},
		{"SPV_TOKEN_URL", "http://fakeanaf/token"},
		{"SPV_AUTHORIZE_URL", "http://fakeanaf/authorize"},
		{"CONTRACT_EXTRACTOR_MODE", "fake-fixtures"},
		{"PIPELINE_DISPATCH_ENABLED", "true"},
	} {
		t.Run(item.key+item.value, func(t *testing.T) {
			t.Setenv(item.key, item.value)
			if _, err := Load(); err == nil {
				t.Fatal("invalid real configuration accepted")
			}
		})
	}
}

func TestInvalidRuntimeSyntaxFailsRatherThanUsingDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://example.invalid/diana")
	for _, key := range []string{"SPV_ENABLED", "WORKER_CONCURRENCY", "WORKER_JOB_TIMEOUT", "PIPELINE_DISPATCH_ENABLED"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, "invalid")
			if _, err := Load(); err == nil {
				t.Fatal("invalid syntax accepted")
			}
		})
	}
}
