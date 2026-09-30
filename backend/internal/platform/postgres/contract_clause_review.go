package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/commercialvalidation"
	"diana-contabilitate/backend/internal/contractingestion"
)

// cuiTokenPattern finds CUI-like numbers in a cited clause: 6 to 10 digits,
// optionally prefixed by RO, standing on their own. A trade-register number
// such as J2018004511402 is a single token and never matches.
var cuiTokenPattern = regexp.MustCompile(`(?i)\b(?:RO\s*)?(\d{6,10})\b`)

func digitsOnly(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, value)
}

// identityCoverage tells whether an identity clause only restates the
// contract's parties: it names at least one CUI and every CUI it names is the
// supplier's (COVERED_BY_SUPPLIER_IDENTITY) or the supplier's and the
// client's (COVERED_BY_PARTY_IDENTITY). The client's CUI is the buyer of
// every invoice Diana receives, since SPV delivers only invoices issued to
// it. A clause naming any other CUI (for instance a party disagreement) is
// left for review: the result is empty.
func identityCoverage(snippet, supplierCUI, buyerCUI string) string {
	supplier, buyer := digitsOnly(supplierCUI), digitsOnly(buyerCUI)
	matches := cuiTokenPattern.FindAllStringSubmatch(snippet, -1)
	if len(matches) == 0 {
		return ""
	}
	onlySupplier := true
	for _, match := range matches {
		switch {
		case supplier != "" && match[1] == supplier:
		case buyer != "" && match[1] == buyer:
			onlySupplier = false
		default:
			return ""
		}
	}
	if onlySupplier {
		return commercialvalidation.ClauseCoveredBySupplierIdentity
	}
	return commercialvalidation.ClauseCoveredByPartyIdentity
}

// coverageContext is what the dossier already checks on every invoice: its
// reference, its parties' CUIs and the rules of its active snapshot.
type coverageContext struct {
	DossierID, Reference, SupplierCUI, BuyerCUI string
	Rules                                       []commercialvalidation.Rule
}

// closeCoveredClauses closes the dossier's proposed clauses it already
// enforces: a second contract reference naming the primary reference, an
// identity clause naming only the parties' CUIs and a pricing clause that
// restates a reviewed service tariff. It returns how many clauses it closed;
// the caller moves the dossier to a new snapshot.
func closeCoveredClauses(ctx context.Context, tx *sql.Tx, coverage coverageContext, actorID string, now time.Time) (int, error) {
	closed := 0
	if strings.TrimSpace(coverage.Reference) != "" {
		result, err := tx.ExecContext(ctx, `UPDATE contract_clause_candidates SET review_status='REJECTED',reviewed_at=$2,reviewed_by_id=$3,review_reason_code=$5,review_reason=$6
			WHERE dossier_id=$1 AND review_status='PROPOSED' AND clause_kind='CONTRACT_REFERENCE'
			AND POSITION(regexp_replace(upper($4),'\s','','g') IN regexp_replace(upper(source_snippet),'\s','','g'))>0`,
			coverage.DossierID, now, nullText(actorID), coverage.Reference, commercialvalidation.ClauseCoveredByContractReference,
			"Referința este verificată pe fiecare factură prin regula de referință a contractului ("+coverage.Reference+").")
		if err != nil {
			return 0, err
		}
		affected, _ := result.RowsAffected()
		closed += int(affected)
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,clause_kind,narrative,source_snippet,normalized_rule FROM contract_clause_candidates WHERE dossier_id=$1 AND review_status='PROPOSED' AND clause_kind IN ('IDENTITY','FIXED_PRICE','UNIT_RATE')`, coverage.DossierID)
	if err != nil {
		return 0, err
	}
	type closing struct{ id, code, reason string }
	var covered []closing
	for rows.Next() {
		var id, kind, narrative, snippet string
		var ruleJSON []byte
		if err = rows.Scan(&id, &kind, &narrative, &snippet, &ruleJSON); err != nil {
			rows.Close()
			return 0, err
		}
		var rule *commercialvalidation.Rule
		if len(ruleJSON) > 0 && string(ruleJSON) != "null" {
			rule = &commercialvalidation.Rule{}
			if json.Unmarshal(ruleJSON, rule) != nil {
				rule = nil
			}
		}
		if kind == string(commercialvalidation.RuleIdentity) {
			literal := ""
			if rule != nil && rule.Expression != nil {
				literal = digitsOnly(rule.Expression.Value)
			}
			code := identityCoverage(snippet, coverage.SupplierCUI, coverage.BuyerCUI)
			if code == "" || (literal != "" && literal != digitsOnly(coverage.SupplierCUI) && literal != digitsOnly(coverage.BuyerCUI)) {
				continue
			}
			reason := supplierIdentityReason(coverage.SupplierCUI)
			if code == commercialvalidation.ClauseCoveredByPartyIdentity {
				reason = partyIdentityReason(coverage.SupplierCUI, coverage.BuyerCUI, saleDossier(ctx, tx, coverage.DossierID))
			}
			covered = append(covered, closing{id, code, reason})
			continue
		}
		if service, restated := restatedTariff(kind, narrative, snippet, rule, coverage.Rules); restated {
			covered = append(covered, closing{id, commercialvalidation.ClauseCoveredByServiceTariff, serviceTariffReason(service)})
		}
	}
	if err = rows.Close(); err != nil {
		return 0, err
	}
	for _, item := range covered {
		result, err := tx.ExecContext(ctx, `UPDATE contract_clause_candidates SET review_status='REJECTED',reviewed_at=$2,reviewed_by_id=$3,review_reason_code=$4,review_reason=$5 WHERE id=$1 AND review_status='PROPOSED'`,
			item.id, now, nullText(actorID), item.code, item.reason)
		if err != nil {
			return 0, err
		}
		affected, _ := result.RowsAffected()
		closed += int(affected)
	}
	return closed, nil
}

// restatedTariff finds the reviewed service tariff a proposed pricing clause
// only restates. A clause with a rule is compared rule to rule; a
// narrative-only clause must name the service and state its price in the
// cited text.
func restatedTariff(kind, narrative, snippet string, rule *commercialvalidation.Rule, rules []commercialvalidation.Rule) (commercialvalidation.Rule, bool) {
	if rule != nil && rule.Expression != nil {
		clause := *rule
		if clause.Narrative == "" {
			clause.Narrative = narrative
		}
		if snippet != "" {
			clause.Evidence = append(append([]commercialvalidation.Evidence(nil), clause.Evidence...), commercialvalidation.Evidence{Snippet: snippet})
		}
		return commercialvalidation.RestatedServiceTariff(clause, rules)
	}
	if kind != string(commercialvalidation.RuleFixedPrice) && kind != string(commercialvalidation.RuleUnitRate) {
		return commercialvalidation.Rule{}, false
	}
	for _, service := range rules {
		if !commercialvalidation.IsServiceTariff(service) || (service.Kind != commercialvalidation.RuleFixedPrice && service.Kind != commercialvalidation.RuleUnitRate) {
			continue
		}
		if commercialvalidation.NamesService(narrative+"\n"+snippet, commercialvalidation.ServiceLabel(service)) && reviewedLiteralSupportedBySource(service, snippet) {
			return service, true
		}
	}
	return commercialvalidation.Rule{}, false
}

func supplierIdentityReason(supplierCUI string) string {
	return "Furnizorul este identificat prin CUI-ul confirmat al contractului (" + supplierCUI + "); facturile se asociază contractului după acest CUI."
}

func partyIdentityReason(supplierCUI, buyerCUI string, sale bool) string {
	if sale {
		return "Clauza numește părțile contractului: clientul, furnizor (locator) prin CUI-ul " + supplierCUI + ", care emite facturile, și cumpărătorul (locatarul) prin CUI-ul " + buyerCUI + ", după care facturile emise se asociază contractului."
	}
	return "Clauza numește părțile contractului, deja verificate pe fiecare factură: clientul prin CUI-ul " + buyerCUI + ", singurul cumpărător al facturilor primite din SPV, și furnizorul prin CUI-ul " + supplierCUI + ", după care facturile se asociază contractului."
}

func serviceTariffReason(service commercialvalidation.Rule) string {
	return "Tariful „" + service.Narrative + "” din „Servicii și tarife” verifică deja această clauză pe facturi."
}

// snapshotAdvance is the dossier state a command moves forward: the snapshot
// it locked and the rules the dossier holds once the command applies.
type snapshotAdvance struct {
	DossierID, ContractID, SnapshotID string
	Coverage                          commercialvalidation.Coverage
	Current, Rules                    []commercialvalidation.Rule
	EffectiveFrom                     time.Time
	EffectiveTo                       sql.NullTime
	ActorID, ActorDisplay             string
}

// advanceSnapshot recounts the dossier's open clauses and activates the
// snapshot for the given rules and the resulting coverage. Coverage is
// complete only when no clause is left proposed; a conflict found when
// documents were combined is kept, since closing clauses does not resolve it.
// Nothing is written when rules and coverage are unchanged. A rule set seen
// before reuses its immutable snapshot; a new one takes the next free version.
func advanceSnapshot(ctx context.Context, tx *sql.Tx, change snapshotAdvance, now time.Time) (bool, error) {
	var pending, conflicted int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FILTER (WHERE review_status='PROPOSED'),COUNT(*) FILTER (WHERE review_status='CONFLICTED') FROM contract_clause_candidates WHERE dossier_id=$1`, change.DossierID).Scan(&pending, &conflicted); err != nil {
		return false, err
	}
	coverage, dossierStatus := commercialvalidation.CoverageComplete, "ACTIVE_COMPLETE"
	if conflicted > 0 || change.Coverage == commercialvalidation.CoverageConflicted {
		coverage, dossierStatus = commercialvalidation.CoverageConflicted, "ACTIVE_PARTIAL"
	} else if pending > 0 {
		coverage, dossierStatus = commercialvalidation.CoveragePartial, "ACTIVE_PARTIAL"
	}
	if coverage == change.Coverage && sameRules(change.Current, change.Rules) {
		return false, nil
	}
	rulesJSON, _ := json.Marshal(change.Rules)
	var effectiveToPointer *time.Time
	var effectiveTo any
	if change.EffectiveTo.Valid {
		effectiveToPointer = &change.EffectiveTo.Time
		effectiveTo = change.EffectiveTo.Time
	}
	hashInput, _ := json.Marshal(struct {
		Schema        string                        `json:"schema"`
		Coverage      commercialvalidation.Coverage `json:"coverage"`
		EffectiveFrom time.Time                     `json:"effectiveFrom"`
		EffectiveTo   *time.Time                    `json:"effectiveTo,omitempty"`
		Rules         json.RawMessage               `json:"rules"`
	}{commercialvalidation.RuleSchemaVersion, coverage, change.EffectiveFrom, effectiveToPointer, rulesJSON})
	sum := sha256.Sum256(hashInput)
	rulesHash := hex.EncodeToString(sum[:])
	snapshotID := stableID("commercial-snapshot", change.DossierID+":"+rulesHash)
	version, err := nextSnapshotVersion(ctx, tx, change.DossierID)
	if err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO contract_commercial_snapshots(id,dossier_id,contract_id,version,schema_version,coverage,effective_from,effective_to,rules,rules_hash,confirmed_by_id,confirmed_by_display,confirmed_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$13) ON CONFLICT(dossier_id,rules_hash) DO NOTHING`,
		snapshotID, change.DossierID, change.ContractID, version, commercialvalidation.RuleSchemaVersion, coverage, change.EffectiveFrom, effectiveTo, rulesJSON, rulesHash, change.ActorID, nullText(change.ActorDisplay), now); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO contract_snapshot_sources(snapshot_id,document_id,clause_id) SELECT $1,document_id,clause_id FROM contract_snapshot_sources WHERE snapshot_id=$2 ON CONFLICT DO NOTHING`, snapshotID, change.SnapshotID); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE contract_dossiers SET active_snapshot_id=$2,status=$3,revision=revision+1,updated_at=$4 WHERE id=$1`, change.DossierID, snapshotID, dossierStatus, now); err != nil {
		return false, err
	}
	return true, nil
}

func sameRules(left, right []commercialvalidation.Rule) bool {
	return len(left) == len(right) && (len(left) == 0 || reflect.DeepEqual(left, right))
}

// nextSnapshotVersion is the first version not used by any snapshot of the
// dossier. The active snapshot is not always the latest one: a revision back
// to an earlier rule set reactivates that older snapshot.
func nextSnapshotVersion(ctx context.Context, tx *sql.Tx, dossierID string) (uint64, error) {
	var version uint64
	err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0)+1 FROM contract_commercial_snapshots WHERE dossier_id=$1`, dossierID).Scan(&version)
	return version, err
}

// CloseCoveredClauses closes, in the dossier of a confirmed document, the
// proposed clauses its identity already covers and activates a snapshot with
// the resulting coverage. It is idempotent and returns how many it closed.
func (s *Store) CloseCoveredClauses(ctx context.Context, clientID, documentID, actorID, actorDisplay string, now time.Time) (int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var dossierID, snapshotID, contractID, reference, supplierCUI, buyerCUI string
	var coverage commercialvalidation.Coverage
	var rulesJSON []byte
	var effectiveFrom time.Time
	var effectiveTo sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT d.id,d.active_snapshot_id,s.contract_id,s.rules,s.coverage,s.effective_from,s.effective_to,d.primary_reference,d.supplier_cui,COALESCE(d.buyer_cui,'')
		FROM contract_source_documents doc
		JOIN contract_dossiers d ON d.id=doc.dossier_id
		JOIN contract_commercial_snapshots s ON s.id=d.active_snapshot_id
		WHERE doc.client_id=$1 AND doc.id=$2 FOR UPDATE OF d`, clientID, documentID).
		Scan(&dossierID, &snapshotID, &contractID, &rulesJSON, &coverage, &effectiveFrom, &effectiveTo, &reference, &supplierCUI, &buyerCUI)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var rules []commercialvalidation.Rule
	if err = json.Unmarshal(rulesJSON, &rules); err != nil {
		return 0, err
	}
	closed, err := closeCoveredClauses(ctx, tx, coverageContext{DossierID: dossierID, Reference: reference, SupplierCUI: supplierCUI, BuyerCUI: buyerCUI, Rules: rules}, actorID, now)
	if err != nil || closed == 0 {
		return 0, err
	}
	if _, err = advanceSnapshot(ctx, tx, snapshotAdvance{DossierID: dossierID, ContractID: contractID, SnapshotID: snapshotID, Coverage: coverage, Current: rules, Rules: rules, EffectiveFrom: effectiveFrom, EffectiveTo: effectiveTo, ActorID: actorID, ActorDisplay: actorDisplay}, now); err != nil {
		return 0, err
	}
	eventKey := "commercial-clauses-closed:" + dossierID + ":" + now.UTC().Format(time.RFC3339Nano)
	if _, err = tx.ExecContext(ctx, `INSERT INTO activity_events(id,client_id,aggregate_type,aggregate_id,event_type,occurred_at,actor_kind,actor_id,actor_display,automatic,detail,idempotency_key) VALUES($1,$2,'CONTRACT_DOSSIER',$3,'COMMERCIAL_CLAUSES_CLOSED',$4,'SYSTEM',$5,$6,true,$7,$8) ON CONFLICT(idempotency_key) DO NOTHING`,
		stableID("evt", eventKey), clientID, dossierID, now, nullText(actorID), nullText(actorDisplay), fmt.Sprintf("Clauze închise automat, acoperite de referința, CUI-urile părților sau tarifele contractului: %d.", closed), eventKey); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return closed, nil
}

// DismissProposedClause closes one proposed clause of a confirmed document:
// the reviewer states it is not checked on invoices, or it only restates the
// parties' CUIs the dossier already checks. The dossier moves to a snapshot
// with the resulting coverage. A replay of a closed clause changes nothing; a
// clause that already became a rule cannot be closed.
func (s *Store) DismissProposedClause(ctx context.Context, command commercialvalidation.ClauseDismissal, now time.Time) (bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	eventKey := "commercial-clause-dismissed:" + command.CommandID
	var prior bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM activity_events WHERE idempotency_key=$1)`, eventKey).Scan(&prior); err != nil {
		return false, err
	}
	if prior {
		return false, nil
	}
	var clauseID, dossierID, snapshotID, contractID, status, kind, snippet, reference, supplierCUI, buyerCUI string
	var coverage commercialvalidation.Coverage
	var rulesJSON []byte
	var effectiveFrom time.Time
	var effectiveTo sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT cc.id,cc.dossier_id,d.active_snapshot_id,s.contract_id,s.rules,s.coverage,s.effective_from,s.effective_to,cc.review_status,cc.clause_kind,cc.source_snippet,d.primary_reference,d.supplier_cui,COALESCE(d.buyer_cui,'')
		FROM contract_clause_candidates cc
		JOIN contract_dossiers d ON d.id=cc.dossier_id
		JOIN contract_commercial_snapshots s ON s.id=d.active_snapshot_id
		JOIN contract_source_documents doc ON doc.id=cc.document_id
		WHERE doc.client_id=$1 AND cc.document_id=$2 AND COALESCE(cc.normalized_rule->>'id',cc.proposal_rule_id)=$3
		ORDER BY cc.created_at DESC LIMIT 1 FOR UPDATE OF cc,d`, command.ClientID, command.DocumentID, command.RuleID).
		Scan(&clauseID, &dossierID, &snapshotID, &contractID, &rulesJSON, &coverage, &effectiveFrom, &effectiveTo, &status, &kind, &snippet, &reference, &supplierCUI, &buyerCUI)
	if errors.Is(err, sql.ErrNoRows) {
		return false, apperrors.ErrNotFound
	}
	if err != nil {
		return false, err
	}
	if status == "REJECTED" {
		return false, nil
	}
	if status != "PROPOSED" {
		return false, apperrors.ErrConflict
	}
	reason := command.Reason
	switch command.ReasonCode {
	case commercialvalidation.ClauseCoveredBySupplierIdentity:
		if kind != string(commercialvalidation.RuleIdentity) || identityCoverage(snippet, supplierCUI, buyerCUI) != command.ReasonCode {
			return false, fmt.Errorf("%w: the clause does not only name the supplier CUI of the contract", apperrors.ErrValidation)
		}
		reason = supplierIdentityReason(supplierCUI)
	case commercialvalidation.ClauseCoveredByPartyIdentity:
		if kind != string(commercialvalidation.RuleIdentity) || identityCoverage(snippet, supplierCUI, buyerCUI) != command.ReasonCode {
			return false, fmt.Errorf("%w: the clause does not only name the CUIs of the contract parties", apperrors.ErrValidation)
		}
		reason = partyIdentityReason(supplierCUI, buyerCUI, saleDossier(ctx, tx, dossierID))
	}
	if _, err = tx.ExecContext(ctx, `UPDATE contract_clause_candidates SET review_status='REJECTED',reviewed_at=$2,reviewed_by_id=$3,review_reason_code=$4,review_reason=$5 WHERE id=$1 AND review_status='PROPOSED'`,
		clauseID, now, command.ActorID, command.ReasonCode, reason); err != nil {
		return false, err
	}
	var rules []commercialvalidation.Rule
	if err = json.Unmarshal(rulesJSON, &rules); err != nil {
		return false, err
	}
	if _, err = closeCoveredClauses(ctx, tx, coverageContext{DossierID: dossierID, Reference: reference, SupplierCUI: supplierCUI, BuyerCUI: buyerCUI, Rules: rules}, command.ActorID, now); err != nil {
		return false, err
	}
	if _, err = advanceSnapshot(ctx, tx, snapshotAdvance{DossierID: dossierID, ContractID: contractID, SnapshotID: snapshotID, Coverage: coverage, Current: rules, Rules: rules, EffectiveFrom: effectiveFrom, EffectiveTo: effectiveTo, ActorID: command.ActorID, ActorDisplay: command.ActorDisplay}, now); err != nil {
		return false, err
	}
	detail := fmt.Sprintf("Clauza %s (%s) nu influențează facturile: %s", command.RuleID, kind, reason)
	switch command.ReasonCode {
	case commercialvalidation.ClauseCoveredBySupplierIdentity:
		detail = fmt.Sprintf("Clauza %s (%s) este acoperită de CUI-ul furnizorului.", command.RuleID, kind)
	case commercialvalidation.ClauseCoveredByPartyIdentity:
		detail = fmt.Sprintf("Clauza %s (%s) este acoperită de CUI-urile părților.", command.RuleID, kind)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO activity_events(id,client_id,aggregate_type,aggregate_id,event_type,occurred_at,actor_kind,actor_id,actor_display,automatic,detail,idempotency_key) VALUES($1,$2,'CONTRACT_DOSSIER',$3,'COMMERCIAL_CLAUSE_DISMISSED',$4,'USER',$5,$6,false,$7,$8)`,
		stableID("evt", eventKey), command.ClientID, dossierID, now, command.ActorID, nullText(command.ActorDisplay), detail, eventKey); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// DocumentCommercialState reads what a confirmed document contributes to
// invoice checks now: the active snapshot's coverage, which of its rules came
// from this document, whether each confirmed service tariff is enforced (and
// why not), and how each proposed clause was settled. It is nil for a
// document that is not part of a dossier snapshot.
func (s *Store) DocumentCommercialState(ctx context.Context, clientID, documentID string) (*commercialvalidation.DocumentCommercialState, error) {
	var dossierID, contractID sql.NullString
	var confirmedJSON, proposalJSON []byte
	err := s.DB.QueryRowContext(ctx, `SELECT doc.dossier_id,doc.confirmed_contract_id,COALESCE(doc.confirmed_values,'null'::jsonb),COALESCE(attempt.proposal,'null'::jsonb)
		FROM contract_source_documents doc LEFT JOIN contract_extraction_attempts attempt ON attempt.id=doc.latest_extraction_id
		WHERE doc.client_id=$1 AND doc.id=$2 AND doc.status='CONFIRMED'`, clientID, documentID).Scan(&dossierID, &contractID, &confirmedJSON, &proposalJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !dossierID.Valid || dossierID.String == "" {
		return nil, nil
	}
	state := commercialvalidation.DocumentCommercialState{DossierID: dossierID.String, ActiveRuleIDs: []string{}, Services: []commercialvalidation.DocumentServiceState{}, Clauses: []commercialvalidation.DocumentClauseState{}}
	var rulesJSON []byte
	err = s.DB.QueryRowContext(ctx, `SELECT s.version,s.coverage,s.rules FROM contract_dossiers d JOIN contract_commercial_snapshots s ON s.id=d.active_snapshot_id WHERE d.id=$1 AND d.client_id=$2`, dossierID.String, clientID).Scan(&state.SnapshotVersion, &state.Coverage, &rulesJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rules []commercialvalidation.Rule
	if err = json.Unmarshal(rulesJSON, &rules); err != nil {
		return nil, err
	}
	active := make(map[string]commercialvalidation.Rule, len(rules))
	for _, rule := range rules {
		active[rule.ID] = rule
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT ON (rule_id) rule_id,clause_kind,review_status,review_reason_code,review_reason,reviewer,reviewed_at FROM (
			SELECT COALESCE(cc.normalized_rule->>'id',cc.proposal_rule_id) AS rule_id,cc.clause_kind,cc.review_status,COALESCE(cc.review_reason_code,'') AS review_reason_code,COALESCE(cc.review_reason,'') AS review_reason,
				COALESCE(reviewer.email,cc.reviewed_by_id,'') AS reviewer,cc.reviewed_at,cc.created_at
			FROM contract_clause_candidates cc LEFT JOIN auth_users reviewer ON reviewer.id=cc.reviewed_by_id
			WHERE cc.document_id=$1 AND cc.dossier_id=$2) clauses
		WHERE rule_id IS NOT NULL
		ORDER BY rule_id,COALESCE(reviewed_at,created_at) DESC`, documentID, dossierID.String)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var clause commercialvalidation.DocumentClauseState
		var reviewedAt sql.NullTime
		if err = rows.Scan(&clause.RuleID, &clause.Kind, &clause.Status, &clause.ReasonCode, &clause.Reason, &clause.ReviewedBy, &reviewedAt); err != nil {
			return nil, err
		}
		if reviewedAt.Valid {
			value := reviewedAt.Time
			clause.ReviewedAt = &value
		}
		if _, enforced := active[clause.RuleID]; enforced && clause.Status == "CONFIRMED" {
			state.ActiveRuleIDs = append(state.ActiveRuleIDs, clause.RuleID)
		}
		state.Clauses = append(state.Clauses, clause)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	var confirmed contractingestion.ReviewedContract
	var proposal contractingestion.Proposal
	if json.Unmarshal(confirmedJSON, &confirmed) != nil || json.Unmarshal(proposalJSON, &proposal) != nil || !contractID.Valid {
		return &state, nil
	}
	state.Services = serviceStates(contractID.String, documentID, confirmed.ServiceTerms, proposal.ServiceTerms, rules)
	return &state, nil
}

// serviceStates tells, per confirmed service tariff, whether invoice lines
// are checked against it and otherwise why not, with the same checks the
// activation applies, so the reason is visible before anyone tries.
func serviceStates(contractID, documentID string, terms []contractingestion.ReviewedServiceTerm, proposed []contractingestion.ProposedServiceTerm, snapshot []commercialvalidation.Rule) []commercialvalidation.DocumentServiceState {
	enforced := make(map[string]bool, len(snapshot))
	pricedByClauses := false
	for _, rule := range snapshot {
		enforced[rule.ID] = true
		if (rule.Kind == commercialvalidation.RuleFixedPrice || rule.Kind == commercialvalidation.RuleUnitRate || rule.Kind == commercialvalidation.RuleTieredPrice) && !strings.HasPrefix(rule.ID, "service-") {
			pricedByClauses = true
		}
	}
	candidates, skipped := reviewedServicePriceRules(documentID, contractID, terms, proposed)
	built := make(map[int]string, len(candidates))
	for _, rule := range candidates {
		built[servicePosition(contractID, rule.ID, len(terms))] = rule.ID
	}
	reasons := make(map[int]string, len(skipped))
	for _, item := range skipped {
		reasons[item.Position] = item.Reason
	}
	result := make([]commercialvalidation.DocumentServiceState, 0, len(terms))
	for index := range terms {
		position := index + 1
		ruleID := stableID("service", fmt.Sprintf("%s:%d", contractID, index))
		item := commercialvalidation.DocumentServiceState{Position: position}
		switch {
		case enforced[ruleID]:
			item.RuleID, item.Active = ruleID, true
		case reasons[position] != "":
			item.SkipReason = reasons[position]
		case pricedByClauses:
			item.SkipReason = commercialvalidation.SkipPricedByClauses
		case built[position] != "":
			item.RuleID = built[position]
		}
		result = append(result, item)
	}
	return result
}

// servicePosition maps a service rule id back to the 1-based position of the
// reviewed term it was built from.
func servicePosition(contractID, ruleID string, count int) int {
	for index := 0; index < count; index++ {
		if stableID("service", fmt.Sprintf("%s:%d", contractID, index)) == ruleID {
			return index + 1
		}
	}
	return 0
}

// ConfirmedDocumentsWithCoverableClauses lists confirmed documents that still
// have proposed contract-reference, identity or pricing clauses, for the
// one-off backfill of documents confirmed before such clauses were closed.
func (s *Store) ConfirmedDocumentsWithCoverableClauses(ctx context.Context) ([]SourceTextBackfillTarget, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT DISTINCT doc.client_id,doc.id,doc.confirmed_by_id,COALESCE(doc.confirmed_by_display,'')
		FROM contract_source_documents doc
		JOIN contract_clause_candidates cc ON cc.document_id=doc.id
		WHERE doc.status='CONFIRMED' AND doc.confirmed_by_id IS NOT NULL
		  AND cc.review_status='PROPOSED' AND cc.clause_kind IN ('CONTRACT_REFERENCE','IDENTITY','FIXED_PRICE','UNIT_RATE')
		ORDER BY doc.client_id,doc.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []SourceTextBackfillTarget
	for rows.Next() {
		var item SourceTextBackfillTarget
		if err = rows.Scan(&item.ClientID, &item.DocumentID, &item.ActorID, &item.ActorDisplay); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// saleDossier reports a dossier whose contract has the client as supplier
// (D-126); its buyer is the counterparty, not the client.
func saleDossier(ctx context.Context, tx *sql.Tx, dossierID string) bool {
	var sale bool
	_ = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM contract_dossiers d JOIN contracts c ON c.id=d.contract_id WHERE d.id=$1 AND c.client_role='SUPPLIER')`, dossierID).Scan(&sale)
	return sale
}
