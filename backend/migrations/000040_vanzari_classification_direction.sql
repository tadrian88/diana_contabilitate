-- Vânzări V1 (D-129). On an issued invoice VAT_DEDUCTIBILITY and
-- EXPENSE_TAX_TREATMENT are not applicable by definition: they become final
-- DIRECTION-sourced decisions carrying profile evidence only, never a rule or
-- mapping (the PROFILE pattern of 000033).
ALTER TABLE line_classifications DROP CONSTRAINT line_classifications_source_check;
ALTER TABLE line_classifications ADD CONSTRAINT line_classifications_source_check
    CHECK(source IN ('RULE','NO_MATCH','AMBIGUOUS','LEARNED_MAPPING','AI_PROPOSAL','PROFILE','DIRECTION'));

ALTER TABLE line_classifications DROP CONSTRAINT line_classifications_effective_source_check;
ALTER TABLE line_classifications ADD CONSTRAINT line_classifications_effective_source_check
    CHECK(effective_source IS NULL OR effective_source IN ('RULE','LEARNED_MAPPING','AI_PROPOSAL','MANUAL','PROFILE','DIRECTION'));

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
    OR (source='PROFILE' AND model_version='ACCOUNTING_DOMAIN_V2' AND dimension='EXPENSE_TAX_TREATMENT' AND rule_version_id IS NULL AND account_mapping_id IS NULL AND account_mapping_version IS NULL)
    OR (source='DIRECTION' AND model_version='ACCOUNTING_DOMAIN_V2' AND dimension IN ('VAT_DEDUCTIBILITY','EXPENSE_TAX_TREATMENT') AND rule_version_id IS NULL AND account_mapping_id IS NULL AND account_mapping_version IS NULL)
    OR (source='NO_MATCH' AND rule_version_id IS NULL AND account_mapping_id IS NULL)
    OR (source='AMBIGUOUS')
);

-- Learned account mappings and approved knowledge are scoped by direction:
-- normalized_supplier_id now holds the normalized counterparty (the customer
-- of an issued invoice). Every existing row is a purchase (INCOMING), so its
-- key does not change.
ALTER TABLE account_mappings
  ADD COLUMN direction text NOT NULL DEFAULT 'INCOMING',
  ADD CONSTRAINT account_mappings_direction_check CHECK (direction IN ('INCOMING', 'OUTGOING'));

DO $$
DECLARE legacy_key text;
BEGIN
  SELECT conname INTO legacy_key FROM pg_constraint
   WHERE conrelid='account_mappings'::regclass AND contype='u'
     AND pg_get_constraintdef(oid) LIKE 'UNIQUE (client_id, normalized_supplier_id, service_identity_kind, service_identity_value, normalizer_version)%';
  IF legacy_key IS NOT NULL THEN
    EXECUTE format('ALTER TABLE account_mappings DROP CONSTRAINT %I', legacy_key);
  END IF;
END $$;
CREATE UNIQUE INDEX account_mappings_scope_key ON account_mappings
  (client_id, direction, normalized_supplier_id, service_identity_kind, service_identity_value, normalizer_version);
DROP INDEX account_mappings_lookup_idx;
CREATE INDEX account_mappings_lookup_idx ON account_mappings (client_id, direction, normalized_supplier_id, status);

ALTER TABLE approved_accounting_knowledge
  ADD COLUMN direction text NOT NULL DEFAULT 'INCOMING',
  ADD CONSTRAINT approved_accounting_knowledge_direction_check CHECK (direction IN ('INCOMING', 'OUTGOING'));
DROP INDEX approved_knowledge_match_idx;
CREATE INDEX approved_knowledge_match_idx ON approved_accounting_knowledge
  (client_id, direction, normalized_supplier_id, dimension, status);

CREATE FUNCTION reject_scope_direction_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.direction IS DISTINCT FROM OLD.direction THEN
    RAISE EXCEPTION 'Learning scope direction is immutable';
  END IF;
  RETURN NEW;
END; $$;
CREATE TRIGGER account_mapping_direction_immutable BEFORE UPDATE ON account_mappings
  FOR EACH ROW EXECUTE FUNCTION reject_scope_direction_update();
CREATE TRIGGER approved_knowledge_direction_immutable BEFORE UPDATE ON approved_accounting_knowledge
  FOR EACH ROW EXECUTE FUNCTION reject_scope_direction_update();
