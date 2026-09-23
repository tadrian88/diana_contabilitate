package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"diana-contabilitate/backend/internal/accountinganalysis"
	"diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/commercialvalidation"
	"diana-contabilitate/backend/internal/contractingestion"
	"diana-contabilitate/backend/internal/contractingestion/fixtures"
	"diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/platform/config"
	"diana-contabilitate/backend/internal/platform/observability"
	"diana-contabilitate/backend/internal/platform/postgres"
	"diana-contabilitate/backend/internal/saga"
	"diana-contabilitate/backend/internal/spv"
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
	logger := observability.NewLogger(cfg.LogLevel, "worker")
	root, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	shutdownTracing, err := observability.SetupTracing(root, observability.TraceConfig{Enabled: cfg.OTelEnabled, Endpoint: cfg.OTelEndpoint, ServiceName: "diana-worker", Environment: cfg.Environment})
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
	redisOptions, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		logger.Error("invalid redis URL", "error", err)
		os.Exit(1)
	}
	redisClient := redis.NewClient(redisOptions)
	defer redisClient.Close()
	asynqOptions, err := asynq.ParseRedisURI(cfg.RedisURL)
	if err != nil {
		logger.Error("invalid Asynq Redis URL", "error", err)
		os.Exit(1)
	}
	asynqClient := asynq.NewClient(asynqOptions)
	defer asynqClient.Close()

	metrics := observability.NewMetrics()
	store.AccountingReadinessObserver = metrics
	var sagaExporter invoicing.SagaExporter = saga.NewFileExporter(store, nil)
	if cfg.SagaMode == "fake" {
		sagaExporter = invoicing.NewFakeSagaExporter("inv-module6-saga-failed", "inv-module7-saga-failed")
	}
	exporter := observability.TracedSagaExporter{Inner: sagaExporter, Metrics: metrics}
	pipeline := invoicing.NewPipelineService(store, exporter, nil)
	contractService := contracts.NewService(store, contracts.BaselinePolicy{}, nil)
	var contractExtractor contractingestion.ContractExtractor = contractingestion.NewGeminiContractExtractor(cfg.GeminiAPIKey, cfg.GeminiModel, cfg.GeminiBaseURL, &http.Client{Timeout: cfg.ContractExtractionTimeout})
	if cfg.ContractExtractorMode == "fake-fixtures" {
		contractExtractor = fixtures.Extractor{}
	}
	contractIngestion := contractingestion.NewService(store, contractExtractor, contractService, cfg.ContractMaxPDFBytes, nil)
	pipeline.SetContractMatchingProcessor(contractService)
	pipeline.SetCommercialValidationProcessor(commercialvalidation.NewService(store, nil))
	pipeline.SetClassificationProcessor(classification.NewService(store, classification.ProductionPolicy{Observer: metrics}, nil))
	publisher := workerruntime.NewAsynqPublisher(asynqClient, cfg.WorkerQueue, cfg.WorkerMaxRetry, cfg.WorkerJobTimeout)
	dispatcher := workerruntime.NewDispatcher(store, publisher, workerruntime.DispatcherConfig{Owner: ownerID(), BatchSize: cfg.DispatcherBatchSize, MaxAttempts: uint(cfg.DispatcherMaxAttempts), PollInterval: cfg.DispatcherPollInterval, ClaimTTL: cfg.DispatcherClaimTTL, RetryMin: cfg.DispatcherRetryMin, RetryMax: cfg.DispatcherRetryMax}, logger, metrics)

	worker := asynq.NewServer(asynqOptions, asynq.Config{
		Concurrency:     cfg.WorkerConcurrency,
		Queues:          map[string]int{cfg.WorkerQueue: 1},
		ShutdownTimeout: cfg.ShutdownTimeout,
		RetryDelayFunc: func(attempt int, _ error, _ *asynq.Task) time.Duration {
			return boundedBackoff(attempt, cfg.DispatcherRetryMin, cfg.DispatcherRetryMax)
		},
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, taskErr error) {
			attempt, _ := asynq.GetRetryCount(ctx)
			jobID, _ := asynq.GetTaskID(ctx)
			logger.Error("Asynq task failed", "job_id", jobID, "job_type", task.Type(), "attempt", attempt+1, "error", taskErr)
			if attempt >= cfg.WorkerMaxRetry {
				metrics.JobFailed()
			}
		}),
	})
	mux := asynq.NewServeMux()
	mux.Handle(workerruntime.ContinueInvoiceTask, workerruntime.NewHandler(pipeline, logger, metrics))
	mux.Handle(workerruntime.ContractAvailableTask, workerruntime.NewContractAvailableHandler(contractService, logger, metrics))
	mux.Handle(workerruntime.ContractExtractionTask, workerruntime.NewContractExtractionHandler(contractIngestion, logger, metrics))
	mux.Handle(workerruntime.ContractActivationTask, workerruntime.NewContractActivationHandler(contractService))
	if cfg.AccountingAnalysisEnabled {
		analyzer := accountinganalysis.NewGeminiAnalyzer(cfg.GeminiAPIKey, cfg.AccountingAnalysisModel, cfg.GeminiBaseURL, &http.Client{Timeout: cfg.AccountingAnalysisTimeout})
		analysisService := accountinganalysis.NewWorkflowService(store, nil, analyzer, "gemini", cfg.AccountingAnalysisModel, metrics)
		mux.Handle(workerruntime.AccountingAnalysisTask, workerruntime.NewAccountingAnalysisHandler(analysisService))
	}
	var spvScheduler *workerruntime.SPVScheduler
	if cfg.SPVEnabled {
		cipher, cipherErr := spv.NewAESGCMCipher(cfg.SPVTokenEncryptionKey)
		if cipherErr != nil {
			logger.Error("SPV encryption setup failed", "error", cipherErr)
			os.Exit(1)
		}
		spvClient := spv.NewHTTPClient(nil, cfg.SPVAPIBaseURL, cfg.SPVTokenURL).WithMinimumCallInterval(cfg.SPVMinimumCallInterval)
		spvService := spv.NewService(store, spvClient, spv.UBLParser{}, pipeline, cipher, spv.ServiceConfig{OAuthClientID: cfg.SPVOAuthClientID, OAuthClientSecret: cfg.SPVOAuthClientSecret, Environment: cfg.SPVEnvironment, InitialWindow: cfg.SPVInitialWindow, Overlap: cfg.SPVOverlapWindow, ClaimTTL: cfg.SPVClaimTTL, RetryDelay: cfg.DispatcherRetryMin})
		spvHandler := workerruntime.NewSPVHandler(spvService, publisher, ownerID(), logger, metrics)
		mux.HandleFunc(workerruntime.SPVSyncTask, spvHandler.ProcessSync)
		mux.HandleFunc(workerruntime.SPVDocumentTask, spvHandler.ProcessDocument)
		spvScheduler = workerruntime.NewSPVScheduler(spvService, publisher, cfg.SPVSyncInterval, logger)
	}

	health := workerruntime.NewHealth(store, redisPinger{client: redisClient}, metrics)
	healthServer := &http.Server{Addr: cfg.WorkerHTTPAddress, Handler: health.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Info("worker health server listening", "address", cfg.WorkerHTTPAddress)
		if serveErr := healthServer.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logger.Error("worker health server stopped", "error", serveErr)
			stop()
		}
	}()
	if err = worker.Start(mux); err != nil {
		logger.Error("Asynq worker failed to start", "error", err)
		os.Exit(1)
	}
	health.SetReady(true)
	dispatchContext, cancelDispatch := context.WithCancel(root)
	dispatchDone := make(chan struct{})
	go func() {
		defer close(dispatchDone)
		_ = dispatcher.Run(dispatchContext)
	}()
	spvScheduleDone := make(chan struct{})
	if spvScheduler != nil {
		go func() { defer close(spvScheduleDone); _ = spvScheduler.Run(dispatchContext) }()
	} else {
		close(spvScheduleDone)
	}
	logger.Info("worker started", "concurrency", cfg.WorkerConcurrency, "queue", cfg.WorkerQueue)

	<-root.Done()
	health.SetReady(false)
	cancelDispatch()
	<-dispatchDone
	<-spvScheduleDone
	worker.Shutdown()
	shutdownContext, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err = healthServer.Shutdown(shutdownContext); err != nil {
		logger.Error("worker health shutdown failed", "error", err)
	}
	logger.Info("worker stopped")
}

type redisPinger struct{ client *redis.Client }

func (p redisPinger) Ping(ctx context.Context) error { return p.client.Ping(ctx).Err() }

func ownerID() string { return fmt.Sprintf("%s:%d", hostname(), os.Getpid()) }

func hostname() string {
	value, err := os.Hostname()
	if err != nil {
		return "worker"
	}
	return value
}

func boundedBackoff(attempt int, minimum, maximum time.Duration) time.Duration {
	if attempt < 1 {
		return minimum
	}
	delay := minimum
	for index := 1; index < attempt && delay < maximum/2; index++ {
		delay *= 2
	}
	if delay > maximum {
		return maximum
	}
	return delay
}
