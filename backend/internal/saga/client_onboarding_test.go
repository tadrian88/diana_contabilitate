package saga

import (
	"diana-contabilitate/backend/internal/clients"
	"strings"
	"testing"
)

func TestClientOnboardingMasterDataReachesSAGAGenerator(t *testing.T) {
	item := validInvoice()
	client := clients.Client{ID: item.ClientID, Name: "Companie configurată SRL", CUI: "RO12345678"}
	artifact, err := Generate(item, ClientIdentity{ID: client.ID, Name: client.Name, CUI: client.CUI})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(artifact.Payload), "<ClientNume>Companie configurată SRL</ClientNume>") || !strings.Contains(string(artifact.Payload), "<ClientCIF>RO12345678</ClientCIF>") {
		t.Fatal(string(artifact.Payload))
	}
}
