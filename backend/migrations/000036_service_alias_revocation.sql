-- A learned service association can be revoked. The row stays as history;
-- a revoked association no longer maps invoice lines and no longer blocks the
-- same wording from being associated with another service.
ALTER TABLE contract_service_aliases
  ADD COLUMN revoked_at timestamptz,
  ADD COLUMN revoked_by_id text,
  ADD COLUMN revoke_command_key text;

DROP INDEX contract_service_aliases_dossier_label_key;

CREATE UNIQUE INDEX contract_service_aliases_dossier_label_key
  ON contract_service_aliases(dossier_id, normalized_label)
  WHERE dossier_id IS NOT NULL AND invoice_id IS NULL AND revoked_at IS NULL;

DROP INDEX contract_service_aliases_invoice_label_key;

CREATE UNIQUE INDEX contract_service_aliases_invoice_label_key
  ON contract_service_aliases(dossier_id, invoice_id, normalized_label)
  WHERE invoice_id IS NOT NULL AND revoked_at IS NULL;
