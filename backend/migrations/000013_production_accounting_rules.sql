-- Existing history is unverified. Never infer eligibility from old legal_basis.
ALTER TABLE rule_versions
    ADD COLUMN production_eligible boolean NOT NULL DEFAULT false,
    ADD COLUMN rule_pack_version text NOT NULL DEFAULT '',
    ADD COLUMN provenance jsonb;
ALTER TABLE line_classifications ADD COLUMN invoice_date_used date;

ALTER TABLE rule_versions DROP CONSTRAINT rule_versions_match_kind_check;
ALTER TABLE rule_versions DROP CONSTRAINT rule_versions_match_shape;
ALTER TABLE rule_versions ADD CONSTRAINT rule_versions_match_kind_check CHECK
    (match_kind IN ('DESCRIPTION_CONTAINS', 'ALWAYS', 'NO_AUTOMATION', 'VAT_SOURCE_RATE_EQUALS'));
ALTER TABLE rule_versions ADD CONSTRAINT rule_versions_match_shape CHECK (
    (match_kind IN ('DESCRIPTION_CONTAINS', 'VAT_SOURCE_RATE_EQUALS') AND match_value IS NOT NULL AND length(btrim(match_value)) > 0)
    OR (match_kind IN ('ALWAYS', 'NO_AUTOMATION') AND match_value IS NULL)
);
ALTER TABLE rule_versions ADD CONSTRAINT rule_versions_production_provenance CHECK (
    NOT production_eligible OR COALESCE((
        length(btrim(rule_pack_version)) > 0
        AND legal_basis NOT LIKE '%Exemplu demonstrativ — bază legală nevalidată%'
        AND jsonb_typeof(provenance) = 'object'
        AND provenance->>'sourceType' IN ('LEGISLATION', 'ACCOUNTING_REGULATION', 'CLIENT_ACCOUNTING_POLICY')
        AND length(btrim(provenance->>'sourceTitle')) > 0
        AND length(btrim(provenance->>'issuer')) > 0
        AND length(btrim(provenance->>'legalInstrument')) > 0
        AND length(btrim(provenance->>'reference')) > 0
        AND length(btrim(provenance->>'verifiedBy')) > 0
        AND length(btrim(provenance->>'notes')) > 0
        AND provenance->>'sourceURL' ~ '^https://(legislatie\.just\.ro|static\.anaf\.ro|www\.anaf\.ro|mfinante\.gov\.ro|www\.mfinante\.gov\.ro)/'
        AND (provenance->>'effectiveFrom')::date <= effective_from
        AND (provenance->>'verifiedAt')::date IS NOT NULL
        AND (provenance->>'effectiveTo' IS NULL OR
            (effective_to IS NOT NULL AND effective_to <= (provenance->>'effectiveTo')::date))
    ), false)
);

-- Updates cannot retrospectively verify, change triggers or rewrite provenance.
-- Deletes retain existing FK protection; privileged fixture cleanup is unchanged.
CREATE FUNCTION reject_rule_version_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'RuleVersion is immutable; insert a new version';
END;
$$;
CREATE TRIGGER rule_version_immutable BEFORE UPDATE ON rule_versions
    FOR EACH ROW EXECUTE FUNCTION reject_rule_version_update();

-- Overlaps are intentionally permitted as history but never selected by recency:
-- the production evaluator routes overlapping applicable versions to review.
CREATE INDEX rule_versions_production_period_idx ON rule_versions (rule_id, effective_from, effective_to)
    WHERE production_eligible;
