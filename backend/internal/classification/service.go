package classification

import (
	"context"
	"fmt"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accounts"
	"diana-contabilitate/backend/internal/apperrors"
)

type Store interface {
	ProcessCommandCommitted(context.Context, string) (bool, error)
	LoadClassificationInput(context.Context, string) (InvoiceContext, error)
	ApplyClassification(context.Context, ProcessCommand, Result, time.Time) (bool, error)
	ReviewClassification(context.Context, ReviewCommand, time.Time) (bool, error)
}

func (s *Service) SearchAccounts(ctx context.Context, query string, limit int) ([]accounts.Account, error) {
	store, ok := s.store.(accounts.Store)
	if !ok {
		return nil, fmt.Errorf("account catalog unavailable")
	}
	return accounts.NewService(store).Search(ctx, query, limit)
}

type Clock func() time.Time

type Service struct {
	store  Store
	policy Policy
	clock  Clock
}

func NewService(store Store, policy Policy, clock Clock) *Service {
	if policy == nil {
		policy = ProductionPolicy{}
	}
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &Service{store: store, policy: policy, clock: clock}
}

func (s *Service) ProcessInvoice(ctx context.Context, command ProcessCommand) (Result, bool, error) {
	if command.InvoiceID == "" || command.ExpectedRevision == 0 || command.CommandID == "" {
		return Result{}, false, apperrors.ErrValidation
	}
	committed, err := s.store.ProcessCommandCommitted(ctx, command.CommandID)
	if err != nil || committed {
		return Result{}, false, err
	}
	input, err := s.store.LoadClassificationInput(ctx, command.InvoiceID)
	if err != nil {
		return Result{}, false, err
	}
	if input.PipelineStatus != "COMMERCIALLY_VALIDATED" || input.Revision != command.ExpectedRevision {
		return Result{}, false, apperrors.ErrConflict
	}
	policy := s.policy
	if input.ModelVersion == accounting.ModelVersion {
		if _, ok := policy.(DomainPolicy); !ok {
			domain := DomainPolicy{}
			if old, ok := policy.(ProductionPolicy); ok {
				domain.Observer = old.Observer
			}
			policy = domain
		}
	}
	result, err := policy.Evaluate(input)
	if err != nil {
		return Result{}, false, err
	}
	result = applyLearnedAccountMappings(input, result)
	if err = validateResult(input, result, policy.Version()); err != nil {
		return Result{}, false, err
	}
	changed, err := s.store.ApplyClassification(ctx, command, result, s.clock())
	return result, changed, err
}

func (s *Service) ProcessClassification(ctx context.Context, invoiceID string, revision uint64, commandID, correlationID string) error {
	_, _, err := s.ProcessInvoice(ctx, ProcessCommand{InvoiceID: invoiceID, ExpectedRevision: revision, CommandID: commandID, CorrelationID: correlationID})
	return err
}

func (s *Service) Review(ctx context.Context, command ReviewCommand) (bool, error) {
	if command.InvoiceID == "" || command.TaskID == "" || command.ClassificationID == "" || command.ExpectedInvoiceRevision == 0 || command.ExpectedTaskRevision == 0 || command.ExpectedClassificationRevision == 0 || command.CommandID == "" || command.ActorDisplay == "" {
		return false, apperrors.ErrValidation
	}
	if command.CorrectedValue != nil {
		value := strings.TrimSpace(*command.CorrectedValue)
		if value == "" {
			return false, apperrors.ErrValidation
		}
		command.CorrectedValue = &value
	}
	if command.MappingAction == "" {
		command.MappingAction = "NONE"
	}
	switch command.MappingAction {
	case "NONE", "CREATE", "VALIDATE", "OCCURRENCE_ONLY", "CORRECT", "POLICY_CHANGE":
	default:
		return false, apperrors.ErrValidation
	}
	if command.MappingAction == "POLICY_CHANGE" && strings.TrimSpace(command.Reason) == "" {
		return false, apperrors.ErrValidation
	}
	return s.store.ReviewClassification(ctx, command, s.clock())
}

func applyLearnedAccountMappings(input InvoiceContext, result Result) Result {
	lines := make(map[string]LineContext, len(input.Lines))
	for _, line := range input.Lines {
		lines[line.ID] = line
	}
	for i := range result.Proposals {
		proposal := &result.Proposals[i]
		if proposal.Dimension != DimensionAccount {
			continue
		}
		line, ok := lines[proposal.InvoiceLineID]
		if !ok {
			continue
		}
		matches := make([]MappingCandidate, 0, 3)
		accounts := map[string]bool{}
		for _, identity := range serviceIdentities(line) {
			for _, candidate := range input.Mappings {
				// A mapping is historical evidence, not a permanent guarantee that its
				// current account remains selectable. PostgreSQL supplies the active,
				// postable catalogue in the same read snapshot used for mapping lookup.
				if input.SelectableAccounts != nil && !input.SelectableAccounts[candidate.AccountCode] {
					continue
				}
				if candidate.Status == "ACTIVE" && candidate.ServiceIdentityKind == string(identity.Kind) && candidate.ServiceIdentityValue == identity.Value && candidate.NormalizerVersion == identity.NormalizerVersion {
					matches = append(matches, candidate)
					accounts[candidate.AccountCode] = true
				}
			}
		}
		if len(matches) == 0 {
			continue
		}
		if len(accounts) > 1 {
			proposal.TypedValue = nil
			proposal.ProposedValue = "Necesită decizie"
			proposal.RequiresReview = true
			proposal.Source = SourceAmbiguous
			proposal.Mapping = nil
			proposal.Confidence = "Identități exacte contradictorii"
			proposal.Explanation = "Identitățile exacte disponibile indică mapări active către conturi diferite; Diana nu a ales arbitrar."
			continue
		}
		matched := matches[0]
		learned := accounting.Value{Kind: "ACCOUNT", Account: matched.AccountCode}
		if proposal.Source == SourceRule && proposal.TypedValue != nil && proposal.TypedValue.Kind == "ACCOUNT" && proposal.TypedValue.Account != matched.AccountCode {
			proposal.TypedValue = nil
			proposal.ProposedValue = "Necesită decizie"
			proposal.RequiresReview = true
			proposal.Source = SourceAmbiguous
			proposal.Mapping = &matched.MappingReference
			proposal.Confidence = "Conflict între regulă și maparea confirmată"
			proposal.Explanation = "Regula deterministă și maparea confirmată propun conturi diferite; este necesară decizia contabilului."
			continue
		}
		proposal.TypedValue = &learned
		proposal.ProposedValue = matched.AccountCode
		proposal.RequiresReview = true
		proposal.Source = SourceLearnedMapping
		proposal.Mapping = &matched.MappingReference
		proposal.Confidence = "Mapare contabilă confirmată anterior"
		proposal.Explanation = fmt.Sprintf("Maparea confirmată de contabil, versiunea %d, pentru aceeași identitate exactă propune contul %s.", matched.Version, matched.AccountCode)
		proposal.LegalBasis = "Confirmare contabilă anterioară; aplicabilitatea curentă necesită validare umană."
	}
	return result
}

func validateResult(input InvoiceContext, result Result, version string) error {
	dimensions := Dimensions
	if result.ModelVersion == accounting.ModelVersion {
		dimensions = nil
		for _, d := range accounting.Dimensions {
			dimensions = append(dimensions, Dimension(d))
		}
	}
	if result.PolicyVersion != version || len(result.Proposals) != len(input.Lines)*len(dimensions) {
		return fmt.Errorf("%w: coverage", ErrInvalidPolicyResult)
	}
	seen := make(map[string]bool, len(result.Proposals))
	lines := make(map[string]bool, len(input.Lines))
	for _, line := range input.Lines {
		lines[line.ID] = true
	}
	for _, proposal := range result.Proposals {
		key := proposal.InvoiceLineID + ":" + string(proposal.Dimension)
		if !lines[proposal.InvoiceLineID] || seen[key] || strings.TrimSpace(proposal.ProposedValue) == "" || proposal.Confidence == "" || proposal.Explanation == "" || proposal.LegalBasis == "" {
			return fmt.Errorf("%w: proposal", ErrInvalidPolicyResult)
		}
		if result.ModelVersion == accounting.ModelVersion && (proposal.ModelVersion != accounting.ModelVersion || !proposal.RequiresReview && (proposal.TypedValue == nil || proposal.TypedValue.Validate(string(proposal.Dimension)) != nil || proposal.Evidence == nil || proposal.Rule == nil)) {
			return fmt.Errorf("%w: typed decision", ErrInvalidPolicyResult)
		}
		seen[key] = true
		validDimension := false
		for _, dimension := range dimensions {
			validDimension = validDimension || proposal.Dimension == dimension
		}
		if !validDimension || (proposal.Source == SourceRule && proposal.Rule == nil) || (proposal.Source == SourceLearnedMapping && proposal.Mapping == nil) {
			return fmt.Errorf("%w: evidence", ErrInvalidPolicyResult)
		}
	}
	return nil
}
