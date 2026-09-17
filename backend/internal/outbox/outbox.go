package outbox

import (
	"context"
	"errors"
	"time"
)

const (
	EventInvoiceContinue             = "INVOICE_CONTINUE"
	EventContractAvailable           = "CONTRACT_AVAILABLE"
	EventContractExtractionRequested = "CONTRACT_EXTRACTION_REQUESTED"
	EventContractActivationRequested = "CONTRACT_ACTIVATION_REQUESTED"
)

var ErrClaimLost = errors.New("outbox claim lost")

type Entry struct {
	ID             string
	EventType      string
	AggregateID    string
	IdempotencyKey string
	CorrelationID  string
	Attempts       uint
}

type Stats struct {
	PendingCount     int64
	FailedCount      int64
	OldestPendingAge time.Duration
}

type Store interface {
	Claim(context.Context, string, int, time.Time, time.Duration) ([]Entry, error)
	MarkDispatched(context.Context, string, string, time.Time) error
	ReleaseClaim(context.Context, string, string, string, time.Time) error
	MarkFailed(context.Context, string, string, string, time.Time) error
	Stats(context.Context, time.Time) (Stats, error)
}
