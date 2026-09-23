ALTER TABLE contract_service_aliases
  ADD COLUMN dossier_id text REFERENCES contract_dossiers(id);

DO $$
DECLARE old_unique text;
BEGIN
  SELECT conname INTO old_unique FROM pg_constraint
  WHERE conrelid = 'contract_service_aliases'::regclass
    AND contype = 'u'
    AND pg_get_constraintdef(oid) = 'UNIQUE (client_id, supplier_cui, service_id, normalized_label)';
  IF old_unique IS NOT NULL THEN
    EXECUTE format('ALTER TABLE contract_service_aliases DROP CONSTRAINT %I', old_unique);
  END IF;
END $$;

-- Legacy supplier-wide aliases remain in the audit table, but are not applied
-- without an explicit dossier-scoped confirmation.
CREATE UNIQUE INDEX contract_service_aliases_dossier_label_key
  ON contract_service_aliases(dossier_id, normalized_label) WHERE dossier_id IS NOT NULL;
