-- Additive correction. Legacy instructions and artifacts are never rewritten.
ALTER TABLE invoices ADD COLUMN model_version text NOT NULL DEFAULT 'LEGACY_V1',
 ADD COLUMN source_facts jsonb, ADD COLUMN accounting_snapshot jsonb,
 ADD COLUMN readiness_reason text NOT NULL DEFAULT '';
ALTER TABLE invoice_lines ADD COLUMN source_facts jsonb;
ALTER TABLE line_classifications ADD COLUMN model_version text NOT NULL DEFAULT 'LEGACY_V1',
 ADD COLUMN proposed_typed_value jsonb, ADD COLUMN effective_typed_value jsonb,
 ADD COLUMN decision_evidence jsonb, ADD COLUMN review_reason text NOT NULL DEFAULT '';
ALTER TABLE rule_versions ADD COLUMN model_version text NOT NULL DEFAULT 'LEGACY_V1', ADD COLUMN domain_rule jsonb;
ALTER TABLE classification_rules DROP CONSTRAINT classification_rules_category_check;
ALTER TABLE classification_rules ADD CONSTRAINT classification_rules_category_check CHECK
 (category IN ('ACCOUNT','VAT','DEDUCTIBILITY','VAT_TREATMENT','VAT_DEDUCTIBILITY','EXPENSE_TAX_TREATMENT'));
ALTER TABLE line_classifications DROP CONSTRAINT line_classifications_dimension_check;
ALTER TABLE line_classifications ADD CONSTRAINT line_classifications_dimension_check CHECK
 (dimension IN ('ACCOUNT','VAT','DEDUCTIBILITY','VAT_TREATMENT','VAT_DEDUCTIBILITY','EXPENSE_TAX_TREATMENT'));
ALTER TABLE line_classifications DROP CONSTRAINT line_classifications_line_dimension_key;
ALTER TABLE line_classifications ADD CONSTRAINT line_classifications_line_dimension_model_key UNIQUE(invoice_line_id,dimension,model_version);
ALTER TABLE line_classifications ADD CONSTRAINT line_classifications_domain_shape CHECK
 (model_version IN ('LEGACY_V1','ACCOUNTING_DOMAIN_V2') AND
 ((model_version='LEGACY_V1' AND dimension IN ('ACCOUNT','VAT','DEDUCTIBILITY')) OR (model_version='ACCOUNTING_DOMAIN_V2' AND dimension IN ('ACCOUNT','VAT_TREATMENT','VAT_DEDUCTIBILITY','EXPENSE_TAX_TREATMENT') AND
 (review_status='PENDING' OR (COALESCE(jsonb_typeof(effective_typed_value)='object',false) AND effective_typed_value->>'kind' IS NOT NULL)))));
CREATE TABLE client_accounting_profiles (
 id text PRIMARY KEY, client_id text NOT NULL REFERENCES clients(id), version integer NOT NULL CHECK(version>0),
 payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object'), created_at timestamptz NOT NULL,
 UNIQUE(client_id,version), CHECK(COALESCE(payload->>'id'=id AND payload->>'clientId'=client_id AND (payload->>'version')::integer=version,false))
);
CREATE TABLE accounting_rule_packs (
 id text PRIMARY KEY, client_id text NOT NULL REFERENCES clients(id), version integer NOT NULL CHECK(version>0),
 payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object'), created_at timestamptz NOT NULL,
 UNIQUE(client_id,version), CHECK(COALESCE(payload->>'id'=id AND payload->>'clientId'=client_id AND (payload->>'version')::integer=version,false))
);
CREATE TRIGGER client_accounting_profile_immutable BEFORE UPDATE ON client_accounting_profiles FOR EACH ROW EXECUTE FUNCTION reject_rule_version_update();
CREATE TRIGGER accounting_rule_pack_immutable BEFORE UPDATE ON accounting_rule_packs FOR EACH ROW EXECUTE FUNCTION reject_rule_version_update();
-- Prevent source reinterpretation and snapshot replacement after classification.
CREATE FUNCTION protect_accounting_source() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.source_facts IS DISTINCT FROM OLD.source_facts THEN RAISE EXCEPTION 'Source facts are immutable; explicit versioned reparse required'; END IF;
 IF TG_TABLE_NAME='invoices' THEN
  IF NEW.model_version IS DISTINCT FROM OLD.model_version THEN RAISE EXCEPTION 'Domain model version is immutable'; END IF;
  IF OLD.accounting_snapshot IS NOT NULL AND NEW.accounting_snapshot IS DISTINCT FROM OLD.accounting_snapshot THEN RAISE EXCEPTION 'Classification snapshot is immutable'; END IF;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER invoice_accounting_source_immutable BEFORE UPDATE ON invoices FOR EACH ROW EXECUTE FUNCTION protect_accounting_source();
CREATE TRIGGER line_accounting_source_immutable BEFORE UPDATE ON invoice_lines FOR EACH ROW EXECUTE FUNCTION protect_accounting_source();
-- New-model corrections can revisit automatic decisions while a mapping blocker
-- remains open. Legacy review-shape semantics remain unchanged.
ALTER TABLE line_classifications DROP CONSTRAINT line_classifications_review_shape;
ALTER TABLE line_classifications ADD CONSTRAINT line_classifications_review_shape CHECK (
 ((required_review OR model_version='ACCOUNTING_DOMAIN_V2') AND review_status='PENDING' AND effective_value IS NULL AND reviewed_at IS NULL)
 OR (review_status='ACCEPTED' AND NOT required_review AND effective_value IS NOT NULL AND reviewed_at IS NULL)
 OR ((required_review OR model_version='ACCOUNTING_DOMAIN_V2') AND review_status IN ('ACCEPTED','CORRECTED') AND effective_value IS NOT NULL AND reviewed_at IS NOT NULL AND reviewed_by_display IS NOT NULL AND length(btrim(reviewed_by_display))>0)
);
CREATE FUNCTION protect_accounting_release_delete() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT COALESCE((OLD.payload->>'testOnly')::boolean,false) THEN RAISE EXCEPTION 'Approved accounting release/profile cannot be deleted'; END IF;
 RETURN OLD;
END; $$;
CREATE TRIGGER accounting_pack_delete_guard BEFORE DELETE ON accounting_rule_packs FOR EACH ROW EXECUTE FUNCTION protect_accounting_release_delete();
CREATE TRIGGER accounting_profile_delete_guard BEFORE DELETE ON client_accounting_profiles FOR EACH ROW EXECUTE FUNCTION protect_accounting_release_delete();
