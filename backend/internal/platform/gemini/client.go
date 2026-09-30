package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/llmusage"
)

const (
	DefaultBaseURL   = "https://generativelanguage.googleapis.com/v1beta"
	Provider         = "GEMINI"
	maxResponseBytes = 2 << 20
)

var (
	// ErrSend means no response arrived (network error, timeout, cancellation).
	ErrSend = errors.New("gemini request was not delivered")
	// ErrRead means a response arrived but its body could not be read.
	ErrRead = errors.New("gemini response could not be read")
)

type Client struct {
	apiKey, baseURL string
	http            *http.Client
	recorder        llmusage.Recorder
	now             func() time.Time
}

func New(apiKey, baseURL string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{apiKey: apiKey, baseURL: strings.TrimRight(baseURL, "/"), http: httpClient, recorder: llmusage.Noop{}, now: func() time.Time { return time.Now().UTC() }}
}

func (c *Client) WithUsageRecorder(recorder llmusage.Recorder) *Client {
	if recorder == nil {
		recorder = llmusage.Noop{}
	}
	c.recorder = recorder
	return c
}

func (c *Client) Configured() bool { return c.apiKey != "" }

type Request struct {
	Model     string
	Operation string
	Payload   any
}

// Response is returned whenever the provider answered, whatever the status.
// EnvelopeErr reports a body that is not a JSON interaction.
type Response struct {
	StatusCode  int
	Envelope    Envelope
	EnvelopeErr error
}

// Interact posts one interaction and records exactly one usage event for
// every request that was sent, before the caller interprets the response.
func (c *Client) Interact(ctx context.Context, request Request) (Response, error) {
	body, err := json.Marshal(request.Payload)
	if err != nil {
		return Response{}, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/interactions", bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("x-goog-api-key", c.apiKey)
	event := llmusage.Event{Provider: Provider, Model: llmusage.NormalizeModel(request.Model), Operation: request.Operation}
	event.Scope, _ = llmusage.ScopeFrom(ctx)
	started := c.now()
	response, err := c.http.Do(httpRequest)
	if err != nil {
		c.record(ctx, event, started, llmusage.OutcomeTransportError, 0, "", llmusage.Usage{})
		return Response{}, fmt.Errorf("%w: %w", ErrSend, err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		c.record(ctx, event, started, llmusage.OutcomeTransportError, response.StatusCode, "", llmusage.Usage{})
		return Response{StatusCode: response.StatusCode}, fmt.Errorf("%w: %w", ErrRead, err)
	}
	envelope, envelopeErr := ParseEnvelope(raw)
	c.record(ctx, event, started, outcome(response.StatusCode, envelope, envelopeErr), response.StatusCode, envelope.Status, envelope.Usage)
	return Response{StatusCode: response.StatusCode, Envelope: envelope, EnvelopeErr: envelopeErr}, nil
}

func outcome(status int, envelope Envelope, envelopeErr error) string {
	switch {
	case status < 200 || status >= 300:
		return llmusage.OutcomeHTTPError
	case envelopeErr != nil:
		return llmusage.OutcomeInvalidEnvelope
	case envelope.Status == "completed":
		return llmusage.OutcomeCompleted
	case envelope.Status == "incomplete":
		return llmusage.OutcomeIncomplete
	default:
		return llmusage.OutcomeProviderFailed
	}
}

func (c *Client) record(ctx context.Context, event llmusage.Event, started time.Time, outcome string, status int, providerStatus string, usage llmusage.Usage) {
	event.ID = llmusage.NewEventID()
	event.OccurredAt = c.now()
	event.Latency = event.OccurredAt.Sub(started)
	event.Outcome = outcome
	event.HTTPStatus = status
	event.ProviderStatus = truncate(providerStatus, 64)
	event.Usage = usage
	// Recorders are best-effort by contract; a recording error never changes
	// the outcome of the provider call.
	_ = c.recorder.RecordLLMCall(ctx, event)
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
