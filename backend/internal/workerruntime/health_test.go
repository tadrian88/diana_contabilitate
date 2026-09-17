package workerruntime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"diana-contabilitate/backend/internal/platform/observability"
)

type pingerStub struct{ err error }

func (p pingerStub) Ping(context.Context) error { return p.err }

func TestWorkerLivenessIsIndependentFromDependenciesAndReadinessIsNot(t *testing.T) {
	health := NewHealth(pingerStub{err: errors.New("postgres down")}, pingerStub{err: errors.New("redis down")}, observability.NewMetrics())
	health.SetReady(true)
	server := httptest.NewServer(health.Handler())
	defer server.Close()
	response, err := http.Get(server.URL + "/healthz")
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("liveness status=%v err=%v", response.StatusCode, err)
	}
	_ = response.Body.Close()
	response, err = http.Get(server.URL + "/readyz")
	if err != nil || response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("readiness status=%v err=%v", response.StatusCode, err)
	}
	_ = response.Body.Close()
}
