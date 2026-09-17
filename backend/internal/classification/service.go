package classification

import (
	"context"
	"fmt"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/apperrors"
)

type Store interface {
	ProcessCommandCommitted(context.Context, string) (bool, error)
	LoadClassificationInput(context.Context, string) (InvoiceContext, error)
	ApplyClassification(context.Context, ProcessCommand, Result, time.Time) (bool, error)
	ReviewClassification(context.Context, ReviewCommand, time.Time) (bool, error)
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
	if input.PipelineStatus != "LINES_READ" || input.Revision != command.ExpectedRevision {
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
	return s.store.ReviewClassification(ctx, command, s.clock())
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
		if !validDimension || (proposal.Source == SourceRule) != (proposal.Rule != nil) {
			return fmt.Errorf("%w: evidence", ErrInvalidPolicyResult)
		}
	}
	return nil
}
