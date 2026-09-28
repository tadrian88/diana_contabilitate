-- Chapter E: explicitly promoted, client-scoped accounting knowledge.
-- ACCOUNT remains in the existing account_mappings model.  This table stores
-- only the other independent accounting dimensions; legislation fragments
-- remain evidence and are never executable rules.
-- Extend the dormant Chapter-D evidence model instead of creating a second
-- knowledge aggregate. Existing rows remain historical and are not silently
-- made runtime-reusable: only rows with the new explicit dimension/scope
-- columns populated are selected by Chapter E.
ALTER TABLE approved_accounting_knowledge DROP CONSTRAINT approved_accounting_knowledge_status_check;
ALTER TABLE approved_accounting_knowledge ADD CONSTRAINT approved_accounting_knowledge_status_check
  CHECK(status IN ('ACTIVE','STALE','INACTIVE','REVOKED'));
ALTER TABLE approved_accounting_knowledge ALTER COLUMN source_review_id DROP NOT NULL;
ALTER TABLE approved_accounting_knowledge
  ADD COLUMN version integer NOT NULL DEFAULT 1 CHECK(version > 0),
  ADD COLUMN dimension text CHECK(dimension IN ('VAT_TREATMENT','VAT_DEDUCTIBILITY','EXPENSE_TAX_TREATMENT')),
  ADD COLUMN approved_value_hash text,
  ADD COLUMN normalizer_version text,
  ADD COLUMN currency text,
  ADD COLUMN document_type text CHECK(document_type IS NULL OR document_type IN ('INVOICE','CREDIT_NOTE')),
  ADD COLUMN vat_rate numeric(20,4),
  ADD COLUMN profile_version integer,
  ADD COLUMN source_invoice_id text REFERENCES invoices(id),
  ADD COLUMN source_invoice_line_id text REFERENCES invoice_lines(id),
  ADD COLUMN source_classification_id text REFERENCES line_classifications(id),
  ADD COLUMN source_classification_run_id text REFERENCES classification_runs(id),
  ADD COLUMN source_classification_revision bigint,
  ADD COLUMN original_source text,
  ADD COLUMN original_provenance jsonb,
  ADD COLUMN promoted_by_id text,
  ADD COLUMN promoted_by_display text,
  ADD COLUMN promoted_at timestamptz,
  ADD COLUMN stale_reason text,
  ADD COLUMN revision bigint NOT NULL DEFAULT 1 CHECK(revision > 0),
  ADD COLUMN supersedes_id text REFERENCES approved_accounting_knowledge(id),
  ADD COLUMN identity_hash text,
  ADD COLUMN command_key text UNIQUE,
  ADD COLUMN revoked_by_id text,
  ADD COLUMN revoked_by_display text,
  ADD COLUMN revoked_at timestamptz,
  ADD COLUMN updated_at timestamptz;
ALTER TABLE approved_accounting_knowledge ADD CONSTRAINT approved_knowledge_runtime_shape CHECK (
  dimension IS NULL OR (
    approved_value_hash IS NOT NULL AND normalizer_version IS NOT NULL AND currency IS NOT NULL
    AND document_type IS NOT NULL AND vat_rate IS NOT NULL AND profile_version IS NOT NULL
    AND source_invoice_id IS NOT NULL AND source_invoice_line_id IS NOT NULL
    AND source_classification_id IS NOT NULL AND source_classification_run_id IS NOT NULL
    AND source_classification_revision IS NOT NULL AND original_source IS NOT NULL
    AND promoted_by_display IS NOT NULL AND promoted_at IS NOT NULL AND identity_hash IS NOT NULL
    AND command_key IS NOT NULL AND updated_at IS NOT NULL
  )
);
ALTER TABLE approved_accounting_knowledge ADD CONSTRAINT approved_knowledge_id_version_key UNIQUE(id,version);
-- The original key prevented a replacement decision for the same identity
-- and date even after the old row was revoked. Chapter E deduplicates active
-- knowledge with the narrower partial index below and retains successors.
DO $$
DECLARE legacy_key text;
BEGIN
  SELECT conname INTO legacy_key FROM pg_constraint
   WHERE conrelid='approved_accounting_knowledge'::regclass AND contype='u'
     AND pg_get_constraintdef(oid) LIKE 'UNIQUE (client_id, normalized_supplier_id, semantic_kind, semantic_value, profile_id, effective_from)%';
  IF legacy_key IS NOT NULL THEN
    EXECUTE format('ALTER TABLE approved_accounting_knowledge DROP CONSTRAINT %I', legacy_key);
  END IF;
END $$;
CREATE INDEX approved_knowledge_match_idx ON approved_accounting_knowledge
  (client_id, normalized_supplier_id, dimension, status);
CREATE UNIQUE INDEX approved_knowledge_active_dedup_idx ON approved_accounting_knowledge
  (client_id, dimension, identity_hash, approved_value_hash) WHERE status='ACTIVE' AND dimension IS NOT NULL;

CREATE FUNCTION reject_approved_knowledge_identity_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.id <> OLD.id OR NEW.version <> OLD.version OR NEW.client_id <> OLD.client_id
     OR NEW.dimension IS DISTINCT FROM OLD.dimension OR NEW.decision <> OLD.decision
     OR NEW.approved_value_hash IS DISTINCT FROM OLD.approved_value_hash
     OR NEW.normalized_supplier_id <> OLD.normalized_supplier_id
     OR NEW.semantic_kind <> OLD.semantic_kind OR NEW.semantic_value <> OLD.semantic_value
     OR NEW.normalizer_version IS DISTINCT FROM OLD.normalizer_version
     OR NEW.currency IS DISTINCT FROM OLD.currency OR NEW.document_type IS DISTINCT FROM OLD.document_type
     OR NEW.vat_rate IS DISTINCT FROM OLD.vat_rate OR NEW.profile_id IS DISTINCT FROM OLD.profile_id
     OR NEW.profile_version IS DISTINCT FROM OLD.profile_version
     OR NEW.legislation_version_ids IS DISTINCT FROM OLD.legislation_version_ids
     OR NEW.source_invoice_id IS DISTINCT FROM OLD.source_invoice_id
     OR NEW.source_invoice_line_id IS DISTINCT FROM OLD.source_invoice_line_id
     OR NEW.source_classification_id IS DISTINCT FROM OLD.source_classification_id
     OR NEW.source_classification_run_id IS DISTINCT FROM OLD.source_classification_run_id
     OR NEW.source_classification_revision IS DISTINCT FROM OLD.source_classification_revision
     OR NEW.original_source IS DISTINCT FROM OLD.original_source
     OR NEW.original_provenance IS DISTINCT FROM OLD.original_provenance
     OR NEW.promoted_by_id IS DISTINCT FROM OLD.promoted_by_id
     OR NEW.promoted_by_display IS DISTINCT FROM OLD.promoted_by_display
     OR NEW.promoted_at IS DISTINCT FROM OLD.promoted_at
     OR NEW.identity_hash IS DISTINCT FROM OLD.identity_hash
     OR NEW.supersedes_id IS DISTINCT FROM OLD.supersedes_id
     OR NEW.command_key IS DISTINCT FROM OLD.command_key
     OR NEW.effective_from IS DISTINCT FROM OLD.effective_from
     OR NEW.created_at IS DISTINCT FROM OLD.created_at
  THEN RAISE EXCEPTION 'Approved knowledge identity and provenance are immutable'; END IF;
  RETURN NEW;
END; $$;
CREATE TRIGGER approved_knowledge_identity_immutable BEFORE UPDATE ON approved_accounting_knowledge
  FOR EACH ROW EXECUTE FUNCTION reject_approved_knowledge_identity_update();
