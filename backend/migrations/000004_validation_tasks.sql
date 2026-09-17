-- Add Backend Module 3 human-intervention persistence.
CREATE UNIQUE INDEX invoices_id_client_id_key ON invoices (id, client_id);

CREATE TABLE validation_tasks (
    id text PRIMARY KEY,
    client_id text NOT NULL REFERENCES clients(id),
    invoice_id text NOT NULL,
    task_type text NOT NULL CHECK (task_type IN ('CONTRACT_MATCH', 'MISSING_CONTRACT', 'CLASSIFICATION')),
    status text NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN', 'WAITING', 'RESOLVED')),
    title text NOT NULL CHECK (length(title) > 0),
    reason text NOT NULL CHECK (length(reason) > 0),
    blocker_code text,
    resolution_metadata jsonb,
    created_by_kind text NOT NULL CHECK (created_by_kind IN ('SYSTEM', 'USER', 'EXTERNAL')),
    created_by_id text,
    created_by_display text,
    creation_key text NOT NULL UNIQUE CHECK (length(creation_key) > 0),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    waiting_since timestamptz,
    resolved_at timestamptz,
    CONSTRAINT validation_tasks_invoice_client_fk
        FOREIGN KEY (invoice_id, client_id) REFERENCES invoices(id, client_id),
    CONSTRAINT validation_tasks_lifecycle_metadata CHECK (
        (status = 'OPEN' AND waiting_since IS NULL AND resolved_at IS NULL)
        OR
        (status = 'WAITING' AND task_type = 'MISSING_CONTRACT' AND waiting_since IS NOT NULL AND resolved_at IS NULL)
        OR
        (status = 'RESOLVED' AND resolved_at IS NOT NULL)
    )
);

CREATE INDEX validation_tasks_client_status_created_idx
    ON validation_tasks (client_id, status, created_at);
CREATE INDEX validation_tasks_invoice_created_idx
    ON validation_tasks (invoice_id, created_at);
CREATE INDEX validation_tasks_type_status_idx
    ON validation_tasks (task_type, status);
CREATE UNIQUE INDEX validation_tasks_one_active_per_invoice
    ON validation_tasks (invoice_id)
    WHERE status <> 'RESOLVED';

ALTER TABLE activity_events
    ADD COLUMN validation_task_id text REFERENCES validation_tasks(id);
CREATE INDEX activity_events_validation_task_id_idx
    ON activity_events (validation_task_id);
