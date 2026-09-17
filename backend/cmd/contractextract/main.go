// Optional, explicit live Gemini smoke test. Not invoked by standard suites.
package main

import (
	"context"
	"diana-contabilitate/backend/internal/contractingestion"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	path := flag.String("file", "", "PDF file (sent to Gemini)")
	flag.Parse()
	key := os.Getenv("GEMINI_API_KEY")
	model := os.Getenv("GEMINI_CONTRACT_MODEL")
	if key == "" || model == "" || *path == "" {
		fmt.Fprintln(os.Stderr, "-file, GEMINI_API_KEY and GEMINI_CONTRACT_MODEL are required")
		os.Exit(2)
	}
	pdf, err := os.ReadFile(*path)
	if err != nil || len(pdf) == 0 || len(pdf) > 20<<20 {
		fmt.Fprintln(os.Stderr, "PDF could not be read or is outside the 20 MiB smoke-test limit")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	extractor := contractingestion.NewGeminiContractExtractor(key, model, "", &http.Client{Timeout: 90 * time.Second})
	result, err := extractor.Extract(ctx, pdf, "application/pdf")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Gemini smoke test failed (no provider payload logged)")
		os.Exit(1)
	}
	fmt.Printf("Structured extraction received; schema=%s; validated=%t\n", contractingestion.ExtractionSchemaVersion, contractingestion.ValidateProposal(result.Proposal) == nil)
}
