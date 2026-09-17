package httpserver

import (
	"bytes"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/clients"
	"diana-contabilitate/backend/internal/platform/requestactor"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

func (s *Server) allowClient(w http.ResponseWriter, r *http.Request, id string) bool {
	if clients.Authorize(r.Context(), id) != nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Clientul nu a fost găsit.")
		return false
	}
	return true
}
func (s *Server) operationalClient(w http.ResponseWriter, r *http.Request, id string) bool {
	if !s.allowClient(w, r, id) {
		return false
	}
	if s.clients == nil {
		return true
	}
	d, err := s.clients.Detail(r.Context(), id)
	// Legacy service stubs have no management reader; production Store always does.
	if errors.Is(err, apperrors.ErrNotFound) {
		return true
	}
	if err != nil {
		s.clientError(w, r, err)
		return false
	}
	if d.Client.Status == clients.Inactive {
		writeError(w, r, http.StatusConflict, "CLIENT_INACTIVE", "Client inactiv: reactivează înainte de operații ANAF.")
		return false
	}
	return true
}
func (s *Server) clientError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
		writeError(w, r, 404, "NOT_FOUND", "Clientul nu a fost găsit.")
	case errors.Is(err, apperrors.ErrValidation):
		writeError(w, r, 400, "VALIDATION_ERROR", "Date invalide: verifică identitatea, profilul, perioada și dovezile aprobării.")
	case errors.Is(err, apperrors.ErrConflict):
		writeError(w, r, 409, "CONFLICT", strings.TrimPrefix(err.Error(), "conflict: "))
	default:
		s.internalError(w, r, err)
	}
}
func (s *Server) enrichClient(r *http.Request, d *clients.Detail) error {
	if s.spv != nil {
		view, err := s.spv.Get(r.Context(), d.Client.ID)
		if err != nil {
			return err
		}
		d.Onboarding.ApplyANAF(d.Client, view)
	}
	return nil
}
func (s *Server) readClient(w http.ResponseWriter, r *http.Request) (clients.Detail, bool) {
	w.Header().Set("Cache-Control", "no-store")
	d, err := s.clients.Detail(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err == nil {
		err = s.enrichClient(r, &d)
	}
	if err != nil {
		s.clientError(w, r, err)
		return d, false
	}
	return d, true
}
func (s *Server) getClient(w http.ResponseWriter, r *http.Request) {
	d, ok := s.readClient(w, r)
	if ok {
		writeJSON(w, 200, d)
	}
}
func (s *Server) getClientOnboarding(w http.ResponseWriter, r *http.Request) {
	d, ok := s.readClient(w, r)
	if ok {
		writeJSON(w, 200, d.Onboarding)
	}
}
func (s *Server) getClientProfiles(w http.ResponseWriter, r *http.Request) {
	d, ok := s.readClient(w, r)
	if ok {
		writeJSON(w, 200, d.Profiles)
	}
}
func (s *Server) clientCommand(w http.ResponseWriter, r *http.Request, kind string) {
	w.Header().Set("Cache-Control", "no-store")

	var raw map[string]json.RawMessage
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	if decoder.Decode(&raw) != nil || decoder.Decode(new(any)) != io.EOF {
		writeError(w, r, 400, "VALIDATION_ERROR", "Comandă invalidă.")
		return
	}
	fields := map[string][]string{"create": {"company"}, "update": {"company", "expectedRevision"}, "lifecycle": {"status", "expectedRevision"}, "profile": {"profile", "approve", "evidence", "expectedProfileVersion", "expectedRevision"}, "saga": {"sagaEnabled", "expectedRevision"}}
	allowed := map[string]bool{}
	for _, f := range fields[kind] {
		allowed[f] = true
	}
	for f := range raw {
		if !allowed[f] {
			writeError(w, r, 400, "VALIDATION_ERROR", "Câmpul nu aparține acestei comenzi.")
			return
		}
	}
	body, _ := json.Marshal(raw)
	var c clients.Command
	bounded := json.NewDecoder(bytes.NewReader(body))
	bounded.DisallowUnknownFields()
	if bounded.Decode(&c) != nil {
		writeError(w, r, 400, "VALIDATION_ERROR", "Date comandă invalide.")
		return
	}

	c.ClientID = strings.TrimSpace(r.PathValue("id"))
	c.CommandID = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	c.CorrelationID = correlationID(r.Context())
	d, err := s.clients.Command(r.Context(), kind, c)
	if err != nil {
		s.clientError(w, r, err)
		return
	}
	// Command already committed: read-only provider status must not turn success into a retryable failure.
	_ = s.enrichClient(r, &d)
	status := 200
	if kind == "create" {
		status = 201
		w.Header().Set("Location", "/api/v1/clients/"+d.Client.ID)
	}
	writeJSON(w, status, d)
}
func (s *Server) createClient(w http.ResponseWriter, r *http.Request) {
	s.clientCommand(w, r, "create")
}
func (s *Server) updateClient(w http.ResponseWriter, r *http.Request) {
	s.clientCommand(w, r, "update")
}
func (s *Server) clientLifecycle(w http.ResponseWriter, r *http.Request) {
	s.clientCommand(w, r, "lifecycle")
}
func (s *Server) createClientProfile(w http.ResponseWriter, r *http.Request) {
	s.clientCommand(w, r, "profile")
}
func (s *Server) configureClientSaga(w http.ResponseWriter, r *http.Request) {
	s.clientCommand(w, r, "saga")
}

func clientAllowed(r *http.Request, id string) bool {
	a, ok := requestactor.FromContext(r.Context())
	return ok && clients.Allows(a, id)
}
func (s *Server) allowInvoice(w http.ResponseWriter, r *http.Request, id string) bool {
	item, err := s.invoices.Get(r.Context(), id)
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		s.internalError(w, r, err)
		return false
	}
	if err != nil || item == nil {
		writeError(w, r, 404, "NOT_FOUND", "Factura nu a fost găsită.")
		return false
	}
	return s.allowClient(w, r, item.ClientID)
}
func (s *Server) allowContract(w http.ResponseWriter, r *http.Request, id string) bool {
	item, err := s.contracts.Get(r.Context(), id)
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		s.internalError(w, r, err)
		return false
	}
	if err != nil || item == nil {
		writeError(w, r, 404, "NOT_FOUND", "Contractul nu a fost găsit.")
		return false
	}
	return s.allowClient(w, r, item.ClientID)
}

func (s *Server) allowRule(w http.ResponseWriter, r *http.Request, id string) bool {
	item, err := s.rules.Get(r.Context(), id)
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		s.internalError(w, r, err)
		return false
	}
	if err != nil || item == nil {
		writeError(w, r, 404, "NOT_FOUND", "Regula nu a fost găsită.")
		return false
	}
	if item.ClientID != nil {
		return s.allowClient(w, r, *item.ClientID)
	}
	return true
}
