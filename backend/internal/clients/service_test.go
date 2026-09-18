package clients

import (
	"context"
	"testing"
)

type readerStub struct{ clients []Client }

func (s readerStub) ListClients(context.Context) ([]Client, error) { return s.clients, nil }

func TestListPreservesApprovedClientRepresentation(t *testing.T) {
	service := NewService(readerStub{clients: []Client{{ID: "client-alfa", Name: "Client Demo Alfa SRL", CUI: "RO91000001"}}})
	result, err := service.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].CUI != "RO91000001" {
		t.Fatalf("unexpected clients: %+v", result)
	}
}
