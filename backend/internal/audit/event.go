package audit

import (
	"encoding/json"
	"time"
)

type ActorKind string

const (
	ActorSystem   ActorKind = "SYSTEM"
	ActorUser     ActorKind = "USER"
	ActorExternal ActorKind = "EXTERNAL"
)

type Event struct {
	ID               string
	ClientID         string
	InvoiceID        *string
	ValidationTaskID *string
	AggregateType    string
	AggregateID      string
	EventType        string
	OccurredAt       time.Time
	ActorKind        ActorKind
	ActorID          *string
	ActorDisplay     *string
	Automatic        bool
	Detail           string
	BeforeSnapshot   json.RawMessage
	AfterSnapshot    json.RawMessage
	CorrelationID    *string
}
