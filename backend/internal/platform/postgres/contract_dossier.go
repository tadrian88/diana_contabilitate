package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"regexp"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/commercialvalidation"
	"diana-contabilitate/backend/internal/contractingestion"
)

// ActivateConfirmedContract composes a new immutable commercial snapshot from
// human-confirmed values. It is intentionally idempotent and is also called on
// confirmation replay, so a transient post-confirmation failure is repairable.
func (s *Store) ActivateConfirmedContract(ctx context.Context, command contractingestion.ConfirmCommand, contractID string, now time.Time) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var proposalJSON []byte
	if err = tx.QueryRowContext(ctx, `SELECT proposal FROM contract_extraction_attempts WHERE id=$1 AND document_id=$2 AND status='SUCCEEDED'`, command.ExtractionAttemptID, command.DocumentID).Scan(&proposalJSON); err != nil {
		return err
	}
	var proposal contractingestion.Proposal
	if err = json.Unmarshal(proposalJSON, &proposal); err != nil {
		return err
	}
	pendingClauses := contractingestion.UnconfirmedCommercialClauses(proposal, command.Contract.CommercialRules)
	var clientCUI, supplierCUI, reference string
	var effectiveFrom time.Time
	var effectiveTo sql.NullTime
	if err = tx.QueryRowContext(ctx, `SELECT cl.cui,c.supplier_cui,c.reference,c.effective_from,c.effective_to FROM contracts c JOIN clients cl ON cl.id=c.client_id WHERE c.id=$1 AND c.client_id=$2`, contractID, command.ClientID).Scan(&clientCUI, &supplierCUI, &reference, &effectiveFrom, &effectiveTo); err != nil {
		return err
	}
	dossierID := stableID("dossier", contractID)
	_ = tx.QueryRowContext(ctx, `SELECT id FROM contract_dossiers WHERE client_id=$1 AND primary_reference=$2 AND status<>'ARCHIVED'`, command.ClientID, reference).Scan(&dossierID)
	var documentAlreadyIntegrated bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM contract_snapshot_sources ss JOIN contract_commercial_snapshots s ON s.id=ss.snapshot_id WHERE s.dossier_id=$1 AND ss.document_id=$2)`, dossierID, command.DocumentID).Scan(&documentAlreadyIntegrated); err != nil {
		return err
	}
	if documentAlreadyIntegrated {
		return nil
	}
	coverage := command.Contract.Coverage
	if coverage == "" {
		coverage = commercialvalidation.CoveragePartial
	}
	if len(pendingClauses) > 0 {
		coverage = commercialvalidation.CoveragePartial
	}
	status := "ACTIVE_PARTIAL"
	if coverage == commercialvalidation.CoverageComplete {
		status = "ACTIVE_COMPLETE"
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO contract_dossiers(id,client_id,contract_id,supplier_cui,buyer_cui,primary_reference,status,revision,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,1,$8,$8) ON CONFLICT(id) DO UPDATE SET contract_id=EXCLUDED.contract_id,supplier_cui=EXCLUDED.supplier_cui,buyer_cui=EXCLUDED.buyer_cui,status=EXCLUDED.status,updated_at=EXCLUDED.updated_at WHERE contract_dossiers.contract_id IS DISTINCT FROM EXCLUDED.contract_id OR contract_dossiers.supplier_cui IS DISTINCT FROM EXCLUDED.supplier_cui OR contract_dossiers.buyer_cui IS DISTINCT FROM EXCLUDED.buyer_cui OR contract_dossiers.status IS DISTINCT FROM EXCLUDED.status`, dossierID, command.ClientID, contractID, supplierCUI, clientCUI, reference, status, now)
	if err != nil {
		return err
	}
	role := command.Contract.DocumentRole
	if role == "" {
		role = "BASE_CONTRACT"
	}
	var parentID any
	if related := strings.TrimSpace(command.Contract.RelatedReference); related != "" {
		var id string
		if scanErr := tx.QueryRowContext(ctx, `SELECT d.id FROM contract_source_documents d JOIN contracts c ON c.id=d.confirmed_contract_id WHERE d.client_id=$1 AND c.reference=$2 ORDER BY d.confirmed_at LIMIT 1`, command.ClientID, related).Scan(&id); scanErr == nil {
			parentID = id
		}
	}
	relationship, _ := json.Marshal(map[string]string{"relatedReference": command.Contract.RelatedReference})
	if _, err = tx.ExecContext(ctx, `UPDATE contract_source_documents SET dossier_id=$2,document_role=$3,parent_document_id=$4,relationship_metadata=$5 WHERE id=$1 AND client_id=$6`, command.DocumentID, dossierID, role, parentID, relationship, command.ClientID); err != nil {
		return err
	}

	newRules := append([]commercialvalidation.Rule(nil), command.Contract.CommercialRules...)
	if !hasConfirmedPricingRule(newRules) {
		newRules = append(newRules, reviewedServicePriceRules(command.DocumentID, contractID, command.Contract.ServiceTerms, proposal.ServiceTerms)...)
	}
	refExists := false
	for _, rule := range newRules {
		if rule.Kind == commercialvalidation.RuleContractReference {
			refExists = true
		}
	}
	if !refExists {
		newRules = append(newRules, commercialvalidation.Rule{ID: "contract-reference", Kind: commercialvalidation.RuleContractReference, Narrative: "Referința contractului confirmat este " + reference, DateBasis: commercialvalidation.DateInvoiceIssue, Expression: &commercialvalidation.Expression{Op: "literal", Value: reference, Scale: 4}, Evidence: []commercialvalidation.Evidence{{DocumentID: command.DocumentID, Snippet: reference}}, Blocking: true})
	}
	for index := range newRules {
		for evidenceIndex := range newRules[index].Evidence {
			if newRules[index].Evidence[evidenceIndex].DocumentID == "" || newRules[index].Evidence[evidenceIndex].DocumentID == "fixture" {
				newRules[index].Evidence[evidenceIndex].DocumentID = command.DocumentID
			}
		}
	}
	rules := append([]commercialvalidation.Rule(nil), newRules...)
	var currentRulesJSON []byte
	var currentVersion uint64
	var currentSnapshotID string
	var pendingAncestorSnapshotID string
	_ = tx.QueryRowContext(ctx, `SELECT s.id,s.rules,s.version FROM contract_dossiers d JOIN contract_commercial_snapshots s ON s.id=d.active_snapshot_id WHERE d.id=$1`, dossierID).Scan(&currentSnapshotID, &currentRulesJSON, &currentVersion)
	if currentSnapshotID != "" {
		var priorPending int
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM contract_snapshot_sources ss JOIN contract_clause_candidates cc ON cc.id=ss.clause_id WHERE ss.snapshot_id=$1 AND cc.review_status='PROPOSED'`, currentSnapshotID).Scan(&priorPending); err != nil {
			return err
		}
		if priorPending > 0 {
			coverage = commercialvalidation.CoveragePartial
			status = "ACTIVE_PARTIAL"
			pendingAncestorSnapshotID = currentSnapshotID
		}
	}
	if len(currentRulesJSON) > 0 {
		var current []commercialvalidation.Rule
		if json.Unmarshal(currentRulesJSON, &current) == nil {
			if hasRuleConflict(current, newRules) {
				coverage = commercialvalidation.CoverageConflicted
				status = "ACTIVE_PARTIAL"
			}
			rules = mergeRules(current, rules)
		}
	}
	rulesJSON, _ := json.Marshal(rules)
	pendingRuleIDs := make([]string, 0, len(pendingClauses))
	for _, clause := range pendingClauses {
		var rule commercialvalidation.Rule
		if err = json.Unmarshal(clause.Rule, &rule); err != nil {
			return err
		}
		pendingRuleIDs = append(pendingRuleIDs, command.DocumentID+":"+rule.ID)
	}
	var effectiveToPointer *time.Time
	if effectiveTo.Valid {
		effectiveToPointer = &effectiveTo.Time
	}
	hashInput, _ := json.Marshal(struct {
		Schema                    string                        `json:"schema"`
		Coverage                  commercialvalidation.Coverage `json:"coverage"`
		EffectiveFrom             time.Time                     `json:"effectiveFrom"`
		EffectiveTo               *time.Time                    `json:"effectiveTo,omitempty"`
		Rules                     json.RawMessage               `json:"rules"`
		PendingRuleIDs            []string                      `json:"pendingRuleIds"`
		PendingAncestorSnapshotID string                        `json:"pendingAncestorSnapshotId,omitempty"`
	}{Schema: commercialvalidation.RuleSchemaVersion, Coverage: coverage, EffectiveFrom: effectiveFrom, EffectiveTo: effectiveToPointer, Rules: rulesJSON, PendingRuleIDs: pendingRuleIDs, PendingAncestorSnapshotID: pendingAncestorSnapshotID})
	sum := sha256.Sum256(hashInput)
	rulesHash := hex.EncodeToString(sum[:])
	version := currentVersion + 1
	if version == 0 {
		version = 1
	}
	snapshotID := stableID("commercial-snapshot", fmt.Sprintf("%s:%s", dossierID, rulesHash))
	var to any
	if effectiveTo.Valid {
		to = effectiveTo.Time
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO contract_commercial_snapshots(id,dossier_id,contract_id,version,schema_version,coverage,effective_from,effective_to,rules,rules_hash,confirmed_by_id,confirmed_by_display,confirmed_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$13) ON CONFLICT(dossier_id,rules_hash) DO NOTHING`, snapshotID, dossierID, contractID, version, commercialvalidation.RuleSchemaVersion, coverage, effectiveFrom, to, rulesJSON, rulesHash, command.Actor.ID, nullText(command.Actor.Display), now)
	if err != nil {
		return err
	}
	if currentSnapshotID != "" {
		if _, err = tx.ExecContext(ctx, `INSERT INTO contract_snapshot_sources(snapshot_id,document_id,clause_id) SELECT $1,document_id,clause_id FROM contract_snapshot_sources WHERE snapshot_id=$2 ON CONFLICT DO NOTHING`, snapshotID, currentSnapshotID); err != nil {
			return err
		}
	}
	for _, rule := range newRules {
		clauseID := stableID("contract-clause", snapshotID+":"+rule.ID)
		ruleJSON, _ := json.Marshal(rule)
		page := any(nil)
		snippet := rule.Narrative
		if len(rule.Evidence) > 0 {
			page = rule.Evidence[0].Page
			snippet = rule.Evidence[0].Snippet
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO contract_clause_candidates(id,dossier_id,document_id,extraction_attempt_id,clause_kind,narrative,normalized_rule,confidence,review_status,source_page,source_snippet,created_at,reviewed_at,reviewed_by_id) VALUES($1,$2,$3,$4,$5,$6,$7,'HIGH','CONFIRMED',$8,$9,$10,$10,$11) ON CONFLICT(id) DO NOTHING`, clauseID, dossierID, command.DocumentID, command.ExtractionAttemptID, rule.Kind, rule.Narrative, ruleJSON, page, snippet, now, command.Actor.ID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO contract_snapshot_sources(snapshot_id,document_id,clause_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, snapshotID, command.DocumentID, clauseID)
		if err != nil {
			return err
		}
		variableNames := append([]string(nil), rule.RequiredVariables...)
		variableNames = append(variableNames, expressionVariables(rule.Expression)...)
		if rule.Kind == commercialvalidation.RuleUnitRate {
			variableNames = append(variableNames, "unit_quantity_"+rule.ID)
		}
		if rule.Applicability.Tranche != nil {
			variableNames = append(variableNames, "tranche")
		}
		for _, name := range variableNames {
			definitionID := stableID("contract-variable-definition", dossierID+":"+name)
			allowed, _ := json.Marshal([]string{"INVOICE", "INTEGRATION", "MANUAL", "CONTRACT"})
			_, err = tx.ExecContext(ctx, `INSERT INTO contract_variable_definitions(id,dossier_id,name,value_type,allowed_sources,required,narrative) VALUES($1,$2,$3,'DECIMAL',$4,true,$5) ON CONFLICT(dossier_id,name) DO NOTHING`, definitionID, dossierID, name, allowed, "Variabilă necesară regulii "+rule.ID)
			if err != nil {
				return err
			}
		}
	}
	for _, clause := range pendingClauses {
		var rule commercialvalidation.Rule
		if err = json.Unmarshal(clause.Rule, &rule); err != nil {
			return err
		}
		clauseID := stableID("contract-clause-pending", snapshotID+":"+command.DocumentID+":"+rule.ID)
		var proposedRule any
		if commercialvalidation.ValidateRule(rule) == nil {
			proposedRule = clause.Rule
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO contract_clause_candidates(id,dossier_id,document_id,extraction_attempt_id,clause_kind,narrative,normalized_rule,confidence,review_status,source_page,source_snippet,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'PROPOSED',$9,$10,$11) ON CONFLICT(id) DO NOTHING`, clauseID, dossierID, command.DocumentID, command.ExtractionAttemptID, rule.Kind, rule.Narrative, proposedRule, clause.Confidence, clause.Evidence.Page, clause.Evidence.Snippet, now)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO contract_snapshot_sources(snapshot_id,document_id,clause_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, snapshotID, command.DocumentID, clauseID); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE contract_dossiers SET active_snapshot_id=$2,status=$3,revision=revision+1,updated_at=$4 WHERE id=$1 AND (active_snapshot_id IS DISTINCT FROM $2 OR status IS DISTINCT FROM $3)`, dossierID, snapshotID, status, now); err != nil {
		return err
	}
	eventKey := "commercial-snapshot-activated:" + snapshotID
	if _, err = tx.ExecContext(ctx, `INSERT INTO activity_events(id,client_id,aggregate_type,aggregate_id,event_type,occurred_at,actor_kind,actor_id,actor_display,automatic,detail,idempotency_key) VALUES($1,$2,'CONTRACT_DOSSIER',$3,'COMMERCIAL_SNAPSHOT_ACTIVATED',$4,'USER',$5,$6,false,$7,$8) ON CONFLICT(idempotency_key) DO NOTHING`, stableID("evt", eventKey), command.ClientID, dossierID, now, command.Actor.ID, nullText(command.Actor.Display), fmt.Sprintf("Snapshot comercial v%d activat cu acoperire %s.", version, coverage), eventKey); err != nil {
		return err
	}
	return tx.Commit()
}

// ConfirmProposedRule promotes one reviewed rule into a new immutable
// snapshot. Existing executable proposals are loaded from storage. For a
// narrative-only proposal, the reviewer may supply the missing normalized
// fields, while kind, narrative and source evidence remain server-owned.
func (s *Store) ConfirmProposedRule(ctx context.Context, command commercialvalidation.RuleConfirmation, now time.Time) (bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	eventKey := "commercial-rule-confirmed:" + command.CommandID
	var prior bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM activity_events WHERE idempotency_key=$1)`, eventKey).Scan(&prior); err != nil {
		return false, err
	}
	if prior {
		return false, nil
	}
	var clauseID, dossierID, snapshotID, contractID, status, clauseKind, clauseNarrative, sourceSnippet, primaryReference string
	var ruleJSON, currentRulesJSON []byte
	var version uint64
	var effectiveFrom time.Time
	var effectiveTo sql.NullTime
	var sourcePage sql.NullInt64
	baseQuery := `
		SELECT cc.id,cc.dossier_id,d.active_snapshot_id,s.contract_id,s.rules,s.version,s.effective_from,s.effective_to,cc.review_status,COALESCE(cc.normalized_rule,'null'::jsonb),cc.clause_kind,cc.narrative,cc.source_page,cc.source_snippet,d.primary_reference
		FROM contract_clause_candidates cc
		JOIN contract_dossiers d ON d.id=cc.dossier_id
		JOIN contract_commercial_snapshots s ON s.id=d.active_snapshot_id
		JOIN contract_source_documents doc ON doc.id=cc.document_id
		WHERE doc.client_id=$1 AND cc.document_id=$2 AND %s
		ORDER BY cc.created_at DESC LIMIT 1 FOR UPDATE OF cc,d`
	row := tx.QueryRowContext(ctx, fmt.Sprintf(baseQuery, `cc.normalized_rule->>'id'=$3`), command.ClientID, command.DocumentID, command.RuleID)
	if command.Rule != nil {
		row = tx.QueryRowContext(ctx, fmt.Sprintf(baseQuery, `cc.review_status='PROPOSED' AND EXISTS (
			SELECT 1 FROM contract_extraction_attempts attempt,
			jsonb_array_elements(attempt.proposal->'commercialClauses') proposed
			WHERE attempt.id=cc.extraction_attempt_id
			AND proposed->'rule'->>'id'=$3
			AND proposed->'kind'->>'value'=cc.clause_kind
			AND proposed->'narrative'->>'value'=cc.narrative
			AND proposed->'evidence'->>'snippet'=cc.source_snippet
		)`), command.ClientID, command.DocumentID, command.RuleID)
	}
	err = row.Scan(&clauseID, &dossierID, &snapshotID, &contractID, &currentRulesJSON, &version, &effectiveFrom, &effectiveTo, &status, &ruleJSON, &clauseKind, &clauseNarrative, &sourcePage, &sourceSnippet, &primaryReference)
	if errors.Is(err, sql.ErrNoRows) {
		return false, apperrors.ErrNotFound
	}
	if err != nil {
		return false, err
	}
	if status == "CONFIRMED" {
		return false, nil
	}
	if status != "PROPOSED" {
		return false, apperrors.ErrConflict
	}
	var rule commercialvalidation.Rule
	storedRuleValid := json.Unmarshal(ruleJSON, &rule) == nil && commercialvalidation.ValidateRule(rule) == nil
	if !storedRuleValid && command.Rule != nil {
		if !reviewedRuleAllowed(*command.Rule) {
			return false, fmt.Errorf("%w: reviewed commercial rule is not supported", apperrors.ErrValidation)
		}
		var proposedRuleJSON []byte
		if err = tx.QueryRowContext(ctx, `SELECT proposed->'rule' FROM contract_clause_candidates cc
			JOIN contract_extraction_attempts attempt ON attempt.id=cc.extraction_attempt_id,
			jsonb_array_elements(attempt.proposal->'commercialClauses') proposed
			WHERE cc.id=$1 AND proposed->'rule'->>'id'=$2
			LIMIT 1`, clauseID, command.RuleID).Scan(&proposedRuleJSON); err != nil {
			return false, fmt.Errorf("load original commercial proposal: %w", err)
		}
		var proposed commercialvalidation.Rule
		if err = json.Unmarshal(proposedRuleJSON, &proposed); err != nil {
			return false, fmt.Errorf("decode original commercial proposal: %w", err)
		}
		if !reviewedRulePreservesProposal(proposed, *command.Rule) {
			return false, fmt.Errorf("%w: reviewed rule changed unconfirmed contract terms", apperrors.ErrValidation)
		}
		if !reviewedLiteralSupportedBySource(*command.Rule, sourceSnippet) {
			return false, fmt.Errorf("%w: reviewed literal or date basis is not stated in the source clause", apperrors.ErrValidation)
		}
		rule = *command.Rule
	}
	if rule.ID != command.RuleID || string(rule.Kind) != clauseKind || rule.Narrative != clauseNarrative || commercialvalidation.ValidateRule(rule) != nil {
		return false, fmt.Errorf("%w: proposed commercial rule is not executable", apperrors.ErrValidation)
	}
	if !storedRuleValid {
		var page *int
		if sourcePage.Valid {
			value := int(sourcePage.Int64)
			page = &value
		}
		rule.Evidence = []commercialvalidation.Evidence{{DocumentID: command.DocumentID, Page: page, Snippet: sourceSnippet}}
	} else {
		for index := range rule.Evidence {
			if rule.Evidence[index].DocumentID == "" {
				rule.Evidence[index].DocumentID = command.DocumentID
			}
		}
	}
	var current []commercialvalidation.Rule
	if err = json.Unmarshal(currentRulesJSON, &current); err != nil {
		return false, err
	}
	rules := mergeRules(current, []commercialvalidation.Rule{rule})
	if _, err = tx.ExecContext(ctx, `UPDATE contract_clause_candidates SET normalized_rule=$2,review_status='CONFIRMED',reviewed_at=$3,reviewed_by_id=$4 WHERE id=$1 AND review_status='PROPOSED'`, clauseID, mustJSON(rule), now, command.ActorID); err != nil {
		return false, err
	}
	// The base contract reference is already active as a system rule. A second
	// provider proposal carrying the same reference must not keep coverage
	// partial forever.
	if _, err = tx.ExecContext(ctx, `UPDATE contract_clause_candidates SET review_status='REJECTED',reviewed_at=$2,reviewed_by_id=$3 WHERE dossier_id=$1 AND review_status='PROPOSED' AND clause_kind='CONTRACT_REFERENCE' AND POSITION(regexp_replace(upper($4),'\s','','g') IN regexp_replace(upper(source_snippet),'\s','','g'))>0`, dossierID, now, command.ActorID, primaryReference); err != nil {
		return false, err
	}
	var pending, conflicted int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FILTER (WHERE review_status='PROPOSED'),COUNT(*) FILTER (WHERE review_status='CONFLICTED') FROM contract_clause_candidates WHERE dossier_id=$1`, dossierID).Scan(&pending, &conflicted); err != nil {
		return false, err
	}
	coverage := commercialvalidation.CoverageComplete
	dossierStatus := "ACTIVE_COMPLETE"
	if conflicted > 0 {
		coverage = commercialvalidation.CoverageConflicted
		dossierStatus = "ACTIVE_PARTIAL"
	} else if pending > 0 {
		coverage = commercialvalidation.CoveragePartial
		dossierStatus = "ACTIVE_PARTIAL"
	}
	rulesJSON, _ := json.Marshal(rules)
	var effectiveToPointer *time.Time
	if effectiveTo.Valid {
		effectiveToPointer = &effectiveTo.Time
	}
	hashInput, _ := json.Marshal(struct {
		Schema        string                        `json:"schema"`
		Coverage      commercialvalidation.Coverage `json:"coverage"`
		EffectiveFrom time.Time                     `json:"effectiveFrom"`
		EffectiveTo   *time.Time                    `json:"effectiveTo,omitempty"`
		Rules         json.RawMessage               `json:"rules"`
	}{commercialvalidation.RuleSchemaVersion, coverage, effectiveFrom, effectiveToPointer, rulesJSON})
	sum := sha256.Sum256(hashInput)
	rulesHash := hex.EncodeToString(sum[:])
	newSnapshotID := stableID("commercial-snapshot", dossierID+":"+rulesHash)
	newVersion := version + 1
	var to any
	if effectiveTo.Valid {
		to = effectiveTo.Time
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO contract_commercial_snapshots(id,dossier_id,contract_id,version,schema_version,coverage,effective_from,effective_to,rules,rules_hash,confirmed_by_id,confirmed_by_display,confirmed_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$13)`, newSnapshotID, dossierID, contractID, newVersion, commercialvalidation.RuleSchemaVersion, coverage, effectiveFrom, to, rulesJSON, rulesHash, command.ActorID, nullText(command.ActorDisplay), now); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO contract_snapshot_sources(snapshot_id,document_id,clause_id) SELECT $1,document_id,clause_id FROM contract_snapshot_sources WHERE snapshot_id=$2 ON CONFLICT DO NOTHING`, newSnapshotID, snapshotID); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE contract_dossiers SET active_snapshot_id=$2,status=$3,revision=revision+1,updated_at=$4 WHERE id=$1`, dossierID, newSnapshotID, dossierStatus, now); err != nil {
		return false, err
	}
	variableNames := append([]string(nil), rule.RequiredVariables...)
	variableNames = append(variableNames, expressionVariables(rule.Expression)...)
	if rule.Applicability.Tranche != nil {
		variableNames = append(variableNames, "tranche")
	}
	for _, name := range variableNames {
		definitionID := stableID("contract-variable-definition", dossierID+":"+name)
		allowed, _ := json.Marshal([]string{"INVOICE", "INTEGRATION", "MANUAL", "CONTRACT"})
		if _, err = tx.ExecContext(ctx, `INSERT INTO contract_variable_definitions(id,dossier_id,name,value_type,allowed_sources,required,narrative) VALUES($1,$2,$3,'DECIMAL',$4,true,$5) ON CONFLICT(dossier_id,name) DO NOTHING`, definitionID, dossierID, name, allowed, "Variabilă necesară regulii "+rule.ID); err != nil {
			return false, err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO activity_events(id,client_id,aggregate_type,aggregate_id,event_type,occurred_at,actor_kind,actor_id,actor_display,automatic,detail,idempotency_key) VALUES($1,$2,'CONTRACT_DOSSIER',$3,'COMMERCIAL_RULE_CONFIRMED',$4,'USER',$5,$6,false,$7,$8)`, stableID("evt", eventKey), command.ClientID, dossierID, now, command.ActorID, nullText(command.ActorDisplay), "Regula comercială "+rule.ID+" a fost confirmată și inclusă într-un snapshot nou.", eventKey); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// ActivateReviewedServicePrices makes already-confirmed service terms usable by
// future invoices. The reviewed amount must still match the original cited
// price; otherwise that service is left unverified for explicit correction.
func (s *Store) ActivateReviewedServicePrices(ctx context.Context, clientID, documentID, actorID, commandID string, now time.Time) (int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var confirmedJSON, proposalJSON, currentRulesJSON []byte
	var contractID, dossierID, snapshotID, coverage string
	var version uint64
	var effectiveFrom time.Time
	var effectiveTo sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT doc.confirmed_values,attempt.proposal,doc.confirmed_contract_id,doc.dossier_id
		FROM contract_source_documents doc JOIN contract_extraction_attempts attempt ON attempt.id=doc.latest_extraction_id
		WHERE doc.id=$1 AND doc.client_id=$2 AND doc.status='CONFIRMED' FOR UPDATE OF doc`, documentID, clientID).
		Scan(&confirmedJSON, &proposalJSON, &contractID, &dossierID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, apperrors.ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	err = tx.QueryRowContext(ctx, `SELECT s.id,s.version,s.coverage,s.effective_from,s.effective_to,s.rules
		FROM contract_dossiers d JOIN contract_commercial_snapshots s ON s.id=d.active_snapshot_id
		WHERE d.id=$1 AND d.client_id=$2 AND s.contract_id=$3 FOR UPDATE OF d`, dossierID, clientID, contractID).
		Scan(&snapshotID, &version, &coverage, &effectiveFrom, &effectiveTo, &currentRulesJSON)
	if err != nil {
		return 0, err
	}
	var confirmed contractingestion.ReviewedContract
	var proposal contractingestion.Proposal
	var current []commercialvalidation.Rule
	if err = json.Unmarshal(confirmedJSON, &confirmed); err != nil {
		return 0, err
	}
	if err = json.Unmarshal(proposalJSON, &proposal); err != nil {
		return 0, err
	}
	if err = json.Unmarshal(currentRulesJSON, &current); err != nil {
		return 0, err
	}
	if hasConfirmedPricingRule(current) {
		for _, rule := range current {
			if rule.Kind == commercialvalidation.RuleFixedPrice || rule.Kind == commercialvalidation.RuleUnitRate || rule.Kind == commercialvalidation.RuleTieredPrice {
				if !strings.HasPrefix(rule.ID, "service-") {
					return 0, apperrors.ErrConflict
				}
			}
		}
	}
	proposedRules := reviewedServicePriceRules(documentID, contractID, confirmed.ServiceTerms, proposal.ServiceTerms)
	if len(proposedRules) == 0 {
		return 0, fmt.Errorf("%w: no source-backed reviewed service prices", apperrors.ErrValidation)
	}
	existing := map[string]commercialvalidation.Rule{}
	for _, rule := range current {
		existing[rule.ID] = rule
	}
	var added []commercialvalidation.Rule
	for _, rule := range proposedRules {
		if prior, found := existing[rule.ID]; found {
			before, _ := json.Marshal(prior)
			after, _ := json.Marshal(rule)
			if string(before) != string(after) {
				return 0, apperrors.ErrConflict
			}
			continue
		}
		added = append(added, rule)
	}
	if len(added) == 0 {
		return 0, nil
	}
	rules := append(current, added...)
	rulesJSON, _ := json.Marshal(rules)
	var effectiveToPointer *time.Time
	if effectiveTo.Valid {
		effectiveToPointer = &effectiveTo.Time
	}
	hashInput, _ := json.Marshal(struct {
		Coverage      string          `json:"coverage"`
		EffectiveFrom time.Time       `json:"effectiveFrom"`
		EffectiveTo   *time.Time      `json:"effectiveTo,omitempty"`
		Rules         json.RawMessage `json:"rules"`
	}{coverage, effectiveFrom, effectiveToPointer, rulesJSON})
	sum := sha256.Sum256(hashInput)
	rulesHash := hex.EncodeToString(sum[:])
	newSnapshotID := stableID("commercial-snapshot", dossierID+":"+rulesHash)
	var effectiveToValue any
	if effectiveTo.Valid {
		effectiveToValue = effectiveTo.Time
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO contract_commercial_snapshots(id,dossier_id,contract_id,version,schema_version,coverage,effective_from,effective_to,rules,rules_hash,confirmed_by_id,confirmed_at,created_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$12) ON CONFLICT(dossier_id,rules_hash) DO NOTHING`, newSnapshotID, dossierID, contractID, version+1, commercialvalidation.RuleSchemaVersion, coverage, effectiveFrom, effectiveToValue, rulesJSON, rulesHash, actorID, now)
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO contract_snapshot_sources(snapshot_id,document_id,clause_id)
		SELECT $1,document_id,clause_id FROM contract_snapshot_sources WHERE snapshot_id=$2 ON CONFLICT DO NOTHING`, newSnapshotID, snapshotID); err != nil {
		return 0, err
	}
	for _, rule := range added {
		clauseID := stableID("contract-clause", newSnapshotID+":"+rule.ID)
		if _, err = tx.ExecContext(ctx, `INSERT INTO contract_clause_candidates(id,dossier_id,document_id,extraction_attempt_id,clause_kind,narrative,normalized_rule,confidence,review_status,source_page,source_snippet,created_at,reviewed_at,reviewed_by_id)
			SELECT $1,$2,$3,latest_extraction_id,$4,$5,$6,'HIGH','CONFIRMED',$7,$8,$9,$9,$10 FROM contract_source_documents WHERE id=$3 ON CONFLICT(id) DO NOTHING`, clauseID, dossierID, documentID, rule.Kind, rule.Narrative, mustJSON(rule), rule.Evidence[0].Page, rule.Evidence[0].Snippet, now, actorID); err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO contract_snapshot_sources(snapshot_id,document_id,clause_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, newSnapshotID, documentID, clauseID); err != nil {
			return 0, err
		}
		if rule.Kind == commercialvalidation.RuleUnitRate {
			name := "unit_quantity_" + rule.ID
			allowed, _ := json.Marshal([]string{"INVOICE", "INTEGRATION", "MANUAL", "CONTRACT"})
			if _, err = tx.ExecContext(ctx, `INSERT INTO contract_variable_definitions(id,dossier_id,name,value_type,allowed_sources,required,narrative)
				VALUES($1,$2,$3,'DECIMAL',$4,true,$5) ON CONFLICT(dossier_id,name) DO NOTHING`, stableID("contract-variable-definition", dossierID+":"+name), dossierID, name, allowed, "Cantitatea verificată pentru "+rule.Narrative); err != nil {
				return 0, err
			}
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE contract_dossiers SET active_snapshot_id=$2,revision=revision+1,updated_at=$3 WHERE id=$1`, dossierID, newSnapshotID, now); err != nil {
		return 0, err
	}
	eventKey := "reviewed-service-prices-activated:" + commandID
	if _, err = tx.ExecContext(ctx, `INSERT INTO activity_events(id,client_id,aggregate_type,aggregate_id,event_type,occurred_at,actor_kind,actor_id,automatic,detail,idempotency_key)
		VALUES($1,$2,'CONTRACT_DOSSIER',$3,'COMMERCIAL_SNAPSHOT_ACTIVATED',$4,'USER',$5,false,$6,$7) ON CONFLICT(idempotency_key) DO NOTHING`, stableID("evt", eventKey), clientID, dossierID, now, actorID, fmt.Sprintf("%d tarife din servicii confirmate activate într-o versiune contractuală nouă.", len(added)), eventKey); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return len(added), nil
}

func reviewedRuleAllowed(rule commercialvalidation.Rule) bool {
	if rule.Kind == commercialvalidation.RuleVAT && rule.Blocking && rule.Currency == "" && rule.DateBasis == commercialvalidation.DateInvoiceIssue &&
		rule.Expression != nil && rule.Expression.Op == "variable" && rule.Expression.Variable == "applicable_vat_rate" && rule.Expression.Value == "" && len(rule.Expression.Args) == 0 && len(rule.Expression.Tiers) == 0 &&
		len(rule.RequiredVariables) == 1 && rule.RequiredVariables[0] == "applicable_vat_rate" {
		return true
	}
	if !rule.Blocking || rule.Expression == nil || rule.Expression.Op != "literal" || rule.Expression.Variable != "" || len(rule.Expression.Args) != 0 || len(rule.Expression.Tiers) != 0 || len(rule.RequiredVariables) != 0 {
		return false
	}
	if _, ok := new(big.Rat).SetString(rule.Expression.Value); !ok {
		return false
	}
	switch rule.Kind {
	case commercialvalidation.RuleFixedPrice, commercialvalidation.RuleUnitRate:
		return len(rule.Currency) == 3 && rule.DateBasis == commercialvalidation.DateInvoiceIssue
	case commercialvalidation.RuleVAT:
		return rule.Currency == "" && rule.DateBasis == commercialvalidation.DateInvoiceIssue
	case commercialvalidation.RulePaymentDue:
		return rule.Currency == "" && (rule.DateBasis == commercialvalidation.DateReceipt || rule.DateBasis == commercialvalidation.DateInvoiceIssue || rule.DateBasis == commercialvalidation.DateAcceptance)
	default:
		return false
	}
}

// reviewedRulePreservesProposal prevents a reviewer from replacing terms that
// were already extracted from the contract. A narrative-only clause may gain
// the inputs required by its tightly allow-listed completion (currently the
// applicable VAT rate); without this exception the safe VAT completion is
// accepted by reviewedRuleAllowed and then rejected as a changed proposal.
func reviewedRulePreservesProposal(proposed, reviewed commercialvalidation.Rule) bool {
	if proposed.Expression != nil || !reflect.DeepEqual(proposed.Applicability, reviewed.Applicability) ||
		(proposed.Currency != "" && proposed.Currency != reviewed.Currency) ||
		(proposed.DateBasis != "" && proposed.DateBasis != reviewed.DateBasis) {
		return false
	}
	return len(proposed.RequiredVariables) == 0 || reflect.DeepEqual(proposed.RequiredVariables, reviewed.RequiredVariables)
}

var clausePercentPattern = regexp.MustCompile(`(\d+(?:[.,]\d+)?)\s*%`)
var invoiceIssuePattern = regexp.MustCompile(`\bemiter`)
var paymentDaysPattern = regexp.MustCompile(`\b(\d+(?:[.,]\d+)?)\s+(?:de\s+)?zile\b`)
var clauseNumberPattern = regexp.MustCompile(`\b\d+(?:[.,]\d+)?\b`)

func reviewedLiteralSupportedBySource(rule commercialvalidation.Rule, source string) bool {
	if rule.Expression == nil {
		return false
	}
	if rule.Kind == commercialvalidation.RuleVAT {
		if rule.Expression.Op == "variable" && rule.Expression.Variable == "applicable_vat_rate" {
			lower := strings.ToLower(source)
			return strings.Contains(lower, "tva") && (strings.Contains(lower, "aferent") || strings.Contains(lower, "aplicabil") || strings.Contains(lower, "legal"))
		}
		actual, ok := new(big.Rat).SetString(rule.Expression.Value)
		if !ok {
			return false
		}
		for _, match := range clausePercentPattern.FindAllStringSubmatch(source, -1) {
			stated, valid := new(big.Rat).SetString(strings.ReplaceAll(match[1], ",", "."))
			if valid && actual.Cmp(stated) == 0 {
				return true
			}
		}
		return false
	}
	if rule.Kind == commercialvalidation.RuleFixedPrice || rule.Kind == commercialvalidation.RuleUnitRate {
		actual, ok := new(big.Rat).SetString(rule.Expression.Value)
		if !ok {
			return false
		}
		upper := strings.ToUpper(source)
		if !strings.Contains(upper, rule.Currency) && !(rule.Currency == "RON" && (strings.Contains(upper, "LEI") || strings.Contains(upper, "LEU"))) {
			return false
		}
		for _, token := range clauseNumberPattern.FindAllString(source, -1) {
			stated, valid := new(big.Rat).SetString(strings.ReplaceAll(token, ",", "."))
			if valid && actual.Cmp(stated) == 0 {
				return true
			}
		}
		return false
	}
	if rule.Kind == commercialvalidation.RulePaymentDue {
		lower := strings.ToLower(source)
		actual, ok := new(big.Rat).SetString(rule.Expression.Value)
		if !ok {
			return false
		}
		statedDays := false
		for _, match := range paymentDaysPattern.FindAllStringSubmatch(lower, -1) {
			stated, valid := new(big.Rat).SetString(strings.ReplaceAll(match[1], ",", "."))
			if valid && actual.Cmp(stated) == 0 {
				statedDays = true
				break
			}
		}
		if !statedDays {
			return false
		}
		switch rule.DateBasis {
		case commercialvalidation.DateReceipt:
			return strings.Contains(lower, "remiter") || strings.Contains(lower, "primir") || strings.Contains(lower, "transmiter")
		case commercialvalidation.DateInvoiceIssue:
			return invoiceIssuePattern.MatchString(lower)
		case commercialvalidation.DateAcceptance:
			return strings.Contains(lower, "accept")
		default:
			return false
		}
	}
	return true
}

func mustJSON(value any) []byte {
	data, _ := json.Marshal(value)
	return data
}

func expressionVariables(expr *commercialvalidation.Expression) []string {
	if expr == nil {
		return nil
	}
	result := []string{}
	if expr.Op == "variable" && expr.Variable != "" {
		result = append(result, expr.Variable)
	}
	for index := range expr.Args {
		result = append(result, expressionVariables(&expr.Args[index])...)
	}
	return result
}

func mergeRules(current, next []commercialvalidation.Rule) []commercialvalidation.Rule {
	index := map[string]int{}
	result := append([]commercialvalidation.Rule(nil), current...)
	for position, rule := range result {
		index[rule.ID] = position
	}
	for _, rule := range next {
		if position, ok := index[rule.ID]; ok {
			result[position] = rule
		} else {
			index[rule.ID] = len(result)
			result = append(result, rule)
		}
	}
	return result
}

func hasRuleConflict(current, next []commercialvalidation.Rule) bool {
	for _, existing := range current {
		for _, candidate := range next {
			if existing.ID == candidate.ID {
				continue
			}
			if existing.Kind != candidate.Kind || existing.Applicability.ServiceID == "" || existing.Applicability.ServiceID != candidate.Applicability.ServiceID {
				continue
			}
			left, _ := json.Marshal(existing)
			right, _ := json.Marshal(candidate)
			if string(left) != string(right) {
				return true
			}
		}
	}
	return false
}

func hasConfirmedPricingRule(rules []commercialvalidation.Rule) bool {
	for _, rule := range rules {
		if rule.Kind == commercialvalidation.RuleFixedPrice || rule.Kind == commercialvalidation.RuleUnitRate || rule.Kind == commercialvalidation.RuleTieredPrice {
			return true
		}
	}
	return false
}

// reviewedServicePriceRules converts only human-confirmed prices that still
// match the extractor's cited source. It never uses an invoice amount to infer
// which contractual service applies.
func reviewedServicePriceRules(documentID, contractID string, terms []contractingestion.ReviewedServiceTerm, proposed []contractingestion.ProposedServiceTerm) []commercialvalidation.Rule {
	result := []commercialvalidation.Rule{}
	for index, term := range terms {
		if index >= len(proposed) || strings.TrimSpace(term.UnitPrice) == "" || strings.TrimSpace(term.ServiceDescription) == "" {
			continue
		}
		if term.PricingModel != "FIXED_FEE" && term.PricingModel != "FIXED_TOTAL" && term.PricingModel != "UNIT_RATE" {
			continue
		}
		priceEvidence := proposed[index].UnitPrice.Evidence
		if priceEvidence.Snippet == "" || proposed[index].UnitPrice.Value == nil || proposed[index].Currency.Value == nil || strings.TrimSpace(*proposed[index].UnitPrice.Value) != strings.TrimSpace(term.UnitPrice) || !strings.EqualFold(strings.TrimSpace(*proposed[index].Currency.Value), strings.TrimSpace(term.Currency)) {
			continue
		}
		serviceID := stableID("service", fmt.Sprintf("%s:%d", contractID, index))
		expression := &commercialvalidation.Expression{Op: "literal", Value: term.UnitPrice, Scale: 4}
		kind := commercialvalidation.RuleFixedPrice
		if term.PricingModel == "UNIT_RATE" {
			kind = commercialvalidation.RuleUnitRate
		}
		narrative := term.ServiceDescription + " · " + term.UnitPrice + " " + term.Currency
		if kind == commercialvalidation.RuleUnitRate && strings.TrimSpace(term.Unit) != "" {
			narrative += " / " + term.Unit
		}
		rule := commercialvalidation.Rule{ID: serviceID, Kind: kind, Narrative: narrative, Applicability: commercialvalidation.Applicability{ServiceID: serviceID, Aliases: []string{term.ServiceDescription}, BillingFrequency: term.BillingFrequency}, DateBasis: commercialvalidation.DateInvoiceIssue, Currency: strings.ToUpper(term.Currency), Expression: expression, Evidence: []commercialvalidation.Evidence{{DocumentID: documentID, Page: priceEvidence.Page, Snippet: priceEvidence.Snippet}}, Blocking: true}
		if commercialvalidation.ValidateRule(rule) == nil && reviewedLiteralSupportedBySource(rule, priceEvidence.Snippet) {
			result = append(result, rule)
		}
	}
	return result
}
