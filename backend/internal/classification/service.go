package classification

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accounts"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/money"
)

type Store interface {
	ProcessCommandCommitted(context.Context, string) (bool, error)
	LoadClassificationInput(context.Context, string) (InvoiceContext, error)
	ApplyClassification(context.Context, ProcessCommand, Result, time.Time) (bool, error)
	ReviewClassification(context.Context, ReviewCommand, time.Time) (bool, error)
}

type ApproveAllStore interface {
	ApproveAllClassifications(context.Context, ApproveAllCommand, time.Time) (bool, error)
}

type BlockingStore interface {
	ApplyClassificationBlock(context.Context, ProcessCommand, *accounting.Snapshot, string, string, time.Time) (bool, error)
}

type ReanalysisStore interface {
	PrepareClassificationReanalysis(context.Context, ReanalysisCommand, time.Time) (uint64, bool, error)
}

type AutomaticAccountingFallback interface {
	EnsureAutomatic(context.Context, string, string, string) error
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
	store       Store
	policy      Policy
	clock       Clock
	automaticAI AutomaticAccountingFallback
}

func (s *Service) SetAutomaticAccountingFallback(fallback AutomaticAccountingFallback) {
	s.automaticAI = fallback
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
	if err != nil {
		return Result{}, false, err
	}
	if committed {
		if s.automaticAI != nil {
			input, loadErr := s.store.LoadClassificationInput(ctx, command.InvoiceID)
			if loadErr == nil {
				loadErr = s.automaticAI.EnsureAutomatic(ctx, input.ClientID, input.ID, "classification:"+command.CommandID)
			}
			return Result{}, false, loadErr
		}
		return Result{}, false, nil
	}
	input, err := s.store.LoadClassificationInput(ctx, command.InvoiceID)
	if err != nil {
		return Result{}, false, err
	}
	if input.PipelineStatus != "COMMERCIALLY_VALIDATED" || input.Revision != command.ExpectedRevision {
		return Result{}, false, apperrors.ErrConflict
	}
	if input.ModelVersion == accounting.ModelVersion && (input.ContextBlocker != "" || input.Snapshot == nil || input.Snapshot.Profile == nil) {
		store, ok := s.store.(BlockingStore)
		if !ok {
			return Result{}, false, fmt.Errorf("%w: MISSING_FISCAL_PROFILE", apperrors.ErrValidation)
		}
		code, message := input.ContextBlocker, input.ContextBlockerMessage
		if code == "" {
			code, message = "MISSING_FISCAL_PROFILE", "Lipsește profilul fiscal aplicabil facturii."
		}
		changed, blockErr := store.ApplyClassificationBlock(ctx, command, input.Snapshot, code, message, s.clock())
		return Result{ModelVersion: accounting.ModelVersion, Snapshot: input.Snapshot}, changed, blockErr
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
	result = applyApprovedKnowledge(input, result)
	result.DeferReviewForAI = s.automaticAI != nil && result.NeedsAI()
	if err = validateResult(input, result, policy.Version()); err != nil {
		return Result{}, false, err
	}
	changed, err := s.store.ApplyClassification(ctx, command, result, s.clock())
	if err != nil || !result.DeferReviewForAI {
		return result, changed, err
	}
	if err = s.automaticAI.EnsureAutomatic(ctx, input.ClientID, input.ID, "classification:"+command.CommandID); err != nil {
		return result, changed, err
	}
	return result, changed, nil
}

func applyApprovedKnowledge(input InvoiceContext, result Result) Result {
	lines := make(map[string]LineContext, len(input.Lines))
	for _, line := range input.Lines {
		lines[line.ID] = line
	}
	for i := range result.Proposals {
		proposal := &result.Proposals[i]
		// Verified deterministic rules retain precedence. ACCOUNT remains owned by
		// the existing exact account_mappings adapter.
		if proposal.Dimension == DimensionAccount || proposal.Source == SourceRule {
			continue
		}
		line, ok := lines[proposal.InvoiceLineID]
		if !ok {
			continue
		}
		identity, hasIdentity := PreferredServiceIdentity(line)
		if !hasIdentity {
			continue
		}
		matches := []KnowledgeCandidate{}
		values := map[string]bool{}
		for _, candidate := range input.Knowledge {
			candidateRate, rateErr := money.Parse(candidate.VATRate)
			if candidate.Status != "ACTIVE" || candidate.Dimension != proposal.Dimension || candidate.NormalizedSupplierID != input.NormalizedSupplierID || candidate.Currency != input.Currency || candidate.DocumentType != input.DocumentType || rateErr != nil || !candidateRate.Equal(line.VATRate) {
				continue
			}
			if input.Snapshot == nil || input.Snapshot.Profile == nil || candidate.ProfileID != input.Snapshot.Profile.ID || candidate.ProfileVersion != input.Snapshot.Profile.Version {
				continue
			}
			if candidate.ServiceIdentityKind != string(identity.Kind) || candidate.ServiceIdentityValue != identity.Value || candidate.NormalizerVersion != identity.NormalizerVersion {
				continue
			}
			matches = append(matches, candidate)
			raw, _ := json.Marshal(candidate.Value)
			values[string(raw)] = true
		}
		if len(matches) == 0 {
			continue
		}
		if len(values) > 1 {
			proposal.TypedValue = nil
			proposal.ProposedValue = "Necesită decizie"
			proposal.RequiresReview = true
			proposal.Source = SourceAmbiguous
			proposal.KnowledgeConflict = true
			proposal.Confidence = "Decizii aprobate anterior contradictorii"
			proposal.Explanation = "Mai multe decizii reutilizabile cu același scope exact indică valori diferite; Diana nu a ales arbitrar."
			proposal.LegalBasis = "Conflict de knowledge păstrat pentru revizuire contabilă."
			continue
		}
		matched := matches[0]
		value := matched.Value
		proposal.TypedValue = &value
		proposal.ProposedValue = value.Text()
		proposal.RequiresReview = true
		proposal.Source = SourceLearnedMapping
		proposal.Knowledge = &KnowledgeReference{ID: matched.ID, Version: matched.Version, SourceInvoiceID: matched.SourceInvoiceID, SourceInvoiceLineID: matched.SourceInvoiceLineID, SourceClassificationID: matched.SourceClassificationID, PromotedBy: matched.PromotedBy, PromotedAt: matched.PromotedAt}
		proposal.Confidence = "Decizie contabilă aprobată anterior"
		proposal.Explanation = "O decizie promovată explicit pentru aceeași identitate exactă și același context contabil propune această valoare."
		proposal.LegalBasis = "Decizie aprobată anterior; aplicabilitatea curentă necesită validare umană."
	}
	return result
}

func (s *Service) ProcessClassification(ctx context.Context, invoiceID string, revision uint64, commandID, correlationID string) error {
	_, _, err := s.ProcessInvoice(ctx, ProcessCommand{InvoiceID: invoiceID, ExpectedRevision: revision, CommandID: commandID, CorrelationID: correlationID})
	return err
}

func (s *Service) Reanalyze(ctx context.Context, command ReanalysisCommand) (bool, error) {
	if command.InvoiceID == "" || command.ClientID == "" || command.CommandID == "" || command.ExpectedRevision == 0 || command.ActorID == "" || command.ActorDisplay == "" {
		return false, apperrors.ErrValidation
	}
	store, ok := s.store.(ReanalysisStore)
	if !ok {
		return false, fmt.Errorf("classification reanalysis unavailable")
	}
	revision, prepared, err := store.PrepareClassificationReanalysis(ctx, command, s.clock())
	if err != nil {
		return false, err
	}
	_, classified, err := s.ProcessInvoice(ctx, ProcessCommand{InvoiceID: command.InvoiceID, ExpectedRevision: revision, CommandID: command.CommandID + ":run", CorrelationID: command.CorrelationID})
	return prepared || classified, err
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
	if command.Action == "" {
		command.Action = "APPROVE"
	}
	if command.Action != "APPROVE" && command.Action != "EDIT" && command.Action != "REJECT" {
		return false, apperrors.ErrValidation
	}
	if command.Action == "REJECT" && (command.TypedValue != nil || command.CorrectedValue != nil || strings.TrimSpace(command.Reason) == "") {
		return false, apperrors.ErrValidation
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

func (s *Service) ApproveAll(ctx context.Context, command ApproveAllCommand) (bool, error) {
	if command.InvoiceID == "" || command.TaskID == "" || command.ExpectedInvoiceRevision == 0 || command.ExpectedTaskRevision == 0 || len(command.Expected) == 0 || command.CommandID == "" || command.ActorDisplay == "" {
		return false, apperrors.ErrValidation
	}
	seen := map[string]bool{}
	for _, item := range command.Expected {
		if item.ID == "" || item.Revision == 0 || seen[item.ID] {
			return false, apperrors.ErrValidation
		}
		seen[item.ID] = true
	}
	store, ok := s.store.(ApproveAllStore)
	if !ok {
		return false, fmt.Errorf("approve all unavailable")
	}
	return store.ApproveAllClassifications(ctx, command, s.clock())
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
				if input.SelectableAccounts != nil && !input.SelectableAccounts[candidate.AccountCode] || input.Snapshot != nil && input.Snapshot.Profile != nil && !input.Snapshot.Profile.AccountAllowed(candidate.AccountCode) {
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
		if result.ModelVersion == accounting.ModelVersion && (proposal.ModelVersion != accounting.ModelVersion || !proposal.RequiresReview && (proposal.TypedValue == nil || proposal.TypedValue.Validate(string(proposal.Dimension)) != nil || proposal.Evidence == nil || proposal.Rule == nil && !validProfileDerived(input, proposal))) {
			return fmt.Errorf("%w: typed decision", ErrInvalidPolicyResult)
		}
		if proposal.Dimension == DimensionAccount && proposal.TypedValue != nil {
			code := proposal.TypedValue.Account
			if input.SelectableAccounts == nil || !input.SelectableAccounts[code] || input.Snapshot == nil || input.Snapshot.Profile == nil || !input.Snapshot.Profile.AccountAllowed(code) {
				return fmt.Errorf("%w: account is not active, postable and allowed by profile", ErrInvalidPolicyResult)
			}
		}
		seen[key] = true
		validDimension := false
		for _, dimension := range dimensions {
			validDimension = validDimension || proposal.Dimension == dimension
		}
		if !validDimension || (proposal.Source == SourceRule && proposal.Rule == nil) || (proposal.Source == SourceProfile && !validProfileDerived(input, proposal)) || (proposal.Source == SourceLearnedMapping && proposal.Mapping == nil && proposal.Knowledge == nil) {
			return fmt.Errorf("%w: evidence", ErrInvalidPolicyResult)
		}
	}
	return nil
}

// validProfileDerived accepts an automatic decision without a rule only when it
// is exactly the value the approved snapshot profile implies.
func validProfileDerived(input InvoiceContext, proposal Proposal) bool {
	if proposal.Source != SourceProfile || proposal.Rule != nil || proposal.Mapping != nil || proposal.Knowledge != nil || proposal.TypedValue == nil || proposal.Evidence == nil || input.Snapshot == nil || input.Snapshot.Profile == nil {
		return false
	}
	profile := input.Snapshot.Profile
	expected, ok := profile.ProfileDerivedExpenseTaxValue()
	return ok && string(proposal.Dimension) == "EXPENSE_TAX_TREATMENT" && proposal.TypedValue.Kind == expected.Kind && proposal.TypedValue.Reason == expected.Reason && proposal.Evidence.ProfileID == profile.ID && proposal.Evidence.ProfileVersion == profile.Version
}
