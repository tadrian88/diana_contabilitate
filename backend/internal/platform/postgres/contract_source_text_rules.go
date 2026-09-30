package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/commercialvalidation"
	"diana-contabilitate/backend/internal/contractingestion"
)

// annotateSourceTextRecognition tells the review UI which pending clauses will
// be confirmed automatically, using the same checks as AutoConfirm.
func annotateSourceTextRecognition(proposal *contractingestion.Proposal) {
	for index, clause := range proposal.CommercialClauses {
		var rule commercialvalidation.Rule
		if json.Unmarshal(clause.Rule, &rule) != nil {
			continue
		}
		recognized, ok := commercialvalidation.RecognizeSourceTextRule(rule, clause.Evidence.Snippet)
		if ok && reviewedRuleAllowed(recognized) && reviewedLiteralSupportedBySource(recognized, clause.Evidence.Snippet) {
			proposal.CommercialClauses[index].RecognizedRule = &recognized
		}
	}
}

// AutoConfirmSourceTextClauses confirms pending narrative-only VAT and
// payment-term clauses whose executable value is stated unambiguously in the
// cited clause. Each goes through ConfirmProposedRule, so the same
// source-support and proposal-preservation checks as a manual confirmation
// apply. Clauses that fail recognition or verification stay in human review.
// It is idempotent per clause and returns how many clauses were confirmed.
func (s *Store) AutoConfirmSourceTextClauses(ctx context.Context, clientID, documentID, actorID, actorDisplay string, now time.Time) (int, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT DISTINCT ON (proposed->'rule'->>'id') cc.id,cc.source_snippet,proposed->'rule'
		FROM contract_clause_candidates cc
		JOIN contract_source_documents doc ON doc.id=cc.document_id
		JOIN contract_extraction_attempts attempt ON attempt.id=cc.extraction_attempt_id,
		jsonb_array_elements(attempt.proposal->'commercialClauses') proposed
		WHERE doc.client_id=$1 AND cc.document_id=$2 AND cc.review_status='PROPOSED'
		  AND cc.clause_kind IN ('VAT','PAYMENT_DUE')
		  AND proposed->'kind'->>'value'=cc.clause_kind
		  AND proposed->'narrative'->>'value'=cc.narrative
		  AND proposed->'evidence'->>'snippet'=cc.source_snippet
		ORDER BY proposed->'rule'->>'id',cc.created_at DESC`, clientID, documentID)
	if err != nil {
		return 0, err
	}
	type candidate struct {
		clauseID string
		rule     commercialvalidation.Rule
	}
	var candidates []candidate
	for rows.Next() {
		var clauseID, snippet string
		var ruleJSON []byte
		if err = rows.Scan(&clauseID, &snippet, &ruleJSON); err != nil {
			rows.Close()
			return 0, err
		}
		var proposed commercialvalidation.Rule
		if json.Unmarshal(ruleJSON, &proposed) != nil {
			continue
		}
		recognized, ok := commercialvalidation.RecognizeSourceTextRule(proposed, snippet)
		if !ok || !reviewedRuleAllowed(recognized) || !reviewedLiteralSupportedBySource(recognized, snippet) {
			continue
		}
		candidates = append(candidates, candidate{clauseID: clauseID, rule: recognized})
	}
	if err = rows.Close(); err != nil {
		return 0, err
	}
	confirmed := 0
	for _, item := range candidates {
		rule := item.rule
		changed, confirmErr := s.ConfirmProposedRule(ctx, commercialvalidation.RuleConfirmation{
			ClientID: clientID, DocumentID: documentID, RuleID: rule.ID, CommandID: "source-text:" + item.clauseID,
			ActorID: actorID, ActorDisplay: actorDisplay, Rule: &rule, Automatic: true,
		}, now)
		if errors.Is(confirmErr, apperrors.ErrValidation) || errors.Is(confirmErr, apperrors.ErrConflict) || errors.Is(confirmErr, apperrors.ErrNotFound) {
			continue
		}
		if confirmErr != nil {
			return confirmed, confirmErr
		}
		if changed {
			confirmed++
		}
	}
	return confirmed, nil
}

// SourceTextBackfillTarget is a confirmed document with pending VAT or
// payment-term clauses; its confirmer is recorded as the automatic actor.
type SourceTextBackfillTarget struct {
	ClientID, DocumentID, ActorID, ActorDisplay string
}

// ConfirmedDocumentsWithSourceTextCandidates lists documents confirmed before
// source-text recognition existed, for the one-off backfill.
func (s *Store) ConfirmedDocumentsWithSourceTextCandidates(ctx context.Context) ([]SourceTextBackfillTarget, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT DISTINCT doc.client_id,doc.id,doc.confirmed_by_id,COALESCE(doc.confirmed_by_display,'')
		FROM contract_source_documents doc
		JOIN contract_clause_candidates cc ON cc.document_id=doc.id
		WHERE doc.status='CONFIRMED' AND doc.confirmed_by_id IS NOT NULL
		  AND cc.review_status='PROPOSED' AND cc.clause_kind IN ('VAT','PAYMENT_DUE')
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
