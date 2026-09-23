package httpserver

import (
	"testing"
	"time"

	"diana-contabilitate/backend/internal/contractingestion"
)

func TestContractDocumentResponsePreservesSafeExtractionCategory(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	attempt := &contractingestion.Attempt{
		ID: "attempt", Provider: "GEMINI", Model: "model",
		SchemaVersion: contractingestion.ExtractionSchemaVersion, PromptVersion: contractingestion.ExtractionPromptVersion,
		Status: "FAILED", SafeErrorCategory: contractingestion.FailureInvalidStructuredOutput, StartedAt: now, CompletedAt: &now,
	}
	response := contractDocumentResponse(contractingestion.Document{ID: "document", ClientID: "client", Status: contractingestion.StatusExtractionFailed, UploadedAt: now, LatestAttempt: attempt, Attempts: []contractingestion.Attempt{*attempt}}, false)
	if response.Extraction == nil || response.Extraction.SafeErrorCategory != contractingestion.FailureInvalidStructuredOutput || len(response.Attempts) != 1 || response.Attempts[0].SafeErrorCategory != contractingestion.FailureInvalidStructuredOutput {
		t.Fatalf("safe category lost: %+v", response)
	}
}
