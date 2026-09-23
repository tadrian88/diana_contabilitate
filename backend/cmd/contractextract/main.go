// Optional, explicit live Gemini smoke test. Not invoked by standard suites.
package main

import (
	"bytes"
	"context"
	"diana-contabilitate/backend/internal/commercialvalidation"
	"diana-contabilitate/backend/internal/contractingestion"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	path := flag.String("file", "", "PDF file (sent to Gemini)")
	flag.Parse()
	key := os.Getenv("GEMINI_API_KEY")
	model := os.Getenv("GEMINI_CONTRACT_MODEL")
	baseURL := os.Getenv("GEMINI_API_BASE_URL")
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
	extractor := contractingestion.NewGeminiContractExtractor(key, model, baseURL, &http.Client{Timeout: 90 * time.Second})
	result, err := extractor.Extract(ctx, pdf, "application/pdf")
	if err != nil {
		if failure, ok := contractingestion.ExtractionFailureDetails(err); ok {
			fmt.Fprintf(os.Stderr, "Gemini smoke test failed; category=%s; http_status=%d; provider=%s; model=%s (no provider payload logged)\n", failure.Category, failure.HTTPStatus, extractor.Provider(), extractor.Model())
		} else {
			fmt.Fprintln(os.Stderr, "Gemini smoke test failed; category=UNKNOWN (no provider payload logged)")
		}
		os.Exit(1)
	}
	if err := contractingestion.ValidateProposal(result.Proposal); err != nil {
		var validation *contractingestion.ProposalValidationError
		if errors.As(err, &validation) {
			fmt.Fprintf(os.Stderr, "Structured extraction failed semantic validation; schema=%s; code=%s; path=%s\n", contractingestion.ExtractionSchemaVersion, validation.Code, validation.Path)
			if strings.HasPrefix(validation.Path, "commercialClauses[") {
				printCommercialRuleShapes(os.Stderr, result.Proposal)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Structured extraction contains no contract data; schema=%s\n", contractingestion.ExtractionSchemaVersion)
		}
		os.Exit(1)
	}
	pending := len(contractingestion.PendingCommercialClauses(result.Proposal))
	fmt.Printf("Structured extraction received; schema=%s; validated=true; pending_commercial_rules=%d; human_review_required=true\n", contractingestion.ExtractionSchemaVersion, pending)
	if pending > 0 {
		fmt.Fprintln(os.Stderr, "Commercial coverage remains PARTIAL: clauses without expressions cannot validate invoice prices.")
	}
}

func printCommercialRuleShapes(out io.Writer, proposal contractingestion.Proposal) {
	for index, clause := range proposal.CommercialClauses {
		if index >= 12 {
			fmt.Fprintf(out, "Commercial rule shapes truncated; total=%d\n", len(proposal.CommercialClauses))
			return
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(clause.Rule, &fields) != nil || fields == nil {
			fmt.Fprintf(out, "Commercial rule shape[%d]: rule=malformed\n", index)
			continue
		}
		kind := stringShape(fields["kind"])
		if kind == "present" {
			var value string
			_ = json.Unmarshal(fields["kind"], &value)
			if !commercialvalidation.ValidRuleKind(commercialvalidation.RuleKind(value)) {
				kind = "unsupported"
			}
		}
		evidence := "missing"
		if raw := fields["evidence"]; len(raw) > 0 && !bytes.Equal(raw, []byte("null")) {
			var items []struct {
				Snippet string `json:"snippet"`
			}
			if json.Unmarshal(raw, &items) != nil {
				evidence = "malformed"
			} else if len(items) > 0 {
				evidence = "present"
				for _, item := range items {
					if strings.TrimSpace(item.Snippet) == "" {
						evidence = "incomplete"
						break
					}
				}
			}
		}
		expression := "missing"
		if raw := fields["expression"]; len(raw) > 0 && !bytes.Equal(raw, []byte("null")) {
			var object map[string]json.RawMessage
			if json.Unmarshal(raw, &object) != nil || object == nil {
				expression = "malformed"
			} else {
				expression = "present"
			}
		}
		fmt.Fprintf(out, "Commercial rule shape[%d]: id=%s; kind=%s; narrative=%s; evidence=%s; expression=%s\n", index, stringShape(fields["id"]), kind, stringShape(fields["narrative"]), evidence, expression)
	}
}

func stringShape(raw json.RawMessage) string {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "missing"
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return "malformed"
	}
	if strings.TrimSpace(value) == "" {
		return "missing"
	}
	return "present"
}
