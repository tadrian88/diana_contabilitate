package classification

import (
	"context"
	"errors"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/apperrors"
)

var ErrKnowledgeDuplicate = errors.New("reusable knowledge already exists")
var ErrKnowledgeConflict = errors.New("conflicting reusable knowledge exists")

type KnowledgeScope struct {
	ClientID             string `json:"clientId"`
	ClientDisplay        string `json:"clientDisplay"`
	SupplierDisplay      string `json:"supplierDisplay"`
	NormalizedSupplierID string `json:"normalizedSupplierId"`
	ServiceIdentityKind  string `json:"serviceIdentityKind"`
	ServiceIdentityValue string `json:"serviceIdentityValue"`
	NormalizerVersion    string `json:"normalizerVersion"`
	Currency             string `json:"currency"`
	DocumentType         string `json:"documentType"`
	VATRate              string `json:"vatRate"`
	ProfileID            string `json:"profileId,omitempty"`
	ProfileVersion       int    `json:"profileVersion,omitempty"`
}

type KnowledgeItem struct {
	ID                     string           `json:"id"`
	Version                int              `json:"version"`
	Dimension              Dimension        `json:"dimension"`
	Value                  accounting.Value `json:"value"`
	Scope                  KnowledgeScope   `json:"scope"`
	Status                 string           `json:"status"`
	StaleReason            string           `json:"staleReason,omitempty"`
	SourceInvoiceID        string           `json:"sourceInvoiceId"`
	SourceInvoiceLineID    string           `json:"sourceInvoiceLineId"`
	SourceClassificationID string           `json:"sourceClassificationId"`
	SourceRunID            string           `json:"sourceClassificationRunId"`
	OriginalSource         string           `json:"originalSource"`
	PromotedBy             string           `json:"promotedBy"`
	PromotedAt             time.Time        `json:"promotedAt"`
	Revision               uint64           `json:"revision"`
	LegislationVersionIDs  []string         `json:"legislationVersionIds,omitempty"`
}

type LegislationSourceView struct {
	ID            string  `json:"id"`
	Kind          string  `json:"kind"`
	Title         string  `json:"title"`
	Issuer        string  `json:"issuer"`
	OfficialURL   string  `json:"officialUrl"`
	VersionID     string  `json:"versionId"`
	VersionLabel  string  `json:"versionLabel"`
	EffectiveFrom string  `json:"effectiveFrom"`
	EffectiveTo   *string `json:"effectiveTo,omitempty"`
	Status        string  `json:"status"`
	FragmentCount int     `json:"fragmentCount"`
}

type PromotionPreview struct {
	ClassificationID       string           `json:"classificationId"`
	ClassificationRevision uint64           `json:"classificationRevision"`
	ClassificationRunID    string           `json:"classificationRunId"`
	Dimension              Dimension        `json:"dimension"`
	Value                  accounting.Value `json:"value"`
	Scope                  KnowledgeScope   `json:"scope"`
}

type PromoteKnowledgeCommand struct {
	ClientID                       string
	InvoiceID                      string
	ClassificationID               string
	ExpectedClassificationRevision uint64
	ExpectedInvoiceRevision        uint64
	ExpectedClassificationRunID    string
	CommandID                      string
	ActorID                        string
	ActorDisplay                   string
	CorrelationID                  string
}

type RevokeKnowledgeCommand struct {
	ClientID, KnowledgeID, CommandID, ActorID, ActorDisplay, CorrelationID string
	ExpectedRevision                                                       uint64
}

type KnowledgeStore interface {
	ListApprovedKnowledge(context.Context, string) ([]KnowledgeItem, error)
	PreviewApprovedKnowledge(context.Context, string, string, string) (*PromotionPreview, error)
	PromoteApprovedKnowledge(context.Context, PromoteKnowledgeCommand, time.Time) (*KnowledgeItem, bool, error)
	RevokeApprovedKnowledge(context.Context, RevokeKnowledgeCommand, time.Time) (*KnowledgeItem, bool, error)
	ListLegislationSources(context.Context) ([]LegislationSourceView, error)
}

func (s *Service) ListLegislationSources(ctx context.Context) ([]LegislationSourceView, error) {
	store, ok := s.store.(KnowledgeStore)
	if !ok {
		return nil, errors.New("legislation sources unavailable")
	}
	return store.ListLegislationSources(ctx)
}

func (s *Service) ListKnowledge(ctx context.Context, clientID string) ([]KnowledgeItem, error) {
	store, ok := s.store.(KnowledgeStore)
	if !ok {
		return nil, errors.New("approved knowledge unavailable")
	}
	return store.ListApprovedKnowledge(ctx, strings.TrimSpace(clientID))
}
func (s *Service) GetKnowledge(ctx context.Context, clientID, id string) (*KnowledgeItem, error) {
	items, err := s.ListKnowledge(ctx, clientID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].ID == id {
			return &items[i], nil
		}
	}
	return nil, apperrors.ErrNotFound
}
func (s *Service) PreviewKnowledge(ctx context.Context, clientID, invoiceID, classificationID string) (*PromotionPreview, error) {
	if strings.TrimSpace(clientID) == "" || strings.TrimSpace(invoiceID) == "" || strings.TrimSpace(classificationID) == "" {
		return nil, apperrors.ErrValidation
	}
	store, ok := s.store.(KnowledgeStore)
	if !ok {
		return nil, errors.New("approved knowledge unavailable")
	}
	return store.PreviewApprovedKnowledge(ctx, clientID, invoiceID, classificationID)
}
func (s *Service) PromoteKnowledge(ctx context.Context, command PromoteKnowledgeCommand) (*KnowledgeItem, bool, error) {
	if command.ClientID == "" || command.InvoiceID == "" || command.ClassificationID == "" || command.ExpectedClassificationRevision == 0 || command.ExpectedInvoiceRevision == 0 || command.ExpectedClassificationRunID == "" || command.CommandID == "" || strings.TrimSpace(command.ActorDisplay) == "" {
		return nil, false, apperrors.ErrValidation
	}
	store, ok := s.store.(KnowledgeStore)
	if !ok {
		return nil, false, errors.New("approved knowledge unavailable")
	}
	return store.PromoteApprovedKnowledge(ctx, command, s.clock())
}
func (s *Service) RevokeKnowledge(ctx context.Context, command RevokeKnowledgeCommand) (*KnowledgeItem, bool, error) {
	if command.ClientID == "" || command.KnowledgeID == "" || command.ExpectedRevision == 0 || command.CommandID == "" || strings.TrimSpace(command.ActorDisplay) == "" {
		return nil, false, apperrors.ErrValidation
	}
	store, ok := s.store.(KnowledgeStore)
	if !ok {
		return nil, false, errors.New("approved knowledge unavailable")
	}
	return store.RevokeApprovedKnowledge(ctx, command, s.clock())
}
