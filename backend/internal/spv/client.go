package spv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"
)

const (
	DefaultAuthorizeURL     = "https://logincert.anaf.ro/anaf-oauth2/v1/authorize"
	DefaultTokenURL         = "https://logincert.anaf.ro/anaf-oauth2/v1/token"
	DefaultTestAPIURL       = "https://api.anaf.ro/test/FCTEL/rest"
	DefaultProductionAPIURL = "https://api.anaf.ro/prod/FCTEL/rest"
	maxResponseBytes        = 50 << 20
)

type HTTPClient struct {
	http                *http.Client
	baseURL, tokenURL   string
	minimumCallInterval time.Duration
	mu                  sync.Mutex
	lastCall            time.Time
}

func NewHTTPClient(client *http.Client, baseURL, tokenURL string) *HTTPClient {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &HTTPClient{http: client, baseURL: strings.TrimRight(baseURL, "/"), tokenURL: tokenURL, minimumCallInterval: 100 * time.Millisecond}
}

func (c *HTTPClient) WithMinimumCallInterval(value time.Duration) *HTTPClient {
	c.minimumCallInterval = value
	return c
}

func AuthorizationURL(clientID, redirectURI, state string) string {
	return AuthorizationURLAt(DefaultAuthorizeURL, clientID, redirectURI, state)
}

func AuthorizationURLAt(authorizeURL, clientID, redirectURI, state string) string {
	q := url.Values{"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {redirectURI}, "state": {state}, "token_content_type": {"jwt"}}
	return strings.TrimRight(authorizeURL, "?") + "?" + q.Encode()
}

func (c *HTTPClient) ExchangeToken(ctx context.Context, code, clientID, clientSecret, redirectURI string) (TokenResponse, error) {
	return c.token(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {clientID}, "client_secret": {clientSecret}, "redirect_uri": {redirectURI}, "token_content_type": {"jwt"}})
}

func (c *HTTPClient) RefreshToken(ctx context.Context, refresh, clientID, clientSecret string) (TokenResponse, error) {
	return c.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {clientID}, "client_secret": {clientSecret}, "token_content_type": {"jwt"}})
}

func (c *HTTPClient) token(ctx context.Context, values url.Values) (TokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, bytes.NewBufferString(values.Encode()))
	if err != nil {
		return TokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if id, secret := values.Get("client_id"), values.Get("client_secret"); id != "" && secret != "" {
		req.SetBasicAuth(id, secret)
	}
	if err = c.pace(ctx); err != nil {
		return TokenResponse{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("%w: token request: %v", ErrTransient, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
		return TokenResponse{}, statusError(ErrTransient, "token", resp)
	}
	if resp.StatusCode >= 400 {
		return TokenResponse{}, statusError(ErrPermanent, "token", resp)
	}
	var raw struct {
		Access         string `json:"access_token"`
		Refresh        string `json:"refresh_token"`
		Expires        int64  `json:"expires_in"`
		RefreshExpires int64  `json:"refresh_expires_in"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&raw); err != nil {
		return TokenResponse{}, fmt.Errorf("%w: decode token response", ErrPermanent)
	}
	if raw.Access == "" || raw.Refresh == "" || raw.Expires <= 0 {
		return TokenResponse{}, fmt.Errorf("%w: incomplete token response", ErrPermanent)
	}
	return TokenResponse{AccessToken: raw.Access, RefreshToken: raw.Refresh, ExpiresIn: time.Duration(raw.Expires) * time.Second, RefreshExpiresIn: time.Duration(raw.RefreshExpires) * time.Second}, nil
}

func (c *HTTPClient) ListIncoming(ctx context.Context, token, cif string, start, end time.Time, page int) ([]Message, int, error) {
	q := url.Values{"startTime": {strconv.FormatInt(start.UnixMilli(), 10)}, "endTime": {strconv.FormatInt(end.UnixMilli(), 10)}, "pagina": {strconv.Itoa(page)}, "cif": {cif}, "filtru": {"P"}}
	var raw struct {
		Messages []json.RawMessage `json:"mesaje"`
		Pages    int               `json:"numar_total_pagini"`
		Error    string            `json:"eroare"`
	}
	if err := c.getJSON(ctx, token, c.baseURL+"/listaMesajePaginatieFactura?"+q.Encode(), &raw); err != nil {
		return nil, 0, err
	}
	if raw.Error != "" && !strings.Contains(strings.ToLower(raw.Error), "nu exista mesaje") {
		return nil, 0, fmt.Errorf("%w: ANAF list response reported an error", ErrPermanent)
	}
	if raw.Pages < 1 {
		raw.Pages = 1
	}
	result := make([]Message, 0, len(raw.Messages))
	for _, item := range raw.Messages {
		message, err := decodeMessage(item)
		if err != nil {
			// An item without a usable ANAF message ID cannot be downloaded. Keep
			// the rest of the independently-addressable page, but never log the
			// raw element (it may contain invoice metadata).
			continue
		}
		result = append(result, message)
	}
	return result, raw.Pages, nil
}

func decodeMessage(data []byte) (Message, error) {
	var raw struct {
		ID, Upload json.RawMessage
		RequestID  string `json:"id_cerere"`
		Type       string `json:"tip"`
		Created    string `json:"data_creare"`
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return Message{}, fmt.Errorf("%w: malformed list item", ErrPermanent)
	}
	raw.ID, raw.Upload = envelope["id"], envelope["id_solicitare"]
	_ = json.Unmarshal(envelope["id_cerere"], &raw.RequestID)
	_ = json.Unmarshal(envelope["tip"], &raw.Type)
	_ = json.Unmarshal(envelope["data_creare"], &raw.Created)
	id, err := flexibleID(raw.ID)
	if err != nil || id == "" {
		return Message{}, fmt.Errorf("%w: list item has invalid message id", ErrPermanent)
	}
	upload, _ := flexibleID(raw.Upload)
	createdAt, _ := ParseMessageCreatedAt(raw.Created)
	return Message{ID: id, UploadID: upload, RequestID: raw.RequestID, Type: raw.Type, CreatedRaw: raw.Created, CreatedAt: createdAt}, nil
}

// ParseMessageCreatedAt converts the ANAF data_creare value to an absolute
// timestamp. Compact values are Romanian civil time; malformed metadata is
// preserved as raw text by the caller but is never guessed.
func ParseMessageCreatedAt(raw string) (*time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	location, err := time.LoadLocation("Europe/Bucharest")
	if err != nil {
		return nil, err
	}
	for _, candidate := range []struct {
		layout   string
		location *time.Location
	}{
		{"200601021504", location},
		{"20060102150405", location},
		{time.RFC3339, time.UTC},
		{"2006-01-02 15:04:05", location},
	} {
		parsed, parseErr := time.ParseInLocation(candidate.layout, value, candidate.location)
		if parseErr == nil {
			return &parsed, nil
		}
	}
	return nil, fmt.Errorf("%w: invalid ANAF data_creare", ErrPermanent)
}

func RomanianCalendarDay(value time.Time) time.Time {
	location, err := time.LoadLocation("Europe/Bucharest")
	if err != nil {
		location = time.UTC
	}
	local := value.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

func flexibleID(raw []byte) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var value string
	if raw[0] == '"' {
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", err
		}
	} else {
		value = string(raw)
	}
	if _, err := strconv.ParseInt(value, 10, 64); err != nil {
		return "", err
	}
	return value, nil
}

func (c *HTTPClient) Download(ctx context.Context, token, messageID string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/descarcare?id="+url.QueryEscape(messageID), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if err = c.pace(ctx); err != nil {
		return nil, "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("%w: download request: %v", ErrTransient, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
		return nil, "", statusError(ErrTransient, "download", resp)
	}
	if resp.StatusCode >= 400 {
		return nil, "", statusError(ErrPermanent, "download", resp)
	}
	data, err := readBounded(resp.Body, maxResponseBytes)
	if err != nil {
		return nil, "", err
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/zip"
	}
	return data, contentType, nil
}

func (c *HTTPClient) getJSON(ctx context.Context, token, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if err = c.pace(ctx); err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: list request: %v", ErrTransient, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
		return statusError(ErrTransient, "list", resp)
	}
	if resp.StatusCode >= 400 {
		return statusError(ErrPermanent, "list", resp)
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(target); err != nil {
		return fmt.Errorf("%w: malformed list response", ErrPermanent)
	}
	return nil
}

func (c *HTTPClient) pace(ctx context.Context) error {
	c.mu.Lock()
	wait := c.minimumCallInterval - time.Since(c.lastCall)
	if c.lastCall.IsZero() || wait <= 0 {
		c.lastCall = time.Now()
		c.mu.Unlock()
		return nil
	}
	c.lastCall = time.Now().Add(wait)
	c.mu.Unlock()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func statusError(kind error, operation string, resp *http.Response) error {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	return fmt.Errorf("%w: ANAF %s returned HTTP %d", kind, operation, resp.StatusCode)
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read response", ErrTransient)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: ANAF response exceeds %d bytes", ErrPermanent, limit)
	}
	return data, nil
}
