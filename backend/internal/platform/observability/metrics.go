package observability

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"diana-contabilitate/backend/internal/outbox"
)

type OutboxStats func(context.Context, time.Time) (outbox.Stats, error)

type Metrics struct {
	accountingReadiness        [2]atomic.Uint64
	classificationCounts       [6][4]atomic.Uint64
	httpRequests               atomic.Uint64
	httpDuration               atomic.Uint64
	dispatched                 atomic.Uint64
	dispatchFail               atomic.Uint64
	jobs                       atomic.Uint64
	jobDuration                atomic.Uint64
	jobFailures                atomic.Uint64
	jobRetries                 atomic.Uint64
	staleJobs                  atomic.Uint64
	progressions               atomic.Uint64
	spvSyncRuns                atomic.Uint64
	spvDiscovered              atomic.Uint64
	spvIngested                atomic.Uint64
	spvFailures                atomic.Uint64
	spvTechnicalDuplicates     atomic.Uint64
	sagaExportsAttempted       atomic.Uint64
	sagaExportsSucceeded       atomic.Uint64
	sagaExportsFailed          atomic.Uint64
	sagaPermanentFailures      atomic.Uint64
	sagaExportDuration         atomic.Uint64
	contractArrivalEvents      atomic.Uint64
	contractResumeEvaluated    atomic.Uint64
	contractAutoResumed        atomic.Uint64
	contractNeedsConfirm       atomic.Uint64
	contractStillMissing       atomic.Uint64
	contractResumeFailures     atomic.Uint64
	contractDocumentsUploaded  atomic.Uint64
	contractExtractions        atomic.Uint64
	contractExtractionFailures atomic.Uint64
	contractExtractionDuration atomic.Uint64
	contractConfirmations      atomic.Uint64
	mu                         sync.Mutex
	jobResults                 map[string]uint64
}

func NewMetrics() *Metrics { return &Metrics{jobResults: make(map[string]uint64)} }

func (m *Metrics) ObserveHTTP(duration time.Duration) {
	m.httpRequests.Add(1)
	m.httpDuration.Add(uint64(max(duration.Milliseconds(), 0)))
}

func (m *Metrics) OutboxDispatched() { m.dispatched.Add(1) }
func (m *Metrics) OutboxFailed()     { m.dispatchFail.Add(1) }
func (m *Metrics) JobRetry()         { m.jobRetries.Add(1) }
func (m *Metrics) JobFailed()        { m.jobFailures.Add(1) }
func (m *Metrics) SPVSyncCompleted(discovered int) {
	m.spvSyncRuns.Add(1)
	if discovered > 0 {
		m.spvDiscovered.Add(uint64(discovered))
	}
}
func (m *Metrics) SPVIngestionCompleted(created bool) {
	if created {
		m.spvIngested.Add(1)
	} else {
		m.spvTechnicalDuplicates.Add(1)
	}
}
func (m *Metrics) SPVIngestionFailed() { m.spvFailures.Add(1) }
func (m *Metrics) SAGAExportCompleted(duration time.Duration, succeeded, permanent bool) {
	m.sagaExportsAttempted.Add(1)
	m.sagaExportDuration.Add(uint64(max(duration.Milliseconds(), 0)))
	if succeeded {
		m.sagaExportsSucceeded.Add(1)
		return
	}
	m.sagaExportsFailed.Add(1)
	if permanent {
		m.sagaPermanentFailures.Add(1)
	}
}
func (m *Metrics) ContractResumeCompleted(evaluated, resumed, confirmation, missing int) {
	m.contractArrivalEvents.Add(1)
	m.contractResumeEvaluated.Add(uint64(max(evaluated, 0)))
	m.contractAutoResumed.Add(uint64(max(resumed, 0)))
	m.contractNeedsConfirm.Add(uint64(max(confirmation, 0)))
	m.contractStillMissing.Add(uint64(max(missing, 0)))
}
func (m *Metrics) ContractResumeFailed()     { m.contractResumeFailures.Add(1) }
func (m *Metrics) ContractDocumentUploaded() { m.contractDocumentsUploaded.Add(1) }
func (m *Metrics) ContractExtractionCompleted(duration time.Duration) {
	m.contractExtractions.Add(1)
	m.contractExtractionDuration.Add(uint64(max(duration.Milliseconds(), 0)))
}
func (m *Metrics) ContractExtractionFailed() {
	m.contractExtractions.Add(1)
	m.contractExtractionFailures.Add(1)
}
func (m *Metrics) ContractConfirmed() { m.contractConfirmations.Add(1) }
func (m *Metrics) JobCompleted(result string, duration time.Duration) {
	m.jobs.Add(1)
	m.jobDuration.Add(uint64(max(duration.Milliseconds(), 0)))
	if result == "STALE_NOOP" {
		m.staleJobs.Add(1)
	}
	if result == "PROCESSED" {
		m.progressions.Add(1)
	}
	m.mu.Lock()
	m.jobResults[safeResult(result)]++
	m.mu.Unlock()
}

func (m *Metrics) Handler(stats OutboxStats) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		var builder strings.Builder
		m.writeClassificationMetrics(&builder)
		writeCounter(&builder, "diana_http_requests_total", m.httpRequests.Load())
		writeCounter(&builder, "diana_http_request_duration_milliseconds_total", m.httpDuration.Load())
		writeCounter(&builder, "diana_outbox_dispatched_total", m.dispatched.Load())
		writeCounter(&builder, "diana_outbox_dispatch_failures_total", m.dispatchFail.Load())
		writeCounter(&builder, "diana_worker_jobs_processed_total", m.jobs.Load())
		writeCounter(&builder, "diana_worker_job_duration_milliseconds_total", m.jobDuration.Load())
		writeCounter(&builder, "diana_pipeline_processing_duration_milliseconds_total", m.jobDuration.Load())
		writeCounter(&builder, "diana_worker_job_failures_total", m.jobFailures.Load())
		writeCounter(&builder, "diana_worker_job_retries_total", m.jobRetries.Load())
		writeCounter(&builder, "diana_worker_stale_jobs_total", m.staleJobs.Load())
		writeCounter(&builder, "diana_pipeline_progressions_total", m.progressions.Load())
		writeCounter(&builder, "diana_spv_sync_runs_total", m.spvSyncRuns.Load())
		writeCounter(&builder, "diana_spv_documents_discovered_total", m.spvDiscovered.Load())
		writeCounter(&builder, "diana_spv_ingestions_succeeded_total", m.spvIngested.Load())
		writeCounter(&builder, "diana_spv_ingestions_failed_total", m.spvFailures.Load())
		writeCounter(&builder, "diana_spv_technical_duplicates_total", m.spvTechnicalDuplicates.Load())
		writeCounter(&builder, "diana_saga_exports_attempted_total", m.sagaExportsAttempted.Load())
		writeCounter(&builder, "diana_saga_export_artifacts_generated_total", m.sagaExportsSucceeded.Load())
		writeCounter(&builder, "diana_saga_exports_failed_total", m.sagaExportsFailed.Load())
		writeCounter(&builder, "diana_saga_export_permanent_failures_total", m.sagaPermanentFailures.Load())
		writeCounter(&builder, "diana_saga_export_duration_milliseconds_total", m.sagaExportDuration.Load())
		writeCounter(&builder, "diana_contract_arrival_events_total", m.contractArrivalEvents.Load())
		writeCounter(&builder, "diana_contract_resume_invoices_evaluated_total", m.contractResumeEvaluated.Load())
		writeCounter(&builder, "diana_contract_resume_automatic_total", m.contractAutoResumed.Load())
		writeCounter(&builder, "diana_contract_resume_confirmation_total", m.contractNeedsConfirm.Load())
		writeCounter(&builder, "diana_contract_resume_still_missing_total", m.contractStillMissing.Load())
		writeCounter(&builder, "diana_contract_resume_failures_total", m.contractResumeFailures.Load())
		writeCounter(&builder, "contract_documents_uploaded_total", m.contractDocumentsUploaded.Load())
		writeCounter(&builder, "contract_extractions_total", m.contractExtractions.Load())
		writeCounter(&builder, "contract_extraction_failures_total", m.contractExtractionFailures.Load())
		writeCounter(&builder, "contract_extraction_duration_milliseconds_total", m.contractExtractionDuration.Load())
		writeCounter(&builder, "contract_extraction_confirmed_total", m.contractConfirmations.Load())
		m.mu.Lock()
		for result, value := range m.jobResults {
			builder.WriteString("diana_worker_job_results_total{result=\"")
			builder.WriteString(result)
			builder.WriteString("\"} ")
			builder.WriteString(strconv.FormatUint(value, 10))
			builder.WriteByte('\n')
		}
		m.mu.Unlock()
		if stats != nil {
			if value, err := stats(r.Context(), time.Now().UTC()); err == nil {
				builder.WriteString(fmt.Sprintf("diana_outbox_pending %d\n", value.PendingCount))
				builder.WriteString(fmt.Sprintf("diana_outbox_failed %d\n", value.FailedCount))
				builder.WriteString(fmt.Sprintf("diana_outbox_oldest_pending_seconds %.3f\n", value.OldestPendingAge.Seconds()))
			}
		}
		_, _ = w.Write([]byte(builder.String()))
	})
}

func writeCounter(builder *strings.Builder, name string, value uint64) {
	builder.WriteString(name)
	builder.WriteByte(' ')
	builder.WriteString(strconv.FormatUint(value, 10))
	builder.WriteByte('\n')
}

func safeResult(value string) string {
	switch value {
	case "PROCESSED", "STALE_NOOP", "FAILED", "RETRY":
		return value
	default:
		return "UNKNOWN"
	}
}
