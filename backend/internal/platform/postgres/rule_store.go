package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"diana-contabilitate/backend/ent"
	"diana-contabilitate/backend/ent/classificationrule"
	"diana-contabilitate/backend/ent/ruleversion"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/audit"
	"diana-contabilitate/backend/internal/rules"
)

func (s *Store) ListRules(ctx context.Context, filter rules.Filter) ([]rules.Rule, error) {
	query := s.Client.ClassificationRule.Query().WithVersions(func(query *ent.RuleVersionQuery) {
		query.Order(ent.Asc(ruleversion.FieldVersion))
	})
	if filter.ClientID != "" {
		query.Where(classificationrule.Or(
			classificationrule.ScopeEQ(classificationrule.ScopeGLOBAL),
			classificationrule.And(classificationrule.ScopeEQ(classificationrule.ScopeCLIENT_OVERRIDE), classificationrule.ClientIDEQ(filter.ClientID)),
		))
	}
	rows, err := query.Order(ent.Asc(classificationrule.FieldReference)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	result := make([]rules.Rule, 0, len(rows))
	for _, row := range rows {
		result = append(result, ruleDomain(row))
	}
	return result, nil
}

func (s *Store) GetRule(ctx context.Context, id string) (*rules.Rule, error) {
	row, err := s.Client.ClassificationRule.Query().Where(classificationrule.IDEQ(id)).WithVersions(func(query *ent.RuleVersionQuery) {
		query.Order(ent.Asc(ruleversion.FieldVersion))
	}).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	result := ruleDomain(row)
	return &result, nil
}

func (s *Store) CreateRuleVersion(ctx context.Context, command rules.CreateVersionCommand, now time.Time) (*rules.Rule, bool, error) {
	commandKey := "rule-version:" + command.CommandID
	if row, err := s.Client.RuleVersion.Query().Where(ruleversion.CommandKeyEQ(commandKey)).Only(ctx); err == nil {
		item, getErr := s.GetRule(ctx, row.RuleID)
		return item, false, getErr
	} else if !ent.IsNotFound(err) {
		return nil, false, err
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return nil, false, err
	}
	rollback := func(cause error) (*rules.Rule, bool, error) { _ = tx.Rollback(); return nil, false, cause }
	ruleRow, err := tx.ClassificationRule.UpdateOneID(command.RuleID).Where(classificationrule.RevisionEQ(command.ExpectedRevision)).AddRevision(1).SetUpdatedAt(now).Save(ctx)
	if ent.IsNotFound(err) {
		_ = tx.Rollback()
		if replay, replayErr := s.Client.RuleVersion.Query().Where(ruleversion.CommandKeyEQ(commandKey)).Only(ctx); replayErr == nil {
			item, getErr := s.GetRule(ctx, replay.RuleID)
			return item, false, getErr
		}
		exists, lookupErr := s.Client.ClassificationRule.Query().Where(classificationrule.IDEQ(command.RuleID)).Exist(ctx)
		if lookupErr != nil {
			return nil, false, lookupErr
		}
		if !exists {
			return nil, false, apperrors.ErrNotFound
		}
		return nil, false, rules.ErrRuleVersionStale
	}
	if err != nil {
		return rollback(err)
	}
	latest, err := tx.RuleVersion.Query().Where(ruleversion.RuleIDEQ(ruleRow.ID)).Order(ent.Desc(ruleversion.FieldVersion)).First(ctx)
	if err != nil {
		return rollback(err)
	}
	version := latest.Version + 1
	create := tx.RuleVersion.Create().SetID(stableID("rv", commandKey)).SetRuleID(ruleRow.ID).SetVersion(version).
		SetCriteria(command.Criteria).SetResult(command.Result).
		SetExplanation("Versiune demonstrativă creată manual; criteriul afișat nu este interpretat ca regulă contabilă executabilă.").
		SetLegalBasis(rules.LegalBasisPlaceholder).SetMatchKind(ruleversion.MatchKindNO_AUTOMATION).
		SetEffectiveFrom(command.EffectiveFrom).SetCreatedByDisplay(command.ActorDisplay).SetCommandKey(commandKey).SetCreatedAt(now)
	if command.EffectiveTo != nil {
		create.SetEffectiveTo(*command.EffectiveTo)
	}
	if command.ActorID != "" {
		create.SetCreatedByID(command.ActorID)
	}
	if _, err = create.Save(ctx); err != nil {
		if IsConstraintError(err) {
			return rollback(rules.ErrRuleVersionStale)
		}
		return rollback(err)
	}
	if err = createAudit(tx, ctx, auditRecord{key: commandKey, clientID: pointerValue(ruleRow.ClientID), aggregateType: "CLASSIFICATION_RULE", aggregateID: ruleRow.ID, eventType: "RULE_VERSION_CREATED", from: fmt.Sprintf("v%d", latest.Version), to: fmt.Sprintf("v%d", version), trigger: "RULE_VERSION_CREATED", detail: fmt.Sprintf("Immutable demonstrative rule version %d created; no historical invoice was reclassified.", version), actor: audit.ActorUser, actorID: command.ActorID, actorDisplay: command.ActorDisplay, correlationID: command.CorrelationID, at: now}); err != nil {
		return rollback(err)
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	item, err := s.GetRule(ctx, ruleRow.ID)
	return item, true, err
}

func (s *Store) CreateClientOverride(ctx context.Context, command rules.CreateOverrideCommand, now time.Time) (*rules.Rule, bool, error) {
	creationKey := "rule-override:" + command.CommandID
	if row, err := s.Client.ClassificationRule.Query().Where(classificationrule.CreationKeyEQ(creationKey)).Only(ctx); err == nil {
		item, getErr := s.GetRule(ctx, row.ID)
		return item, false, getErr
	} else if !ent.IsNotFound(err) {
		return nil, false, err
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return nil, false, err
	}
	rollback := func(cause error) (*rules.Rule, bool, error) { _ = tx.Rollback(); return nil, false, cause }
	parent, err := tx.ClassificationRule.Query().Where(classificationrule.IDEQ(command.ParentRuleID), classificationrule.ScopeEQ(classificationrule.ScopeGLOBAL)).Only(ctx)
	if ent.IsNotFound(err) {
		return rollback(apperrors.ErrValidation)
	}
	if err != nil {
		return rollback(err)
	}
	if exists, queryErr := tx.ClassificationRule.Query().Where(classificationrule.ParentRuleIDEQ(parent.ID), classificationrule.ClientIDEQ(command.ClientID)).Exist(ctx); queryErr != nil {
		return rollback(queryErr)
	} else if exists {
		return rollback(rules.ErrOverrideAlreadyExists)
	}
	id := stableID("rule", parent.ID+":"+command.ClientID)
	reference := parent.Reference + "-OVR-" + strings.ToUpper(strings.ReplaceAll(command.ClientID, "client-", ""))
	created, err := tx.ClassificationRule.Create().SetID(id).SetReference(reference).SetName(parent.Name + " — variație client").
		SetCategory(classificationrule.Category(parent.Category)).SetScope(classificationrule.ScopeCLIENT_OVERRIDE).
		SetClientID(command.ClientID).SetParentRuleID(parent.ID).SetParentScope(classificationrule.ParentScopeGLOBAL).
		SetCreationKey(creationKey).SetRevision(1).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		if IsConstraintError(err) {
			_ = tx.Rollback()
			if replay, replayErr := s.Client.ClassificationRule.Query().Where(classificationrule.CreationKeyEQ(creationKey)).Only(ctx); replayErr == nil {
				item, getErr := s.GetRule(ctx, replay.ID)
				return item, false, getErr
			}
			return nil, false, rules.ErrOverrideAlreadyExists
		}
		return rollback(err)
	}
	versionKey := creationKey + ":v1"
	versionCreate := tx.RuleVersion.Create().SetID(stableID("rv", versionKey)).SetRuleID(created.ID).SetVersion(1).
		SetCriteria(command.Criteria).SetResult(command.Result).
		SetExplanation("Override demonstrativ creat manual; criteriul afișat nu este interpretat ca regulă contabilă executabilă.").
		SetLegalBasis(rules.LegalBasisPlaceholder).SetMatchKind(ruleversion.MatchKindNO_AUTOMATION).
		SetEffectiveFrom(command.EffectiveFrom).SetCreatedByDisplay(command.ActorDisplay).SetCommandKey(versionKey).SetCreatedAt(now)
	if command.EffectiveTo != nil {
		versionCreate.SetEffectiveTo(*command.EffectiveTo)
	}
	if command.ActorID != "" {
		versionCreate.SetCreatedByID(command.ActorID)
	}
	if _, err = versionCreate.Save(ctx); err != nil {
		return rollback(err)
	}
	if err = createAudit(tx, ctx, auditRecord{key: creationKey, clientID: command.ClientID, aggregateType: "CLASSIFICATION_RULE", aggregateID: created.ID, eventType: "CLIENT_OVERRIDE_CREATED", to: created.ID, trigger: "CLIENT_OVERRIDE_CREATED", detail: fmt.Sprintf("Client override created from global rule %s; global rule remains unchanged.", parent.Reference), actor: audit.ActorUser, actorID: command.ActorID, actorDisplay: command.ActorDisplay, correlationID: command.CorrelationID, at: now}); err != nil {
		return rollback(err)
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	item, err := s.GetRule(ctx, created.ID)
	return item, true, err
}

func ruleDomain(row *ent.ClassificationRule) rules.Rule {
	result := rules.Rule{ID: row.ID, Reference: row.Reference, Name: row.Name, Category: rules.Category(row.Category), Scope: rules.Scope(row.Scope), ClientID: row.ClientID, ParentRuleID: row.ParentRuleID, Revision: row.Revision, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	for _, version := range row.Edges.Versions {
		result.Versions = append(result.Versions, rules.Version{ProductionEligible: version.ProductionEligible, RulePackVersion: version.RulePackVersion, Provenance: version.Provenance, ID: version.ID, RuleID: version.RuleID, Version: version.Version, Criteria: version.Criteria, Result: version.Result, Explanation: version.Explanation, LegalBasis: version.LegalBasis, MatchKind: rules.MatchKind(version.MatchKind), MatchValue: version.MatchValue, EffectiveFrom: version.EffectiveFrom, EffectiveTo: version.EffectiveTo, CreatedByID: version.CreatedByID, CreatedByDisplay: version.CreatedByDisplay, CreatedAt: version.CreatedAt})
	}
	return result
}

func pointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
