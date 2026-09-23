package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"diana-contabilitate/backend/internal/contractingestion"
)

func TestCommercialRuleShapeDiagnosticDoesNotExposeProviderValues(t *testing.T) {
	proposal := contractingestion.Proposal{CommercialClauses: []contractingestion.ProposedCommercialClause{
		{Rule: json.RawMessage(`{"id":"private-contract-id","kind":"UNKNOWN_PRIVATE_VALUE","narrative":"Private price 500 RON","evidence":[{"snippet":"Secret source text"}],"expression":{"op":"literal","value":"500"}}`)},
		{Rule: json.RawMessage(`{"id":"","kind":null,"narrative":"","evidence":[],"expression":null}`)},
	}}
	var output bytes.Buffer
	printCommercialRuleShapes(&output, proposal)
	text := output.String()
	if !strings.Contains(text, "shape[0]: id=present; kind=unsupported; narrative=present; evidence=present; expression=present") ||
		!strings.Contains(text, "shape[1]: id=missing; kind=missing; narrative=missing; evidence=missing; expression=missing") {
		t.Fatalf("unexpected structural diagnostic: %s", text)
	}
	for _, secret := range []string{"private-contract-id", "UNKNOWN_PRIVATE_VALUE", "Private price", "500", "Secret source text"} {
		if strings.Contains(text, secret) {
			t.Fatal("diagnostic exposed provider content")
		}
	}
}
