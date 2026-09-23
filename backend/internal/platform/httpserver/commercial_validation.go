package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/commercialvalidation"
	"diana-contabilitate/backend/internal/platform/requestactor"
)

func (s *Server) getCommercialValidation(w http.ResponseWriter, r *http.Request) {
	actor, _ := requestactor.FromContext(r.Context())
	clientID := strings.TrimSpace(r.PathValue("clientId"))
	if s.commercialValidation == nil || !actor.AllowsClient(clientID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Validarea comercială nu a fost găsită.")
		return
	}
	run, err := s.commercialValidation.Get(r.Context(), clientID, strings.TrimSpace(r.PathValue("invoiceId")))
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) createContractDossier(w http.ResponseWriter, r *http.Request) {
	actor, _ := requestactor.FromContext(r.Context())
	clientID := strings.TrimSpace(r.PathValue("clientId"))
	if s.commercialValidation == nil || !actor.AllowsClient(clientID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Clientul nu a fost găsit.")
		return
	}
	var request struct {
		SupplierCUI      string `json:"supplierCui"`
		BuyerCUI         string `json:"buyerCui"`
		PrimaryReference string `json:"primaryReference"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" || decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Dosarul contractual este invalid.")
		return
	}
	dossier, changed, err := s.commercialValidation.CreateDossier(r.Context(), commercialvalidation.Dossier{ClientID: clientID, SupplierCUI: strings.TrimSpace(request.SupplierCUI), BuyerCUI: strings.TrimSpace(request.BuyerCUI), PrimaryReference: strings.TrimSpace(request.PrimaryReference)}, key)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	status := http.StatusOK
	if changed {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]any{"dossier": dossier, "changed": changed})
}

func (s *Server) listContractDossiers(w http.ResponseWriter, r *http.Request) {
	actor, _ := requestactor.FromContext(r.Context())
	clientID := strings.TrimSpace(r.PathValue("clientId"))
	if s.commercialValidation == nil || !actor.AllowsClient(clientID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Clientul nu a fost găsit.")
		return
	}
	items, err := s.commercialValidation.ListDossiers(r.Context(), clientID)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) getContractDossier(w http.ResponseWriter, r *http.Request) {
	actor, _ := requestactor.FromContext(r.Context())
	clientID := strings.TrimSpace(r.PathValue("clientId"))
	if s.commercialValidation == nil || !actor.AllowsClient(clientID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Dosarul nu a fost găsit.")
		return
	}
	item, err := s.commercialValidation.GetDossier(r.Context(), clientID, strings.TrimSpace(r.PathValue("dossierId")))
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) resolveCommercialValidation(w http.ResponseWriter, r *http.Request) {
	actor, _ := requestactor.FromContext(r.Context())
	clientID := strings.TrimSpace(r.PathValue("clientId"))
	if s.commercialValidation == nil || !actor.AllowsClient(clientID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Validarea comercială nu a fost găsită.")
		return
	}
	var request struct {
		RunID                   string `json:"runId"`
		FindingID               string `json:"findingId"`
		ExpectedInvoiceRevision uint64 `json:"expectedInvoiceRevision"`
		Action                  string `json:"action"`
		Reason                  string `json:"reason"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" || decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Comanda de review comercial este invalidă.")
		return
	}
	changed, err := s.commercialValidation.Resolve(r.Context(), commercialvalidation.ReviewResolution{ClientID: clientID, InvoiceID: strings.TrimSpace(r.PathValue("invoiceId")), RunID: request.RunID, FindingID: request.FindingID, ExpectedInvoiceRevision: request.ExpectedInvoiceRevision, Action: request.Action, Reason: strings.TrimSpace(request.Reason), ActorID: actor.ID, ActorDisplay: actor.Display, CommandID: key, CorrelationID: correlationID(r.Context())})
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": changed})
}

func (s *Server) putCommercialVariable(w http.ResponseWriter, r *http.Request) {
	actor, _ := requestactor.FromContext(r.Context())
	clientID := strings.TrimSpace(r.PathValue("clientId"))
	if s.commercialValidation == nil || !actor.AllowsClient(clientID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Dosarul contractual nu a fost găsit.")
		return
	}
	var request struct {
		Name            string `json:"name"`
		Value           string `json:"value"`
		Source          string `json:"source"`
		SourceReference string `json:"sourceReference"`
		PeriodStart     string `json:"periodStart"`
		PeriodEnd       string `json:"periodEnd"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" || decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Valoarea variabilei este invalidă.")
		return
	}
	periodStart, startErr := parseOptionalCommercialDate(request.PeriodStart)
	periodEnd, endErr := parseOptionalCommercialDate(request.PeriodEnd)
	if startErr != nil || endErr != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Perioada variabilei trebuie să fie o dată ISO YYYY-MM-DD.")
		return
	}
	changed, err := s.commercialValidation.PutVariable(r.Context(), clientID, strings.TrimSpace(r.PathValue("dossierId")), commercialvalidation.VariableValue{Name: request.Name, Value: request.Value, Source: request.Source, SourceReference: request.SourceReference, PeriodStart: periodStart, PeriodEnd: periodEnd, RecordedByID: actor.ID}, key)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": changed})
}

func parseOptionalCommercialDate(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func (s *Server) confirmCommercialAlias(w http.ResponseWriter, r *http.Request) {
	actor, _ := requestactor.FromContext(r.Context())
	clientID := strings.TrimSpace(r.PathValue("clientId"))
	if s.commercialValidation == nil || !actor.AllowsClient(clientID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Clientul nu a fost găsit.")
		return
	}
	var request struct {
		InvoiceID       string     `json:"invoiceId"`
		LineID          string     `json:"lineId"`
		ServiceID       string     `json:"serviceId"`
		ReuseForDossier bool       `json:"reuseForDossier"`
		EffectiveFrom   *time.Time `json:"effectiveFrom"`
		EffectiveTo     *time.Time `json:"effectiveTo"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" || decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Aliasul este invalid.")
		return
	}
	changed, err := s.commercialValidation.ConfirmAlias(r.Context(), commercialvalidation.Alias{ClientID: clientID, InvoiceID: request.InvoiceID, LineID: request.LineID, ServiceID: request.ServiceID, ReuseForDossier: request.ReuseForDossier, EffectiveFrom: request.EffectiveFrom, EffectiveTo: request.EffectiveTo, ConfirmedByID: actor.ID}, key)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": changed})
}

func (s *Server) confirmProposedCommercialRule(w http.ResponseWriter, r *http.Request) {
	actor, _ := requestactor.FromContext(r.Context())
	clientID := strings.TrimSpace(r.PathValue("clientId"))
	if s.commercialValidation == nil || !actor.AllowsClient(clientID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Clauza comercială nu a fost găsită.")
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Confirmarea regulii necesită o cheie de idempotency.")
		return
	}
	var request struct {
		Rule *commercialvalidation.Rule `json:"rule"`
	}
	if r.ContentLength != 0 {
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Regula comercială revizuită este invalidă.")
			return
		}
	}
	changed, err := s.commercialValidation.ConfirmProposedRule(r.Context(), commercialvalidation.RuleConfirmation{ClientID: clientID, DocumentID: strings.TrimSpace(r.PathValue("documentId")), RuleID: strings.TrimSpace(r.PathValue("ruleId")), CommandID: key, ActorID: actor.ID, ActorDisplay: actor.Display, Rule: request.Rule})
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": changed})
}

func (s *Server) activateReviewedServicePrices(w http.ResponseWriter, r *http.Request) {
	actor, _ := requestactor.FromContext(r.Context())
	clientID := strings.TrimSpace(r.PathValue("clientId"))
	if s.commercialValidation == nil || !actor.AllowsClient(clientID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Contractul nu a fost găsit.")
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Confirmarea tarifelor necesită o cheie de idempotency.")
		return
	}
	count, err := s.commercialValidation.ActivateReviewedServicePrices(r.Context(), clientID, strings.TrimSpace(r.PathValue("documentId")), actor.ID, key)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"activated": count})
}

func (s *Server) putCommercialDateFact(w http.ResponseWriter, r *http.Request) {
	actor, _ := requestactor.FromContext(r.Context())
	clientID := strings.TrimSpace(r.PathValue("clientId"))
	if s.commercialValidation == nil || !actor.AllowsClient(clientID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Factura nu a fost găsită.")
		return
	}
	var request struct {
		Kind            string `json:"kind"`
		Date            string `json:"date"`
		SourceReference string `json:"sourceReference"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" || decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Data și sursa ei sunt obligatorii.")
		return
	}
	date, err := time.Parse("2006-01-02", request.Date)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Data trebuie să fie în formatul AAAA-LL-ZZ.")
		return
	}
	changed, err := s.commercialValidation.PutInvoiceDateFact(r.Context(), commercialvalidation.InvoiceDateFact{ClientID: clientID, InvoiceID: strings.TrimSpace(r.PathValue("invoiceId")), Kind: request.Kind, Date: date, SourceReference: request.SourceReference, ActorID: actor.ID, CommandID: key})
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": changed})
}

func (s *Server) previewCommercialRevalidation(w http.ResponseWriter, r *http.Request) {
	actor, _ := requestactor.FromContext(r.Context())
	clientID := strings.TrimSpace(r.PathValue("clientId"))
	if s.commercialValidation == nil || !actor.AllowsClient(clientID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Snapshot-ul nu a fost găsit.")
		return
	}
	items, err := s.commercialValidation.PreviewRevalidation(r.Context(), clientID, strings.TrimSpace(r.PathValue("snapshotId")))
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Resursa nu a fost găsită.")
	case errors.Is(err, apperrors.ErrConflict):
		writeError(w, r, http.StatusConflict, "CONFLICT", "Datele s-au modificat. Reîncarcă și încearcă din nou.")
	case errors.Is(err, apperrors.ErrValidation):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Comanda este invalidă.")
	default:
		s.internalError(w, r, err)
	}
}
