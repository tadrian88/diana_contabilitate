//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"diana-contabilitate/backend/ent/contract"
	"diana-contabilitate/backend/ent/contractextractionattempt"
	"diana-contabilitate/backend/ent/contractserviceterm"
	"diana-contabilitate/backend/ent/contractsourcedocument"
	"diana-contabilitate/backend/ent/invoice"
	"diana-contabilitate/backend/ent/outboxentry"
	"diana-contabilitate/backend/ent/validationtask"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/commercialvalidation"
	ci "diana-contabilitate/backend/internal/contractingestion"
	"diana-contabilitate/backend/internal/contractingestion/fixtures"
	"diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/outbox"
	"diana-contabilitate/backend/internal/validationtasks"
)

func TestPendingCommercialClausePersistsWithoutExecutablePriceFallback(t *testing.T) {
	tc := newContractTestContext(t)
	service, _, extractor := contractIngestionFixture(t, tc)
	var rule map[string]any
	if err := json.Unmarshal(extractor.proposal.CommercialClauses[0].Rule, &rule); err != nil {
		t.Fatal(err)
	}
	delete(rule, "expression")
	delete(rule, "blocking") // The extractor may omit this internal flag.
	extractor.proposal.CommercialClauses[0].Rule, _ = json.Marshal(rule)
	doc := ingestionUpload(t, tc, service)
	command := ingestionReview(t, tc, service, doc)
	command.Contract.Coverage = commercialvalidation.CoverageComplete // A client cannot promote an unresolved clause.
	contractID, _, err := service.Confirm(tc.ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	var coverage string
	var rulesJSON []byte
	if err = tc.store.DB.QueryRowContext(tc.ctx, `SELECT coverage,rules FROM contract_commercial_snapshots WHERE contract_id=$1`, contractID).Scan(&coverage, &rulesJSON); err != nil {
		t.Fatal(err)
	}
	var rules []commercialvalidation.Rule
	if err = json.Unmarshal(rulesJSON, &rules); err != nil {
		t.Fatal(err)
	}
	if coverage != string(commercialvalidation.CoveragePartial) || len(rules) != 1 || rules[0].Kind != commercialvalidation.RuleContractReference {
		t.Fatalf("unsafe active snapshot coverage=%s rules=%+v", coverage, rules)
	}
	var pendingCount int
	if err = tc.store.DB.QueryRowContext(tc.ctx, `SELECT COUNT(*) FROM contract_clause_candidates WHERE document_id=$1 AND review_status='PROPOSED' AND normalized_rule IS NULL`, doc.ID).Scan(&pendingCount); err != nil {
		t.Fatal(err)
	}
	if pendingCount != 1 {
		t.Fatalf("pending narrative clause count=%d", pendingCount)
	}
	if replayID, changed, replayErr := service.Confirm(tc.ctx, command); replayErr != nil || changed || replayID != contractID {
		t.Fatalf("confirmation replay id=%s changed=%t err=%v", replayID, changed, replayErr)
	}
	var snapshotCount int
	if err = tc.store.DB.QueryRowContext(tc.ctx, `SELECT COUNT(*) FROM contract_commercial_snapshots WHERE contract_id=$1`, contractID).Scan(&snapshotCount); err != nil || snapshotCount != 1 {
		t.Fatalf("replay created another snapshot count=%d err=%v", snapshotCount, err)
	}
	var reviewed commercialvalidation.Rule
	if err = json.Unmarshal(extractor.proposal.CommercialClauses[0].Rule, &reviewed); err != nil {
		t.Fatal(err)
	}
	reviewed.Expression = &commercialvalidation.Expression{Op: "literal", Value: "125000.00", Scale: 2}
	reviewed.Blocking = true
	commercial := commercialvalidation.NewService(tc.store, func() time.Time { return tc.now.Add(time.Minute) })
	changed, err := commercial.ConfirmProposedRule(tc.ctx, commercialvalidation.RuleConfirmation{ClientID: tc.clientID, DocumentID: doc.ID, RuleID: reviewed.ID, Rule: &reviewed, CommandID: "complete-narrative-" + doc.ID, ActorID: "reviewer"})
	if err != nil || !changed {
		t.Fatalf("complete narrative rule changed=%t err=%v", changed, err)
	}
	var latestCoverage string
	if err = tc.store.DB.QueryRowContext(tc.ctx, `SELECT coverage FROM contract_commercial_snapshots WHERE contract_id=$1 ORDER BY version DESC LIMIT 1`, contractID).Scan(&latestCoverage); err != nil || latestCoverage != string(commercialvalidation.CoverageComplete) {
		t.Fatalf("completed rule coverage=%s err=%v", latestCoverage, err)
	}
}

// narrativeOnlyClause turns the fixture's first commercial clause into a
// narrative-only rule of the given kind citing exactly snippet.
func narrativeOnlyClause(t *testing.T, extractor *ingestionIntegrationExtractor, kind commercialvalidation.RuleKind, snippet string) commercialvalidation.Rule {
	t.Helper()
	kindValue := string(kind)
	clause := &extractor.proposal.CommercialClauses[0]
	clause.Kind.Value = &kindValue
	clause.Narrative.Value = &snippet
	clause.Evidence.Snippet = snippet
	var proposed commercialvalidation.Rule
	if err := json.Unmarshal(clause.Rule, &proposed); err != nil {
		t.Fatal(err)
	}
	proposed.Kind = kind
	proposed.Narrative = snippet
	proposed.DateBasis = ""
	proposed.Currency = ""
	proposed.Expression = nil
	proposed.RequiredVariables = nil
	proposed.Blocking = false
	proposed.Evidence = []commercialvalidation.Evidence{{Snippet: snippet}}
	clause.Rule, _ = json.Marshal(proposed)
	return proposed
}

func TestApplicableVATNarrativeIsConfirmedAutomaticallyFromSourceText(t *testing.T) {
	tc := newContractTestContext(t)
	ingestion, _, extractor := contractIngestionFixture(t, tc)
	proposed := narrativeOnlyClause(t, extractor, commercialvalidation.RuleVAT, "Prețurile sunt fără TVA; se adaugă TVA aferent.")
	doc := ingestionUpload(t, tc, ingestion)
	command := ingestionReview(t, tc, ingestion, doc)
	contractID, _, err := ingestion.Confirm(tc.ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	var coverage string
	var variableCount, autoEvents int
	var ruleJSON []byte
	if err = tc.store.DB.QueryRowContext(tc.ctx, `SELECT coverage FROM contract_commercial_snapshots WHERE contract_id=$1 ORDER BY version DESC LIMIT 1`, contractID).Scan(&coverage); err != nil {
		t.Fatal(err)
	}
	if err = tc.store.DB.QueryRowContext(tc.ctx, `SELECT COUNT(*) FROM contract_variable_definitions v JOIN contract_dossiers d ON d.id=v.dossier_id WHERE d.contract_id=$1 AND v.name='applicable_vat_rate'`, contractID).Scan(&variableCount); err != nil {
		t.Fatal(err)
	}
	if err = tc.store.DB.QueryRowContext(tc.ctx, `SELECT normalized_rule FROM contract_clause_candidates WHERE document_id=$1 AND clause_kind='VAT' AND review_status='CONFIRMED'`, doc.ID).Scan(&ruleJSON); err != nil {
		t.Fatal(err)
	}
	if err = tc.store.DB.QueryRowContext(tc.ctx, `SELECT COUNT(*) FROM activity_events WHERE event_type='COMMERCIAL_RULE_AUTO_CONFIRMED' AND automatic AND actor_kind='SYSTEM'`).Scan(&autoEvents); err != nil {
		t.Fatal(err)
	}
	var confirmed commercialvalidation.Rule
	if err = json.Unmarshal(ruleJSON, &confirmed); err != nil {
		t.Fatal(err)
	}
	if coverage != string(commercialvalidation.CoverageComplete) || variableCount != 1 || autoEvents != 1 || confirmed.Origin != commercialvalidation.RuleOriginSourceText || confirmed.Expression == nil || confirmed.Expression.Variable != "applicable_vat_rate" {
		t.Fatalf("coverage=%s variables=%d autoEvents=%d rule=%+v", coverage, variableCount, autoEvents, confirmed)
	}
	// The clause is no longer pending, so a stale manual confirmation cannot
	// replace it; replaying the contract confirmation is a no-op.
	reviewed := proposed
	reviewed.DateBasis = commercialvalidation.DateInvoiceIssue
	reviewed.Expression = &commercialvalidation.Expression{Op: "variable", Variable: "applicable_vat_rate"}
	reviewed.RequiredVariables = []string{"applicable_vat_rate"}
	reviewed.Blocking = true
	commercial := commercialvalidation.NewService(tc.store, func() time.Time { return tc.now.Add(time.Minute) })
	if changed, err := commercial.ConfirmProposedRule(tc.ctx, commercialvalidation.RuleConfirmation{ClientID: tc.clientID, DocumentID: doc.ID, RuleID: reviewed.ID, Rule: &reviewed, CommandID: "confirm-applicable-vat-" + doc.ID, ActorID: "reviewer"}); !errors.Is(err, apperrors.ErrNotFound) || changed {
		t.Fatalf("manual confirmation after automatic changed=%t err=%v", changed, err)
	}
	if _, _, err = ingestion.Confirm(tc.ctx, command); err != nil {
		t.Fatalf("confirmation replay err=%v", err)
	}
	// The source states no percentage, so a fixed-rate revision is refused.
	fixed := reviewed
	fixed.Expression = &commercialvalidation.Expression{Op: "literal", Value: "21", Scale: 2}
	fixed.RequiredVariables = nil
	if _, err = commercial.ConfirmProposedRule(tc.ctx, commercialvalidation.RuleConfirmation{ClientID: tc.clientID, DocumentID: doc.ID, RuleID: fixed.ID, Rule: &fixed, CommandID: "revise-fixed-" + doc.ID, ActorID: "reviewer", Revise: true}); !errors.Is(err, apperrors.ErrValidation) {
		t.Fatalf("unsupported revision err=%v", err)
	}
}

func TestSourceTextRuleCanBeRevisedWithinTheCitedClause(t *testing.T) {
	tc := newContractTestContext(t)
	ingestion, _, extractor := contractIngestionFixture(t, tc)
	proposed := narrativeOnlyClause(t, extractor, commercialvalidation.RuleVAT, "Prețurile nu includ TVA. TVA se aplică în cota legală în vigoare (21% la data semnării).")
	doc := ingestionUpload(t, tc, ingestion)
	contractID, _, err := ingestion.Confirm(tc.ctx, ingestionReview(t, tc, ingestion, doc))
	if err != nil {
		t.Fatal(err)
	}
	var before int
	if err = tc.store.DB.QueryRowContext(tc.ctx, `SELECT MAX(version) FROM contract_commercial_snapshots WHERE contract_id=$1`, contractID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	revised := proposed
	revised.DateBasis = commercialvalidation.DateInvoiceIssue
	revised.Expression = &commercialvalidation.Expression{Op: "literal", Value: "21", Scale: 2}
	revised.Blocking = true
	revised.Origin = commercialvalidation.RuleOriginSourceText // client-supplied provenance is ignored
	commercial := commercialvalidation.NewService(tc.store, func() time.Time { return tc.now.Add(time.Minute) })
	changed, err := commercial.ConfirmProposedRule(tc.ctx, commercialvalidation.RuleConfirmation{ClientID: tc.clientID, DocumentID: doc.ID, RuleID: revised.ID, Rule: &revised, CommandID: "revise-" + doc.ID, ActorID: "reviewer", Revise: true})
	if err != nil || !changed {
		t.Fatalf("revision changed=%t err=%v", changed, err)
	}
	var after int
	var rulesJSON []byte
	if err = tc.store.DB.QueryRowContext(tc.ctx, `SELECT version,rules FROM contract_commercial_snapshots WHERE contract_id=$1 ORDER BY version DESC LIMIT 1`, contractID).Scan(&after, &rulesJSON); err != nil {
		t.Fatal(err)
	}
	var rules []commercialvalidation.Rule
	if err = json.Unmarshal(rulesJSON, &rules); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, rule := range rules {
		if rule.ID == revised.ID {
			found = rule.Expression != nil && rule.Expression.Op == "literal" && rule.Expression.Value == "21" && rule.Origin == "" && len(rule.RequiredVariables) == 0
		}
	}
	if after != before+1 || !found {
		t.Fatalf("revision versions %d->%d rules=%+v", before, after, rules)
	}
	stored, err := tc.store.GetDocument(tc.ctx, tc.clientID, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	shown := false
	for _, rule := range stored.ConfirmedValues.CommercialRules {
		if rule.ID == revised.ID && rule.Expression != nil && rule.Expression.Value == "21" {
			shown = true
		}
	}
	if !shown {
		t.Fatalf("document does not show the revised rule: %+v", stored.ConfirmedValues.CommercialRules)
	}
	if changed, err = commercial.ConfirmProposedRule(tc.ctx, commercialvalidation.RuleConfirmation{ClientID: tc.clientID, DocumentID: doc.ID, RuleID: revised.ID, Rule: &revised, CommandID: "revise-again-" + doc.ID, ActorID: "reviewer", Revise: true}); err != nil || changed {
		t.Fatalf("identical revision changed=%t err=%v", changed, err)
	}
}

func TestPaymentTermIsConfirmedAutomaticallyAndLegalVATRateResolves(t *testing.T) {
	tc := newContractTestContext(t)
	ingestion, _, extractor := contractIngestionFixture(t, tc)
	narrativeOnlyClause(t, extractor, commercialvalidation.RulePaymentDue, "în termen de 10 zile calendaristice de la data emiterii facturii")
	doc := ingestionUpload(t, tc, ingestion)
	if _, _, err := ingestion.Confirm(tc.ctx, ingestionReview(t, tc, ingestion, doc)); err != nil {
		t.Fatal(err)
	}
	var ruleJSON []byte
	if err := tc.store.DB.QueryRowContext(tc.ctx, `SELECT normalized_rule FROM contract_clause_candidates WHERE document_id=$1 AND clause_kind='PAYMENT_DUE' AND review_status='CONFIRMED'`, doc.ID).Scan(&ruleJSON); err != nil {
		t.Fatal(err)
	}
	var rule commercialvalidation.Rule
	if err := json.Unmarshal(ruleJSON, &rule); err != nil {
		t.Fatal(err)
	}
	if rule.DateBasis != commercialvalidation.DateInvoiceIssue || rule.Expression == nil || rule.Expression.Value != "10" || !rule.Blocking {
		t.Fatalf("payment term rule=%+v", rule)
	}

	vat := commercialvalidation.Rule{ID: "vat", Kind: commercialvalidation.RuleVAT, Expression: &commercialvalidation.Expression{Op: "variable", Variable: "applicable_vat_rate"}, RequiredVariables: []string{"applicable_vat_rate"}}
	for day, want := range map[string]string{"2025-07-31": "19", "2025-08-01": "21", "2026-09-29": "21"} {
		issue, _ := time.Parse("2006-01-02", day)
		input := commercialvalidation.Input{Snapshot: commercialvalidation.Snapshot{Rules: []commercialvalidation.Rule{vat}}, Invoice: invoicing.Invoice{IssueDay: issue}, Variables: map[string]commercialvalidation.VariableValue{}, UnavailableVariables: map[string]commercialvalidation.VariableValue{"applicable_vat_rate": {Value: "19"}}}
		if err := tc.store.resolveLegalVATRate(tc.ctx, &input); err != nil {
			t.Fatal(err)
		}
		got := input.Variables["applicable_vat_rate"]
		if got.Value != want || got.Source != "LEGISLATION" || got.SourceReference == "" || len(input.UnavailableVariables) != 0 {
			t.Fatalf("%s: legal VAT=%+v unavailable=%+v", day, got, input.UnavailableVariables)
		}
	}
	recorded := commercialvalidation.Input{Snapshot: commercialvalidation.Snapshot{Rules: []commercialvalidation.Rule{vat}}, Invoice: invoicing.Invoice{IssueDay: tc.now}, Variables: map[string]commercialvalidation.VariableValue{"applicable_vat_rate": {Value: "9", Source: "MANUAL"}}}
	if err := tc.store.resolveLegalVATRate(tc.ctx, &recorded); err != nil || recorded.Variables["applicable_vat_rate"].Value != "9" {
		t.Fatalf("dossier value must win: %+v err=%v", recorded.Variables, err)
	}
}

func TestNormalizedCommercialClauseRequiresHumanSelection(t *testing.T) {
	for _, selected := range []bool{false, true} {
		t.Run(fmt.Sprintf("selected=%t", selected), func(t *testing.T) {
			tc := newContractTestContext(t)
			service, _, extractor := contractIngestionFixture(t, tc)
			var proposed commercialvalidation.Rule
			if err := json.Unmarshal(extractor.proposal.CommercialClauses[0].Rule, &proposed); err != nil {
				t.Fatal(err)
			}
			doc := ingestionUpload(t, tc, service)
			command := ingestionReview(t, tc, service, doc)
			command.Contract.Coverage = commercialvalidation.CoverageComplete
			if selected {
				command.Contract.CommercialRules = []commercialvalidation.Rule{proposed}
			}
			contractID, _, err := service.Confirm(tc.ctx, command)
			if err != nil {
				t.Fatal(err)
			}
			var coverage string
			var rulesJSON []byte
			if err = tc.store.DB.QueryRowContext(tc.ctx, `SELECT coverage,rules FROM contract_commercial_snapshots WHERE contract_id=$1`, contractID).Scan(&coverage, &rulesJSON); err != nil {
				t.Fatal(err)
			}
			var rules []commercialvalidation.Rule
			if err = json.Unmarshal(rulesJSON, &rules); err != nil {
				t.Fatal(err)
			}
			if selected {
				if coverage != string(commercialvalidation.CoverageComplete) || len(rules) != 2 {
					t.Fatalf("confirmed normalized rule coverage=%s rules=%+v", coverage, rules)
				}
			} else {
				if coverage != string(commercialvalidation.CoveragePartial) || len(rules) != 1 || rules[0].Kind != commercialvalidation.RuleContractReference {
					t.Fatalf("unconfirmed normalized rule activated coverage=%s rules=%+v", coverage, rules)
				}
				var proposedCount int
				if err = tc.store.DB.QueryRowContext(tc.ctx, `SELECT COUNT(*) FROM contract_clause_candidates WHERE document_id=$1 AND review_status='PROPOSED' AND normalized_rule IS NOT NULL`, doc.ID).Scan(&proposedCount); err != nil || proposedCount != 1 {
					t.Fatalf("normalized proposal not retained count=%d err=%v", proposedCount, err)
				}
			}
		})
	}
}

func TestExecutableProposalCanBeConfirmedAfterContractConfirmation(t *testing.T) {
	tc := newContractTestContext(t)
	ingestion, _, extractor := contractIngestionFixture(t, tc)
	var proposed commercialvalidation.Rule
	if err := json.Unmarshal(extractor.proposal.CommercialClauses[0].Rule, &proposed); err != nil {
		t.Fatal(err)
	}
	doc := ingestionUpload(t, tc, ingestion)
	command := ingestionReview(t, tc, ingestion, doc)
	command.Contract.Coverage = commercialvalidation.CoverageComplete
	contractID, _, err := ingestion.Confirm(tc.ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	commercial := commercialvalidation.NewService(tc.store, func() time.Time { return tc.now.Add(time.Minute) })
	confirmation := commercialvalidation.RuleConfirmation{ClientID: tc.clientID, DocumentID: doc.ID, RuleID: proposed.ID, CommandID: "confirm-proposed-" + doc.ID, ActorID: "reviewer", ActorDisplay: "Reviewer"}
	changed, err := commercial.ConfirmProposedRule(tc.ctx, confirmation)
	if err != nil || !changed {
		t.Fatalf("confirm proposed changed=%t err=%v", changed, err)
	}
	if replay, replayErr := commercial.ConfirmProposedRule(tc.ctx, confirmation); replayErr != nil || replay {
		t.Fatalf("confirmation replay changed=%t err=%v", replay, replayErr)
	}
	var version int
	var coverage string
	var rulesJSON []byte
	if err = tc.store.DB.QueryRowContext(tc.ctx, `SELECT version,coverage,rules FROM contract_commercial_snapshots WHERE contract_id=$1 ORDER BY version DESC LIMIT 1`, contractID).Scan(&version, &coverage, &rulesJSON); err != nil {
		t.Fatal(err)
	}
	var rules []commercialvalidation.Rule
	if err = json.Unmarshal(rulesJSON, &rules); err != nil {
		t.Fatal(err)
	}
	if version != 2 || coverage != string(commercialvalidation.CoverageComplete) || len(rules) != 2 {
		t.Fatalf("version=%d coverage=%s rules=%+v", version, coverage, rules)
	}
	refreshed, err := ingestion.Get(tc.ctx, tc.clientID, doc.ID, ci.Actor{AllClients: true})
	found := false
	if err == nil && refreshed.ConfirmedValues != nil {
		for _, rule := range refreshed.ConfirmedValues.CommercialRules {
			found = found || rule.ID == proposed.ID
		}
	}
	if err != nil || refreshed.ConfirmedValues == nil || !found {
		t.Fatalf("confirmed document did not expose activated rule: %+v err=%v", refreshed.ConfirmedValues, err)
	}
}

func TestReviewedServicePricesCanBeActivatedForAnExistingConfirmedContract(t *testing.T) {
	tc := newContractTestContext(t)
	ingestion, _, extractor := contractIngestionFixture(t, tc)
	// The older extraction had a price field but no source text that included
	// its currency, so it correctly could not create an executable price rule.
	doc := ingestionUpload(t, tc, ingestion)
	command := ingestionReview(t, tc, ingestion, doc)
	contractID, _, err := ingestion.Confirm(tc.ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	var before int
	if err = tc.store.DB.QueryRowContext(tc.ctx, `SELECT jsonb_array_length(rules) FROM contract_commercial_snapshots WHERE contract_id=$1 ORDER BY version DESC LIMIT 1`, contractID).Scan(&before); err != nil || before != 1 {
		t.Fatalf("unexpected initial rules=%d err=%v", before, err)
	}
	// Model a pre-existing, already-reviewed document whose extraction evidence
	// has been corrected by an extraction retry, without changing the user's
	// reviewed amount. Only the cited source now supports activation.
	extractor.proposal.ServiceTerms[0].UnitPrice.Evidence.Snippet = "Servicii 125000.00 RON"
	proposalJSON, _ := json.Marshal(extractor.proposal)
	if _, err = tc.store.DB.ExecContext(tc.ctx, `UPDATE contract_extraction_attempts SET proposal=$2 WHERE document_id=$1 AND status='SUCCEEDED'`, doc.ID, proposalJSON); err != nil {
		t.Fatal(err)
	}
	commercial := commercialvalidation.NewService(tc.store, func() time.Time { return tc.now.Add(time.Minute) })
	activation, err := commercial.ActivateReviewedServicePrices(tc.ctx, tc.clientID, doc.ID, "reviewer", "activate-prices-"+doc.ID)
	if err != nil || activation.Activated != 1 || len(activation.Skipped) != 0 {
		t.Fatalf("activate reviewed prices result=%+v err=%v", activation, err)
	}
	if activation, err = commercial.ActivateReviewedServicePrices(tc.ctx, tc.clientID, doc.ID, "reviewer", "activate-prices-"+doc.ID); err != nil || activation.Activated != 0 {
		t.Fatalf("idempotent replay result=%+v err=%v", activation, err)
	}
	var version, after int
	if err = tc.store.DB.QueryRowContext(tc.ctx, `SELECT version,jsonb_array_length(rules) FROM contract_commercial_snapshots WHERE contract_id=$1 ORDER BY version DESC LIMIT 1`, contractID).Scan(&version, &after); err != nil || version != 2 || after != 2 {
		t.Fatalf("activated snapshot version=%d rules=%d err=%v", version, after, err)
	}
}

func TestClientIdentityClauseClosesAsCoveredByTheParties(t *testing.T) {
	tc := newContractTestContext(t)
	ingestion, _, extractor := contractIngestionFixture(t, tc)
	narrativeOnlyClause(t, extractor, commercialvalidation.RuleIdentity, "1.2. SOFTCO2 S.R.L., J2024004283403, cod de înregistrare fiscală RO49678244, denumită în continuare BENEFICIAR")
	doc := ingestionUpload(t, tc, ingestion)
	contractID, _, err := ingestion.Confirm(tc.ctx, ingestionReview(t, tc, ingestion, doc))
	if err != nil {
		t.Fatal(err)
	}
	commercial := commercialvalidation.NewService(tc.store, func() time.Time { return tc.now.Add(time.Minute) })
	identity := func() commercialvalidation.DocumentClauseState {
		state, err := commercial.DocumentCommercialState(tc.ctx, tc.clientID, doc.ID)
		if err != nil || state == nil {
			t.Fatalf("commercial state=%+v err=%v", state, err)
		}
		for _, clause := range state.Clauses {
			if clause.RuleID == "fixture-fixed" { // narrativeOnlyClause keeps the fixture rule id.
				return clause
			}
		}
		t.Fatalf("identity clause missing: %+v", state.Clauses)
		return commercialvalidation.DocumentClauseState{}
	}
	if clause := identity(); clause.Status != "PROPOSED" {
		t.Fatalf("a CUI that is neither party's stays in review: %+v", clause)
	}
	// The fixture client's CUI is not numeric; give the dossier the client
	// CUI the clause names, as for a real client, and settle it again.
	if _, err = tc.store.DB.ExecContext(tc.ctx, `UPDATE contract_dossiers SET buyer_cui='RO49678244' WHERE contract_id=$1`, contractID); err != nil {
		t.Fatal(err)
	}
	if _, err = tc.store.CloseCoveredClauses(tc.ctx, tc.clientID, doc.ID, "backfill", "Backfill", tc.now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if clause := identity(); clause.Status != "REJECTED" || clause.ReasonCode != commercialvalidation.ClauseCoveredByPartyIdentity {
		t.Fatalf("the client's own party clause is covered by the parties' CUIs: %+v", clause)
	}
}

func TestCoveredIdentityClosesAndAnotherClauseIsDismissedWithAReason(t *testing.T) {
	tc := newContractTestContext(t)
	ingestion, _, extractor := contractIngestionFixture(t, tc)
	// The supplier's party clause names only its own CUI (the fixture supplier
	// is RO12345678); a loyalty discount has no executable completion.
	narrativeOnlyClause(t, extractor, commercialvalidation.RuleIdentity, "Furnizor extras SRL, J40/123/2020, cod de înregistrare fiscală RO12345678")
	discount := extractor.proposal.CommercialClauses[0]
	discountKind, discountText := string(commercialvalidation.RuleDiscount), "Discountul de fidelitate se negociază anual."
	discount.Kind.Value, discount.Narrative.Value, discount.Evidence.Snippet = &discountKind, &discountText, discountText
	var discountRule commercialvalidation.Rule
	if err := json.Unmarshal(discount.Rule, &discountRule); err != nil {
		t.Fatal(err)
	}
	discountRule.ID, discountRule.Kind, discountRule.Narrative = "fixture-discount", commercialvalidation.RuleDiscount, discountText
	discountRule.Evidence = []commercialvalidation.Evidence{{Snippet: discountText}}
	discount.Rule, _ = json.Marshal(discountRule)
	extractor.proposal.CommercialClauses = append(extractor.proposal.CommercialClauses, discount)

	doc := ingestionUpload(t, tc, ingestion)
	contractID, _, err := ingestion.Confirm(tc.ctx, ingestionReview(t, tc, ingestion, doc))
	if err != nil {
		t.Fatal(err)
	}
	activeCoverage := func() string {
		var coverage string
		if err := tc.store.DB.QueryRowContext(tc.ctx, `SELECT s.coverage FROM contract_dossiers d JOIN contract_commercial_snapshots s ON s.id=d.active_snapshot_id WHERE d.contract_id=$1`, contractID).Scan(&coverage); err != nil {
			t.Fatal(err)
		}
		return coverage
	}
	commercial := commercialvalidation.NewService(tc.store, func() time.Time { return tc.now.Add(time.Minute) })
	state, err := commercial.DocumentCommercialState(tc.ctx, tc.clientID, doc.ID)
	if err != nil || state == nil {
		t.Fatalf("commercial state=%+v err=%v", state, err)
	}
	clauses := map[string]commercialvalidation.DocumentClauseState{}
	for _, clause := range state.Clauses {
		clauses[clause.RuleID] = clause
	}
	identityID := "fixture-fixed" // narrativeOnlyClause keeps the fixture rule id.
	if clauses[identityID].Status != "REJECTED" || clauses[identityID].ReasonCode != commercialvalidation.ClauseCoveredBySupplierIdentity || clauses["fixture-discount"].Status != "PROPOSED" {
		t.Fatalf("identity must close at confirmation, the discount stays open: %+v", state.Clauses)
	}
	if state.Coverage != commercialvalidation.CoveragePartial || activeCoverage() != string(commercialvalidation.CoveragePartial) {
		t.Fatalf("an open clause keeps coverage partial: %s", state.Coverage)
	}
	if len(state.Services) != 1 || state.Services[0].Active || state.Services[0].SkipReason != commercialvalidation.SkipPriceNotInSource {
		t.Fatalf("a tariff whose amount is not cited with its currency reports why: %+v", state.Services)
	}

	dismiss := commercialvalidation.ClauseDismissal{ClientID: tc.clientID, DocumentID: doc.ID, RuleID: "fixture-discount", CommandID: "dismiss-discount-" + doc.ID, ActorID: "reviewer", ReasonCode: commercialvalidation.ClauseNotInvoiceVerifiable, Reason: "scurt"}
	if _, err = commercial.DismissProposedClause(tc.ctx, dismiss); !errors.Is(err, apperrors.ErrValidation) {
		t.Fatalf("a dismissal needs a real reason, err=%v", err)
	}
	identityCode := dismiss
	identityCode.ReasonCode = commercialvalidation.ClauseCoveredBySupplierIdentity
	if _, err = commercial.DismissProposedClause(tc.ctx, identityCode); !errors.Is(err, apperrors.ErrValidation) {
		t.Fatalf("only an identity clause can be covered by the supplier CUI, err=%v", err)
	}
	dismiss.Reason = "Discountul se negociază separat și nu apare pe facturi."
	if changed, dismissErr := commercial.DismissProposedClause(tc.ctx, dismiss); dismissErr != nil || !changed {
		t.Fatalf("dismiss changed=%t err=%v", changed, dismissErr)
	}
	if coverage := activeCoverage(); coverage != string(commercialvalidation.CoverageComplete) {
		t.Fatalf("with no clause left open coverage is complete, got %s", coverage)
	}
	if changed, replayErr := commercial.DismissProposedClause(tc.ctx, dismiss); replayErr != nil || changed {
		t.Fatalf("replay changed=%t err=%v", changed, replayErr)
	}
	again := dismiss
	again.CommandID = "dismiss-discount-again-" + doc.ID
	if changed, againErr := commercial.DismissProposedClause(tc.ctx, again); againErr != nil || changed {
		t.Fatalf("a closed clause stays closed without a new snapshot, changed=%t err=%v", changed, againErr)
	}
	reference := dismiss
	reference.RuleID, reference.CommandID = "contract-reference", "dismiss-reference-"+doc.ID
	if _, err = commercial.DismissProposedClause(tc.ctx, reference); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a confirmed rule cannot be dismissed, err=%v", err)
	}
	var events int
	if err = tc.store.DB.QueryRowContext(tc.ctx, `SELECT COUNT(*) FROM activity_events WHERE event_type='COMMERCIAL_CLAUSE_DISMISSED' AND idempotency_key=$1`, "commercial-clause-dismissed:"+dismiss.CommandID).Scan(&events); err != nil || events != 1 {
		t.Fatalf("dismissal events=%d err=%v", events, err)
	}
	var versions, distinct int
	if err = tc.store.DB.QueryRowContext(tc.ctx, `SELECT COUNT(*),COUNT(DISTINCT version) FROM contract_commercial_snapshots WHERE contract_id=$1`, contractID).Scan(&versions, &distinct); err != nil || versions != distinct {
		t.Fatalf("snapshot versions must stay unique: snapshots=%d distinct=%d err=%v", versions, distinct, err)
	}
}

type ingestionIntegrationExtractor struct {
	proposal ci.Proposal
	failure  error
}

func (*ingestionIntegrationExtractor) Provider() string { return "DETERMINISTIC_TEST" }
func (*ingestionIntegrationExtractor) Model() string    { return "contract-fixture-v1" }
func (e *ingestionIntegrationExtractor) Extract(context.Context, []byte, string) (ci.ExtractionResult, error) {
	return ci.ExtractionResult{Proposal: e.proposal}, e.failure
}
func contractIngestionFixture(t *testing.T, tc *contractTestContext) (*ci.Service, *contracts.Service, *ingestionIntegrationExtractor) {
	t.Helper()
	client, err := tc.store.Client.AccountingClient.Get(tc.ctx, tc.clientID)
	if err != nil {
		t.Fatal(err)
	}
	p := fixtures.Proposal("romanian")
	p.BuyerCUI.Value = &client.Cui
	p.BuyerCUI.Evidence.Snippet = client.Cui
	extractor := &ingestionIntegrationExtractor{proposal: p}
	matching := contracts.NewService(tc.store, contracts.BaselinePolicy{}, func() time.Time { return tc.now })
	service := ci.NewService(tc.store, extractor, matching, 0, func() time.Time { return tc.now })
	t.Cleanup(func() {
		cleanupCommercialFixture(t, tc)
		ids, _ := tc.store.Client.ContractSourceDocument.Query().Where(contractsourcedocument.ClientIDEQ(tc.clientID)).IDs(tc.ctx)
		contractIDs, _ := tc.store.Client.Contract.Query().Where(contract.ClientIDEQ(tc.clientID)).IDs(tc.ctx)
		_, _ = tc.store.Client.OutboxEntry.Delete().Where(outboxentry.AggregateIDIn(append(ids, contractIDs...)...)).Exec(tc.ctx)
		_, _ = tc.store.Client.ContractServiceTerm.Delete().Where(contractserviceterm.ContractIDIn(contractIDs...)).Exec(tc.ctx)
		_, _ = tc.store.Client.ContractExtractionAttempt.Delete().Where(contractextractionattempt.DocumentIDIn(ids...)).Exec(tc.ctx)
		_, _ = tc.store.Client.ContractSourceDocument.Delete().Where(contractsourcedocument.ClientIDEQ(tc.clientID)).Exec(tc.ctx)
	})
	return service, matching, extractor
}

func cleanupCommercialFixture(t *testing.T, tc *contractTestContext) {
	t.Helper()
	queries := []string{
		`DELETE FROM commercial_review_commands WHERE client_id=$1`,
		`DELETE FROM invoice_commercial_overrides WHERE finding_id IN (SELECT f.id FROM invoice_commercial_findings f JOIN invoice_commercial_validation_runs r ON r.id=f.run_id JOIN invoices i ON i.id=r.invoice_id WHERE i.client_id=$1)`,
		`DELETE FROM invoice_commercial_findings WHERE run_id IN (SELECT r.id FROM invoice_commercial_validation_runs r JOIN invoices i ON i.id=r.invoice_id WHERE i.client_id=$1)`,
		`DELETE FROM invoice_commercial_validation_runs WHERE invoice_id IN (SELECT id FROM invoices WHERE client_id=$1)`,
		`DELETE FROM contract_commercial_ledger WHERE dossier_id IN (SELECT id FROM contract_dossiers WHERE client_id=$1)`,
		`DELETE FROM contract_snapshot_sources WHERE snapshot_id IN (SELECT s.id FROM contract_commercial_snapshots s JOIN contract_dossiers d ON d.id=s.dossier_id WHERE d.client_id=$1)`,
		`UPDATE contract_dossiers SET active_snapshot_id=NULL WHERE client_id=$1`,
		`DELETE FROM contract_clause_candidates WHERE dossier_id IN (SELECT id FROM contract_dossiers WHERE client_id=$1)`,
		`DELETE FROM contract_commercial_snapshots WHERE dossier_id IN (SELECT id FROM contract_dossiers WHERE client_id=$1)`,
		`DELETE FROM contract_variable_values WHERE definition_id IN (SELECT v.id FROM contract_variable_definitions v JOIN contract_dossiers d ON d.id=v.dossier_id WHERE d.client_id=$1)`,
		`DELETE FROM contract_variable_definitions WHERE dossier_id IN (SELECT id FROM contract_dossiers WHERE client_id=$1)`,
		`DELETE FROM contract_service_aliases WHERE client_id=$1`,
		`UPDATE contract_source_documents SET dossier_id=NULL,parent_document_id=NULL WHERE client_id=$1`,
		`DELETE FROM contract_dossiers WHERE client_id=$1`,
	}
	for _, query := range queries {
		if _, err := tc.store.DB.ExecContext(tc.ctx, query, tc.clientID); err != nil {
			t.Errorf("commercial fixture cleanup for %s: %v", tc.clientID, err)
		}
	}
}
func ingestionUpload(t *testing.T, tc *contractTestContext, s *ci.Service) ci.Document {
	t.Helper()
	doc, _, err := s.Upload(tc.ctx, ci.Upload{ClientID: tc.clientID, Filename: "contract.pdf", ContentType: "application/pdf", Bytes: fixtures.PDF("romanian"), Actor: ci.Actor{ID: "uploader", Display: "Uploader", AllClients: true}})
	if err != nil {
		t.Fatal(err)
	}
	return doc
}
func ingestionReview(t *testing.T, tc *contractTestContext, s *ci.Service, doc ci.Document) ci.ConfirmCommand {
	t.Helper()
	if err := s.Extract(tc.ctx, doc.ID); err != nil {
		t.Fatal(err)
	}
	doc, err := s.Get(tc.ctx, tc.clientID, doc.ID, ci.Actor{AllClients: true})
	if err != nil {
		t.Fatal(err)
	}
	p := doc.LatestAttempt.Proposal
	field := func(value *string) string {
		if value == nil {
			return ""
		}
		return *value
	}
	reviewed := ci.ReviewedContract{SupplierName: field(p.SupplierName.Value), SupplierCUI: field(p.SupplierCUI.Value), BuyerCUI: field(p.BuyerCUI.Value), Reference: field(p.Reference.Value), EffectiveFrom: field(p.EffectiveFrom.Value), EffectiveTo: field(p.EffectiveTo.Value), PeriodType: field(p.PeriodType.Value), TotalValue: field(p.TotalValue.Value), Currency: field(p.Currency.Value), UnitType: field(p.UnitType.Value), PaymentTerms: field(p.PaymentTerms.Value)}
	for _, term := range p.ServiceTerms {
		reviewed.ServiceTerms = append(reviewed.ServiceTerms, ci.ReviewedServiceTerm{ServiceDescription: field(term.ServiceDescription.Value), PricingModel: field(term.PricingModel.Value), UnitPrice: field(term.UnitPrice.Value), Currency: field(term.Currency.Value), Unit: field(term.Unit.Value), QuantitySource: field(term.QuantitySource.Value), QuantityValue: field(term.QuantityValue.Value), QuantityDriver: field(term.QuantityDriver.Value), BillingFrequency: field(term.BillingFrequency.Value), Evidence: term.ServiceDescription.Evidence})
	}
	return ci.ConfirmCommand{ClientID: tc.clientID, DocumentID: doc.ID, ExtractionAttemptID: doc.LatestAttempt.ID, ExpectedDocumentRevision: doc.Revision, CommandID: "confirm-" + doc.ID, Actor: ci.Actor{ID: "reviewer", Display: "Reviewer", AllClients: true}, Contract: reviewed}
}

func TestContractIngestionPersistenceDuplicateAndProvenance(t *testing.T) {
	tc := newContractTestContext(t)
	service, _, _ := contractIngestionFixture(t, tc)
	doc := ingestionUpload(t, tc, service)
	source, err := service.File(tc.ctx, tc.clientID, doc.ID, ci.Actor{AllClients: true})
	if err != nil || string(source.Bytes) != string(fixtures.PDF("romanian")) {
		t.Fatal("source changed")
	}
	duplicate, isDuplicate, err := service.Upload(tc.ctx, ci.Upload{ClientID: tc.clientID, Filename: "different-name.pdf", Bytes: fixtures.PDF("romanian"), Actor: ci.Actor{AllClients: true}})
	if err != nil || !isDuplicate || duplicate.ID != doc.ID {
		t.Fatal("duplicate source")
	}
	count, _ := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(doc.ID), outboxentry.EventTypeEQ(outbox.EventContractExtractionRequested)).Count(tc.ctx)
	if count != 1 {
		t.Fatal("duplicate AI cost")
	}
	command := ingestionReview(t, tc, service, doc)
	command.Contract.Reference = "USER-CORRECTED"
	id, changed, err := service.Confirm(tc.ctx, command)
	if err != nil || !changed {
		t.Fatalf("confirm=%v", err)
	}
	row, err := tc.store.Client.Contract.Get(tc.ctx, id)
	if err != nil || row.Reference != "USER-CORRECTED" || row.SourceDocumentID == nil || *row.SourceDocumentID != doc.ID {
		t.Fatal("authoritative contract/provenance")
	}
	again, changed, err := service.Confirm(tc.ctx, command)
	if err != nil || changed || again != id {
		t.Fatalf("replay=%v", err)
	}
	command.Contract.Reference = "CHANGED-REPLAY"
	if _, _, err = service.Confirm(tc.ctx, command); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("idempotency payload reuse")
	}
	review, err := service.Get(tc.ctx, tc.clientID, doc.ID, ci.Actor{AllClients: true})
	if err != nil || *review.LatestAttempt.Proposal.Reference.Value != "CTR-2026-01" || review.ConfirmedValues.Reference != "USER-CORRECTED" || review.ConfirmedByID == nil || *review.ConfirmedByID != "reviewer" {
		t.Fatal("proposal mutated or reviewer lost")
	}
	count, _ = tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(id), outboxentry.EventTypeEQ(outbox.EventContractAvailable)).Count(tc.ctx)
	if count != 1 {
		t.Fatalf("available events=%d", count)
	}
}

func TestMistakenContractCanBeDiscardedAndSamePDFReferenceReingested(t *testing.T) {
	tc := newContractTestContext(t)
	service, matching, _ := contractIngestionFixture(t, tc)
	first := ingestionUpload(t, tc, service)
	firstID, _, err := service.Confirm(tc.ctx, ingestionReview(t, tc, service, first))
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := matching.DeleteMistaken(tc.ctx, firstID, 1, "reviewer", "Reviewer"); err != nil || !changed {
		t.Fatalf("discard changed=%v err=%v", changed, err)
	}
	old, err := tc.store.Client.ContractSourceDocument.Get(tc.ctx, first.ID)
	if err != nil || old.LifecycleState != contractsourcedocument.LifecycleStateDISCARDED {
		t.Fatalf("old source=%+v err=%v", old, err)
	}
	second, duplicate, err := service.Upload(tc.ctx, ci.Upload{ClientID: tc.clientID, Filename: "contract-retry.pdf", Bytes: fixtures.PDF("romanian"), Actor: ci.Actor{ID: "reviewer", Display: "Reviewer", AllClients: true}})
	if err != nil || duplicate || second.ID == first.ID {
		t.Fatalf("reupload doc=%+v duplicate=%t err=%v", second, duplicate, err)
	}
	secondID, _, err := service.Confirm(tc.ctx, ingestionReview(t, tc, service, second))
	if err != nil || secondID == firstID {
		t.Fatalf("reconfirm id=%s first=%s err=%v", secondID, firstID, err)
	}
	var oldStatus, newStatus string
	if err = tc.store.DB.QueryRowContext(tc.ctx, `SELECT status FROM contract_dossiers WHERE contract_id=$1`, firstID).Scan(&oldStatus); err != nil {
		t.Fatal(err)
	}
	if err = tc.store.DB.QueryRowContext(tc.ctx, `SELECT status FROM contract_dossiers WHERE contract_id=$1`, secondID).Scan(&newStatus); err != nil {
		t.Fatal(err)
	}
	if oldStatus != "ARCHIVED" || newStatus == "ARCHIVED" {
		t.Fatalf("dossier statuses old=%s new=%s", oldStatus, newStatus)
	}
}

func TestArchivedValidContractCanBeRenewedUnderSameReference(t *testing.T) {
	tc := newContractTestContext(t)
	service, matching, _ := contractIngestionFixture(t, tc)
	first := ingestionUpload(t, tc, service)
	firstID, _, err := service.Confirm(tc.ctx, ingestionReview(t, tc, service, first))
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := matching.Archive(tc.ctx, firstID, 1, "reviewer", "Reviewer"); err != nil || !changed {
		t.Fatalf("archive changed=%v err=%v", changed, err)
	}
	second, duplicate, err := service.Upload(tc.ctx, ci.Upload{ClientID: tc.clientID, Filename: "renewed.pdf", Bytes: fixtures.PDF("service-indefinite"), Actor: ci.Actor{ID: "reviewer", Display: "Reviewer", AllClients: true}})
	if err != nil || duplicate {
		t.Fatalf("renewed upload duplicate=%t err=%v", duplicate, err)
	}
	secondID, _, err := service.Confirm(tc.ctx, ingestionReview(t, tc, service, second))
	if err != nil || secondID == firstID {
		t.Fatalf("renewed confirmation id=%s first=%s err=%v", secondID, firstID, err)
	}
	oldDoc, err := tc.store.Client.ContractSourceDocument.Get(tc.ctx, first.ID)
	if err != nil || oldDoc.LifecycleState == contractsourcedocument.LifecycleStateDISCARDED {
		t.Fatalf("old valid source=%+v err=%v", oldDoc, err)
	}
	var oldStatus, newStatus string
	if err = tc.store.DB.QueryRowContext(tc.ctx, `SELECT status FROM contract_dossiers WHERE contract_id=$1`, firstID).Scan(&oldStatus); err != nil {
		t.Fatal(err)
	}
	if err = tc.store.DB.QueryRowContext(tc.ctx, `SELECT status FROM contract_dossiers WHERE contract_id=$1`, secondID).Scan(&newStatus); err != nil {
		t.Fatal(err)
	}
	if oldStatus != "ARCHIVED" || newStatus == "ARCHIVED" {
		t.Fatalf("dossier statuses old=%s new=%s", oldStatus, newStatus)
	}
}

func TestIndefiniteMultipleServiceTermsPersistWithProvenance(t *testing.T) {
	tc := newContractTestContext(t)
	service, _, extractor := contractIngestionFixture(t, tc)
	extractor.proposal = fixtures.Proposal("service-indefinite")
	client, err := tc.store.Client.AccountingClient.Get(tc.ctx, tc.clientID)
	if err != nil {
		t.Fatal(err)
	}
	extractor.proposal.BuyerCUI.Value = &client.Cui
	extractor.proposal.BuyerCUI.Evidence.Snippet = client.Cui
	doc := ingestionUpload(t, tc, service)
	contractID, _, err := service.Confirm(tc.ctx, ingestionReview(t, tc, service, doc))
	if err != nil {
		t.Fatal(err)
	}
	row, err := tc.store.Client.Contract.Query().Where(contract.IDEQ(contractID)).WithServiceTerms().Only(tc.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if row.EffectiveTo != nil || row.PeriodType != contract.PeriodTypeINDEFINITE_TERM || row.HasLegacyTotalValue || len(row.Edges.ServiceTerms) != 2 {
		t.Fatalf("contract period=%s end=%v legacy=%t terms=%d", row.PeriodType, row.EffectiveTo, row.HasLegacyTotalValue, len(row.Edges.ServiceTerms))
	}
	terms := row.Edges.ServiceTerms
	if len(terms[0].SourceEvidence) == 0 || len(terms[1].SourceEvidence) == 0 {
		t.Fatal("service evidence missing")
	}
}
func TestContractIngestionExtractionFailureRetryAndStaleReview(t *testing.T) {
	tc := newContractTestContext(t)
	service, _, extractor := contractIngestionFixture(t, tc)
	doc := ingestionUpload(t, tc, service)
	extractor.failure = ci.ErrExtractionTransient
	if err := service.Extract(tc.ctx, doc.ID); !errors.Is(err, ci.ErrExtractionTransient) {
		t.Fatal("transient category")
	}
	extractor.failure = nil
	command := ingestionReview(t, tc, service, doc)
	if err := service.Retry(tc.ctx, tc.clientID, doc.ID, command.ExpectedDocumentRevision, ci.Actor{AllClients: true}); err != nil {
		t.Fatal(err)
	}
	if err := service.Extract(tc.ctx, doc.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Confirm(tc.ctx, command); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("stale review accepted")
	}
	attempts, _ := tc.store.Client.ContractExtractionAttempt.Query().Where(contractextractionattempt.DocumentIDEQ(doc.ID)).All(tc.ctx)
	if len(attempts) != 3 || attempts[0].ID == attempts[1].ID {
		t.Fatalf("history length=%d", len(attempts))
	}
}
func TestContractIngestionBuyerMismatchAndCrossClient(t *testing.T) {
	tc := newContractTestContext(t)
	service, _, extractor := contractIngestionFixture(t, tc)
	doc := ingestionUpload(t, tc, service)
	wrong := "RO99999999"
	extractor.proposal.BuyerCUI.Value = &wrong
	command := ingestionReview(t, tc, service, doc)
	command.Contract.BuyerCUI = wrong
	if _, _, err := service.Confirm(tc.ctx, command); !errors.Is(err, ci.ErrBuyerMismatch) {
		t.Fatal("buyer mismatch activated")
	}
	client, _ := tc.store.Client.AccountingClient.Get(tc.ctx, tc.clientID)
	command.Contract.BuyerCUI = client.Cui
	if _, _, err := service.Confirm(tc.ctx, command); err != nil {
		t.Fatalf("reviewed buyer correction did not unblock: %v", err)
	}
	actor := ci.Actor{AuthorizedClientIDs: []string{"another-client"}}
	if _, err := service.File(tc.ctx, tc.clientID, doc.ID, actor); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("cross-client file")
	}
	if _, err := service.Get(tc.ctx, tc.clientID, doc.ID, actor); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("cross-client proposal")
	}
}
func TestContractIngestionConcurrentConfirmation(t *testing.T) {
	tc := newContractTestContext(t)
	service, _, _ := contractIngestionFixture(t, tc)
	doc := ingestionUpload(t, tc, service)
	command := ingestionReview(t, tc, service, doc)
	var successes atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			copy := command
			copy.CommandID = fmt.Sprintf("reviewer-%d", i)
			_, changed, err := service.Confirm(tc.ctx, copy)
			if err == nil && changed {
				successes.Add(1)
			} else if err != nil && !errors.Is(err, apperrors.ErrConflict) {
				t.Errorf("confirm=%v", err)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("confirmations=%d", successes.Load())
	}
	count, _ := tc.store.Client.Contract.Query().Where(contract.ClientIDEQ(tc.clientID)).Count(tc.ctx)
	if count != 1 {
		t.Fatalf("contracts=%d", count)
	}
}
func TestContractIngestionMissingContractResumeE2E(t *testing.T) {
	for _, journey := range []string{"unique", "multiple", "irrelevant"} {
		t.Run(journey, func(t *testing.T) {
			tc := newContractTestContext(t)
			service, matching, _ := contractIngestionFixture(t, tc)
			invoiceID := tc.createInvoice(t, "ingestion-"+journey, "RO12345678", "RON")
			if _, _, err := matching.MatchInvoice(tc.ctx, contracts.MatchCommand{InvoiceID: invoiceID, ExpectedRevision: 1, CommandID: "missing:" + invoiceID}); err != nil {
				t.Fatal(err)
			}
			before, err := tc.store.GetInvoice(tc.ctx, invoiceID)
			if err != nil || before.ActiveTask == nil {
				t.Fatal("missing-contract setup")
			}
			if _, _, err = validationtasks.NewService(tc.store, func() time.Time { return tc.now }).RequestMissingContract(tc.ctx, validationtasks.RequestMissingContractCommand{InvoiceID: invoiceID, TaskID: before.ActiveTask.ID, ExpectedRevision: before.ActiveTask.Revision, CommandID: "request:" + invoiceID}); err != nil {
				t.Fatal(err)
			}
			doc := ingestionUpload(t, tc, service)
			command := ingestionReview(t, tc, service, doc)
			if journey == "multiple" {
				tc.createContract(t, "other", "RO12345678", "RON")
			}
			if journey == "irrelevant" {
				command.Contract.SupplierCUI = "RO87654321"
			}
			id, _, err := service.Confirm(tc.ctx, command)
			if err != nil {
				t.Fatal(err)
			}
			event, err := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(id), outboxentry.EventTypeEQ(outbox.EventContractAvailable)).Only(tc.ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = matching.ProcessContractAvailable(tc.ctx, id, event.IdempotencyKey, "ingestion-e2e"); err != nil {
				t.Fatal(err)
			}
			after, err := tc.store.Client.Invoice.Get(tc.ctx, invoiceID)
			if err != nil {
				t.Fatal(err)
			}
			want := invoice.PipelineStatusDEDUPE_CHECKED
			if journey == "multiple" {
				want = invoice.PipelineStatusAWAITING_MATCH_CONFIRM
			}
			if journey == "irrelevant" {
				want = invoice.PipelineStatusAWAITING_CONTRACT
			}
			if after.PipelineStatus != want {
				t.Fatalf("pipeline=%s want=%s", after.PipelineStatus, want)
			}
			task, err := tc.store.Client.ValidationTask.Get(tc.ctx, before.ActiveTask.ID)
			if err != nil {
				t.Fatal(err)
			}
			wantTask := validationtask.StatusRESOLVED
			if journey == "irrelevant" {
				wantTask = validationtask.StatusWAITING
			}
			if task.Status != wantTask {
				t.Fatalf("missing task=%s want=%s", task.Status, wantTask)
			}
		})
	}
}

func TestOneConfirmedContractReevaluatesTwoWaitingInvoices(t *testing.T) {
	tc := newContractTestContext(t)
	service, matching, _ := contractIngestionFixture(t, tc)
	invoiceIDs := []string{tc.createInvoice(t, "shared-a", "RO12345678", "RON"), tc.createInvoice(t, "shared-b", "RO12345678", "RON")}
	for _, invoiceID := range invoiceIDs {
		if _, _, err := matching.MatchInvoice(tc.ctx, contracts.MatchCommand{InvoiceID: invoiceID, ExpectedRevision: 1, CommandID: "missing:" + invoiceID}); err != nil {
			t.Fatal(err)
		}
		item, err := tc.store.GetInvoice(tc.ctx, invoiceID)
		if err != nil || item.ActiveTask == nil {
			t.Fatal("missing-contract setup")
		}
		if _, _, err = validationtasks.NewService(tc.store, func() time.Time { return tc.now }).RequestMissingContract(tc.ctx, validationtasks.RequestMissingContractCommand{InvoiceID: invoiceID, TaskID: item.ActiveTask.ID, ExpectedRevision: item.ActiveTask.Revision, CommandID: "request:" + invoiceID}); err != nil {
			t.Fatal(err)
		}
	}
	doc := ingestionUpload(t, tc, service)
	contractID, _, err := service.Confirm(tc.ctx, ingestionReview(t, tc, service, doc))
	if err != nil {
		t.Fatal(err)
	}
	event, err := tc.store.Client.OutboxEntry.Query().Where(outboxentry.AggregateIDEQ(contractID), outboxentry.EventTypeEQ(outbox.EventContractAvailable)).Only(tc.ctx)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := matching.ProcessContractAvailable(tc.ctx, contractID, event.IdempotencyKey, "shared-contract")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Evaluated != 2 {
		t.Fatalf("evaluated=%d summary=%+v", summary.Evaluated, summary)
	}
	for _, invoiceID := range invoiceIDs {
		row, err := tc.store.Client.Invoice.Get(tc.ctx, invoiceID)
		if err != nil || row.PipelineStatus == invoice.PipelineStatusAWAITING_CONTRACT {
			t.Fatalf("invoice %s was not reevaluated: status=%s err=%v", invoiceID, row.PipelineStatus, err)
		}
	}
}

func TestDiscardUnconfirmedDocumentLeavesInvoiceWaiting(t *testing.T) {
	tc := newContractTestContext(t)
	service, matching, _ := contractIngestionFixture(t, tc)
	invoiceID := tc.createInvoice(t, "discard-waiting", "RO12345678", "RON")
	if _, _, err := matching.MatchInvoice(tc.ctx, contracts.MatchCommand{InvoiceID: invoiceID, ExpectedRevision: 1, CommandID: "missing:" + invoiceID}); err != nil {
		t.Fatal(err)
	}
	before, err := tc.store.GetInvoice(tc.ctx, invoiceID)
	if err != nil || before.ActiveTask == nil {
		t.Fatal("missing-contract setup")
	}
	if _, _, err = validationtasks.NewService(tc.store, func() time.Time { return tc.now }).RequestMissingContract(tc.ctx, validationtasks.RequestMissingContractCommand{InvoiceID: invoiceID, TaskID: before.ActiveTask.ID, ExpectedRevision: before.ActiveTask.Revision, CommandID: "request:" + invoiceID}); err != nil {
		t.Fatal(err)
	}
	doc := ingestionUpload(t, tc, service)
	if err = service.Discard(tc.ctx, tc.clientID, doc.ID, doc.Revision, ci.Actor{AllClients: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Get(tc.ctx, tc.clientID, doc.ID, ci.Actor{AllClients: true}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("discarded document remained active: %v", err)
	}
	after, err := tc.store.GetInvoice(tc.ctx, invoiceID)
	if err != nil || after.PipelineStatus != invoicing.StatusAwaitingContract || after.ActiveTask == nil || after.ActiveTask.Status != validationtasks.StatusWaiting {
		t.Fatalf("waiting workflow changed: %+v err=%v", after, err)
	}
	if err = service.Discard(tc.ctx, "another-client", doc.ID, doc.Revision, ci.Actor{AuthorizedClientIDs: []string{"another-client"}}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("cross-client discard=%v", err)
	}
}
