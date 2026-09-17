package contracts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
)

type MatchCommand struct {
	InvoiceID        string
	ExpectedRevision uint64
	CommandID        string
	CorrelationID    string
}

type ConfirmCommand struct {
	InvoiceID               string
	TaskID                  string
	ContractID              string
	ExpectedInvoiceRevision uint64
	ExpectedTaskRevision    uint64
	CommandID               string
	ActorID                 string
	ActorDisplay            string
	CorrelationID           string
}

type AvailableCommand struct {
	ContractID    string
	CommandID     string
	CorrelationID string
}

type ResumeCommand struct {
	InvoiceID         string
	ExpectedRevision  uint64
	TriggerContractID string
	CommandID         string
	CorrelationID     string
}

type Store interface {
	ListContracts(context.Context, Filter) ([]Contract, error)
	GetContract(context.Context, string) (*Contract, error)
	ListContractInvoices(context.Context, string) ([]AssociatedInvoice, error)
	MatchCommandCommitted(context.Context, string) (bool, error)
	LoadMatchingInput(context.Context, string) (InvoiceContext, []Contract, error)
	ApplyMatchDecision(context.Context, MatchCommand, MatchDecision, time.Time) (bool, error)
	ConfirmContractMatch(context.Context, ConfirmCommand, time.Time) (bool, error)
	RecordContractAvailable(context.Context, AvailableCommand, time.Time) (bool, error)
	ListBlockedInvoicesForContract(context.Context, string, string, int) ([]BlockedInvoice, error)
	ResumeCommandCommitted(context.Context, string) (bool, error)
	ApplyResumeDecision(context.Context, ResumeCommand, MatchDecision, time.Time) (bool, error)
}

type Clock func() time.Time

type Service struct {
	store  Store
	policy MatchingPolicy
	clock  Clock
}

func NewService(store Store, policy MatchingPolicy, clock Clock) *Service {
	if policy == nil {
		policy = BaselinePolicy{}
	}
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &Service{store: store, policy: policy, clock: clock}
}

func (s *Service) List(ctx context.Context, filter Filter) ([]Contract, error) {
	return s.store.ListContracts(ctx, filter)
}

func (s *Service) Get(ctx context.Context, id string) (*Contract, error) {
	if id == "" {
		return nil, apperrors.ErrValidation
	}
	return s.store.GetContract(ctx, id)
}

func (s *Service) ListInvoices(ctx context.Context, contractID string) ([]AssociatedInvoice, error) {
	if contractID == "" {
		return nil, apperrors.ErrValidation
	}
	return s.store.ListContractInvoices(ctx, contractID)
}

func (s *Service) MatchInvoice(ctx context.Context, command MatchCommand) (MatchDecision, bool, error) {
	if command.InvoiceID == "" || command.ExpectedRevision == 0 || command.CommandID == "" {
		return MatchDecision{}, false, apperrors.ErrValidation
	}
	committed, err := s.store.MatchCommandCommitted(ctx, command.CommandID)
	if err != nil {
		return MatchDecision{}, false, err
	}
	if committed {
		return MatchDecision{}, false, nil
	}
	invoice, candidates, err := s.store.LoadMatchingInput(ctx, command.InvoiceID)
	if err != nil {
		return MatchDecision{}, false, err
	}
	if invoice.PipelineStatus != "MATCHING" || invoice.Revision != command.ExpectedRevision {
		return MatchDecision{}, false, apperrors.ErrConflict
	}
	decision, err := s.policy.Evaluate(invoice, candidates)
	if err != nil {
		return MatchDecision{}, false, err
	}
	if err = validateMatchDecision(decision, s.policy.Version()); err != nil {
		return MatchDecision{}, false, err
	}
	changed, err := s.store.ApplyMatchDecision(ctx, command, decision, s.clock())
	return decision, changed, err
}

func validateMatchDecision(decision MatchDecision, expectedPolicyVersion string) error {
	if decision.PolicyVersion == "" || decision.PolicyVersion != expectedPolicyVersion {
		return fmt.Errorf("%w: policy version", ErrInvalidMatchDecision)
	}
	wantCandidates := -1
	switch decision.Outcome {
	case OutcomeNoMatch:
		wantCandidates = 0
	case OutcomeUniqueCompatible, OutcomeUniqueIncompatible:
		wantCandidates = 1
	case OutcomeMultiplePlausible:
		if len(decision.Candidates) < 2 {
			return fmt.Errorf("%w: multiple outcome requires at least two candidates", ErrInvalidMatchDecision)
		}
	default:
		return fmt.Errorf("%w: outcome", ErrInvalidMatchDecision)
	}
	if wantCandidates >= 0 && len(decision.Candidates) != wantCandidates {
		return fmt.Errorf("%w: candidate count", ErrInvalidMatchDecision)
	}
	recommended := 0
	seen := make(map[string]struct{}, len(decision.Candidates))
	for index, candidate := range decision.Candidates {
		if candidate.ContractID == "" || candidate.ContractRevision == 0 || candidate.Rank != index+1 || candidate.Confidence == "" || len(candidate.Reasons) == 0 {
			return fmt.Errorf("%w: candidate evidence", ErrInvalidMatchDecision)
		}
		if _, exists := seen[candidate.ContractID]; exists {
			return fmt.Errorf("%w: duplicate candidate", ErrInvalidMatchDecision)
		}
		seen[candidate.ContractID] = struct{}{}
		if candidate.Compatibility != Compatible && candidate.Compatibility != Incompatible {
			return fmt.Errorf("%w: candidate compatibility", ErrInvalidMatchDecision)
		}
		if candidate.Recommended {
			recommended++
		}
	}
	if len(decision.Candidates) > 0 && recommended != 1 {
		return fmt.Errorf("%w: recommendation", ErrInvalidMatchDecision)
	}
	if decision.Outcome == OutcomeUniqueCompatible && decision.Candidates[0].Compatibility != Compatible {
		return fmt.Errorf("%w: unique-compatible evidence", ErrInvalidMatchDecision)
	}
	if decision.Outcome == OutcomeUniqueIncompatible && decision.Candidates[0].Compatibility != Incompatible {
		return fmt.Errorf("%w: unique-incompatible evidence", ErrInvalidMatchDecision)
	}
	return nil
}

func (s *Service) ProcessContractMatching(ctx context.Context, invoiceID string, revision uint64, commandID, correlationID string) error {
	_, _, err := s.MatchInvoice(ctx, MatchCommand{InvoiceID: invoiceID, ExpectedRevision: revision, CommandID: commandID, CorrelationID: correlationID})
	return err
}

func (s *Service) Confirm(ctx context.Context, command ConfirmCommand) (bool, error) {
	if command.InvoiceID == "" || command.TaskID == "" || command.ContractID == "" || command.ExpectedInvoiceRevision == 0 || command.ExpectedTaskRevision == 0 || command.CommandID == "" {
		return false, apperrors.ErrValidation
	}
	return s.store.ConfirmContractMatch(ctx, command, s.clock())
}

// ContractAvailable is the durable integration point for future contract
// ingestion. It is called only after the Contract row exists and records a
// domain-specific outbox event; ingestion never manipulates invoices or tasks.
func (s *Service) ContractAvailable(ctx context.Context, command AvailableCommand) (bool, error) {
	if command.ContractID == "" || command.CommandID == "" {
		return false, apperrors.ErrValidation
	}
	return s.store.RecordContractAvailable(ctx, command, s.clock())
}

// ResumeBlockedInvoice is the narrow recovery/test boundary for one invoice.
// Normal product flow invokes it through ProcessContractAvailable.
func (s *Service) ResumeBlockedInvoice(ctx context.Context, command ResumeCommand) (MatchDecision, ResumeOutcome, error) {
	if command.InvoiceID == "" || command.ExpectedRevision == 0 || command.CommandID == "" {
		return MatchDecision{}, ResumeStaleNoop, apperrors.ErrValidation
	}
	committed, err := s.store.ResumeCommandCommitted(ctx, command.CommandID)
	if err != nil {
		return MatchDecision{}, ResumeStaleNoop, err
	}
	if committed {
		return MatchDecision{}, ResumeStaleNoop, nil
	}
	input, candidates, err := s.store.LoadMatchingInput(ctx, command.InvoiceID)
	if err != nil {
		return MatchDecision{}, ResumeStaleNoop, err
	}
	if input.PipelineStatus != "AWAITING_CONTRACT" || input.Revision != command.ExpectedRevision {
		return MatchDecision{}, ResumeStaleNoop, nil
	}
	decision, err := s.policy.Evaluate(input, candidates)
	if err != nil {
		return MatchDecision{}, ResumeStaleNoop, err
	}
	if err = validateMatchDecision(decision, s.policy.Version()); err != nil {
		return MatchDecision{}, ResumeStaleNoop, err
	}
	changed, err := s.store.ApplyResumeDecision(ctx, command, decision, s.clock())
	if err != nil {
		if errors.Is(err, apperrors.ErrConflict) {
			return decision, ResumeStaleNoop, nil
		}
		return decision, ResumeStaleNoop, err
	}
	if !changed {
		return decision, ResumeStaleNoop, nil
	}
	switch decision.Outcome {
	case OutcomeUniqueCompatible:
		return decision, ResumeAutomaticallyAssociated, nil
	case OutcomeMultiplePlausible, OutcomeUniqueIncompatible:
		return decision, ResumeNeedsConfirmation, nil
	default:
		return decision, ResumeStillMissing, nil
	}
}

const contractResumeBatchSize = 100

// ProcessContractAvailable performs bounded, cursor-based fan-out. Each
// invoice is resumed in its own transaction, so one failure cannot roll back
// successful siblings; retrying the event safely revisits only stale/idempotent
// work.
func (s *Service) ProcessContractAvailable(ctx context.Context, contractID, eventKey, correlationID string) (ResumeSummary, error) {
	if contractID == "" || eventKey == "" {
		return ResumeSummary{}, apperrors.ErrValidation
	}
	var summary ResumeSummary
	var transientFailures []error
	var permanentFailures []error
	after := ""
	for {
		blocked, err := s.store.ListBlockedInvoicesForContract(ctx, contractID, after, contractResumeBatchSize)
		if err != nil {
			return summary, err
		}
		if len(blocked) == 0 {
			break
		}
		for _, item := range blocked {
			summary.Evaluated++
			_, outcome, resumeErr := s.ResumeBlockedInvoice(ctx, ResumeCommand{
				InvoiceID: item.ID, ExpectedRevision: item.Revision,
				TriggerContractID: contractID,
				CommandID:         eventKey + ":invoice:" + item.ID, CorrelationID: correlationID,
			})
			if resumeErr != nil {
				wrapped := fmt.Errorf("resume invoice %s: %w", item.ID, resumeErr)
				if errors.Is(resumeErr, ErrExpiredContractSemantics) || errors.Is(resumeErr, ErrInvalidMatchDecision) || errors.Is(resumeErr, apperrors.ErrValidation) || errors.Is(resumeErr, apperrors.ErrNotFound) {
					permanentFailures = append(permanentFailures, wrapped)
				} else {
					transientFailures = append(transientFailures, wrapped)
				}
				continue
			}
			switch outcome {
			case ResumeAutomaticallyAssociated:
				summary.AutomaticallyResumed++
			case ResumeNeedsConfirmation:
				summary.ConfirmationRequired++
			case ResumeStillMissing:
				summary.StillMissingContract++
			case ResumeStaleNoop:
				summary.Stale++
			}
		}
		after = blocked[len(blocked)-1].ID
		if len(blocked) < contractResumeBatchSize {
			break
		}
	}
	// A transient sibling must keep the event retryable even when another
	// invoice has a permanent domain outcome in the same fan-out.
	if len(transientFailures) > 0 {
		return summary, errors.Join(transientFailures...)
	}
	return summary, errors.Join(permanentFailures...)
}
