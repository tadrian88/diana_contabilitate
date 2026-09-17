//go:build redis_integration

package workerruntime

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/platform/observability"
	"diana-contabilitate/backend/internal/spv"

	"github.com/hibiken/asynq"
)

type fakeSPVProcessor struct{ processed atomic.Int32 }

func (*fakeSPVProcessor) Connections(context.Context) ([]spv.Connection, error) {
	return []spv.Connection{{ID: "connection-1"}}, nil
}
func (*fakeSPVProcessor) Sync(context.Context, string) (spv.SyncResult, error) {
	return spv.SyncResult{Pages: 1, Documents: []spv.SourceDocument{{ID: "document-1", Status: "DISCOVERED"}}}, nil
}
func (p *fakeSPVProcessor) ProcessDocument(context.Context, string, string) (string, bool, error) {
	if p.processed.CompareAndSwap(0, 1) {
		return "invoice-1", true, nil
	}
	return "invoice-1", false, nil
}

func TestRealAsynqSPVSyncDispatchesDocumentByIdentifier(t *testing.T) {
	options, _ := isolatedRedis(t)
	client := asynq.NewClient(options)
	defer client.Close()
	publisher := NewAsynqPublisher(client, "workflow-test", 3, time.Minute)
	processor := &fakeSPVProcessor{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewSPVHandler(processor, publisher, "worker-test", logger, observability.NewMetrics())
	server := asynq.NewServer(options, asynq.Config{Concurrency: 2, Queues: map[string]int{"workflow-test": 1}, DelayedTaskCheckInterval: 20 * time.Millisecond})
	mux := asynq.NewServeMux()
	mux.HandleFunc(SPVSyncTask, handler.ProcessSync)
	mux.HandleFunc(SPVDocumentTask, handler.ProcessDocument)
	if err := server.Start(mux); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Shutdown)
	if _, err := publisher.PublishSPVSync(context.Background(), "connection-1"); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return processor.processed.Load() == 1 })
}
