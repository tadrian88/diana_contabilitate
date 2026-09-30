-- A proposed commercial clause can be closed without becoming a rule: it is
-- covered by the dossier identity (contract reference, supplier CUI) or the
-- reviewer states, with a reason, that it is not checked on invoices. The row
-- keeps why it was closed. Narrative-only proposals are stored without a
-- normalized rule, so they also keep the id of the proposal they came from.
ALTER TABLE contract_clause_candidates
  ADD COLUMN proposal_rule_id text,
  ADD COLUMN review_reason_code text,
  ADD COLUMN review_reason text;

UPDATE contract_clause_candidates cc
SET proposal_rule_id = proposed->'rule'->>'id'
FROM contract_extraction_attempts attempt,
  jsonb_array_elements(COALESCE(attempt.proposal->'commercialClauses', '[]'::jsonb)) proposed
WHERE attempt.id = cc.extraction_attempt_id
  AND cc.proposal_rule_id IS NULL
  AND cc.normalized_rule IS NULL
  AND proposed->'kind'->>'value' = cc.clause_kind
  AND proposed->'evidence'->>'snippet' = cc.source_snippet
  AND (proposed->'rule'->>'narrative' = cc.narrative OR proposed->'narrative'->>'value' = cc.narrative);

UPDATE contract_clause_candidates
SET review_reason_code = 'COVERED_BY_CONTRACT_REFERENCE'
WHERE review_status = 'REJECTED' AND clause_kind = 'CONTRACT_REFERENCE' AND review_reason_code IS NULL;
