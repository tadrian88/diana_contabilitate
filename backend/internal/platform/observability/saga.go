package observability

import (
	"context"
	"errors"
	"time"

	"diana-contabilitate/backend/internal/invoicing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type TracedSagaExporter struct {
	Inner   invoicing.SagaExporter
	Metrics *Metrics
}

func (e TracedSagaExporter) Export(ctx context.Context, invoice *invoicing.Invoice, idempotencyKey string) (invoicing.SagaExportResult, error) {
	started := time.Now()
	ctx, span := otel.Tracer("diana/adapters").Start(ctx, "saga.export")
	span.SetAttributes(attribute.String("invoice.id", invoice.ID), attribute.String("operation.idempotency_key", idempotencyKey))
	defer span.End()
	result, err := e.Inner.Export(ctx, invoice, idempotencyKey)
	if err != nil {
		span.RecordError(err)
	}
	if e.Metrics != nil {
		var classified interface{ Permanent() bool }
		permanent := errors.As(err, &classified) && classified.Permanent()
		e.Metrics.SAGAExportCompleted(time.Since(started), err == nil, permanent)
	}
	span.SetAttributes(attribute.Bool("saga.export.confirmed", result.Confirmed), attribute.String("saga.export.reference", result.Reference))
	return result, err
}
