package httpserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountinganalysis"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/clients"
	"diana-contabilitate/backend/internal/commercialvalidation"
	"diana-contabilitate/backend/internal/contractingestion"
	contractdomain "diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/outbox"
	"diana-contabilitate/backend/internal/platform/observability"
	"diana-contabilitate/backend/internal/platform/requestactor"
	"diana-contabilitate/backend/internal/rules"
	"diana-contabilitate/backend/internal/saga"
	spvdomain "diana-contabilitate/backend/internal/spv"
	"diana-contabilitate/backend/internal/validationtasks"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type Readiness interface{ Ping(context.Context) error }

type Server struct {
	clients              *clients.Service
	invoices             *invoicing.Service
	tasks                *validationtasks.Service
	contracts            *contractdomain.Service
	contractIngestion    *contractingestion.Service
	commercialValidation *commercialvalidation.Service
	accountingAnalysis   *accountinganalysis.WorkflowService
	classifications      *classification.Service
	rules                *rules.Service
	spv                  *spvdomain.ConnectionManager
	sagaHandoff          *saga.HandoffService
	ready                Readiness
	logger               *slog.Logger
	metrics              *observability.Metrics
}

func New(clientService *clients.Service, invoiceService *invoicing.Service, taskService *validationtasks.Service, contractService *contractdomain.Service, classificationService *classification.Service, ruleService *rules.Service, ready Readiness, logger *slog.Logger) http.Handler {
	return NewWithMetrics(clientService, invoiceService, taskService, contractService, classificationService, ruleService, ready, logger, observability.NewMetrics())
}

func NewWithMetrics(clientService *clients.Service, invoiceService *invoicing.Service, taskService *validationtasks.Service, contractService *contractdomain.Service, classificationService *classification.Service, ruleService *rules.Service, ready Readiness, logger *slog.Logger, metrics *observability.Metrics) http.Handler {
	return NewWithSPV(clientService, invoiceService, taskService, contractService, classificationService, ruleService, nil, ready, logger, metrics)
}

func NewWithSPV(clientService *clients.Service, invoiceService *invoicing.Service, taskService *validationtasks.Service, contractService *contractdomain.Service, classificationService *classification.Service, ruleService *rules.Service, spvService *spvdomain.ConnectionManager, ready Readiness, logger *slog.Logger, metrics *observability.Metrics) http.Handler {
	return NewWithIntegrations(clientService, invoiceService, taskService, contractService, classificationService, ruleService, spvService, nil, ready, logger, metrics)
}

func NewWithIntegrations(clientService *clients.Service, invoiceService *invoicing.Service, taskService *validationtasks.Service, contractService *contractdomain.Service, classificationService *classification.Service, ruleService *rules.Service, spvService *spvdomain.ConnectionManager, sagaHandoff *saga.HandoffService, ready Readiness, logger *slog.Logger, metrics *observability.Metrics) http.Handler {
	return NewWithContractIngestion(clientService, invoiceService, taskService, contractService, classificationService, ruleService, spvService, sagaHandoff, nil, ready, logger, metrics)
}

func NewWithContractIngestion(clientService *clients.Service, invoiceService *invoicing.Service, taskService *validationtasks.Service, contractService *contractdomain.Service, classificationService *classification.Service, ruleService *rules.Service, spvService *spvdomain.ConnectionManager, sagaHandoff *saga.HandoffService, ingestion *contractingestion.Service, ready Readiness, logger *slog.Logger, metrics *observability.Metrics) http.Handler {
	return NewWithCommercialValidation(clientService, invoiceService, taskService, contractService, classificationService, ruleService, spvService, sagaHandoff, ingestion, nil, ready, logger, metrics)
}

func NewWithCommercialValidation(clientService *clients.Service, invoiceService *invoicing.Service, taskService *validationtasks.Service, contractService *contractdomain.Service, classificationService *classification.Service, ruleService *rules.Service, spvService *spvdomain.ConnectionManager, sagaHandoff *saga.HandoffService, ingestion *contractingestion.Service, commercial *commercialvalidation.Service, ready Readiness, logger *slog.Logger, metrics *observability.Metrics) http.Handler {
	return NewWithAccountingAnalysis(clientService, invoiceService, taskService, contractService, classificationService, ruleService, spvService, sagaHandoff, ingestion, commercial, nil, ready, logger, metrics)
}

func NewWithAccountingAnalysis(clientService *clients.Service, invoiceService *invoicing.Service, taskService *validationtasks.Service, contractService *contractdomain.Service, classificationService *classification.Service, ruleService *rules.Service, spvService *spvdomain.ConnectionManager, sagaHandoff *saga.HandoffService, ingestion *contractingestion.Service, commercial *commercialvalidation.Service, analysis *accountinganalysis.WorkflowService, ready Readiness, logger *slog.Logger, metrics *observability.Metrics) http.Handler {
	s := &Server{clients: clientService, invoices: invoiceService, tasks: taskService, contracts: contractService, contractIngestion: ingestion, commercialValidation: commercial, accountingAnalysis: analysis, classifications: classificationService, rules: ruleService, spv: spvService, sagaHandoff: sagaHandoff, ready: ready, logger: logger, metrics: metrics}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /readyz", s.readiness)
	var stats observability.OutboxStats
	if provider, ok := ready.(interface {
		Stats(context.Context, time.Time) (outbox.Stats, error)
	}); ok {
		stats = provider.Stats
	}
	mux.Handle("GET /metrics", metrics.Handler(stats))
	mux.HandleFunc("GET /api/v1/clients", s.listClients)
	mux.HandleFunc("POST /api/v1/clients", s.createClient)
	mux.HandleFunc("GET /api/v1/clients/{id}", s.getClient)
	mux.HandleFunc("POST /api/v1/clients/{id}/company", s.updateClient)
	mux.HandleFunc("POST /api/v1/clients/{id}/lifecycle", s.clientLifecycle)
	mux.HandleFunc("GET /api/v1/clients/{id}/onboarding", s.getClientOnboarding)
	mux.HandleFunc("GET /api/v1/clients/{id}/accounting-profiles", s.getClientProfiles)
	mux.HandleFunc("POST /api/v1/clients/{id}/accounting-profiles", s.createClientProfile)
	mux.HandleFunc("POST /api/v1/clients/{id}/saga-configuration", s.configureClientSaga)
	mux.HandleFunc("GET /api/v1/invoices", s.listInvoices)
	mux.HandleFunc("GET /api/v1/accounts", s.searchAccounts)
	mux.HandleFunc("GET /api/v1/invoices/{id}", s.getInvoice)
	mux.HandleFunc("GET /api/v1/validation-tasks", s.listValidationTasks)
	mux.HandleFunc("POST /api/v1/invoices/{id}/contract-requests", s.requestMissingContract)
	mux.HandleFunc("GET /api/v1/contracts", s.listContracts)
	mux.HandleFunc("GET /api/v1/contracts/{id}", s.getContract)
	mux.HandleFunc("POST /api/v1/contracts/{id}/archive", s.archiveContract)
	mux.HandleFunc("POST /api/v1/contracts/{id}/discard", s.discardContract)
	mux.HandleFunc("GET /api/v1/contracts/{id}/invoices", s.listContractInvoices)
	mux.HandleFunc("POST /api/v1/invoices/{id}/contract-confirmations", s.confirmContractMatch)
	mux.HandleFunc("POST /api/v1/invoices/{id}/classification-decisions", s.reviewClassification)
	mux.HandleFunc("GET /api/v1/rules", s.listRules)
	mux.HandleFunc("GET /api/v1/rules/{id}", s.getRule)
	mux.HandleFunc("POST /api/v1/rules/{id}/versions", s.createRuleVersion)
	mux.HandleFunc("POST /api/v1/rules/{id}/client-overrides", s.createClientOverride)
	mux.HandleFunc("GET /api/v1/clients/{id}/spv", s.getSPVConnection)
	mux.HandleFunc("POST /api/v1/clients/{id}/spv/oauth/start", s.startSPVOAuth)
	mux.HandleFunc("GET /api/v1/integrations/anaf/callback", s.handleANAFCallback)
	mux.HandleFunc("POST /api/v1/clients/{id}/spv/sync", s.requestSPVSync)
	mux.HandleFunc("POST /api/v1/clients/{id}/spv/disconnect", s.disconnectSPV)
	mux.HandleFunc("GET /api/v1/clients/{clientId}/invoices/{invoiceId}/saga-export", s.getSagaExport)
	mux.HandleFunc("GET /api/v1/clients/{clientId}/invoices/{invoiceId}/saga-export/artifact", s.downloadSagaArtifact)
	mux.HandleFunc("POST /api/v1/clients/{clientId}/invoices/{invoiceId}/saga-export/confirm-import", s.confirmSagaImport)
	mux.HandleFunc("POST /api/v1/clients/{clientId}/contract-documents", s.uploadContractDocument)
	mux.HandleFunc("GET /api/v1/clients/{clientId}/contract-documents", s.listContractDocuments)
	mux.HandleFunc("GET /api/v1/clients/{clientId}/contract-documents/{documentId}", s.getContractDocument)
	mux.HandleFunc("GET /api/v1/clients/{clientId}/contract-documents/{documentId}/extraction", s.getContractDocument)
	mux.HandleFunc("GET /api/v1/clients/{clientId}/contract-documents/{documentId}/file", s.downloadContractDocument)
	mux.HandleFunc("POST /api/v1/clients/{clientId}/contract-documents/{documentId}/confirm", s.confirmContractDocument)
	mux.HandleFunc("POST /api/v1/clients/{clientId}/contract-documents/{documentId}/commercial-rules/{ruleId}/confirm", s.confirmProposedCommercialRule)
	mux.HandleFunc("POST /api/v1/clients/{clientId}/contract-documents/{documentId}/reviewed-service-prices/activate", s.activateReviewedServicePrices)
	mux.HandleFunc("POST /api/v1/clients/{clientId}/invoices/{invoiceId}/commercial-date-facts", s.putCommercialDateFact)
	mux.HandleFunc("POST /api/v1/clients/{clientId}/contract-documents/{documentId}/reextract", s.retryContractDocument)
	mux.HandleFunc("POST /api/v1/clients/{clientId}/contract-documents/{documentId}/discard", s.discardContractDocument)
	mux.HandleFunc("GET /api/v1/clients/{clientId}/invoices/{invoiceId}/commercial-validation", s.getCommercialValidation)
	mux.HandleFunc("GET /api/v1/clients/{clientId}/invoices/{invoiceId}/accounting-analysis", s.getAccountingAnalysis)
	mux.HandleFunc("POST /api/v1/clients/{clientId}/invoices/{invoiceId}/accounting-analysis", s.requestAccountingAnalysis)
	mux.HandleFunc("POST /api/v1/clients/{clientId}/invoices/{invoiceId}/accounting-analysis/review", s.reviewAccountingAnalysis)
	mux.HandleFunc("POST /api/v1/clients/{clientId}/contract-dossiers", s.createContractDossier)
	mux.HandleFunc("GET /api/v1/clients/{clientId}/contract-dossiers", s.listContractDossiers)
	mux.HandleFunc("GET /api/v1/clients/{clientId}/contract-dossiers/{dossierId}", s.getContractDossier)
	mux.HandleFunc("POST /api/v1/clients/{clientId}/invoices/{invoiceId}/commercial-validation/resolve", s.resolveCommercialValidation)
	mux.HandleFunc("POST /api/v1/clients/{clientId}/contract-dossiers/{dossierId}/variables", s.putCommercialVariable)
	mux.HandleFunc("POST /api/v1/clients/{clientId}/commercial-service-aliases", s.confirmCommercialAlias)
	mux.HandleFunc("GET /api/v1/clients/{clientId}/commercial-snapshots/{snapshotId}/revalidation-preview", s.previewCommercialRevalidation)
	return s.middleware(mux)
}

func (s *Server) searchAccounts(w http.ResponseWriter, r *http.Request) {
	limit := 25
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > 100 {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid account search limit.")
			return
		}
		limit = parsed
	}
	items, err := s.classifications.SearchAccounts(r.Context(), r.URL.Query().Get("q"), limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func ingestionActor(actor requestactor.Actor, r *http.Request) contractingestion.Actor {
	return contractingestion.Actor{ID: actor.ID, Display: actor.Display, CorrelationID: correlationID(r.Context()), AuthorizedClientIDs: actor.AuthorizedClientIDs, AllClients: actor.AllClients}
}

func (s *Server) uploadContractDocument(w http.ResponseWriter, r *http.Request) {
	if s.contractIngestion == nil {
		writeError(w, r, http.StatusServiceUnavailable, "CONTRACT_INGESTION_UNAVAILABLE", "Extragerea contractelor nu este configurată.")
		return
	}
	clientID := strings.TrimSpace(r.PathValue("clientId"))
	r.Body = http.MaxBytesReader(w, r.Body, s.contractIngestion.MaxPDFBytes()+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		writeError(w, r, http.StatusRequestEntityTooLarge, "DOCUMENT_TOO_LARGE", "PDF-ul depășește limita configurată.")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Fișierul PDF este obligatoriu.")
		return
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	actor, _ := requestactor.FromContext(r.Context())
	doc, duplicate, err := s.contractIngestion.Upload(r.Context(), contractingestion.Upload{ClientID: clientID, Filename: header.Filename, ContentType: header.Header.Get("Content-Type"), Bytes: content, Actor: ingestionActor(actor, r)})
	if s.writeContractIngestionError(w, r, err) {
		return
	}
	if !duplicate {
		s.metrics.ContractDocumentUploaded()
	}
	status := http.StatusAccepted
	if duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, contractDocumentResponse(doc, duplicate))
}
func (s *Server) listContractDocuments(w http.ResponseWriter, r *http.Request) {
	if s.contractIngestion == nil {
		writeError(w, r, http.StatusServiceUnavailable, "CONTRACT_INGESTION_UNAVAILABLE", "Extragerea contractelor nu este configurată.")
		return
	}
	actor, _ := requestactor.FromContext(r.Context())
	items, err := s.contractIngestion.List(r.Context(), strings.TrimSpace(r.PathValue("clientId")), ingestionActor(actor, r))
	if s.writeContractIngestionError(w, r, err) {
		return
	}
	response := make([]contractDocumentDTO, 0, len(items))
	for _, item := range items {
		response = append(response, contractDocumentResponse(item, false))
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) retryContractDocument(w http.ResponseWriter, r *http.Request) {
	if s.contractIngestion == nil {
		writeError(w, r, http.StatusServiceUnavailable, "CONTRACT_INGESTION_UNAVAILABLE", "Extragerea contractelor nu este configurată.")
		return
	}
	var req struct {
		Revision uint64 `json:"expectedDocumentRevision"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&req) != nil || req.Revision == 0 {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Revizia documentului este obligatorie.")
		return
	}
	actor, _ := requestactor.FromContext(r.Context())
	err := s.contractIngestion.Retry(r.Context(), r.PathValue("clientId"), r.PathValue("documentId"), req.Revision, ingestionActor(actor, r))
	if s.writeContractIngestionError(w, r, err) {
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "UPLOADED"})
}
func (s *Server) getContractDocument(w http.ResponseWriter, r *http.Request) {
	if s.contractIngestion == nil {
		writeError(w, r, http.StatusServiceUnavailable, "CONTRACT_INGESTION_UNAVAILABLE", "Extragerea contractelor nu este configurată.")
		return
	}
	actor, _ := requestactor.FromContext(r.Context())
	item, err := s.contractIngestion.Get(r.Context(), strings.TrimSpace(r.PathValue("clientId")), strings.TrimSpace(r.PathValue("documentId")), ingestionActor(actor, r))
	if s.writeContractIngestionError(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, contractDocumentResponse(item, false))
}
func (s *Server) downloadContractDocument(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if s.contractIngestion == nil {
		writeError(w, r, http.StatusServiceUnavailable, "CONTRACT_INGESTION_UNAVAILABLE", "Extragerea contractelor nu este configurată.")
		return
	}
	actor, _ := requestactor.FromContext(r.Context())
	source, err := s.contractIngestion.File(r.Context(), strings.TrimSpace(r.PathValue("clientId")), strings.TrimSpace(r.PathValue("documentId")), ingestionActor(actor, r))
	if s.writeContractIngestionError(w, r, err) {
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": source.Document.OriginalFilename}))
	http.ServeContent(w, r, source.Document.OriginalFilename, source.Document.UploadedAt, bytes.NewReader(source.Bytes))
}

type confirmContractDocumentRequest struct {
	ExtractionAttemptID      string                             `json:"extractionAttemptId"`
	ExpectedDocumentRevision uint64                             `json:"expectedDocumentRevision"`
	Contract                 contractingestion.ReviewedContract `json:"contract"`
}

func (s *Server) confirmContractDocument(w http.ResponseWriter, r *http.Request) {
	if s.contractIngestion == nil {
		writeError(w, r, http.StatusServiceUnavailable, "CONTRACT_INGESTION_UNAVAILABLE", "Extragerea contractelor nu este configurată.")
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	var req confirmContractDocumentRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if key == "" || len(key) > 256 || decoder.Decode(&req) != nil || decoder.Decode(new(any)) != io.EOF {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Propunerea, revizia și Idempotency-Key sunt obligatorii.")
		return
	}
	actor, _ := requestactor.FromContext(r.Context())
	id, changed, err := s.contractIngestion.Confirm(r.Context(), contractingestion.ConfirmCommand{ClientID: strings.TrimSpace(r.PathValue("clientId")), DocumentID: strings.TrimSpace(r.PathValue("documentId")), ExtractionAttemptID: req.ExtractionAttemptID, ExpectedDocumentRevision: req.ExpectedDocumentRevision, Contract: req.Contract, CommandID: key, Actor: ingestionActor(actor, r)})
	if s.writeContractIngestionError(w, r, err) {
		return
	}
	if changed {
		s.metrics.ContractConfirmed()
	}
	writeJSON(w, http.StatusOK, map[string]any{"contractId": id, "changed": changed})
}

func (s *Server) discardContractDocument(w http.ResponseWriter, r *http.Request) {
	if s.contractIngestion == nil {
		writeError(w, r, http.StatusServiceUnavailable, "CONTRACT_INGESTION_UNAVAILABLE", "Extragerea contractelor nu este configurată.")
		return
	}
	var req struct {
		Revision uint64 `json:"expectedDocumentRevision"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&req) != nil || req.Revision == 0 {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Revizia documentului este obligatorie.")
		return
	}
	actor, _ := requestactor.FromContext(r.Context())
	if err := s.contractIngestion.Discard(r.Context(), strings.TrimSpace(r.PathValue("clientId")), strings.TrimSpace(r.PathValue("documentId")), req.Revision, ingestionActor(actor, r)); s.writeContractIngestionError(w, r, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) writeContractIngestionError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Documentul contractual nu există.")
	case errors.Is(err, contractingestion.ErrDocumentTooLarge):
		writeError(w, r, http.StatusRequestEntityTooLarge, "DOCUMENT_TOO_LARGE", "PDF-ul depășește limita configurată.")
	case errors.Is(err, contractingestion.ErrInvalidPDF):
		writeError(w, r, http.StatusUnsupportedMediaType, "INVALID_PDF", "Este acceptat doar un PDF valid.")
	case errors.Is(err, contractingestion.ErrBuyerMismatch):
		writeError(w, r, http.StatusConflict, "BUYER_MISMATCH", "Cumpărătorul din document nu corespunde clientului selectat.")
	case errors.Is(err, apperrors.ErrConflict):
		writeError(w, r, http.StatusConflict, "STALE_EXTRACTION", "Extragerea s-a modificat sau contractul a fost deja confirmat.")
	case errors.Is(err, apperrors.ErrValidation):
		writeError(w, r, http.StatusUnprocessableEntity, "INVALID_CONTRACT", "Completează și corectează câmpurile contractuale obligatorii.")
	default:
		s.internalError(w, r, err)
	}
	return true
}

type confirmSagaImportRequest struct {
	AttemptID               string `json:"attemptId"`
	ExpectedInvoiceRevision uint64 `json:"expectedInvoiceRevision"`
	Note                    string `json:"note"`
}

func (s *Server) getSagaExport(w http.ResponseWriter, r *http.Request) {
	if s.sagaHandoff == nil {
		writeError(w, r, http.StatusServiceUnavailable, "SAGA_HANDOFF_UNAVAILABLE", "Exportul SAGA nu este configurat.")
		return
	}
	actor, _ := requestactor.FromContext(r.Context())
	view, err := s.sagaHandoff.View(r.Context(), strings.TrimSpace(r.PathValue("clientId")), strings.TrimSpace(r.PathValue("invoiceId")), sagaActor(actor, r))
	if !s.writeSagaError(w, r, err) {
		writeJSON(w, http.StatusOK, sagaExportResponse(view))
	}
}

func (s *Server) downloadSagaArtifact(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if s.sagaHandoff == nil {
		writeError(w, r, http.StatusServiceUnavailable, "SAGA_HANDOFF_UNAVAILABLE", "Exportul SAGA nu este configurat.")
		return
	}
	actor, _ := requestactor.FromContext(r.Context())
	download, err := s.sagaHandoff.Download(r.Context(), strings.TrimSpace(r.PathValue("clientId")), strings.TrimSpace(r.PathValue("invoiceId")), sagaActor(actor, r))
	if s.writeSagaError(w, r, err) {
		return
	}
	const maxArtifactBytes = 10 << 20
	if len(download.Artifact.Payload) > maxArtifactBytes {
		writeError(w, r, http.StatusInternalServerError, "SAGA_ARTIFACT_TOO_LARGE", "Fișierul SAGA depășește limita permisă.")
		return
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": download.Artifact.Filename})
	w.Header().Set("Content-Type", download.Artifact.ContentType)
	w.Header().Set("Content-Disposition", disposition)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(download.Artifact.Payload)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(download.Artifact.Payload)
}

func (s *Server) confirmSagaImport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, private")
	if s.sagaHandoff == nil {
		writeError(w, r, http.StatusServiceUnavailable, "SAGA_HANDOFF_UNAVAILABLE", "Exportul SAGA nu este configurat.")
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	var request confirmSagaImportRequest
	if key == "" || json.NewDecoder(r.Body).Decode(&request) != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Attempt, invoice revision și Idempotency-Key sunt obligatorii.")
		return
	}
	actor, _ := requestactor.FromContext(r.Context())
	item, view, changed, err := s.sagaHandoff.Confirm(r.Context(), saga.ConfirmCommand{
		ClientID: strings.TrimSpace(r.PathValue("clientId")), InvoiceID: strings.TrimSpace(r.PathValue("invoiceId")),
		AttemptID: request.AttemptID, ExpectedInvoiceRevision: request.ExpectedInvoiceRevision,
		Note: request.Note, CommandID: key, Actor: sagaActor(actor, r),
	})
	if s.writeSagaError(w, r, err) {
		return
	}
	dto, err := invoiceResponse(item)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	dto.SagaExport = sagaExportResponse(view)
	writeJSON(w, http.StatusOK, map[string]any{"invoice": dto, "sagaExport": dto.SagaExport, "changed": changed})
}

func sagaActor(actor requestactor.Actor, r *http.Request) saga.Actor {
	return saga.Actor{ID: actor.ID, Display: actor.Display, CorrelationID: correlationID(r.Context()), AuthorizedClientIDs: actor.AuthorizedClientIDs, AllClients: actor.AllClients}
}

func (s *Server) writeSagaError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Factura sau exportul SAGA nu există.")
	case errors.Is(err, saga.ErrArtifactUnavailable):
		writeError(w, r, http.StatusConflict, "SAGA_ARTIFACT_UNAVAILABLE", "Nu există un fișier SAGA generat disponibil.")
	case errors.Is(err, apperrors.ErrValidation):
		writeError(w, r, http.StatusUnprocessableEntity, "INVALID_TRANSITION", "Importul SAGA nu poate fi confirmat în starea curentă.")
	case errors.Is(err, apperrors.ErrConflict):
		writeError(w, r, http.StatusConflict, "CONFLICT", "Exportul SAGA s-a modificat. Reîncarcă factura și încearcă din nou.")
	default:
		s.internalError(w, r, err)
	}
	return true
}

type spvConnectionDTO struct {
	Status               string  `json:"status"`
	Environment          string  `json:"environment,omitempty"`
	ConnectedAt          *string `json:"connectedAt,omitempty"`
	LastSyncAt           *string `json:"lastSyncAt,omitempty"`
	LastSuccessfulSyncAt *string `json:"lastSuccessfulSyncAt,omitempty"`
	LastSyncStatus       string  `json:"lastSyncStatus"`
	SafeErrorCode        string  `json:"safeErrorCode,omitempty"`
	SafeError            string  `json:"safeError,omitempty"`
	ImportAutomatic      bool    `json:"importAutomatic"`
	ConfigurationReady   bool    `json:"configurationReady"`
	IdentityValidation   string  `json:"identityValidation"`
}

func spvResponse(item spvdomain.ConnectionView) spvConnectionDTO {
	result := spvConnectionDTO{Status: string(item.Status), Environment: item.Environment, LastSyncStatus: item.LastSyncStatus, SafeErrorCode: item.SafeErrorCode, SafeError: item.SafeError, ImportAutomatic: item.ImportAutomatic, ConfigurationReady: item.ConfigurationReady, IdentityValidation: item.IdentityValidation}
	format := func(value *time.Time) *string {
		if value == nil {
			return nil
		}
		text := value.Format(time.RFC3339)
		return &text
	}
	result.ConnectedAt, result.LastSyncAt, result.LastSuccessfulSyncAt = format(item.ConnectedAt), format(item.LastSyncAt), format(item.LastSuccessfulSyncAt)
	return result
}

func (s *Server) getSPVConnection(w http.ResponseWriter, r *http.Request) {
	if !s.allowClient(w, r, r.PathValue("id")) {
		return
	}
	if s.spv == nil {
		writeError(w, r, http.StatusServiceUnavailable, "SPV_UNAVAILABLE", "Integrarea ANAF nu este configurată.")
		return
	}
	item, err := s.spv.Get(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if errors.Is(err, apperrors.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Clientul nu a fost găsit.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, spvResponse(item))
}

func (s *Server) startSPVOAuth(w http.ResponseWriter, r *http.Request) {
	if !s.operationalClient(w, r, r.PathValue("id")) {
		return
	}
	if !s.allowClient(w, r, r.PathValue("id")) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if s.spv == nil {
		writeError(w, r, http.StatusServiceUnavailable, "SPV_UNAVAILABLE", "Integrarea ANAF nu este configurată.")
		return
	}
	actor, _ := requestactor.FromContext(r.Context())
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		writeError(w, r, 400, "VALIDATION_ERROR", "Idempotency-Key obligatoriu.")
		return
	}
	authorizationURL, err := s.spv.StartOAuthCommand(r.Context(), strings.TrimSpace(r.PathValue("id")), key, spvdomain.Actor{ID: actor.ID, Display: actor.Display, CorrelationID: correlationID(r.Context())})
	if errors.Is(err, apperrors.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Clientul nu a fost găsit.")
		return
	}
	if errors.Is(err, spvdomain.ErrConfiguration) {
		writeError(w, r, http.StatusServiceUnavailable, "SPV_CONFIGURATION_INCOMPLETE", "Configurația aplicației ANAF este incompletă.")
		return
	}
	if errors.Is(err, apperrors.ErrValidation) || errors.Is(err, apperrors.ErrConflict) || errors.Is(err, spvdomain.ErrInvalidOAuthState) {
		writeError(w, r, 409, "OAUTH_ATTEMPT_EXPIRED", "Autorizarea a expirat sau a fost folosită. Pornește o încercare nouă.")
		return
	}
	if errors.Is(err, spvdomain.ErrConnectionInactive) {
		writeError(w, r, 409, "CLIENT_INACTIVE", "Client inactiv.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"authorizationUrl": authorizationURL})
}

func (s *Server) handleANAFCallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if s.spv == nil {
		http.Redirect(w, r, "/clients?integration=anaf&result=error&reason=configuration", http.StatusSeeOther)
		return
	}
	actor, _ := requestactor.FromContext(r.Context())
	redirect, err := s.spv.Callback(r.Context(), r.URL.Query().Get("state"), r.URL.Query().Get("code"), r.URL.Query().Get("error"), spvdomain.Actor{ID: actor.ID, Display: actor.Display, CorrelationID: correlationID(r.Context())})
	if err != nil {
		s.logger.Warn("ANAF OAuth callback rejected", "correlation_id", correlationID(r.Context()), "category", "safe_callback_failure")
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (s *Server) requestSPVSync(w http.ResponseWriter, r *http.Request) {
	if !s.operationalClient(w, r, r.PathValue("id")) {
		return
	}
	if !s.allowClient(w, r, r.PathValue("id")) {
		return
	}
	if s.spv == nil {
		writeError(w, r, http.StatusServiceUnavailable, "SPV_UNAVAILABLE", "Integrarea ANAF nu este configurată.")
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Idempotency-Key este obligatoriu.")
		return
	}
	actor, _ := requestactor.FromContext(r.Context())
	item, err := s.spv.RequestSync(r.Context(), strings.TrimSpace(r.PathValue("id")), key, spvdomain.Actor{ID: actor.ID, Display: actor.Display, CorrelationID: correlationID(r.Context())})
	if errors.Is(err, apperrors.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Conexiunea ANAF nu există.")
		return
	}
	if errors.Is(err, spvdomain.ErrConnectionInactive) {
		writeError(w, r, http.StatusConflict, "SPV_REAUTHENTICATION_REQUIRED", "Conexiunea ANAF necesită reconectare.")
		return
	}
	if errors.Is(err, spvdomain.ErrConfiguration) {
		writeError(w, r, http.StatusServiceUnavailable, "SPV_CONFIGURATION_INCOMPLETE", "Configurația aplicației ANAF este incompletă.")
		return
	}
	if errors.Is(err, spvdomain.ErrTransient) {
		writeError(w, r, http.StatusServiceUnavailable, "SPV_SYNC_UNAVAILABLE", "Sincronizarea ANAF nu poate fi pornită momentan.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, spvResponse(item))
}

func (s *Server) disconnectSPV(w http.ResponseWriter, r *http.Request) {
	if !s.allowClient(w, r, r.PathValue("id")) {
		return
	}
	if s.spv == nil {
		writeError(w, r, http.StatusServiceUnavailable, "SPV_UNAVAILABLE", "Integrarea ANAF nu este configurată.")
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Idempotency-Key este obligatoriu.")
		return
	}
	actor, _ := requestactor.FromContext(r.Context())
	item, err := s.spv.Disconnect(r.Context(), strings.TrimSpace(r.PathValue("id")), key, spvdomain.Actor{ID: actor.ID, Display: actor.Display, CorrelationID: correlationID(r.Context())})
	if errors.Is(err, apperrors.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Conexiunea ANAF nu există.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, spvResponse(item))
}

func (s *Server) listInvoices(w http.ResponseWriter, r *http.Request) {
	if id := strings.TrimSpace(r.URL.Query().Get("clientId")); id != "" && !s.allowClient(w, r, id) {
		return
	}
	items, err := s.invoices.List(r.Context(), invoicing.Filter{ClientID: strings.TrimSpace(r.URL.Query().Get("clientId"))})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	response := make([]invoiceDTO, 0, len(items))
	for index := range items {
		if !clientAllowed(r, items[index].ClientID) {
			continue
		}
		dto, dtoErr := invoiceResponse(&items[index])
		if dtoErr != nil {
			s.internalError(w, r, dtoErr)
			return
		}
		response = append(response, dto)
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) listRules(w http.ResponseWriter, r *http.Request) {
	if id := strings.TrimSpace(r.URL.Query().Get("clientId")); id != "" && !s.allowClient(w, r, id) {
		return
	}
	items, err := s.rules.List(r.Context(), rules.Filter{ClientID: strings.TrimSpace(r.URL.Query().Get("clientId"))})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	response := make([]ruleDTO, 0, len(items))
	for index := range items {
		if items[index].ClientID != nil && !clientAllowed(r, *items[index].ClientID) {
			continue
		}
		response = append(response, ruleResponse(&items[index]))
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) getRule(w http.ResponseWriter, r *http.Request) {
	if !s.allowRule(w, r, r.PathValue("id")) {
		return
	}
	item, err := s.rules.Get(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if errors.Is(err, apperrors.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Rule was not found.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ruleResponse(item))
}

type ruleMutationDTO struct {
	ExpectedRevision uint64  `json:"expectedRevision"`
	ClientID         string  `json:"clientId"`
	Criteria         string  `json:"criteria"`
	Result           string  `json:"result"`
	EffectiveFrom    string  `json:"effectiveFrom"`
	EffectiveTo      *string `json:"effectiveTo"`
}

func (s *Server) createRuleVersion(w http.ResponseWriter, r *http.Request) {
	if !s.allowRule(w, r, r.PathValue("id")) {
		return
	}
	r, endSpan := applicationCommand(r, "rules.create_version")
	defer endSpan()
	request, from, to, ok := decodeRuleMutation(w, r)
	if !ok || request.ExpectedRevision == 0 {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Rule revision and version fields are required.")
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Idempotency-Key is required.")
		return
	}
	actor, _ := requestactor.FromContext(r.Context())
	item, _, err := s.rules.CreateVersion(r.Context(), rules.CreateVersionCommand{RuleID: strings.TrimSpace(r.PathValue("id")), ExpectedRevision: request.ExpectedRevision, Criteria: request.Criteria, Result: request.Result, EffectiveFrom: from, EffectiveTo: to, CommandID: key, ActorID: actor.ID, ActorDisplay: actor.Display, CorrelationID: correlationID(r.Context())})
	s.writeRuleMutation(w, r, item, err)
}

func (s *Server) createClientOverride(w http.ResponseWriter, r *http.Request) {
	r, endSpan := applicationCommand(r, "rules.create_client_override")
	defer endSpan()
	request, from, to, ok := decodeRuleMutation(w, r)
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if !ok || request.ClientID == "" || key == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Client, rule fields, and Idempotency-Key are required.")
		return
	}
	actor, _ := requestactor.FromContext(r.Context())
	if !s.allowClient(w, r, request.ClientID) {
		return
	}
	item, _, err := s.rules.CreateOverride(r.Context(), rules.CreateOverrideCommand{ParentRuleID: strings.TrimSpace(r.PathValue("id")), ClientID: request.ClientID, Criteria: request.Criteria, Result: request.Result, EffectiveFrom: from, EffectiveTo: to, CommandID: key, ActorID: actor.ID, ActorDisplay: actor.Display, CorrelationID: correlationID(r.Context())})
	s.writeRuleMutation(w, r, item, err)
}

func decodeRuleMutation(w http.ResponseWriter, r *http.Request) (ruleMutationDTO, time.Time, *time.Time, bool) {
	var request ruleMutationDTO
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || strings.TrimSpace(request.Criteria) == "" || strings.TrimSpace(request.Result) == "" {
		return request, time.Time{}, nil, false
	}
	from, err := time.Parse("2006-01-02", request.EffectiveFrom)
	if err != nil {
		return request, time.Time{}, nil, false
	}
	var to *time.Time
	if request.EffectiveTo != nil && *request.EffectiveTo != "" {
		value, parseErr := time.Parse("2006-01-02", *request.EffectiveTo)
		if parseErr != nil {
			return request, time.Time{}, nil, false
		}
		to = &value
	}
	return request, from, to, true
}

func (s *Server) writeRuleMutation(w http.ResponseWriter, r *http.Request, item *rules.Rule, err error) {
	if errors.Is(err, apperrors.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Rule was not found.")
		return
	}
	if errors.Is(err, rules.ErrRuleVersionStale) || errors.Is(err, rules.ErrOverrideAlreadyExists) || errors.Is(err, apperrors.ErrConflict) {
		writeError(w, r, http.StatusConflict, "CONFLICT", "Rule changed concurrently or the override already exists.")
		return
	}
	if errors.Is(err, apperrors.ErrValidation) {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid rule mutation.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ruleResponse(item))
}

type classificationDecisionDTO struct {
	TaskID                         string            `json:"taskId"`
	ClassificationID               string            `json:"classificationId"`
	ExpectedInvoiceRevision        uint64            `json:"expectedInvoiceRevision"`
	ExpectedTaskRevision           uint64            `json:"expectedTaskRevision"`
	ExpectedClassificationRevision uint64            `json:"expectedClassificationRevision"`
	TypedValue                     *accounting.Value `json:"typedValue"`
	Reason                         string            `json:"reason"`
	CorrectedValue                 *string           `json:"correctedValue"`
	MappingAction                  string            `json:"mappingAction"`
	ExpectedMappingRevision        uint64            `json:"expectedMappingRevision"`
}

func (s *Server) reviewClassification(w http.ResponseWriter, r *http.Request) {
	if !s.allowInvoice(w, r, r.PathValue("id")) {
		return
	}
	r, endSpan := applicationCommand(r, "classification.review")
	defer endSpan()
	var request classificationDecisionDTO
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	invoiceID := strings.TrimSpace(r.PathValue("id"))
	if decoder.Decode(&request) != nil || key == "" || invoiceID == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Classification decision context is required.")
		return
	}
	actor, _ := requestactor.FromContext(r.Context())
	_, err := s.classifications.Review(r.Context(), classification.ReviewCommand{InvoiceID: invoiceID, TaskID: request.TaskID, ClassificationID: request.ClassificationID, ExpectedInvoiceRevision: request.ExpectedInvoiceRevision, ExpectedTaskRevision: request.ExpectedTaskRevision, ExpectedClassificationRevision: request.ExpectedClassificationRevision, TypedValue: request.TypedValue, Reason: request.Reason, CorrectedValue: request.CorrectedValue, CommandID: key, ActorID: actor.ID, ActorDisplay: actor.Display, CorrelationID: correlationID(r.Context()), MappingAction: request.MappingAction, ExpectedMappingRevision: request.ExpectedMappingRevision})
	if errors.Is(err, apperrors.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Classification review context was not found.")
		return
	}
	if errors.Is(err, classification.ErrStaleReview) || errors.Is(err, validationtasks.ErrTaskAlreadyResolved) || errors.Is(err, apperrors.ErrConflict) {
		writeError(w, r, http.StatusConflict, "CONFLICT", "Classification review is stale or already resolved.")
		return
	}
	if errors.Is(err, apperrors.ErrValidation) {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid classification decision.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	item, err := s.invoices.Get(r.Context(), invoiceID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	response, err := invoiceResponse(item)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) listContracts(w http.ResponseWriter, r *http.Request) {
	if id := strings.TrimSpace(r.URL.Query().Get("clientId")); id != "" && !s.allowClient(w, r, id) {
		return
	}
	items, err := s.contracts.List(r.Context(), contractdomain.Filter{ClientID: strings.TrimSpace(r.URL.Query().Get("clientId")), Query: strings.TrimSpace(r.URL.Query().Get("q"))})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	response := make([]contractDTO, 0, len(items))
	for _, item := range items {
		if !clientAllowed(r, item.ClientID) {
			continue
		}
		response = append(response, contractResponse(&item))
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) getContract(w http.ResponseWriter, r *http.Request) {
	if !s.allowContract(w, r, r.PathValue("id")) {
		return
	}
	item, err := s.contracts.Get(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if errors.Is(err, apperrors.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Contract was not found.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, contractResponse(item))
}

func (s *Server) archiveContract(w http.ResponseWriter, r *http.Request) {
	s.changeContractLifecycle(w, r, false)
}

func (s *Server) discardContract(w http.ResponseWriter, r *http.Request) {
	s.changeContractLifecycle(w, r, true)
}

func (s *Server) changeContractLifecycle(w http.ResponseWriter, r *http.Request, mistaken bool) {
	id := strings.TrimSpace(r.PathValue("id"))
	if !s.allowContract(w, r, id) {
		return
	}
	var request struct {
		ExpectedRevision uint64 `json:"expectedRevision"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF || request.ExpectedRevision == 0 {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Revizia contractului este obligatorie.")
		return
	}
	actor, ok := requestactor.FromContext(r.Context())
	if !ok || actor.ID == "" || actor.Display == "" {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "Identitatea utilizatorului este obligatorie.")
		return
	}
	var changed bool
	var err error
	if mistaken {
		changed, err = s.contracts.DeleteMistaken(r.Context(), id, request.ExpectedRevision, actor.ID, actor.Display)
	} else {
		changed, err = s.contracts.Archive(r.Context(), id, request.ExpectedRevision, actor.ID, actor.Display)
	}
	if errors.Is(err, contractdomain.ErrContractInUse) {
		writeError(w, r, http.StatusConflict, "CONTRACT_IN_USE", "Contractul are facturi sau validări istorice. Scoate-l din utilizare în loc să ștergi încărcarea.")
		return
	}
	if errors.Is(err, apperrors.ErrConflict) {
		writeError(w, r, http.StatusConflict, "CONFLICT", "Contractul s-a modificat. Reîncarcă pagina și încearcă din nou.")
		return
	}
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"changed": changed})
}

func (s *Server) listContractInvoices(w http.ResponseWriter, r *http.Request) {
	if !s.allowContract(w, r, r.PathValue("id")) {
		return
	}
	items, err := s.contracts.ListInvoices(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if errors.Is(err, apperrors.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Contract was not found.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	response := make([]contractInvoiceDTO, 0, len(items))
	for _, item := range items {
		response = append(response, contractInvoiceResponse(item))
	}
	writeJSON(w, http.StatusOK, response)
}

type confirmContractMatchDTO struct {
	TaskID                  string `json:"taskId"`
	ContractID              string `json:"contractId"`
	ExpectedInvoiceRevision uint64 `json:"expectedInvoiceRevision"`
	ExpectedTaskRevision    uint64 `json:"expectedTaskRevision"`
}

func (s *Server) confirmContractMatch(w http.ResponseWriter, r *http.Request) {
	if !s.allowInvoice(w, r, r.PathValue("id")) {
		return
	}
	r, endSpan := applicationCommand(r, "contracts.confirm_match")
	defer endSpan()
	invoiceID := strings.TrimSpace(r.PathValue("id"))
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	var request confirmContractMatchDTO
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if invoiceID == "" || idempotencyKey == "" || decoder.Decode(&request) != nil || request.TaskID == "" || request.ContractID == "" || request.ExpectedInvoiceRevision == 0 || request.ExpectedTaskRevision == 0 {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Invoice, task, candidate, revisions, and Idempotency-Key are required.")
		return
	}
	actor, _ := requestactor.FromContext(r.Context())
	_, err := s.contracts.Confirm(r.Context(), contractdomain.ConfirmCommand{
		InvoiceID: invoiceID, TaskID: request.TaskID, ContractID: request.ContractID,
		ExpectedInvoiceRevision: request.ExpectedInvoiceRevision, ExpectedTaskRevision: request.ExpectedTaskRevision,
		CommandID: idempotencyKey, ActorID: actor.ID, ActorDisplay: actor.Display, CorrelationID: correlationID(r.Context()),
	})
	if errors.Is(err, apperrors.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Contract match context was not found.")
		return
	}
	if errors.Is(err, contractdomain.ErrStaleMatchResult) {
		writeError(w, r, http.StatusConflict, "STALE_MATCH_RESULT", "Contract match result is stale.")
		return
	}
	if errors.Is(err, validationtasks.ErrTaskAlreadyResolved) || errors.Is(err, apperrors.ErrConflict) {
		writeError(w, r, http.StatusConflict, "CONFLICT", "Contract match was already resolved or changed concurrently.")
		return
	}
	if errors.Is(err, apperrors.ErrValidation) {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Selected contract is not a valid candidate.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	item, err := s.invoices.Get(r.Context(), invoiceID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	response, err := invoiceResponse(item)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) listValidationTasks(w http.ResponseWriter, r *http.Request) {
	if id := strings.TrimSpace(r.URL.Query().Get("clientId")); id != "" && !s.allowClient(w, r, id) {
		return
	}
	filter := validationtasks.Filter{ClientID: strings.TrimSpace(r.URL.Query().Get("clientId")), InvoiceID: strings.TrimSpace(r.URL.Query().Get("invoiceId"))}
	if raw := strings.TrimSpace(r.URL.Query().Get("type")); raw != "" {
		value := validationtasks.Type(raw)
		filter.Type = &value
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("status")); raw != "" {
		value := validationtasks.Status(raw)
		filter.Status = &value
	}
	items, err := s.tasks.List(r.Context(), filter)
	if errors.Is(err, apperrors.ErrValidation) {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid validation-task filter.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	response := make([]validationTaskInboxDTO, 0, len(items))
	for _, item := range items {
		if !clientAllowed(r, item.Invoice.ClientID) {
			continue
		}
		dto, dtoErr := validationTaskInboxResponse(item)
		if dtoErr != nil {
			s.internalError(w, r, dtoErr)
			return
		}
		response = append(response, dto)
	}
	writeJSON(w, http.StatusOK, response)
}

type requestMissingContractDTO struct {
	TaskID           string `json:"taskId"`
	ExpectedRevision uint64 `json:"expectedRevision"`
}

func (s *Server) requestMissingContract(w http.ResponseWriter, r *http.Request) {
	if !s.allowInvoice(w, r, r.PathValue("id")) {
		return
	}
	r, endSpan := applicationCommand(r, "contracts.request_missing")
	defer endSpan()
	invoiceID := strings.TrimSpace(r.PathValue("id"))
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	var request requestMissingContractDTO
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if invoiceID == "" || idempotencyKey == "" || decoder.Decode(&request) != nil || request.TaskID == "" || request.ExpectedRevision == 0 {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Invoice, task, revision, and Idempotency-Key are required.")
		return
	}
	actor, _ := requestactor.FromContext(r.Context())
	_, _, err := s.tasks.RequestMissingContract(r.Context(), validationtasks.RequestMissingContractCommand{
		InvoiceID: invoiceID, TaskID: request.TaskID, ExpectedRevision: request.ExpectedRevision,
		CommandID: idempotencyKey, ActorID: actor.ID, ActorDisplay: actor.Display, CorrelationID: correlationID(r.Context()),
	})
	if errors.Is(err, apperrors.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Validation task or invoice was not found.")
		return
	}
	if errors.Is(err, validationtasks.ErrTaskAlreadyResolved) {
		writeError(w, r, http.StatusConflict, "TASK_ALREADY_RESOLVED", "Validation task is already resolved.")
		return
	}
	if errors.Is(err, apperrors.ErrConflict) {
		writeError(w, r, http.StatusConflict, "CONFLICT", "Validation task changed concurrently.")
		return
	}
	if errors.Is(err, apperrors.ErrValidation) {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Task is not compatible with this contract request.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	item, err := s.invoices.Get(r.Context(), invoiceID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	dto, err := invoiceResponse(item)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) readiness(w http.ResponseWriter, r *http.Request) {
	if err := s.ready.Ping(r.Context()); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "INTERNAL_ERROR", "Service is not ready.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) listClients(w http.ResponseWriter, r *http.Request) {
	items, err := s.clients.List(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) getInvoice(w http.ResponseWriter, r *http.Request) {
	if !s.allowInvoice(w, r, r.PathValue("id")) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Invoice ID is required.")
		return
	}
	item, err := s.invoices.Get(r.Context(), id)
	if errors.Is(err, apperrors.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Invoice was not found.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	dto, err := invoiceResponse(item)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if s.sagaHandoff != nil && item.ClientID != "" {
		actor, _ := requestactor.FromContext(r.Context())
		view, viewErr := s.sagaHandoff.View(r.Context(), item.ClientID, item.ID, sagaActor(actor, r))
		if viewErr == nil {
			dto.SagaExport = sagaExportResponse(view)
		} else if !errors.Is(viewErr, apperrors.ErrNotFound) {
			s.internalError(w, r, viewErr)
			return
		}
	}
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	s.logger.Error("request failed", "correlation_id", correlationID(r.Context()), "error", err)
	writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "An internal error occurred.")
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = newRequestID()
		}
		ctx, span := otel.Tracer("diana/http").Start(r.Context(), r.Method+" "+r.URL.Path)
		span.SetAttributes(attribute.String("http.request.method", r.Method), attribute.String("url.path", r.URL.Path), attribute.String("request.id", requestID))
		defer span.End()
		ctx = context.WithValue(ctx, requestIDKey{}, requestID)
		if strings.Contains(r.URL.Path, "/contract-documents") {
			w.Header().Set("Cache-Control", "private, no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
		}
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		recorder.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(recorder, r.WithContext(ctx))
		span.SetAttributes(attribute.Int("http.response.status_code", recorder.status))
		s.metrics.ObserveHTTP(time.Since(started))
		if r.URL.Path != "/healthz" && r.URL.Path != "/readyz" {
			s.logger.Info("http request", "correlation_id", requestID, "method", r.Method, "route", r.URL.Path, "status", recorder.status, "duration_ms", time.Since(started).Milliseconds())
		}
	})
}

type requestIDKey struct{}

func correlationID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}

func newRequestID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "request-unknown"
	}
	return hex.EncodeToString(buffer)
}

func applicationCommand(r *http.Request, name string) (*http.Request, func()) {
	ctx, span := otel.Tracer("diana/application").Start(r.Context(), name)
	span.SetAttributes(attribute.String("correlation.id", correlationID(r.Context())))
	return r.WithContext(ctx), func() { span.End() }
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

type errorDTO struct {
	Code          string `json:"code"`
	Message       string `json:"message"`
	CorrelationID string `json:"correlation_id"`
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, status, errorDTO{Code: code, Message: message, CorrelationID: correlationID(r.Context())})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
