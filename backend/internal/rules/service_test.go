package rules

import (
	"context"
	"testing"
	"time"
)

type captureStore struct {
	version  CreateVersionCommand
	override CreateOverrideCommand
}

func (*captureStore) ListRules(context.Context, Filter) ([]Rule, error) { return nil, nil }
func (*captureStore) GetRule(context.Context, string) (*Rule, error)    { return nil, nil }
func (s *captureStore) CreateRuleVersion(_ context.Context, command CreateVersionCommand, _ time.Time) (*Rule, bool, error) {
	s.version = command
	return &Rule{ID: command.RuleID}, true, nil
}
func (s *captureStore) CreateClientOverride(_ context.Context, command CreateOverrideCommand, _ time.Time) (*Rule, bool, error) {
	s.override = command
	return &Rule{ID: "override"}, true, nil
}

func TestRuleCommandsPreserveNarrowVersionAndOverrideBoundaries(t *testing.T) {
	store := &captureStore{}
	service := NewService(store, nil)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, _, err := service.CreateVersion(context.Background(), CreateVersionCommand{RuleID: "rule", ExpectedRevision: 1, Criteria: " demo ", Result: " value ", EffectiveFrom: from, CommandID: "v", ActorDisplay: "Contabil"}); err != nil {
		t.Fatal(err)
	}
	if store.version.Criteria != "demo" || store.version.Result != "value" {
		t.Fatalf("version=%+v", store.version)
	}
	if _, _, err := service.CreateOverride(context.Background(), CreateOverrideCommand{ParentRuleID: "rule", ClientID: "client", Criteria: "demo", Result: "value", EffectiveFrom: from, CommandID: "o", ActorDisplay: "Contabil"}); err != nil {
		t.Fatal(err)
	}
}

func TestRulePeriodUsesCivilDatesInsteadOfInstants(t *testing.T) {
	from := time.Date(2025, 8, 1, 23, 0, 0, 0, time.FixedZone("RO", 3*3600))
	to := time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC)
	if invalidPeriod(from, &to) {
		t.Fatal("same civil day rejected by instant ordering")
	}
	normalized, end := normalizePeriod(from, &to)
	if normalized.Location() != time.UTC || normalized.Hour() != 0 || !normalized.Equal(*end) {
		t.Fatal("date persistence was not normalized")
	}
}
