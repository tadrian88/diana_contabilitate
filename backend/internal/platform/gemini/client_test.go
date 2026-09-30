package gemini_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"diana-contabilitate/backend/internal/llmusage"
	"diana-contabilitate/backend/internal/llmusage/llmusagetest"
	"diana-contabilitate/backend/internal/platform/gemini"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type failingBody struct{}

func (failingBody) Read([]byte) (int, error) { return 0, errors.New("connection reset") }
func (failingBody) Close() error             { return nil }

var testScope = llmusage.Scope{RunKind: llmusage.RunAccountingAnalysis, RunID: "analysis-1", ClientID: "client-a"}

func TestInteractRecordsExactlyOneEventPerSentRequest(t *testing.T) {
	usage := `"usage":{"total_input_tokens":100,"total_output_tokens":20,"total_thought_tokens":30,"total_tokens":150}`
	for _, tc := range []struct {
		name, body, outcome string
		status              int
		reported            bool
	}{
		{"completed", `{"status":"completed","steps":[],` + usage + `}`, llmusage.OutcomeCompleted, 200, true},
		{"incomplete", `{"status":"incomplete",` + usage + `}`, llmusage.OutcomeIncomplete, 200, true},
		{"failed", `{"status":"failed",` + usage + `}`, llmusage.OutcomeProviderFailed, 200, true},
		{"invalid json", `<html>`, llmusage.OutcomeInvalidEnvelope, 200, false},
		{"rate limited with usage", `{` + usage + `}`, llmusage.OutcomeHTTPError, 429, true},
		{"rate limited without usage", `quota`, llmusage.OutcomeHTTPError, 429, false},
		{"unavailable", `oops`, llmusage.OutcomeHTTPError, 500, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			recorder := &llmusagetest.MemoryRecorder{}
			client := gemini.New("key", server.URL, server.Client()).WithUsageRecorder(recorder)
			response, err := client.Interact(llmusage.WithScope(context.Background(), testScope), gemini.Request{Model: "Models/Gemini-3.8-Flash", Operation: llmusage.OperationAccountingAnalysis, Payload: map[string]any{"model": "x"}})
			if err != nil || response.StatusCode != tc.status {
				t.Fatalf("response=%+v err=%v", response, err)
			}
			events := recorder.Events()
			if len(events) != 1 {
				t.Fatalf("events=%d", len(events))
			}
			event := events[0]
			if event.Outcome != tc.outcome || event.HTTPStatus != tc.status || event.Usage.Reported != tc.reported || event.Scope != testScope ||
				event.Provider != gemini.Provider || event.Model != "gemini-3.8-flash" || event.Operation != llmusage.OperationAccountingAnalysis ||
				!strings.HasPrefix(event.ID, "llmcall-") || event.OccurredAt.IsZero() || event.Latency < 0 {
				t.Fatalf("event=%+v", event)
			}
			if tc.reported && (event.Usage.InputTokens != 100 || event.Usage.ThoughtTokens != 30) {
				t.Fatalf("usage=%+v", event.Usage)
			}
		})
	}
}

func TestInteractRecordsTransportFailures(t *testing.T) {
	for _, tc := range []struct {
		name      string
		transport roundTripFunc
		want      error
		status    int
	}{
		{"send", func(*http.Request) (*http.Response, error) { return nil, errors.New("dial failed") }, gemini.ErrSend, 0},
		{"read", func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: failingBody{}, Header: make(http.Header)}, nil
		}, gemini.ErrRead, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := &llmusagetest.MemoryRecorder{}
			client := gemini.New("key", "http://provider.invalid", &http.Client{Transport: tc.transport}).WithUsageRecorder(recorder)
			_, err := client.Interact(llmusage.WithScope(context.Background(), testScope), gemini.Request{Model: "m", Operation: "OP", Payload: map[string]any{}})
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v", err)
			}
			events := recorder.Events()
			if len(events) != 1 || events[0].Outcome != llmusage.OutcomeTransportError || events[0].Usage.Reported || events[0].HTTPStatus != tc.status {
				t.Fatalf("events=%+v", events)
			}
		})
	}
}

func TestInteractKeepsTheProviderRequestUnchanged(t *testing.T) {
	var seen map[string]any
	client := gemini.New("secret", "http://provider.invalid/v1beta/", &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != "http://provider.invalid/v1beta/interactions" || request.Header.Get("x-goog-api-key") != "secret" || request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("request boundary changed: %s %s", request.Method, request.URL)
		}
		_ = json.NewDecoder(request.Body).Decode(&seen)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"status":"completed"}`)), Header: make(http.Header)}, nil
	})})
	if _, err := client.Interact(context.Background(), gemini.Request{Model: "m", Operation: "OP", Payload: map[string]any{"model": "m", "store": false}}); err != nil {
		t.Fatal(err)
	}
	if seen["model"] != "m" || seen["store"] != false {
		t.Fatalf("payload=%v", seen)
	}
}

func TestInteractRecorderErrorNeverChangesTheResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"completed","usage":{"total_input_tokens":1,"total_output_tokens":1}}`))
	}))
	defer server.Close()
	failing := llmusage.RecorderFunc(func(context.Context, llmusage.Event) error { return errors.New("database down") })
	response, err := gemini.New("key", server.URL, server.Client()).WithUsageRecorder(failing).Interact(llmusage.WithScope(context.Background(), testScope), gemini.Request{Model: "m", Operation: "OP", Payload: map[string]any{}})
	if err != nil || response.Envelope.Status != "completed" {
		t.Fatalf("response=%+v err=%v", response, err)
	}
}
