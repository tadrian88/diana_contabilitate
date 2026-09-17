package observability

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

type TraceConfig struct {
	Enabled     bool
	Endpoint    string
	ServiceName string
	Environment string
}

func SetupTracing(ctx context.Context, cfg TraceConfig) (func(context.Context) error, error) {
	if !cfg.Enabled {
		return func(context.Context) error { return nil }, nil
	}
	options := []otlptracegrpc.Option{}
	if strings.TrimSpace(cfg.Endpoint) != "" {
		options = append(options, otlptracegrpc.WithEndpointURL(cfg.Endpoint))
	}
	exporter, err := otlptracegrpc.New(ctx, options...)
	if err != nil {
		return nil, err
	}
	res, err := resource.New(ctx, resource.WithAttributes(
		semconv.ServiceName(cfg.ServiceName),
		attribute.String("deployment.environment", cfg.Environment),
	))
	if err != nil {
		return nil, err
	}
	provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter), sdktrace.WithResource(res))
	otel.SetTracerProvider(provider)
	return provider.Shutdown, nil
}

func NewLogger(level, process string) *slog.Logger {
	var parsed slog.Level
	if err := parsed.UnmarshalText([]byte(level)); err != nil {
		parsed = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parsed})).With("service", "diana-backend", "process", process)
}
