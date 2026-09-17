-- Add Backend Module 4 contract matching and immutable invoice association snapshots.
CREATE TABLE contracts (
    id text PRIMARY KEY,
    client_id text NOT NULL REFERENCES clients(id),
    supplier_name text NOT NULL CHECK (length(supplier_name) > 0),
    supplier_cui text NOT NULL CHECK (length(supplier_cui) > 0),
    normalized_supplier_cui text NOT NULL CHECK (length(normalized_supplier_cui) > 0),
    reference text NOT NULL CHECK (length(reference) > 0),
    effective_from date NOT NULL,
    effective_to date NOT NULL,
    total_value numeric(20,4) NOT NULL,
    currency char(3) NOT NULL,
    unit_type text NOT NULL CHECK (length(unit_type) > 0),
    payment_terms text NOT NULL CHECK (length(payment_terms) > 0),
    source_reference text,
    source_metadata text,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT contracts_effective_period CHECK (effective_from <= effective_to),
    CONSTRAINT contracts_id_client_key UNIQUE (id, client_id),
    CONSTRAINT contracts_client_reference_key UNIQUE (client_id, reference)
);
CREATE INDEX contracts_client_supplier_idx ON contracts (client_id, normalized_supplier_cui);

CREATE TABLE contract_match_runs (
    id text PRIMARY KEY,
    client_id text NOT NULL REFERENCES clients(id),
    invoice_id text NOT NULL,
    policy_version text NOT NULL CHECK (length(policy_version) > 0),
    outcome text NOT NULL CHECK (outcome IN ('UNIQUE_COMPATIBLE', 'MULTIPLE_PLAUSIBLE', 'UNIQUE_INCOMPATIBLE', 'NO_MATCH')),
    invoice_revision bigint NOT NULL CHECK (invoice_revision > 0),
    command_key text NOT NULL UNIQUE CHECK (length(command_key) > 0),
    created_at timestamptz NOT NULL,
    CONSTRAINT contract_match_runs_invoice_client_fk
        FOREIGN KEY (invoice_id, client_id) REFERENCES invoices(id, client_id),
    CONSTRAINT contract_match_runs_id_client_key UNIQUE (id, client_id),
    CONSTRAINT contract_match_runs_identity_key UNIQUE (id, invoice_id, client_id)
);
CREATE INDEX contract_match_runs_invoice_created_idx ON contract_match_runs (invoice_id, created_at);

CREATE TABLE contract_match_candidates (
    id text PRIMARY KEY,
    client_id text NOT NULL REFERENCES clients(id),
    match_run_id text NOT NULL,
    contract_id text NOT NULL,
    contract_revision bigint NOT NULL CHECK (contract_revision > 0),
    rank integer NOT NULL CHECK (rank > 0),
    recommended boolean NOT NULL,
    compatibility text NOT NULL CHECK (compatibility IN ('COMPATIBLE', 'INCOMPATIBLE')),
    confidence_display text NOT NULL CHECK (length(confidence_display) > 0),
    reasons jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT contract_match_candidates_run_client_fk
        FOREIGN KEY (match_run_id, client_id) REFERENCES contract_match_runs(id, client_id),
    CONSTRAINT contract_match_candidates_contract_client_fk
        FOREIGN KEY (contract_id, client_id) REFERENCES contracts(id, client_id),
    CONSTRAINT contract_match_candidates_run_rank_key UNIQUE (match_run_id, rank),
    CONSTRAINT contract_match_candidates_run_contract_key UNIQUE (match_run_id, contract_id)
);
CREATE UNIQUE INDEX contract_match_candidates_one_recommended
    ON contract_match_candidates (match_run_id)
    WHERE recommended;

CREATE TABLE invoice_contract_associations (
    id text PRIMARY KEY,
    client_id text NOT NULL REFERENCES clients(id),
    invoice_id text NOT NULL UNIQUE,
    contract_id text NOT NULL,
    match_run_id text NOT NULL,
    association_kind text NOT NULL CHECK (association_kind IN ('AUTOMATIC', 'HUMAN_CONFIRMED')),
    policy_version text NOT NULL CHECK (length(policy_version) > 0),
    contract_reference text NOT NULL CHECK (length(contract_reference) > 0),
    supplier_name text NOT NULL CHECK (length(supplier_name) > 0),
    effective_from date NOT NULL,
    effective_to date NOT NULL,
    total_value numeric(20,4) NOT NULL,
    currency char(3) NOT NULL,
    unit_type text NOT NULL CHECK (length(unit_type) > 0),
    payment_terms text NOT NULL CHECK (length(payment_terms) > 0),
    associated_at timestamptz NOT NULL,
    associated_by_id text,
    associated_by_display text,
    CONSTRAINT invoice_contract_associations_invoice_client_fk
        FOREIGN KEY (invoice_id, client_id) REFERENCES invoices(id, client_id),
    CONSTRAINT invoice_contract_associations_contract_client_fk
        FOREIGN KEY (contract_id, client_id) REFERENCES contracts(id, client_id),
    CONSTRAINT invoice_contract_associations_run_invoice_client_fk
        FOREIGN KEY (match_run_id, invoice_id, client_id) REFERENCES contract_match_runs(id, invoice_id, client_id),
    CONSTRAINT invoice_contract_associations_snapshot_period CHECK (effective_from <= effective_to)
);
CREATE INDEX invoice_contract_associations_contract_created_idx
    ON invoice_contract_associations (contract_id, associated_at);

ALTER TABLE validation_tasks
    ADD COLUMN contract_match_run_id text;
ALTER TABLE validation_tasks
    ADD CONSTRAINT validation_tasks_match_run_invoice_client_fk
    FOREIGN KEY (contract_match_run_id, invoice_id, client_id)
    REFERENCES contract_match_runs(id, invoice_id, client_id);
CREATE UNIQUE INDEX validation_tasks_contract_match_run_key
    ON validation_tasks (contract_match_run_id)
    WHERE contract_match_run_id IS NOT NULL;
ALTER TABLE validation_tasks
    ADD CONSTRAINT validation_tasks_match_run_type_check
    CHECK (contract_match_run_id IS NULL OR task_type = 'CONTRACT_MATCH');
