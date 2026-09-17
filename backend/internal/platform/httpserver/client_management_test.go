package httpserver

import (
	"context"
	"diana-contabilitate/backend/internal/clients"
	"diana-contabilitate/backend/internal/platform/requestactor"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type clientHTTPReader struct{ calls int }

func (*clientHTTPReader) ListClients(context.Context) ([]clients.Client, error) {
	return []clients.Client{{ID: "a", Name: "A", CUI: "12345678"}, {ID: "b", Name: "B", CUI: "87654321"}}, nil
}
func (s *clientHTTPReader) GetClientDetail(_ context.Context, id string) (clients.Detail, error) {
	s.calls++
	return clients.Detail{Client: clients.Client{ID: id, Name: "Firmă", CUI: "12345678"}, Profiles: nil, History: []clients.History{}, Onboarding: clients.Readiness{}}, nil
}
func (s *clientHTTPReader) ExecuteClientCommand(_ context.Context, _ string, c clients.Command) (clients.Detail, error) {
	s.calls++
	return clients.Detail{Client: clients.Client{ID: c.ClientID, Name: c.Company.Name, CUI: c.Company.CUI}}, nil
}
func TestClientManagementHTTPAuthorizationAndBoundedDTO(t *testing.T) {
	reader := &clientHTTPReader{}
	handler := New(clients.NewService(reader), nil, nil, nil, nil, nil, readyStub{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	call := func(method, path, body, key string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r = r.WithContext(requestactor.WithActor(r.Context(), requestactor.Actor{ID: "accountant-a", Display: "A", AuthorizedClientIDs: []string{"a"}}))
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/api/v1/clients/b", "/api/v1/clients/b/onboarding", "/api/v1/clients/b/accounting-profiles", "/api/v1/clients/b/spv"} {
		w := call("GET", path, "", "")
		if w.Code != 404 {
			t.Fatalf("%s %d %s", path, w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"company", "lifecycle", "accounting-profiles", "saga-configuration"} {
		w := call("POST", "/api/v1/clients/b/"+path, `{}`, "x")
		if w.Code != 404 {
			t.Fatalf("%s %d", path, w.Code)
		}
	}
	if reader.calls != 0 {
		t.Fatal("unauthorized requests reached persistence")
	}
	w := call("GET", "/api/v1/clients/a", "", "")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	var response any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal("invalid detail JSON", err)
	}
	assertNoClientSecrets(t, response)
	w = call("GET", "/api/v1/clients", "", "")
	var list []clients.Client
	if json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list) != 1 || list[0].ID != "a" {
		t.Fatal(w.Body.String())
	}
	w = call(http.MethodPost, "/api/v1/clients", `{"company":{"name":"Firma","cui":"12345678","country":"RO","defaultCurrency":"RON"}}`, "x")
	if w.Code != 404 {
		t.Fatal("grant-only actor created a new tenant", w.Code)
	}
	w = call("POST", "/api/v1/clients/a/company", `{"privateKey":"secret"}`, "x")
	if w.Code != 400 {
		t.Fatal("unknown command field accepted", w.Code)
	}
}

// Inspect field names structurally: "pin" is forbidden, "sagaMappingApproved" is not.
func assertNoClientSecrets(t *testing.T, value any) {
	t.Helper()
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			normalized := strings.ToLower(strings.ReplaceAll(key, "_", ""))
			switch normalized {
			case "privatekey", "certificatepassword", "pin", "accesstoken", "refreshtoken", "ciphertext":
				t.Fatal("secret-shaped field leaked", key)
			}
			assertNoClientSecrets(t, child)
		}
	case []any:
		for _, child := range v {
			assertNoClientSecrets(t, child)
		}
	}
}
