package accountinganalysis

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"diana-contabilitate/backend/internal/legislation"
)

type Observer interface {
	AccountingAnalysisCompleted(duration time.Duration, inputTokens, outputTokens int64, validationFailures int)
}

type Service struct {
	legislation legislation.Store
	analyzer    Analyzer
	accounts    AccountCatalog
	observer    Observer
}

func NewService(corpus legislation.Store, analyzer Analyzer, accounts AccountCatalog, observer Observer) *Service {
	return &Service{legislation: corpus, analyzer: analyzer, accounts: accounts, observer: observer}
}

// Analyze performs bounded retrieval and a single structured provider call.
// Persistence/retry/idempotency belong to the workflow adapter, while this
// boundary remains deterministic and straightforward to evaluate.
func (s *Service) Analyze(ctx context.Context, input Input, approved []ApprovedKnowledge) (ProviderResult, []ValidationIssue, error) {
	if s.legislation == nil || s.analyzer == nil || s.accounts == nil || input.ClientID == "" || input.InvoiceID == "" || input.InvoiceRevision == 0 || !input.IssueDate.Valid() || input.Profile == nil || !input.Profile.Valid(input.ClientID, input.IssueDate) || len(input.Lines) == 0 {
		return ProviderResult{}, nil, fmt.Errorf("invalid accounting analysis input")
	}
	for _, knowledge := range approved {
		if knowledge.ClientID != input.ClientID {
			return ProviderResult{}, nil, fmt.Errorf("tenant-isolation violation in approved knowledge")
		}
	}
	fragments, err := s.legislation.Retrieve(ctx, legislation.Query{Terms: retrievalTerms(input), ApplicableDate: input.IssueDate, Limit: 12})
	if err != nil {
		return ProviderResult{}, nil, err
	}
	started := time.Now()
	result, err := s.analyzer.Analyze(ctx, AnalysisRequest{Input: input, Fragments: fragments, ApprovedKnowledge: approved})
	if err != nil {
		return ProviderResult{}, nil, err
	}
	issues := Validate(input, result.Proposal, fragments, s.accounts)
	if s.observer != nil {
		var in, out int64
		if result.InputTokens != nil {
			in = *result.InputTokens
		}
		if result.OutputTokens != nil {
			out = *result.OutputTokens
		}
		s.observer.AccountingAnalysisCompleted(time.Since(started), in, out, len(issues))
	}
	return result, issues, nil
}

func retrievalTerms(input Input) []string {
	seen := map[string]bool{}
	result := make([]string, 0, 32)
	add := func(value string) {
		if !utf8.ValidString(value) {
			return
		}
		for _, word := range strings.Fields(strings.ToLower(value)) {
			word = strings.Trim(word, ".,;:!?()[]{}\"'")
			if len([]rune(word)) < 3 || seen[word] {
				continue
			}
			seen[word] = true
			result = append(result, word)
			if len(result) == 48 {
				return
			}
		}
	}
	add("contabilitate TVA deductibilitate cheltuieli")
	add(input.SupplierName)
	for _, line := range input.Lines {
		if len(result) >= 48 {
			break
		}
		add(line.Description)
		if line.Facts != nil {
			add(line.Facts.ItemName)
			add(line.Facts.ItemDescription)
		}
	}
	add(input.Profile.TaxRegime)
	add(input.Profile.VATRegistration)
	add(input.Profile.CashAccounting)
	return result
}
