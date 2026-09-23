package postgres

import (
	"bytes"
	"context"
	"diana-contabilitate/backend/ent"
	"diana-contabilitate/backend/ent/classificationrule"
	"diana-contabilitate/backend/ent/clientaccountingprofile"
	"diana-contabilitate/backend/ent/ruleversion"
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/audit"
	"diana-contabilitate/backend/internal/rules"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ApproveAccountingProfile is an operator-only backend mechanism. It is not
// exposed through public Rule Administration. New IDs/versions are mandatory.
func (s *Store) ApproveAccountingProfile(ctx context.Context, p accounting.Profile) error {
	if p.TestOnly || !p.Valid(p.ClientID, p.EffectiveFrom) {
		return fmt.Errorf("invalid approved accounting profile")
	}
	var selectable int
	if err := s.DB.QueryRowContext(ctx, `
		SELECT count(*)
		FROM accounts
		WHERE code = ANY($1::text[]) AND is_active AND postable`, p.AccountCodes).Scan(&selectable); err != nil {
		return fmt.Errorf("validate approved profile accounts: %w", err)
	}
	if selectable != len(p.AccountCodes) {
		return fmt.Errorf("approved profile contains missing, inactive, or non-postable accounts")
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ClientAccountingProfile.Create().SetID(p.ID).SetClientID(p.ClientID).SetVersion(p.Version).SetPayload(&p).SetCreatedAt(p.Approval.At).Save(ctx); err != nil {
		return err
	}
	if err = createAudit(tx, ctx, auditRecord{key: "accounting-profile:" + p.ID, clientID: p.ClientID, aggregateType: "ACCOUNTING_PROFILE", aggregateID: p.ID, eventType: "ACCOUNTING_PROFILE_APPROVED", trigger: "OPERATOR_APPROVAL", detail: accountingApprovalDetail(p), actor: audit.ActorUser, actorDisplay: p.Approval.Actor, at: p.Approval.At}); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) ReleaseAccountingPack(ctx context.Context, p accounting.Pack) error {
	// Synthetic releases have a separate explicit fixture mechanism, never this CLI.
	if p.TestOnly || p.Mapping.TestOnly {
		return fmt.Errorf("TEST_ONLY pack cannot be released for production")
	}
	profile, err := s.Client.ClientAccountingProfile.Query().Where(clientaccountingprofile.IDEQ(p.ProfileID)).Only(ctx)
	if err != nil {
		return err
	}
	if !profile.Payload.Valid(p.ClientID, p.EffectiveFrom) || !p.Valid(profile.Payload, p.ClientID, p.EffectiveFrom, false) || len(p.Rules) == 0 || !p.Mapping.Approved || !p.Mapping.Approval.Valid() || p.Mapping.Version == "" {
		return fmt.Errorf("invalid reviewed release/profile/mapping")
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range p.Rules {
		// Release only references existing immutable reviewed RuleVersions. Ordinary
		// UI edits are NO_AUTOMATION and have no domain payload/eligibility.
		row, err := tx.RuleVersion.Query().Where(ruleversion.IDEQ(r.VersionID)).WithRule().Only(ctx)
		if err != nil {
			return err
		}
		if accountingdate.FromTime(row.EffectiveFrom) != r.EffectiveFrom || !sameJSON(accountingDatePointer(row.EffectiveTo), r.EffectiveTo) || row.Provenance == nil || row.Provenance.EffectiveFrom != r.EffectiveFrom || !sameJSON(row.Provenance.EffectiveTo, r.EffectiveTo) {
			return fmt.Errorf("release/rule provenance period mismatch")
		}
		if row.ModelVersion != accounting.ModelVersion || row.DomainRule == nil || !row.ProductionEligible || row.RulePackVersion != fmt.Sprintf("%s/%d", p.ID, p.Version) || !row.Provenance.Valid() || row.RuleID != r.ID || row.Version != r.Version || string(row.Edges.Rule.Category) != r.Dimension {
			return fmt.Errorf("rule version not eligible for this release")
		}
		if !sameJSON(row.DomainRule, r) {
			return fmt.Errorf("release differs from immutable rule version")
		}
		if string(row.Edges.Rule.Scope) != r.Scope || r.Scope == "CLIENT_OVERRIDE" && (row.Edges.Rule.ParentRuleID == nil || *row.Edges.Rule.ParentRuleID != r.ParentID) || r.Scope == "CLIENT_OVERRIDE" && (row.Edges.Rule.ClientID == nil || *row.Edges.Rule.ClientID != p.ClientID) {
			return fmt.Errorf("rule client scope mismatch")
		}
	}
	if _, err = tx.AccountingRulePack.Create().SetID(p.ID).SetClientID(p.ClientID).SetVersion(p.Version).SetPayload(&p).SetCreatedAt(p.Approval.At).Save(ctx); err != nil {
		return err
	}
	if err = createAudit(tx, ctx, auditRecord{key: "accounting-pack:" + p.ID, clientID: p.ClientID, aggregateType: "ACCOUNTING_RULE_PACK", aggregateID: p.ID, eventType: "ACCOUNTING_PACK_RELEASED", trigger: "OPERATOR_APPROVAL", detail: accountingApprovalDetail(struct {
		Pack           accounting.Pack `json:"pack"`
		ProfileVersion int             `json:"profileVersion"`
		RuleCount      int             `json:"ruleCount"`
	}{p, profile.Payload.Version, len(p.Rules)}), actor: audit.ActorUser, actorDisplay: p.Approval.Actor, at: p.Approval.At}); err != nil {
		return err
	}
	return tx.Commit()
}

// Reviewed configuration insertion is deliberately not a public promotion endpoint.

func sameJSON(a, b any) bool {
	left, e1 := json.Marshal(a)
	right, e2 := json.Marshal(b)
	return e1 == nil && e2 == nil && bytes.Equal(left, right)
}

// ReviewedAccountingRule is private operator input, separate from public rule
// editing. Profile and acquisition evidence must already have been reviewed.
type ReviewedAccountingRule struct {
	Rule        accounting.Rule     `json:"rule"`
	ClientID    string              `json:"clientId"`
	ProfileID   string              `json:"profileId"`
	PackID      string              `json:"packId"`
	PackVersion int                 `json:"packVersion"`
	Provenance  *rules.Provenance   `json:"provenance"`
	Approval    accounting.Approval `json:"approval"`
}

func (s *Store) InstallReviewedAccountingRule(ctx context.Context, input ReviewedAccountingRule) error {
	r := input.Rule
	if !r.Valid() || !input.Approval.Valid() || !input.Provenance.Valid() || input.ClientID == "" || input.ProfileID == "" || input.PackID == "" || input.PackVersion <= 0 || input.Provenance.EffectiveFrom != r.EffectiveFrom || !sameJSON(input.Provenance.EffectiveTo, r.EffectiveTo) {
		return fmt.Errorf("invalid reviewed domain rule/configuration")
	}
	profile, err := s.Client.ClientAccountingProfile.Get(ctx, input.ProfileID)
	if err != nil {
		return err
	}
	if profile.Payload.TestOnly || !profile.Payload.Valid(input.ClientID, r.EffectiveFrom) || r.ClientPolicy != profile.Payload.ChartPolicy || r.Dimension == "ACCOUNT" && !profile.Payload.AccountAllowed(r.Result.Account) || r.Dimension == "ACCOUNT" && (input.Provenance.AccountingRegime != profile.Payload.Framework || input.Provenance.ClientPolicyReference != r.ClientPolicy) {
		return fmt.Errorf("rule/profile/account policy mismatch")
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	row, err := tx.ClassificationRule.Get(ctx, r.ID)
	if ent.IsNotFound(err) {
		create := tx.ClassificationRule.Create().SetID(r.ID).SetReference(r.ID).SetName(r.Explanation).SetCategory(classificationrule.Category(r.Dimension)).SetScope(classificationrule.Scope(r.Scope)).SetCreationKey("reviewed-accounting:" + r.ID).SetCreatedAt(input.Approval.At).SetUpdatedAt(input.Approval.At)
		if r.Scope == "CLIENT_OVERRIDE" {
			parent, err := tx.ClassificationRule.Get(ctx, r.ParentID)
			if err != nil {
				return err
			}
			if parent.Scope != classificationrule.ScopeGLOBAL || string(parent.Category) != r.Dimension {
				return fmt.Errorf("invalid direct global override")
			}
			create.SetClientID(input.ClientID).SetParentRuleID(parent.ID).SetParentScope(classificationrule.ParentScopeGLOBAL)
		}
		row, err = create.Save(ctx)
	}
	if err != nil {
		return err
	}
	if strings.HasPrefix(row.CreationKey, "seed:") {
		return fmt.Errorf("demo rule identity cannot be promoted into a production domain release")
	}
	if string(row.Category) != r.Dimension || string(row.Scope) != r.Scope || r.Scope == "CLIENT_OVERRIDE" && (row.ClientID == nil || *row.ClientID != input.ClientID || row.ParentRuleID == nil || *row.ParentRuleID != r.ParentID) {
		return fmt.Errorf("immutable rule identity mismatch")
	}
	from, err := time.Parse("2006-01-02", string(r.EffectiveFrom))
	if err != nil {
		return err
	}
	create := tx.RuleVersion.Create().SetID(r.VersionID).SetRuleID(r.ID).SetVersion(r.Version).SetModelVersion(accounting.ModelVersion).SetDomainRule(&r).SetProductionEligible(true).SetRulePackVersion(fmt.Sprintf("%s/%d", input.PackID, input.PackVersion)).SetProvenance(input.Provenance).SetCriteria("ORDINARY_INCOMING_V1; immutable structured predicate in domain_rule").SetResult(r.Result.Text()).SetExplanation(r.Explanation).SetLegalBasis(r.LegalBasis).SetMatchKind(ruleversion.MatchKindNO_AUTOMATION).SetEffectiveFrom(from).SetCreatedByDisplay(input.Approval.Actor).SetCommandKey("reviewed-accounting:" + r.VersionID).SetCreatedAt(input.Approval.At)
	if r.EffectiveTo != nil {
		to, err := time.Parse("2006-01-02", string(*r.EffectiveTo))
		if err != nil {
			return err
		}
		create.SetEffectiveTo(to)
	}
	if _, err := create.Save(ctx); err != nil {
		return err
	}
	if err := createAudit(tx, ctx, auditRecord{key: "reviewed-accounting:" + r.VersionID, clientID: input.ClientID, aggregateType: "CLASSIFICATION_RULE", aggregateID: r.ID, eventType: "ACCOUNTING_RULE_REVIEWED", trigger: "OPERATOR_APPROVAL", detail: accountingApprovalDetail(input), actor: audit.ActorUser, actorDisplay: input.Approval.Actor, at: input.Approval.At}); err != nil {
		return err
	}
	return tx.Commit()
}

// These are restricted audit records, never metric labels or generic logs.
// Native configuration retains exact approval evidence and all referenced rules;
// no invoice XML or commercial payload is included.
func accountingApprovalDetail(configuration any) string {
	raw, err := json.Marshal(configuration)
	if err != nil {
		return "Accounting approval serialization failed"
	}
	return string(raw)
}
