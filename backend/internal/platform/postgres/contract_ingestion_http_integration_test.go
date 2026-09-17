//go:build integration

package postgres

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"diana-contabilitate/backend/internal/contractingestion/fixtures"
	"diana-contabilitate/backend/internal/platform/httpserver"
	"diana-contabilitate/backend/internal/platform/observability"
	"diana-contabilitate/backend/internal/platform/requestactor"
)

func TestContractIngestionHTTPUploadReviewConfirmAndTenantIsolation(t *testing.T) {
	tc := newContractTestContext(t)
	service, _, _ := contractIngestionFixture(t, tc)
	handler := httpserver.NewWithContractIngestion(nil, nil, nil, nil, nil, nil, nil, nil, service, tc.store, slog.New(slog.NewTextHandler(io.Discard, nil)), observability.NewMetrics())
	actor := requestactor.Actor{ID: "http-reviewer", Display: "HTTP Reviewer", AuthorizedClientIDs: []string{tc.clientID}}
	send := func(method, path string, body io.Reader, contentType, key string, a requestactor.Actor) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, body)
		r = r.WithContext(requestactor.WithActor(r.Context(), a))
		r.Header.Set("Content-Type", contentType)
		if key != "" {
			r.Header.Set("Idempotency-Key", key)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	base := fmt.Sprintf("/api/v1/clients/%s/contract-documents", tc.clientID)
	pdf := fixtures.PDF("romanian")
	upload := func(a requestactor.Actor) *httptest.ResponseRecorder {
		var b bytes.Buffer
		m := multipart.NewWriter(&b)
		part, err := m.CreateFormFile("file", "source.pdf")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write(pdf)
		_ = m.Close()
		return send(http.MethodPost, base, &b, m.FormDataContentType(), "", a)
	}
	denied := upload(requestactor.Actor{ID: "outsider", AuthorizedClientIDs: []string{"another-client"}})
	if denied.Code != http.StatusNotFound {
		t.Fatalf("unauthorized=%d %s", denied.Code, denied.Body.String())
	}
	uploaded := upload(actor)
	if uploaded.Code != http.StatusAccepted {
		t.Fatalf("upload=%d %s", uploaded.Code, uploaded.Body.String())
	}
	var identity struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(uploaded.Body.Bytes(), &identity); err != nil || identity.ID == "" {
		t.Fatal("upload identity")
	}
	if duplicate := upload(actor); duplicate.Code != http.StatusOK {
		t.Fatalf("duplicate=%d", duplicate.Code)
	}
	path := base + "/" + identity.ID
	file := send(http.MethodGet, path+"/file", nil, "", "", actor)
	if file.Code != 200 || !bytes.Equal(file.Body.Bytes(), pdf) || file.Header().Get("Cache-Control") != "private, no-store" || file.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("immutable/private PDF=%d headers=%v", file.Code, file.Header())
	}
	if denied := send(http.MethodGet, path+"/file", nil, "", "", requestactor.Actor{AuthorizedClientIDs: []string{"other"}}); denied.Code != 404 {
		t.Fatal("cross-client file access")
	}
	doc, err := tc.store.GetDocument(tc.ctx, tc.clientID, identity.ID)
	if err != nil {
		t.Fatal(err)
	}
	command := ingestionReview(t, tc, service, doc)
	body, _ := json.Marshal(map[string]any{"extractionAttemptId": command.ExtractionAttemptID, "expectedDocumentRevision": command.ExpectedDocumentRevision, "contract": command.Contract})
	if missing := send(http.MethodPost, path+"/confirm", bytes.NewReader(body), "application/json", "", actor); missing.Code != 400 {
		t.Fatal("missing idempotency key accepted")
	}
	confirmed := send(http.MethodPost, path+"/confirm", bytes.NewReader(body), "application/json", "http-confirm", actor)
	if confirmed.Code != 200 {
		t.Fatalf("confirm=%d %s", confirmed.Code, confirmed.Body.String())
	}
	replay := send(http.MethodPost, path+"/confirm", bytes.NewReader(body), "application/json", "http-confirm", actor)
	if replay.Code != 200 {
		t.Fatalf("replay=%d %s", replay.Code, replay.Body.String())
	}
	stale := send(http.MethodPost, path+"/confirm", bytes.NewReader(body), "application/json", "different-reviewer", actor)
	if stale.Code != 409 {
		t.Fatalf("stale=%d", stale.Code)
	}
}
