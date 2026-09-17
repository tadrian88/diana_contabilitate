package rules

import (
	"context"
	"errors"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/apperrors"
)

var ErrRuleVersionStale = errors.New("rule version is stale")
var ErrOverrideAlreadyExists = errors.New("client override already exists")

type CreateVersionCommand struct {
	RuleID           string
	ExpectedRevision uint64
	Criteria         string
	Result           string
	EffectiveFrom    time.Time
	EffectiveTo      *time.Time
	CommandID        string
	ActorID          string
	ActorDisplay     string
	CorrelationID    string
}

type CreateOverrideCommand struct {
	ParentRuleID  string
	ClientID      string
	Criteria      string
	Result        string
	EffectiveFrom time.Time
	EffectiveTo   *time.Time
	CommandID     string
	ActorID       string
	ActorDisplay  string
	CorrelationID string
}

type Store interface {
	ListRules(context.Context, Filter) ([]Rule, error)
	GetRule(context.Context, string) (*Rule, error)
	CreateRuleVersion(context.Context, CreateVersionCommand, time.Time) (*Rule, bool, error)
	CreateClientOverride(context.Context, CreateOverrideCommand, time.Time) (*Rule, bool, error)
}

type Clock func() time.Time

type Service struct {
	store Store
	clock Clock
}

func NewService(store Store, clock Clock) *Service {
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &Service{store: store, clock: clock}
}

func (s *Service) List(ctx context.Context, filter Filter) ([]Rule, error) {
	return s.store.ListRules(ctx, filter)
}

func (s *Service) Get(ctx context.Context, id string) (*Rule, error) {
	if strings.TrimSpace(id) == "" {
		return nil, apperrors.ErrValidation
	}
	return s.store.GetRule(ctx, id)
}

func (s *Service) CreateVersion(ctx context.Context, command CreateVersionCommand) (*Rule, bool, error) {
	if command.RuleID == "" || command.ExpectedRevision == 0 || strings.TrimSpace(command.Criteria) == "" || strings.TrimSpace(command.Result) == "" || command.EffectiveFrom.IsZero() || command.CommandID == "" || command.ActorDisplay == "" || invalidPeriod(command.EffectiveFrom, command.EffectiveTo) {
		return nil, false, apperrors.ErrValidation
	}
	command.EffectiveFrom, command.EffectiveTo = normalizePeriod(command.EffectiveFrom, command.EffectiveTo)
	command.Criteria = strings.TrimSpace(command.Criteria)
	command.Result = strings.TrimSpace(command.Result)
	return s.store.CreateRuleVersion(ctx, command, s.clock())
}

func (s *Service) CreateOverride(ctx context.Context, command CreateOverrideCommand) (*Rule, bool, error) {
	if command.ParentRuleID == "" || command.ClientID == "" || strings.TrimSpace(command.Criteria) == "" || strings.TrimSpace(command.Result) == "" || command.EffectiveFrom.IsZero() || command.CommandID == "" || command.ActorDisplay == "" || invalidPeriod(command.EffectiveFrom, command.EffectiveTo) {
		return nil, false, apperrors.ErrValidation
	}
	command.EffectiveFrom, command.EffectiveTo = normalizePeriod(command.EffectiveFrom, command.EffectiveTo)
	command.Criteria = strings.TrimSpace(command.Criteria)
	command.Result = strings.TrimSpace(command.Result)
	return s.store.CreateClientOverride(ctx, command, s.clock())
}

func invalidPeriod(from time.Time, to *time.Time) bool {
	date := accountingdate.FromTime(from)
	if !date.Valid() {
		return true
	}
	return to != nil && (!accountingdate.FromTime(*to).Valid() || accountingdate.FromTime(*to) < date)
}
func normalizePeriod(from time.Time, to *time.Time) (time.Time, *time.Time) {
	normalized, _ := time.Parse("2006-01-02", string(accountingdate.FromTime(from)))
	if to == nil {
		return normalized, nil
	}
	end, _ := time.Parse("2006-01-02", string(accountingdate.FromTime(*to)))
	return normalized, &end
}
