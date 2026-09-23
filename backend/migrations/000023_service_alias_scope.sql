ALTER TABLE contract_service_aliases ADD COLUMN invoice_id text REFERENCES invoices(id);

ALTER TABLE invoice_commercial_findings
  ADD COLUMN service_candidates jsonb NOT NULL DEFAULT '[]'::jsonb;

DROP INDEX contract_service_aliases_dossier_label_key;

CREATE UNIQUE INDEX contract_service_aliases_dossier_label_key
  ON contract_service_aliases(dossier_id, normalized_label)
  WHERE dossier_id IS NOT NULL AND invoice_id IS NULL;

CREATE UNIQUE INDEX contract_service_aliases_invoice_label_key
  ON contract_service_aliases(dossier_id, invoice_id, normalized_label)
  WHERE invoice_id IS NOT NULL;

CREATE TABLE invoice_commercial_date_facts (
  id text PRIMARY KEY,
  invoice_id text NOT NULL REFERENCES invoices(id),
  kind text NOT NULL CHECK (kind IN ('REMITTANCE','RECEIPT','ACCEPTANCE')),
  occurred_on date NOT NULL,
  source_reference text NOT NULL CHECK (length(trim(source_reference)) > 0),
  recorded_by_id text NOT NULL,
  recorded_at timestamptz NOT NULL,
  command_key text NOT NULL UNIQUE
);
CREATE INDEX invoice_commercial_date_facts_lookup_idx
  ON invoice_commercial_date_facts(invoice_id,kind,recorded_at DESC);
