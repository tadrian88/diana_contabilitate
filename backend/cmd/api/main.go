package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"diana-contabilitate/backend/internal/authentication"
	"diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/clients"
	"diana-contabilitate/backend/internal/contractingestion"
	"diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/outbox"
	"diana-contabilitate/backend/internal/platform/config"
	"diana-contabilitate/backend/internal/platform/httpserver"
	"diana-contabilitate/backend/internal/platform/observability"
	"diana-contabilitate/backend/internal/platform/postgres"
	"diana-contabilitate/backend/internal/rules"
	"diana-contabilitate/backend/internal/saga"
	"diana-contabilitate/backend/internal/spv"
	"diana-contabilitate/backend/internal/validationtasks"
	"diana-contabilitate/backend/internal/workerruntime"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	logger := observability.NewLogger(cfg.LogLevel, "api")
	shutdownTracing, err := observability.SetupTracing(context.Background(), observability.TraceConfig{Enabled: cfg.OTelEnabled, Endpoint: cfg.OTelEndpoint, ServiceName: "diana-api", Environment: cfg.Environment})
	if err != nil {
		logger.Error("tracing setup failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = shutdownTracing(context.Background()) }()
	store, err := postgres.OpenWithPool(cfg.DatabaseURL, postgres.PoolConfig{MaxOpenConnections: cfg.DatabaseMaxOpenConns, MaxIdleConnections: cfg.DatabaseMaxIdleConns, ConnectionLifetime: cfg.DatabaseConnMaxLifetime})
	if err != nil {
		logger.Error("database setup failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	contractService := contracts.NewService(store, contracts.BaselinePolicy{}, nil)
	contractExtractor := contractingestion.NewGeminiContractExtractor(cfg.GeminiAPIKey, cfg.GeminiModel, cfg.GeminiBaseURL, &http.Client{Timeout: cfg.ContractExtractionTimeout})
	contractIngestion := contractingestion.NewService(store, contractExtractor, contractService, cfg.ContractMaxPDFBytes, nil)
	metrics := observability.NewMetrics()
	store.AccountingReadinessObserver = metrics
	classificationService := classification.NewService(store, classification.ProductionPolicy{Observer: metrics}, nil)
	var sagaExporter invoicing.SagaExporter = saga.NewFileExporter(store, nil)
	if cfg.SagaMode == "fake" {
		sagaExporter = invoicing.NewFakeSagaExporter("inv-module6-saga-failed")
	}
	pipelineService := invoicing.NewPipelineService(store, sagaExporter, nil)
	pipelineService.SetContractMatchingProcessor(contractService)
	pipelineService.SetClassificationProcessor(classificationService)
	redisOptions, err := asynq.ParseRedisURI(cfg.RedisURL)
	if err != nil {
		logger.Error("invalid Asynq Redis URL", "error", err)
		os.Exit(1)
	}
	asynqClient := asynq.NewClient(redisOptions)
	defer asynqClient.Close()
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		logger.Error("invalid Redis URL")
		os.Exit(1)
	}
	redisClient := redis.NewClient(redisOpts)
	defer redisClient.Close()
	readiness := dependencyReadiness{database: store, redis: redisClient}
	var tokenCipher spv.TokenCipher
	if cfg.SPVTokenEncryptionKey != "" {
		cipher, cipherErr := spv.NewAESGCMCipher(cfg.SPVTokenEncryptionKey)
		if cipherErr != nil {
			logger.Warn("SPV connection UX is not configured", "reason", "invalid token encryption key")
		} else {
			tokenCipher = cipher
		}
	}
	spvClient := spv.NewHTTPClient(nil, cfg.SPVAPIBaseURL, cfg.SPVTokenURL).WithMinimumCallInterval(cfg.SPVMinimumCallInterval)
	spvManager := spv.NewConnectionManager(store, spvClient, tokenCipher, workerruntime.NewAsynqPublisher(asynqClient, cfg.WorkerQueue, cfg.WorkerMaxRetry, cfg.WorkerJobTimeout), spv.ConnectionManagerConfig{Environment: cfg.SPVEnvironment, AuthorizeURL: cfg.SPVAuthorizeURL, OAuthClientID: cfg.SPVOAuthClientID, OAuthClientSecret: cfg.SPVOAuthClientSecret, RedirectURI: cfg.SPVOAuthRedirectURI, FrontendBaseURL: cfg.FrontendBaseURL, StateTTL: cfg.SPVOAuthStateTTL, Enabled: cfg.SPVEnabled})
	sagaHandoff := saga.NewHandoffService(store, nil)
	handler := httpserver.NewWithContractIngestion(clients.NewService(store), pipelineService, validationtasks.NewService(store, nil), contractService, classificationService, rules.NewService(store, nil), spvManager, sagaHandoff, contractIngestion, readiness, logger, metrics)
	authHTTP := authentication.NewHTTP(authentication.SQLStore{DB: store.DB}, redisClient, authentication.Config{
		SecureCookies: cfg.Environment == "cloud-test" || cfg.Environment == "production",
		SessionTTL:    cfg.AuthSessionTTL, FrontendURL: cfg.FrontendBaseURL,
		LoginLimit: int64(cfg.AuthLoginLimit), LoginWindow: cfg.AuthLoginWindow,
	}, logger)
	handler = authHTTP.Wrap(handler)
	server := &http.Server{Addr: cfg.HTTPAddress, Handler: handler, ReadHeaderTimeout: 5 * time.Second}

	shutdownSignal, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if cfg.PipelineDispatchEnabled {
		go dispatchPipeline(shutdownSignal, pipelineService, logger)
	}

	go func() {
		logger.Info("api listening", "address", cfg.HTTPAddress, "environment", cfg.Environment)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("api stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}()

	<-shutdownSignal.Done()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
}

func dispatchPipeline(ctx context.Context, service *invoicing.Service, logger *slog.Logger) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := service.DispatchPending(ctx, 25); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("pipeline dispatch failed", "error", err)
			}
		}
	}
}

// Readiness reflects both durable storage and the queue dependency.
type dependencyReadiness struct {
	database interface{ Ping(context.Context) error }
	redis    *redis.Client
}

func (d dependencyReadiness) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := d.database.Ping(ctx); err != nil {
		return err
	}
	return d.redis.Ping(ctx).Err()
}

func (d dependencyReadiness) Stats(ctx context.Context, now time.Time) (outbox.Stats, error) {
	if provider, ok := d.database.(interface {
		Stats(context.Context, time.Time) (outbox.Stats, error)
	}); ok {
		return provider.Stats(ctx, now)
	}
	return outbox.Stats{}, errors.New("outbox statistics unavailable")
}
