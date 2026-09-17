-- Create the minimal Backend Module 1 persistence slice.
CREATE TABLE clients (
    id text PRIMARY KEY,
    name text NOT NULL CHECK (length(name) > 0),
    cui text NOT NULL UNIQUE CHECK (length(cui) > 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE TABLE invoices (
    id text PRIMARY KEY,
    client_id text NOT NULL REFERENCES clients(id),
    supplier_name text NOT NULL CHECK (length(supplier_name) > 0),
    supplier_cui text,
    document_number text NOT NULL CHECK (length(document_number) > 0),
    issue_date timestamptz NOT NULL,
    due_date timestamptz,
    total_amount numeric(20,4) NOT NULL,
    currency char(3) NOT NULL,
    spv_reference text NOT NULL CHECK (length(spv_reference) > 0),
    pipeline_status text NOT NULL CHECK (pipeline_status IN (
        'DOWNLOADED', 'ARCHIVED', 'MATCHING', 'AWAITING_CONTRACT',
        'AWAITING_MATCH_CONFIRM', 'DEDUPE_CHECKED', 'HEADER_READ', 'LINES_READ',
        'CLASSIFIED', 'AWAITING_REVIEW', 'READY_FOR_SAGA', 'EXPORTING', 'EXPORTED', 'DUPLICATE'
    )),
    saga_status text NOT NULL CHECK (saga_status IN ('NOT_READY', 'READY', 'EXPORTING', 'EXPORTED', 'FAILED')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE INDEX invoices_client_id_idx ON invoices (client_id);
CREATE INDEX invoices_pipeline_status_idx ON invoices (pipeline_status);
CREATE INDEX invoices_issue_date_idx ON invoices (issue_date);

CREATE TABLE activity_events (
    id text PRIMARY KEY,
    client_id text NOT NULL REFERENCES clients(id),
    invoice_id text REFERENCES invoices(id),
    aggregate_type text NOT NULL CHECK (length(aggregate_type) > 0),
    aggregate_id text NOT NULL CHECK (length(aggregate_id) > 0),
    event_type text NOT NULL CHECK (length(event_type) > 0),
    occurred_at timestamptz NOT NULL,
    actor_kind text NOT NULL CHECK (actor_kind IN ('SYSTEM', 'USER', 'EXTERNAL')),
    actor_id text,
    actor_display text,
    automatic boolean NOT NULL,
    detail text NOT NULL CHECK (length(detail) > 0),
    before_snapshot jsonb,
    after_snapshot jsonb,
    correlation_id text
);

CREATE INDEX activity_events_client_id_idx ON activity_events (client_id);
CREATE INDEX activity_events_aggregate_idx ON activity_events (aggregate_type, aggregate_id, occurred_at);
CREATE INDEX activity_events_occurred_at_idx ON activity_events (occurred_at);
