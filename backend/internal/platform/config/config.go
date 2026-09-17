package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddress               string
	WorkerHTTPAddress         string
	DatabaseURL               string
	RedisURL                  string
	Environment               string
	LogLevel                  string
	PipelineDispatchEnabled   bool
	WorkerConcurrency         int
	WorkerQueue               string
	WorkerMaxRetry            int
	WorkerJobTimeout          time.Duration
	DispatcherBatchSize       int
	DispatcherMaxAttempts     int
	DispatcherPollInterval    time.Duration
	DispatcherClaimTTL        time.Duration
	DispatcherRetryMin        time.Duration
	DispatcherRetryMax        time.Duration
	ShutdownTimeout           time.Duration
	DatabaseMaxOpenConns      int
	DatabaseMaxIdleConns      int
	DatabaseConnMaxLifetime   time.Duration
	OTelEnabled               bool
	OTelEndpoint              string
	SPVEnabled                bool
	SPVEnvironment            string
	SPVAPIBaseURL             string
	SPVTokenURL               string
	SPVAuthorizeURL           string
	SPVOAuthRedirectURI       string
	FrontendBaseURL           string
	SPVOAuthStateTTL          time.Duration
	SPVOAuthClientID          string
	SPVOAuthClientSecret      string
	SPVTokenEncryptionKey     string
	SPVSyncInterval           time.Duration
	SPVInitialWindow          time.Duration
	SPVOverlapWindow          time.Duration
	SPVClaimTTL               time.Duration
	SPVMinimumCallInterval    time.Duration
	SagaMode                  string
	GeminiAPIKey              string
	GeminiModel               string
	GeminiBaseURL             string
	ContractMaxPDFBytes       int64
	ContractExtractionTimeout time.Duration
	ContractExtractorMode     string
	AuthSessionTTL            time.Duration
	AuthLoginLimit            int
	AuthLoginWindow           time.Duration
}

func Load() (Config, error) {
	for _, name := range []string{"PIPELINE_DISPATCH_ENABLED", "OTEL_ENABLED", "SPV_ENABLED"} {
		if raw := os.Getenv(name); raw != "" {
			if _, err := strconv.ParseBool(raw); err != nil {
				return Config{}, fmt.Errorf("%s has invalid syntax", name)
			}
		}
	}
	for _, name := range []string{"WORKER_CONCURRENCY", "WORKER_MAX_RETRY", "OUTBOX_BATCH_SIZE", "OUTBOX_MAX_ATTEMPTS", "DATABASE_MAX_OPEN_CONNS", "DATABASE_MAX_IDLE_CONNS", "CONTRACT_MAX_PDF_BYTES", "AUTH_LOGIN_LIMIT"} {
		if raw := os.Getenv(name); raw != "" {
			if _, err := strconv.Atoi(raw); err != nil {
				return Config{}, fmt.Errorf("%s has invalid syntax", name)
			}
		}
	}
	for _, name := range []string{"WORKER_JOB_TIMEOUT", "OUTBOX_POLL_INTERVAL", "OUTBOX_CLAIM_TTL", "OUTBOX_RETRY_MIN", "OUTBOX_RETRY_MAX", "SHUTDOWN_TIMEOUT", "DATABASE_CONN_MAX_LIFETIME", "SPV_OAUTH_STATE_TTL", "SPV_SYNC_INTERVAL", "SPV_INITIAL_WINDOW", "SPV_OVERLAP_WINDOW", "SPV_DOCUMENT_CLAIM_TTL", "SPV_MIN_CALL_INTERVAL", "CONTRACT_EXTRACTION_TIMEOUT", "AUTH_SESSION_TTL", "AUTH_LOGIN_WINDOW"} {
		if raw := os.Getenv(name); raw != "" {
			if _, err := time.ParseDuration(raw); err != nil {
				return Config{}, fmt.Errorf("%s has invalid syntax", name)
			}
		}
	}

	cfg := Config{
		HTTPAddress:               envOr("HTTP_ADDRESS", ":8080"),
		WorkerHTTPAddress:         envOr("WORKER_HTTP_ADDRESS", ":8081"),
		DatabaseURL:               os.Getenv("DATABASE_URL"),
		RedisURL:                  envOr("REDIS_URL", "redis://127.0.0.1:6382/0"),
		Environment:               strings.ToLower(strings.TrimSpace(envOr("APP_ENV", "development"))),
		LogLevel:                  envOr("LOG_LEVEL", "info"),
		PipelineDispatchEnabled:   envBool("PIPELINE_DISPATCH_ENABLED", false),
		WorkerConcurrency:         envInt("WORKER_CONCURRENCY", 10),
		WorkerQueue:               envOr("WORKER_QUEUE", "workflow"),
		WorkerMaxRetry:            envInt("WORKER_MAX_RETRY", 8),
		WorkerJobTimeout:          envDuration("WORKER_JOB_TIMEOUT", 2*time.Minute),
		DispatcherBatchSize:       envInt("OUTBOX_BATCH_SIZE", 50),
		DispatcherMaxAttempts:     envInt("OUTBOX_MAX_ATTEMPTS", 20),
		DispatcherPollInterval:    envDuration("OUTBOX_POLL_INTERVAL", time.Second),
		DispatcherClaimTTL:        envDuration("OUTBOX_CLAIM_TTL", time.Minute),
		DispatcherRetryMin:        envDuration("OUTBOX_RETRY_MIN", time.Second),
		DispatcherRetryMax:        envDuration("OUTBOX_RETRY_MAX", time.Minute),
		ShutdownTimeout:           envDuration("SHUTDOWN_TIMEOUT", 15*time.Second),
		DatabaseMaxOpenConns:      envInt("DATABASE_MAX_OPEN_CONNS", 20),
		DatabaseMaxIdleConns:      envInt("DATABASE_MAX_IDLE_CONNS", 5),
		DatabaseConnMaxLifetime:   envDuration("DATABASE_CONN_MAX_LIFETIME", 30*time.Minute),
		OTelEnabled:               envBool("OTEL_ENABLED", false),
		OTelEndpoint:              os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		SPVEnabled:                envBool("SPV_ENABLED", false),
		SPVEnvironment:            envOr("SPV_ENVIRONMENT", "TEST"),
		SPVTokenURL:               envOr("SPV_TOKEN_URL", "https://logincert.anaf.ro/anaf-oauth2/v1/token"),
		SPVAuthorizeURL:           envOr("SPV_AUTHORIZE_URL", "https://logincert.anaf.ro/anaf-oauth2/v1/authorize"),
		SPVOAuthRedirectURI:       envOr("SPV_OAUTH_REDIRECT_URI", "http://127.0.0.1:8080/api/v1/integrations/anaf/callback"),
		FrontendBaseURL:           envOr("FRONTEND_BASE_URL", "http://localhost:5173"),
		SPVOAuthStateTTL:          envDuration("SPV_OAUTH_STATE_TTL", 10*time.Minute),
		SPVOAuthClientID:          os.Getenv("SPV_OAUTH_CLIENT_ID"),
		SPVOAuthClientSecret:      os.Getenv("SPV_OAUTH_CLIENT_SECRET"),
		SPVTokenEncryptionKey:     os.Getenv("SPV_TOKEN_ENCRYPTION_KEY"),
		SPVSyncInterval:           envDuration("SPV_SYNC_INTERVAL", time.Hour),
		SPVInitialWindow:          envDuration("SPV_INITIAL_WINDOW", 60*24*time.Hour),
		SPVOverlapWindow:          envDuration("SPV_OVERLAP_WINDOW", 72*time.Hour),
		SPVClaimTTL:               envDuration("SPV_DOCUMENT_CLAIM_TTL", 10*time.Minute),
		SPVMinimumCallInterval:    envDuration("SPV_MIN_CALL_INTERVAL", 100*time.Millisecond),
		SagaMode:                  strings.ToLower(envOr("SAGA_MODE", "fake")),
		GeminiAPIKey:              os.Getenv("GEMINI_API_KEY"),
		GeminiModel:               envOr("GEMINI_CONTRACT_MODEL", "gemini-3.8-flash"),
		GeminiBaseURL:             envOr("GEMINI_API_BASE_URL", "https://generativelanguage.googleapis.com/v1beta"),
		ContractMaxPDFBytes:       int64(envInt("CONTRACT_MAX_PDF_BYTES", 20<<20)),
		ContractExtractionTimeout: envDuration("CONTRACT_EXTRACTION_TIMEOUT", 90*time.Second),
		ContractExtractorMode:     envOr("CONTRACT_EXTRACTOR_MODE", "gemini"),
		AuthSessionTTL:            envDuration("AUTH_SESSION_TTL", 12*time.Hour),
		AuthLoginLimit:            envInt("AUTH_LOGIN_LIMIT", 5),
		AuthLoginWindow:           envDuration("AUTH_LOGIN_WINDOW", 15*time.Minute),
	}
	if cfg.Environment != "development" && cfg.Environment != "test" && cfg.Environment != "local-real" && cfg.Environment != "cloud-test" && cfg.Environment != "production" {
		return Config{}, fmt.Errorf("APP_ENV must be development, test, local-real, cloud-test or production")
	}
	if (cfg.Environment == "cloud-test" || cfg.Environment == "production") && cfg.PipelineDispatchEnabled {
		return Config{}, fmt.Errorf("cloud-test and production require separate worker dispatch")
	}
	if cfg.SPVEnvironment != "PRODUCTION" && cfg.SPVEnvironment != "TEST" {
		return Config{}, fmt.Errorf("SPV_ENVIRONMENT must be TEST or PRODUCTION")
	}
	if cfg.SPVEnvironment == "PRODUCTION" {
		cfg.SPVAPIBaseURL = envOr("SPV_API_BASE_URL", "https://api.anaf.ro/prod/FCTEL/rest")
	} else {
		cfg.SPVEnvironment = "TEST"
		cfg.SPVAPIBaseURL = envOr("SPV_API_BASE_URL", "https://api.anaf.ro/test/FCTEL/rest")
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.RedisURL == "" || cfg.WorkerConcurrency < 1 || cfg.WorkerMaxRetry < 0 || cfg.DispatcherBatchSize < 1 || cfg.DispatcherMaxAttempts < 1 || cfg.WorkerJobTimeout <= 0 || cfg.DispatcherPollInterval <= 0 || cfg.DispatcherClaimTTL <= 0 || cfg.DispatcherRetryMin <= 0 || cfg.DispatcherRetryMax < cfg.DispatcherRetryMin || cfg.ShutdownTimeout <= 0 || cfg.DatabaseMaxOpenConns < 1 || cfg.DatabaseMaxIdleConns < 0 || cfg.DatabaseMaxIdleConns > cfg.DatabaseMaxOpenConns || cfg.DatabaseConnMaxLifetime <= 0 {
		return Config{}, fmt.Errorf("worker/runtime configuration is invalid")
	}
	if cfg.SPVEnabled {
		missing := missingNames(map[string]string{
			"SPV_OAUTH_CLIENT_ID":      cfg.SPVOAuthClientID,
			"SPV_OAUTH_CLIENT_SECRET":  cfg.SPVOAuthClientSecret,
			"SPV_TOKEN_ENCRYPTION_KEY": cfg.SPVTokenEncryptionKey,
			"SPV_AUTHORIZE_URL":        cfg.SPVAuthorizeURL,
			"SPV_OAUTH_REDIRECT_URI":   cfg.SPVOAuthRedirectURI,
			"FRONTEND_BASE_URL":        cfg.FrontendBaseURL,
		})
		if len(missing) > 0 {
			return Config{}, fmt.Errorf("enabled SPV configuration is missing: %s", strings.Join(missing, ", "))
		}
		if cfg.SPVSyncInterval <= 0 || cfg.SPVInitialWindow <= 0 || cfg.SPVOverlapWindow <= 0 || cfg.SPVClaimTTL <= 0 || cfg.SPVMinimumCallInterval <= 0 || cfg.SPVOAuthStateTTL <= 0 {
			return Config{}, fmt.Errorf("enabled SPV duration configuration must be positive")
		}
	}
	if cfg.SPVEnabled {
		key, err := hex.DecodeString(cfg.SPVTokenEncryptionKey)
		if err != nil || len(key) != 32 {
			return Config{}, fmt.Errorf("SPV_TOKEN_ENCRYPTION_KEY must encode 32 bytes in hex")
		}
	}
	if cfg.Environment == "local-real" {
		if !cfg.SPVEnabled || cfg.SagaMode != "file" || cfg.ContractExtractorMode != "gemini" || cfg.PipelineDispatchEnabled {
			return Config{}, fmt.Errorf("local-real requires SPV_ENABLED=true, SAGA_MODE=file, CONTRACT_EXTRACTOR_MODE=gemini and separate worker dispatch")
		}
	}
	if cfg.AuthSessionTTL <= 0 || cfg.AuthLoginLimit < 1 || cfg.AuthLoginWindow <= 0 {
		return Config{}, fmt.Errorf("authentication configuration is invalid")
	}
	if cfg.Environment == "local-real" || cfg.Environment == "cloud-test" || strings.EqualFold(cfg.Environment, "production") {
		base := "https://api.anaf.ro/test/FCTEL/rest"
		if cfg.SPVEnvironment == "PRODUCTION" {
			base = "https://api.anaf.ro/prod/FCTEL/rest"
		}
		if cfg.SPVEnabled && (cfg.SPVAPIBaseURL != base || cfg.SPVTokenURL != "https://logincert.anaf.ro/anaf-oauth2/v1/token" || cfg.SPVAuthorizeURL != "https://logincert.anaf.ro/anaf-oauth2/v1/authorize") {
			return Config{}, fmt.Errorf("real SPV mode requires official ANAF endpoints")
		}
	}
	if cfg.SagaMode != "fake" && cfg.SagaMode != "file" {
		return Config{}, fmt.Errorf("SAGA_MODE must be fake or file")
	}
	if (cfg.Environment == "cloud-test" || strings.EqualFold(cfg.Environment, "production")) && cfg.SagaMode == "fake" {
		return Config{}, fmt.Errorf("SAGA_MODE=fake is forbidden in cloud-test and production")
	}
	if cfg.ContractMaxPDFBytes < 1 || cfg.ContractExtractionTimeout <= 0 || cfg.GeminiModel == "" {
		return Config{}, fmt.Errorf("contract extraction configuration is invalid")
	}
	if cfg.ContractExtractorMode != "gemini" && cfg.ContractExtractorMode != "fake-fixtures" {
		return Config{}, fmt.Errorf("invalid contract extractor mode")
	}
	if cfg.ContractExtractorMode == "fake-fixtures" && cfg.Environment != "test" {
		return Config{}, fmt.Errorf("fake contract extractor requires APP_ENV=test")
	}
	return cfg, nil
}

func missingNames(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for name, value := range values {
		if strings.TrimSpace(value) == "" {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
