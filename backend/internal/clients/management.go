package clients

import (
	"context"
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/platform/requestactor"
	"fmt"
	"strings"
	"time"
)

var validationTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

type Command struct {
	ClientID               string              `json:"-"`
	Company                Company             `json:"company"`
	ExpectedRevision       uint64              `json:"expectedRevision"`
	Status                 Lifecycle           `json:"status"`
	Profile                *accounting.Profile `json:"profile,omitempty"`
	Approve                bool                `json:"approve"`
	Evidence               []string            `json:"evidence"`
	ExpectedProfileVersion int                 `json:"expectedProfileVersion"`
	SagaEnabled            bool                `json:"sagaEnabled"`
	CommandID              string              `json:"-"`
	Actor                  requestactor.Actor  `json:"-"`
	CorrelationID          string              `json:"-"`
}
type History struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Actor     string `json:"actor"`
	Timestamp string `json:"timestamp"`
	Detail    string `json:"detail"`
}
type Detail struct {
	Client      Client                `json:"client"`
	Profiles    []*accounting.Profile `json:"profiles"`
	SagaEnabled bool                  `json:"sagaEnabled"`
	History     []History             `json:"history"`
	Onboarding  Readiness             `json:"onboarding"`
}
type Manager interface {
	GetClientDetail(context.Context, string) (Detail, error)
	ExecuteClientCommand(context.Context, string, Command) (Detail, error)
}

func Allows(a requestactor.Actor, id string) bool { return a.AllowsClient(id) }

func Authorize(ctx context.Context, id string) error {
	a, ok := requestactor.FromContext(ctx)
	if !ok || !Allows(a, id) {
		return apperrors.ErrNotFound
	}
	return nil
}
func (s *Service) Detail(ctx context.Context, id string) (Detail, error) {
	if err := Authorize(ctx, id); err != nil {
		return Detail{}, err
	}
	m, ok := s.reader.(Manager)
	if !ok {
		return Detail{}, apperrors.ErrNotFound
	}
	return m.GetClientDetail(ctx, id)
}
func (s *Service) Command(ctx context.Context, kind string, c Command) (Detail, error) {
	a, ok := requestactor.FromContext(ctx)
	if !ok || a.ID == "" || (kind == "create" && !a.AllClients) || (kind != "create" && !Allows(a, c.ClientID)) {
		return Detail{}, apperrors.ErrNotFound
	}
	if strings.TrimSpace(c.CommandID) == "" || len(c.CommandID) > 200 {
		return Detail{}, fmt.Errorf("%w: Idempotency-Key obligatoriu", apperrors.ErrValidation)
	}
	c.Actor = a
	switch kind {
	case "create":
		if _, err := c.Company.Normalize(); err != nil {
			return Detail{}, err
		}
	case "update":
		before, err := s.Detail(ctx, c.ClientID)
		if err != nil {
			return Detail{}, err
		}
		if _, err = c.Company.NormalizeExisting(before.Client); err != nil {
			return Detail{}, err
		}
	case "lifecycle":
		if c.Status != Onboarding && c.Status != Active && c.Status != Inactive {
			return Detail{}, apperrors.ErrValidation
		}
	case "profile":
		if c.Profile == nil || c.Profile.TestOnly || ValidateProfile(c.Profile) != nil {
			return Detail{}, apperrors.ErrValidation
		}
		if c.Approve && (len(c.Evidence) == 0 || strings.TrimSpace(strings.Join(c.Evidence, "")) == "") {
			return Detail{}, apperrors.ErrValidation
		}
	case "saga":
	default:
		return Detail{}, apperrors.ErrValidation
	}
	m, ok := s.reader.(Manager)
	if !ok {
		return Detail{}, apperrors.ErrNotFound
	}
	return m.ExecuteClientCommand(ctx, kind, c)
}

// Validate against the current V2 model using an ephemeral approval; it is never persisted.
func ValidateProfile(p *accounting.Profile) error {
	copy := *p
	copy.ID = "validation"
	copy.ClientID = "validation"
	copy.Version = 1
	copy.Approval = accounting.Approval{Actor: "validation", At: validationTime, Evidence: []string{"validation"}}
	if !copy.Valid(copy.ClientID, copy.EffectiveFrom) || (copy.EffectiveTo != nil && (!copy.EffectiveTo.Valid() || *copy.EffectiveTo < copy.EffectiveFrom)) {
		return apperrors.ErrValidation
	}
	return nil
}
