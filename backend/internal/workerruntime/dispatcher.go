package workerruntime

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"time"

	"diana-contabilitate/backend/internal/outbox"
	"diana-contabilitate/backend/internal/platform/observability"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type DispatcherConfig struct {
	Owner        string
	BatchSize    int
	MaxAttempts  uint
	PollInterval time.Duration
	ClaimTTL     time.Duration
	RetryMin     time.Duration
	RetryMax     time.Duration
}

type Dispatcher struct {
	store     outbox.Store
	publisher Publisher
	config    DispatcherConfig
	logger    *slog.Logger
	metrics   *observability.Metrics
	clock     func() time.Time
}

func NewDispatcher(store outbox.Store, publisher Publisher, config DispatcherConfig, logger *slog.Logger, metrics *observability.Metrics) *Dispatcher {
	return &Dispatcher{store: store, publisher: publisher, config: config, logger: logger, metrics: metrics, clock: func() time.Time { return time.Now().UTC() }}
}

func (d *Dispatcher) DispatchBatch(ctx context.Context) (int, error) {
	now := d.clock()
	ctx, span := otel.Tracer("diana/worker").Start(ctx, "outbox.dispatch")
	span.SetAttributes(attribute.String("outbox.claim_owner", d.config.Owner), attribute.Int("outbox.batch_size", d.config.BatchSize))
	defer span.End()
	entries, err := d.store.Claim(ctx, d.config.Owner, d.config.BatchSize, now, d.config.ClaimTTL)
	if err != nil {
		span.RecordError(err)
		return 0, err
	}
	dispatched := 0
	var failures []error
	for _, entry := range entries {
		jobID, publishErr := d.publisher.Publish(ctx, JobFromOutbox(entry))
		if publishErr != nil {
			d.metrics.OutboxFailed()
			if entry.Attempts >= d.config.MaxAttempts {
				failErr := d.store.MarkFailed(ctx, entry.ID, d.config.Owner, publishErr.Error(), d.clock())
				failures = append(failures, publishErr, failErr)
				d.logger.Error("outbox publish retries exhausted", "outbox_id", entry.ID, "invoice_id", entry.AggregateID, "correlation_id", entry.CorrelationID, "attempt", entry.Attempts, "error", publishErr)
				continue
			}
			delay := d.backoff(entry.Attempts)
			releaseErr := d.store.ReleaseClaim(ctx, entry.ID, d.config.Owner, publishErr.Error(), d.clock().Add(delay))
			failures = append(failures, publishErr, releaseErr)
			d.logger.Error("outbox publish failed", "outbox_id", entry.ID, "invoice_id", entry.AggregateID, "correlation_id", entry.CorrelationID, "attempt", entry.Attempts, "retry_in_ms", delay.Milliseconds(), "error", publishErr)
			continue
		}
		if err = d.store.MarkDispatched(ctx, entry.ID, d.config.Owner, d.clock()); err != nil {
			// The job may already be in Redis. Leave the claim to expire so the row
			// is republished; consumer idempotency closes this crash window.
			d.metrics.OutboxFailed()
			failures = append(failures, err)
			d.logger.Error("outbox dispatch mark failed", "outbox_id", entry.ID, "job_id", jobID, "invoice_id", entry.AggregateID, "correlation_id", entry.CorrelationID, "error", err)
			continue
		}
		d.metrics.OutboxDispatched()
		dispatched++
		d.logger.Info("outbox dispatched", "outbox_id", entry.ID, "job_id", jobID, "invoice_id", entry.AggregateID, "correlation_id", entry.CorrelationID, "attempt", entry.Attempts)
	}
	return dispatched, errors.Join(failures...)
}

func (d *Dispatcher) Run(ctx context.Context) error {
	ticker := time.NewTicker(d.config.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if _, err := d.DispatchBatch(ctx); err != nil && !errors.Is(err, context.Canceled) {
			d.logger.Error("outbox batch failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (d *Dispatcher) backoff(attempt uint) time.Duration {
	if attempt <= 1 {
		return d.config.RetryMin
	}
	exponent := min(float64(attempt-1), 20)
	delay := time.Duration(float64(d.config.RetryMin) * math.Pow(2, exponent))
	if delay > d.config.RetryMax {
		return d.config.RetryMax
	}
	return delay
}
