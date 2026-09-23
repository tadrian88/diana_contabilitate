package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"diana-contabilitate/backend/internal/contractingestion"
)

func main() {
	pdf, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	extractor := contractingestion.NewGeminiContractExtractor(os.Getenv("GEMINI_API_KEY"), os.Getenv("GEMINI_CONTRACT_MODEL"), os.Getenv("GEMINI_API_BASE_URL"), &http.Client{Timeout: 90 * time.Second})
	result, err := extractor.Extract(ctx, pdf, "application/pdf")
	if err != nil {
		panic(err)
	}
	for i, clause := range result.Proposal.CommercialClauses {
		var compact any
		_ = json.Unmarshal(clause.Rule, &compact)
		encoded, _ := json.Marshal(compact)
		fmt.Printf("rule[%d]=%s\n", i, encoded)
	}
	fmt.Printf("validation=%v\n", contractingestion.ValidateProposal(result.Proposal))
}
