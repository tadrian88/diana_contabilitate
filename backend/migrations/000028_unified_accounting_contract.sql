-- Chapter B: AI results become granular proposals on canonical line classifications.
ALTER TABLE line_classifications
	ADD COLUMN effective_source text CHECK(effective_source IS NULL OR effective_source IN ('RULE','LEARNED_MAPPING','AI_PROPOSAL','MANUAL')),
    ADD COLUMN legal_citations jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK(jsonb_typeof(legal_citations)='array'),
    ADD COLUMN validation_results jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK(jsonb_typeof(validation_results)='array'),
    ADD COLUMN proposal_provenance jsonb
        CHECK(proposal_provenance IS NULL OR jsonb_typeof(proposal_provenance)='object');

UPDATE line_classifications
SET effective_source=CASE WHEN reviewed_at IS NOT NULL THEN 'MANUAL' WHEN source='RULE' THEN 'RULE' ELSE 'MANUAL' END
WHERE effective_value IS NOT NULL;

ALTER TABLE line_classifications DROP CONSTRAINT line_classifications_source_check;
ALTER TABLE line_classifications ADD CONSTRAINT line_classifications_source_check
    CHECK(source IN ('RULE','NO_MATCH','AMBIGUOUS','LEARNED_MAPPING','AI_PROPOSAL'));

ALTER TABLE line_classifications DROP CONSTRAINT line_classifications_review_status_check;
ALTER TABLE line_classifications ADD CONSTRAINT line_classifications_review_status_check
    CHECK(review_status IN ('PENDING','ACCEPTED','CORRECTED','REJECTED'));

ALTER TABLE accounting_analysis_runs DROP CONSTRAINT accounting_analysis_runs_status_check;
ALTER TABLE accounting_analysis_runs ADD CONSTRAINT accounting_analysis_runs_status_check
    CHECK(status IN ('RUNNING','PROPOSED','PARTIAL_VALIDATION','VALIDATION_FAILED','PROVIDER_FAILED'));

COMMENT ON COLUMN accounting_analysis_runs.raw_structured_response IS
    'Immutable provider artifact. V2 is non-authoritative; canonical current proposals live on line_classifications.';
