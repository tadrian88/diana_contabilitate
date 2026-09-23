package commercialvalidation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
)

type Service struct {
	store  Store
	engine Engine
	clock  Clock
}

func NewService(store Store, clock Clock) *Service {
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &Service{store: store, clock: clock}
}

func (s *Service) ProcessCommercialValidation(ctx context.Context, invoiceID string, revision uint64, commandID, correlationID string) error {
	input, err := s.store.LoadInput(ctx, invoiceID)
	if err != nil && !errors.Is(err, ErrNoSnapshot) {
		return err
	}
	if errors.Is(err, ErrNoSnapshot) {
		input.Snapshot = Snapshot{Version: 1, SchemaVersion: RuleSchemaVersion, Coverage: CoveragePartial}
	}
	if input.Invoice.Revision != revision {
		return nil
	}
	run := s.engine.Validate(input, s.clock())
	run.ID = stableID("commercial-run", invoiceID+":"+commandID)
	for index := range run.Findings {
		run.Findings[index].ID = stableID("commercial-finding", run.ID+":"+fmt.Sprintf("%d", index))
	}
	_, err = s.store.SaveRun(ctx, run, correlationID)
	return err
}

func (s *Service) Get(ctx context.Context, clientID, invoiceID string) (*Run, error) {
	return s.store.GetRun(ctx, clientID, invoiceID)
}

func (s *Service) Resolve(ctx context.Context, resolution ReviewResolution) (bool, error) {
	if resolution.InvoiceID == "" || resolution.RunID == "" || resolution.CommandID == "" || resolution.ExpectedInvoiceRevision == 0 {
		return false, fmt.Errorf("%w: invalid commercial review resolution", apperrors.ErrValidation)
	}
	if resolution.Action != "ACCEPT_EXCEPTION" && resolution.Action != "WAIT_FOR_CORRECTION" && resolution.Action != "RERUN" {
		return false, fmt.Errorf("%w: invalid commercial review action", apperrors.ErrValidation)
	}
	if resolution.Action == "ACCEPT_EXCEPTION" && resolution.Reason == "" {
		return false, fmt.Errorf("%w: commercial exception reason is required", apperrors.ErrValidation)
	}
	return s.store.Resolve(ctx, resolution, s.clock())
}

func (s *Service) PutVariable(ctx context.Context, clientID, dossierID string, value VariableValue, commandID string) (bool, error) {
	if clientID == "" || dossierID == "" || value.Name == "" || value.Value == "" || value.Source == "" || commandID == "" {
		return false, fmt.Errorf("%w: invalid commercial variable", apperrors.ErrValidation)
	}
	if _, ok := rat(value.Value); !ok || (value.Source != "INVOICE" && value.Source != "INTEGRATION" && value.Source != "MANUAL" && value.Source != "CONTRACT") || strings.TrimSpace(value.SourceReference) == "" || value.PeriodStart != nil && value.PeriodEnd != nil && value.PeriodEnd.Before(*value.PeriodStart) {
		return false, fmt.Errorf("%w: commercial variable requires a decimal value and explicit provenance", apperrors.ErrValidation)
	}
	value.RecordedAt = s.clock()
	return s.store.PutVariable(ctx, clientID, dossierID, value, commandID)
}

func (s *Service) ConfirmAlias(ctx context.Context, alias Alias, commandID string) (bool, error) {
	if alias.ClientID == "" || alias.InvoiceID == "" || alias.LineID == "" || alias.ServiceID == "" || alias.ConfirmedByID == "" || commandID == "" {
		return false, fmt.Errorf("%w: invalid commercial alias", apperrors.ErrValidation)
	}
	alias.ConfirmedAt = s.clock()
	return s.store.ConfirmAlias(ctx, alias, commandID)
}

func (s *Service) ConfirmProposedRule(ctx context.Context, command RuleConfirmation) (bool, error) {
	if command.ClientID == "" || command.DocumentID == "" || command.RuleID == "" || command.CommandID == "" || command.ActorID == "" {
		return false, fmt.Errorf("%w: invalid commercial rule confirmation", apperrors.ErrValidation)
	}
	if command.Rule != nil && (command.Rule.ID != command.RuleID || ValidateRule(*command.Rule) != nil) {
		return false, fmt.Errorf("%w: invalid reviewed commercial rule", apperrors.ErrValidation)
	}
	return s.store.ConfirmProposedRule(ctx, command, s.clock())
}

func (s *Service) ActivateReviewedServicePrices(ctx context.Context, clientID, documentID, actorID, commandID string) (int, error) {
	if clientID == "" || documentID == "" || actorID == "" || commandID == "" {
		return 0, fmt.Errorf("%w: invalid service price activation", apperrors.ErrValidation)
	}
	return s.store.ActivateReviewedServicePrices(ctx, clientID, documentID, actorID, commandID, s.clock())
}

func (s *Service) PutInvoiceDateFact(ctx context.Context, fact InvoiceDateFact) (bool, error) {
	if fact.ClientID == "" || fact.InvoiceID == "" || fact.ActorID == "" || fact.CommandID == "" || strings.TrimSpace(fact.SourceReference) == "" || fact.Date.IsZero() ||
		(fact.Kind != "REMITTANCE" && fact.Kind != "RECEIPT" && fact.Kind != "ACCEPTANCE") {
		return false, fmt.Errorf("%w: invalid invoice commercial date", apperrors.ErrValidation)
	}
	return s.store.PutInvoiceDateFact(ctx, fact, s.clock())
}

func (s *Service) PreviewRevalidation(ctx context.Context, clientID, snapshotID string) ([]RevalidationCandidate, error) {
	if clientID == "" || snapshotID == "" {
		return nil, fmt.Errorf("%w: snapshot id is required", apperrors.ErrValidation)
	}
	return s.store.PreviewRevalidation(ctx, clientID, snapshotID)
}

func (s *Service) CreateDossier(ctx context.Context, dossier Dossier, commandID string) (Dossier, bool, error) {
	if dossier.ClientID == "" || dossier.SupplierCUI == "" || dossier.BuyerCUI == "" || dossier.PrimaryReference == "" || commandID == "" {
		return Dossier{}, false, fmt.Errorf("%w: invalid contract dossier", apperrors.ErrValidation)
	}
	dossier.ID = stableID("dossier-draft", dossier.ClientID+":"+commandID)
	dossier.Status = "DRAFT"
	dossier.Revision = 1
	dossier.CreatedAt = s.clock()
	dossier.UpdatedAt = dossier.CreatedAt
	return s.store.CreateDossier(ctx, dossier, commandID)
}

func (s *Service) ListDossiers(ctx context.Context, clientID string) ([]Dossier, error) {
	if clientID == "" {
		return nil, fmt.Errorf("%w: client id is required", apperrors.ErrValidation)
	}
	return s.store.ListDossiers(ctx, clientID)
}

func (s *Service) GetDossier(ctx context.Context, clientID, dossierID string) (Dossier, error) {
	if clientID == "" || dossierID == "" {
		return Dossier{}, fmt.Errorf("%w: dossier identity is required", apperrors.ErrValidation)
	}
	return s.store.GetDossier(ctx, clientID, dossierID)
}

func stableID(prefix, value string) string {
	// Store IDs only need deterministic idempotency; the database still owns
	// uniqueness and authorization boundaries.
	var hash uint64 = 1469598103934665603
	for index := 0; index < len(value); index++ {
		hash ^= uint64(value[index])
		hash *= 1099511628211
	}
	return fmt.Sprintf("%s-%016x", prefix, hash)
}
