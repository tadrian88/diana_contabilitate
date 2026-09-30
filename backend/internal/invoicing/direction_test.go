package invoicing

import (
	"context"
	"errors"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/money"
)

type capturingIngestStore struct {
	noMutationPipelineStore
	input *IngestionInput
}

func (s *capturingIngestStore) IngestInvoice(_ context.Context, input IngestionInput, _ string, _ time.Time) (*Invoice, bool, error) {
	s.input = &input
	return &Invoice{ID: "invoice-1", Direction: input.Direction}, true, nil
}

func directionIngestInput() IngestionInput {
	supplier := "RO51741718"
	return IngestionInput{ClientID: "client-1", Source: "TEST", ExternalDeliveryID: "delivery-1", SupplierName: "VICTORIA TEST SRL", SupplierCUI: &supplier,
		DocumentNumber: "VE1", IssueDate: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Total: money.Money{Amount: money.MustParse("121.00"), Currency: "RON"}, SPVReference: "spv-1"}
}

func TestIngestDefaultsToIncoming(t *testing.T) {
	store := &capturingIngestStore{}
	if _, _, err := NewPipelineService(store, unconfirmedExporter{}, nil).Ingest(context.Background(), directionIngestInput()); err != nil {
		t.Fatal(err)
	}
	if store.input == nil || store.input.Direction != DirectionIncoming {
		t.Fatalf("direction = %#v", store.input)
	}
}

func TestIngestOutgoingRequiresCustomer(t *testing.T) {
	for _, tc := range []struct{ name, identifier string }{{"", "RO26999270"}, {"CLIENT TEST SRL", ""}, {"CLIENT TEST SRL", "   "}} {
		input := directionIngestInput()
		input.Direction, input.CustomerName, input.CustomerIdentifier = DirectionOutgoing, tc.name, tc.identifier
		store := &capturingIngestStore{}
		if _, _, err := NewPipelineService(store, unconfirmedExporter{}, nil).Ingest(context.Background(), input); !errors.Is(err, apperrors.ErrValidation) || store.input != nil {
			t.Fatalf("customer %q/%q: err=%v stored=%v", tc.name, tc.identifier, err, store.input != nil)
		}
	}
	input := directionIngestInput()
	input.Direction, input.CustomerName, input.CustomerIdentifier = DirectionOutgoing, "PERSOANĂ FIZICĂ TEST", "1800101420010"
	store := &capturingIngestStore{}
	if _, _, err := NewPipelineService(store, unconfirmedExporter{}, nil).Ingest(context.Background(), input); err != nil || store.input.Direction != DirectionOutgoing {
		t.Fatalf("err=%v input=%#v", err, store.input)
	}
}

func TestIngestRejectsUnknownDirection(t *testing.T) {
	input := directionIngestInput()
	input.Direction = "SIDEWAYS"
	if _, _, err := NewPipelineService(&capturingIngestStore{}, unconfirmedExporter{}, nil).Ingest(context.Background(), input); !errors.Is(err, apperrors.ErrValidation) {
		t.Fatalf("err=%v", err)
	}
}

func TestCounterpartyFollowsDirection(t *testing.T) {
	supplier, normalizedSupplier := "RO51741718", "51741718"
	customer, customerID, normalizedCustomer, kind := "CLIENT TEST SRL", "RO26999270", "26999270", "CUI"
	incoming := Invoice{SupplierName: "FURNIZOR TEST SRL", SupplierCUI: &supplier, NormalizedSupplierCUI: &normalizedSupplier}
	if party := incoming.Counterparty(); party.Name != "FURNIZOR TEST SRL" || *party.NormalizedIdentifier != "51741718" {
		t.Fatalf("incoming counterparty = %#v", party)
	}
	outgoing := incoming
	outgoing.Direction, outgoing.CustomerName, outgoing.CustomerIdentifier, outgoing.NormalizedCustomerID, outgoing.CustomerIdentifierKind = DirectionOutgoing, &customer, &customerID, &normalizedCustomer, &kind
	if party := outgoing.Counterparty(); party.Name != customer || *party.NormalizedIdentifier != normalizedCustomer || party.IdentifierKind != "CUI" {
		t.Fatalf("outgoing counterparty = %#v", party)
	}
}
