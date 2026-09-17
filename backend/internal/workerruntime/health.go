package workerruntime

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	"diana-contabilitate/backend/internal/outbox"
	"diana-contabilitate/backend/internal/platform/observability"
)

type Pinger interface{ Ping(context.Context) error }

type Health struct {
	postgres Pinger
	redis    Pinger
	ready    atomic.Bool
	metrics  *observability.Metrics
}

func NewHealth(postgres, redis Pinger, metrics *observability.Metrics) *Health {
	return &Health{postgres: postgres, redis: redis, metrics: metrics}
}

func (h *Health) SetReady(value bool) { h.ready.Store(value) }

func (h *Health) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { writeStatus(w, http.StatusOK, "ok") })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if !h.ready.Load() || h.postgres.Ping(r.Context()) != nil || h.redis.Ping(r.Context()) != nil {
			writeStatus(w, http.StatusServiceUnavailable, "not_ready")
			return
		}
		writeStatus(w, http.StatusOK, "ready")
	})
	var stats observability.OutboxStats
	if provider, ok := h.postgres.(interface {
		Stats(context.Context, time.Time) (outbox.Stats, error)
	}); ok {
		stats = provider.Stats
	}
	mux.Handle("GET /metrics", h.metrics.Handler(stats))
	return mux
}

func writeStatus(w http.ResponseWriter, status int, value string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": value})
}
