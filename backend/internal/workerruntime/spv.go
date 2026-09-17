package workerruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"diana-contabilitate/backend/internal/platform/observability"
	"diana-contabilitate/backend/internal/spv"

	"github.com/hibiken/asynq"
)

const (
	SPVSyncTask     = "spv:sync_account"
	SPVDocumentTask = "spv:ingest_document"
)

type SPVSyncJob struct {
	ConnectionID string `json:"connection_id"`
}
type SPVDocumentJob struct {
	DocumentID string `json:"document_id"`
}

type SPVProcessor interface {
	Connections(context.Context) ([]spv.Connection, error)
	Sync(context.Context, string) (spv.SyncResult, error)
	ProcessDocument(context.Context, string, string) (string, bool, error)
}
type SPVPublisher interface {
	PublishSPVSync(context.Context, string) (string, error)
	PublishSPVDocument(context.Context, string) (string, error)
}

type SPVHandler struct {
	service   SPVProcessor
	publisher SPVPublisher
	owner     string
	logger    *slog.Logger
	metrics   *observability.Metrics
}

func NewSPVHandler(service SPVProcessor, publisher SPVPublisher, owner string, logger *slog.Logger, metrics *observability.Metrics) *SPVHandler {
	return &SPVHandler{service: service, publisher: publisher, owner: owner, logger: logger, metrics: metrics}
}

func (h *SPVHandler) ProcessSync(ctx context.Context, task *asynq.Task) error {
	var job SPVSyncJob
	if task.Type() != SPVSyncTask || json.Unmarshal(task.Payload(), &job) != nil || job.ConnectionID == "" {
		return fmt.Errorf("%w: malformed SPV sync job", asynq.SkipRetry)
	}
	started := time.Now()
	jobID, _ := asynq.GetTaskID(ctx)
	h.logger.Info("SPV sync started", "job_id", jobID, "connection_id", job.ConnectionID)
	result, err := h.service.Sync(ctx, job.ConnectionID)
	if err != nil {
		return spvTaskError(err)
	}
	for _, document := range result.Documents {
		if document.Status == "PROCESSED" || document.FailureKind == "PERMANENT" {
			continue
		}
		if _, err = h.publisher.PublishSPVDocument(ctx, document.ID); err != nil {
			return err
		}
	}
	h.metrics.SPVSyncCompleted(len(result.Documents))
	h.logger.Info("SPV sync completed", "source", "ANAF_SPV", "job_id", jobID, "connection_id", job.ConnectionID, "pages", result.Pages, "documents", len(result.Documents), "duration_ms", time.Since(started).Milliseconds())
	return nil
}

func (h *SPVHandler) ProcessDocument(ctx context.Context, task *asynq.Task) error {
	var job SPVDocumentJob
	if task.Type() != SPVDocumentTask || json.Unmarshal(task.Payload(), &job) != nil || job.DocumentID == "" {
		return fmt.Errorf("%w: malformed SPV document job", asynq.SkipRetry)
	}
	started := time.Now()
	jobID, _ := asynq.GetTaskID(ctx)
	h.logger.Info("SPV document processing started", "job_id", jobID, "source_document_id", job.DocumentID)
	invoiceID, created, err := h.service.ProcessDocument(ctx, job.DocumentID, h.owner)
	if errors.Is(err, spv.ErrDocumentClaimed) {
		return nil
	}
	if err != nil {
		h.metrics.SPVIngestionFailed()
		return spvTaskError(err)
	}
	h.metrics.SPVIngestionCompleted(created)
	h.logger.Info("SPV document ingested", "source", "ANAF_SPV", "job_id", jobID, "source_document_id", job.DocumentID, "invoice_id", invoiceID, "created", created, "duration_ms", time.Since(started).Milliseconds())
	return nil
}

func spvTaskError(err error) error {
	if errors.Is(err, spv.ErrPermanent) {
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	return err
}

type SPVScheduler struct {
	service   SPVProcessor
	publisher SPVPublisher
	interval  time.Duration
	logger    *slog.Logger
}

func NewSPVScheduler(service SPVProcessor, publisher SPVPublisher, interval time.Duration, logger *slog.Logger) *SPVScheduler {
	return &SPVScheduler{service: service, publisher: publisher, interval: interval, logger: logger}
}
func (s *SPVScheduler) Run(ctx context.Context) error {
	if s.interval <= 0 {
		return fmt.Errorf("SPV sync interval must be positive")
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		if err := s.enqueue(ctx); err != nil && !errors.Is(err, context.Canceled) {
			s.logger.Error("SPV scheduler iteration failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
func (s *SPVScheduler) enqueue(ctx context.Context) error {
	connections, err := s.service.Connections(ctx)
	if err != nil {
		return err
	}
	for _, connection := range connections {
		if _, err = s.publisher.PublishSPVSync(ctx, connection.ID); err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
			return err
		}
	}
	return nil
}
