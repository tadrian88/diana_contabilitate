package spv

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPClientListsPagesAndAcceptsStringOrNumericIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Error("missing bearer")
		}
		if r.URL.Query().Get("filtru") != "P" {
			t.Error("not incoming filter")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"mesaje":[{"id":"101","id_solicitare":22,"tip":"FACTURA PRIMITA","data_creare":"202609141200"}],"numar_total_pagini":2}`))
	}))
	defer server.Close()
	client := NewHTTPClient(server.Client(), server.URL, server.URL)
	messages, pages, err := client.ListIncoming(context.Background(), "token", "22222222", time.Unix(0, 0), time.Now(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if pages != 2 || len(messages) != 1 || messages[0].ID != "101" || messages[0].UploadID != "22" || messages[0].CreatedAt == nil || messages[0].CreatedAt.Format("200601021504") != "202609141200" {
		t.Fatalf("unexpected: %+v %d", messages, pages)
	}
}

func TestParseMessageCreatedAtRejectsGuessingAndPreservesRomanianCalendarDay(t *testing.T) {
	value, err := ParseMessageCreatedAt("202609140005")
	if err != nil || value == nil || RomanianCalendarDay(*value).Format("2006-01-02") != "2026-09-14" {
		t.Fatalf("parsed=%v err=%v", value, err)
	}
	if value, err = ParseMessageCreatedAt("not-a-date"); err == nil || value != nil {
		t.Fatalf("invalid metadata was accepted: %v %v", value, err)
	}
}

func TestHTTPClientClassifiesFailuresAndBoundsDownload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "descarcare") {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	client := NewHTTPClient(server.Client(), server.URL, server.URL)
	if _, _, err := client.Download(context.Background(), "x", "1"); !strings.Contains(err.Error(), ErrTransient.Error()) {
		t.Fatalf("want transient: %v", err)
	}
}

func TestHTTPClientExchangesTokenWithBasicAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, secret, ok := r.BasicAuth()
		if !ok || id != "app" || secret != "secret" {
			t.Fatalf("missing OAuth basic authentication")
		}
		if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("token_content_type") != "jwt" {
			t.Fatalf("unexpected form: %v %v", r.Form, err)
		}
		_, _ = w.Write([]byte(`{"access_token":"access","refresh_token":"refresh","expires_in":3600,"refresh_expires_in":7200}`))
	}))
	defer server.Close()
	result, err := NewHTTPClient(server.Client(), server.URL, server.URL).WithMinimumCallInterval(time.Nanosecond).ExchangeToken(context.Background(), "code", "app", "secret", "https://callback.invalid")
	if err != nil || result.AccessToken != "access" || result.ExpiresIn != time.Hour {
		t.Fatalf("token=%+v err=%v", result, err)
	}
}

func TestAESGCMCipherRoundTripAndRejectsWrongKey(t *testing.T) {
	a, _ := NewAESGCMCipher(strings.Repeat("01", 32))
	b, _ := NewAESGCMCipher(strings.Repeat("02", 32))
	encrypted, err := a.Encrypt("secret")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := a.Decrypt(encrypted)
	if err != nil || plain != "secret" {
		t.Fatalf("roundtrip %q %v", plain, err)
	}
	if _, err = b.Decrypt(encrypted); err == nil {
		t.Fatal("wrong key accepted")
	}
}
