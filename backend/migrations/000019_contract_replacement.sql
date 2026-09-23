-- Preserve history while allowing a discarded mistaken upload to be ingested
-- again and an archived contract to be replaced under the same reference.
ALTER TABLE contracts DROP CONSTRAINT contracts_client_reference_key;
CREATE UNIQUE INDEX contracts_client_reference_active_idx
  ON contracts(client_id, reference) WHERE lifecycle_state = 'ACTIVE';

DROP INDEX contract_source_documents_client_id_sha256;
CREATE UNIQUE INDEX contract_source_documents_client_sha_active_idx
  ON contract_source_documents(client_id, sha256) WHERE lifecycle_state <> 'DISCARDED';

ALTER TABLE contract_dossiers DROP CONSTRAINT contract_dossiers_client_id_primary_reference_key;
CREATE UNIQUE INDEX contract_dossiers_client_reference_active_idx
  ON contract_dossiers(client_id, primary_reference) WHERE status <> 'ARCHIVED';
