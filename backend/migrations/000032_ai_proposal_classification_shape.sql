-- AI proposals are persisted as review-only decisions without a rule or account mapping.
ALTER TABLE line_classifications DROP CONSTRAINT line_classifications_rule_shape;
ALTER TABLE line_classifications ADD CONSTRAINT line_classifications_rule_shape CHECK (
    (source='RULE' AND rule_version_id IS NOT NULL AND account_mapping_id IS NULL)
    OR (source='LEARNED_MAPPING' AND (
        (dimension='ACCOUNT' AND account_mapping_id IS NOT NULL AND account_mapping_version IS NOT NULL)
        OR (
            dimension<>'ACCOUNT'
            AND account_mapping_id IS NULL
            AND account_mapping_version IS NULL
            AND proposal_provenance->>'knowledgeId' IS NOT NULL
            AND proposal_provenance->>'knowledgeVersion' IS NOT NULL
        )
    ))
    OR (source='AI_PROPOSAL' AND model_version='ACCOUNTING_DOMAIN_V2' AND rule_version_id IS NULL AND account_mapping_id IS NULL AND account_mapping_version IS NULL)
    OR (source='NO_MATCH' AND rule_version_id IS NULL AND account_mapping_id IS NULL)
    OR (source='AMBIGUOUS')
);
