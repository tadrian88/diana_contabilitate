package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"diana-contabilitate/backend/internal/accountinganalysis"
	"diana-contabilitate/backend/internal/platform/requestactor"
)

func (s *Server) accountingAnalysisAccess(w http.ResponseWriter, r *http.Request, review bool) (requestactor.Actor, bool) {
	actor, _ := requestactor.FromContext(r.Context())
	if s.accountingAnalysis == nil {
		writeError(w, r, http.StatusServiceUnavailable, "FEATURE_DISABLED", "Analiza contabilă locală nu este activată.")
		return actor, false
	}
	if !actor.AllowsClient(strings.TrimSpace(r.PathValue("clientId"))) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Factura nu a fost găsită.")
		return actor, false
	}
	if review && actor.Persona != "CONTABIL" {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "Doar un contabil poate valida analiza.")
		return actor, false
	}
	return actor, true
}

func (s *Server) getAccountingAnalysis(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.accountingAnalysisAccess(w, r, false); !ok {
		return
	}
	run, err := s.accountingAnalysis.Get(r.Context(), r.PathValue("clientId"), r.PathValue("invoiceId"))
	if err != nil {
		s.writeAccountingAnalysisError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) requestAccountingAnalysis(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.accountingAnalysisAccess(w, r, false)
	if !ok {
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Idempotency-Key este obligatoriu.")
		return
	}
	run, err := s.accountingAnalysis.Request(r.Context(), accountinganalysis.RequestCommand{ClientID: r.PathValue("clientId"), InvoiceID: r.PathValue("invoiceId"), CommandID: key, ActorID: actor.ID, ActorDisplay: actor.Display})
	if err != nil {
		s.writeAccountingAnalysisError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) reviewAccountingAnalysis(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.accountingAnalysisAccess(w, r, true)
	if !ok {
		return
	}
	var body struct {
		AnalysisID    string                       `json:"analysisId"`
		Action        string                       `json:"action"`
		Reason        string                       `json:"reason"`
		FinalDecision *accountinganalysis.Proposal `json:"finalDecision"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" || decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF || strings.TrimSpace(body.AnalysisID) == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Comanda de review este invalidă.")
		return
	}
	run, err := s.accountingAnalysis.Review(r.Context(), accountinganalysis.ReviewCommand{ClientID: r.PathValue("clientId"), InvoiceID: r.PathValue("invoiceId"), AnalysisID: body.AnalysisID, Action: body.Action, Reason: body.Reason, FinalDecision: body.FinalDecision, CommandID: key, ActorID: actor.ID, ActorDisplay: actor.Display})
	if err != nil {
		s.writeAccountingAnalysisError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) writeAccountingAnalysisError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, accountinganalysis.ErrAnalysisNotFound) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Analiza contabilă nu a fost găsită.")
		return
	}
	s.writeDomainError(w, r, err)
}
