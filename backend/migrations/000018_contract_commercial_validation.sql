-- Versioned contract dossiers and deterministic commercial invoice validation.
-- This migration is additive. Existing contracts remain matchable with PARTIAL
-- commercial coverage and are never promoted to confirmed executable rules.

ALTER TABLE invoices DROP CONSTRAINT invoices_pipeline_status_check;
ALTER TABLE invoices ADD CONSTRAINT invoices_pipeline_status_check CHECK (pipeline_status IN (
  'DOWNLOADED','ARCHIVED','MATCHING','AWAITING_CONTRACT','AWAITING_MATCH_CONFIRM',
  'DEDUPE_CHECKED','HEADER_READ','LINES_READ','COMMERCIAL_VALIDATING',
  'AWAITING_COMMERCIAL_REVIEW','COMMERCIALLY_VALIDATED','CLASSIFIED',
  'AWAITING_REVIEW','READY_FOR_SAGA','EXPORTING','EXPORTED','DUPLICATE'
));

ALTER TABLE validation_tasks DROP CONSTRAINT validation_tasks_task_type_check;
ALTER TABLE validation_tasks ADD CONSTRAINT validation_tasks_task_type_check
  CHECK (task_type IN ('CONTRACT_MATCH','MISSING_CONTRACT','COMMERCIAL_REVIEW','CLASSIFICATION'));
ALTER TABLE validation_tasks DROP CONSTRAINT validation_tasks_lifecycle_metadata;
ALTER TABLE validation_tasks ADD CONSTRAINT validation_tasks_lifecycle_metadata CHECK (
  (status = 'OPEN' AND waiting_since IS NULL AND resolved_at IS NULL) OR
  (status = 'WAITING' AND task_type IN ('MISSING_CONTRACT','COMMERCIAL_REVIEW') AND waiting_since IS NOT NULL AND resolved_at IS NULL) OR
  (status = 'RESOLVED' AND resolved_at IS NOT NULL)
);

CREATE TABLE contract_dossiers (
  id text PRIMARY KEY,
  client_id text NOT NULL REFERENCES clients(id),
  contract_id text REFERENCES contracts(id),
  supplier_cui text NOT NULL,
  buyer_cui text NOT NULL,
  primary_reference text NOT NULL,
  status text NOT NULL CHECK (status IN ('DRAFT','ACTIVE_PARTIAL','ACTIVE_COMPLETE','ARCHIVED')),
  active_snapshot_id text,
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  creation_key text UNIQUE,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE (client_id, primary_reference),
  UNIQUE (contract_id)
);
CREATE INDEX contract_dossiers_client_status_idx ON contract_dossiers(client_id,status);
CREATE INDEX contract_dossiers_supplier_idx ON contract_dossiers(client_id,supplier_cui);

ALTER TABLE contract_source_documents
  ADD COLUMN dossier_id text REFERENCES contract_dossiers(id),
  ADD COLUMN document_role text,
  ADD COLUMN parent_document_id text REFERENCES contract_source_documents(id),
  ADD COLUMN relationship_metadata jsonb;
ALTER TABLE contract_source_documents ADD CONSTRAINT contract_source_documents_role_check
  CHECK (document_role IS NULL OR document_role IN ('BASE_CONTRACT','ANNEX','AMENDMENT','SOW','ORDER','PRICE_LIST','OTHER'));
CREATE INDEX contract_source_documents_dossier_idx ON contract_source_documents(dossier_id,uploaded_at);
DROP INDEX contract_source_documents_confirmed_contract_id;
CREATE INDEX contract_source_documents_confirmed_contract_id_idx
  ON contract_source_documents(confirmed_contract_id);

CREATE TABLE contract_clause_candidates (
  id text PRIMARY KEY,
  dossier_id text NOT NULL REFERENCES contract_dossiers(id),
  document_id text NOT NULL REFERENCES contract_source_documents(id),
  extraction_attempt_id text NOT NULL REFERENCES contract_extraction_attempts(id),
  clause_kind text NOT NULL,
  narrative text NOT NULL CHECK (length(narrative) > 0),
  normalized_rule jsonb,
  confidence text NOT NULL CHECK (confidence IN ('HIGH','MEDIUM','LOW','UNKNOWN')),
  review_status text NOT NULL CHECK (review_status IN ('PROPOSED','CONFIRMED','REJECTED','CONFLICTED')),
  source_page bigint CHECK (source_page IS NULL OR source_page > 0),
  source_snippet text NOT NULL,
  supersedes_clause_id text REFERENCES contract_clause_candidates(id),
  created_at timestamptz NOT NULL,
  reviewed_at timestamptz,
  reviewed_by_id text
);
CREATE INDEX contract_clause_candidates_dossier_review_idx ON contract_clause_candidates(dossier_id,review_status);
CREATE UNIQUE INDEX contract_clause_candidates_attempt_identity_idx
  ON contract_clause_candidates(extraction_attempt_id,id);

CREATE TABLE contract_commercial_snapshots (
  id text PRIMARY KEY,
  dossier_id text NOT NULL REFERENCES contract_dossiers(id),
  contract_id text NOT NULL REFERENCES contracts(id),
  version bigint NOT NULL CHECK (version > 0),
  schema_version text NOT NULL,
  coverage text NOT NULL CHECK (coverage IN ('COMPLETE','PARTIAL','CONFLICTED')),
  effective_from date NOT NULL,
  effective_to date,
  rules jsonb NOT NULL,
  rules_hash text NOT NULL,
  confirmed_by_id text NOT NULL,
  confirmed_by_display text,
  confirmed_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL,
  CHECK (effective_to IS NULL OR effective_to >= effective_from),
  UNIQUE (dossier_id,version),
  UNIQUE (dossier_id,rules_hash)
);
ALTER TABLE contract_dossiers ADD CONSTRAINT contract_dossiers_active_snapshot_fk
  FOREIGN KEY (active_snapshot_id) REFERENCES contract_commercial_snapshots(id);
CREATE INDEX contract_commercial_snapshots_effective_idx
  ON contract_commercial_snapshots(contract_id,effective_from,effective_to);

CREATE TABLE contract_snapshot_sources (
  snapshot_id text NOT NULL REFERENCES contract_commercial_snapshots(id),
  document_id text NOT NULL REFERENCES contract_source_documents(id),
  clause_id text REFERENCES contract_clause_candidates(id),
  PRIMARY KEY(snapshot_id,document_id,clause_id)
);

CREATE TABLE contract_variable_definitions (
  id text PRIMARY KEY,
  dossier_id text NOT NULL REFERENCES contract_dossiers(id),
  name text NOT NULL,
  value_type text NOT NULL CHECK (value_type IN ('DECIMAL','DATE','BOOLEAN','TEXT')),
  allowed_sources jsonb NOT NULL,
  required boolean NOT NULL DEFAULT true,
  narrative text NOT NULL,
  UNIQUE(dossier_id,name)
);

CREATE TABLE contract_variable_values (
  id text PRIMARY KEY,
  definition_id text NOT NULL REFERENCES contract_variable_definitions(id),
  period_start date,
  period_end date,
  value text NOT NULL,
  source text NOT NULL CHECK (source IN ('INVOICE','INTEGRATION','MANUAL','CONTRACT')),
  source_reference text,
  recorded_by_id text NOT NULL,
  recorded_at timestamptz NOT NULL,
  command_key text NOT NULL UNIQUE,
  CHECK (period_end IS NULL OR period_start IS NULL OR period_end >= period_start)
);
CREATE INDEX contract_variable_values_lookup_idx
  ON contract_variable_values(definition_id,period_start,period_end,recorded_at DESC);

CREATE TABLE contract_service_aliases (
  id text PRIMARY KEY,
  client_id text NOT NULL REFERENCES clients(id),
  supplier_cui text NOT NULL,
  service_id text NOT NULL,
  normalized_label text NOT NULL,
  effective_from date,
  effective_to date,
  confirmed_by_id text NOT NULL,
  confirmed_at timestamptz NOT NULL,
  command_key text NOT NULL UNIQUE,
  CHECK (effective_to IS NULL OR effective_from IS NULL OR effective_to >= effective_from),
  UNIQUE(client_id,supplier_cui,service_id,normalized_label)
);

CREATE TABLE invoice_commercial_validation_runs (
  id text PRIMARY KEY,
  invoice_id text NOT NULL REFERENCES invoices(id),
  snapshot_id text REFERENCES contract_commercial_snapshots(id),
  invoice_revision bigint NOT NULL CHECK (invoice_revision > 0),
  snapshot_version bigint NOT NULL CHECK (snapshot_version > 0),
  engine_version text NOT NULL,
  outcome text NOT NULL CHECK (outcome IN ('CONFORM','NECONFORM','NEVERIFICABIL')),
  command_key text NOT NULL UNIQUE,
  created_at timestamptz NOT NULL,
  completed_at timestamptz NOT NULL
);
CREATE INDEX invoice_commercial_runs_invoice_created_idx
  ON invoice_commercial_validation_runs(invoice_id,created_at DESC);

CREATE TABLE invoice_commercial_findings (
  id text PRIMARY KEY,
  run_id text NOT NULL REFERENCES invoice_commercial_validation_runs(id),
  rule_id text NOT NULL,
  invoice_line_id text REFERENCES invoice_lines(id),
  code text NOT NULL,
  outcome text NOT NULL CHECK (outcome IN ('CONFORM','NECONFORM','NEVERIFICABIL')),
  actual_value text,
  expected_value text,
  calculation text,
  reason text NOT NULL,
  missing_inputs jsonb NOT NULL DEFAULT '[]'::jsonb,
  evidence jsonb NOT NULL DEFAULT '[]'::jsonb
);
CREATE INDEX invoice_commercial_findings_run_idx ON invoice_commercial_findings(run_id);

CREATE TABLE invoice_commercial_overrides (
  id text PRIMARY KEY,
  finding_id text NOT NULL UNIQUE REFERENCES invoice_commercial_findings(id),
  reason text NOT NULL CHECK (length(reason) > 0),
  actor_id text NOT NULL,
  actor_display text,
  command_key text NOT NULL UNIQUE,
  created_at timestamptz NOT NULL
);

CREATE TABLE commercial_review_commands (
  command_key text PRIMARY KEY,
  client_id text NOT NULL REFERENCES clients(id),
  invoice_id text NOT NULL REFERENCES invoices(id),
  fingerprint text NOT NULL,
  created_at timestamptz NOT NULL
);

CREATE TABLE contract_commercial_ledger (
  id text PRIMARY KEY,
  dossier_id text NOT NULL REFERENCES contract_dossiers(id),
  snapshot_id text NOT NULL REFERENCES contract_commercial_snapshots(id),
  rule_id text NOT NULL,
  invoice_id text NOT NULL REFERENCES invoices(id),
  invoice_line_id text REFERENCES invoice_lines(id),
  period_key text NOT NULL,
  amount numeric(20,4) NOT NULL,
  currency char(3) NOT NULL,
  quantity numeric(20,4),
  reversed_entry_id text REFERENCES contract_commercial_ledger(id),
  created_at timestamptz NOT NULL,
  UNIQUE(rule_id,invoice_id,invoice_line_id)
);
CREATE INDEX contract_commercial_ledger_period_idx
  ON contract_commercial_ledger(dossier_id,rule_id,period_key);

-- Every legacy authoritative contract gets a dossier with deliberately PARTIAL
-- coverage. No executable commercial rule is inferred from old flat fields.
INSERT INTO contract_dossiers (
  id,client_id,contract_id,supplier_cui,buyer_cui,primary_reference,status,revision,created_at,updated_at
)
SELECT 'dossier-' || c.id,c.client_id,c.id,c.supplier_cui,cl.cui,c.reference,'ACTIVE_PARTIAL',1,c.created_at,c.updated_at
FROM contracts c JOIN clients cl ON cl.id=c.client_id
ON CONFLICT (contract_id) DO NOTHING;

UPDATE contract_source_documents d
SET dossier_id='dossier-' || d.confirmed_contract_id,
    document_role=COALESCE(d.document_role,'BASE_CONTRACT')
WHERE d.confirmed_contract_id IS NOT NULL AND d.dossier_id IS NULL;
