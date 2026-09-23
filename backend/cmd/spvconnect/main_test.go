package main

import (
	"context"
	"strings"
	"testing"

	"diana-contabilitate/backend/internal/platform/config"
)

func TestImportFixtureRejectsNonLocalEnvironmentBeforeIO(t *testing.T) {
	err := importFixture(context.Background(), config.Config{Environment: "production"}, fixtureImport{})
	if err == nil || !strings.Contains(err.Error(), "only in development or test") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFixtureSPVClientReturnsIndependentZIPBytes(t *testing.T) {
	original := []byte("zip payload")
	client := fixtureSPVClient{raw: original}
	got, contentType, err := client.Download(context.Background(), "token", "message")
	if err != nil || contentType != "application/zip" || string(got) != string(original) {
		t.Fatalf("download=%q content_type=%q err=%v", got, contentType, err)
	}
	got[0] = 'X'
	if string(original) != "zip payload" {
		t.Fatal("fixture client exposed its source buffer")
	}
}
