-- Expand the approved foundation with Backend Module 2 pipeline persistence.
ALTER TABLE invoices ADD COLUMN ingestion_source text;
ALTER TABLE invoices ADD COLUMN external_delivery_id text;
ALTER TABLE invoices ADD COLUMN normalized_supplier_cui text;
ALTER TABLE invoices ADD COLUMN normalized_document_number text;
ALTER TABLE invoices ADD COLUMN issue_day date;
ALTER TABLE invoices ADD COLUMN duplicate_of_invoice_id text REFERENCES invoices(id);
ALTER TABLE invoices ADD COLUMN duplicate_amount_matches boolean;
ALTER TABLE invoices ADD COLUMN duplicate_currency_matches boolean;

UPDATE invoices
SET ingestion_source = 'DEVELOPMENT_SEED',
    external_delivery_id = spv_reference,
    normalized_supplier_cui = CASE
        WHEN supplier_cui IS NULL OR btrim(supplier_cui) = '' THEN NULL
        ELSE upper(regexp_replace(btrim(supplier_cui), '\s+', ' ', 'g'))
    END,
    normalized_document_number = upper(regexp_replace(btrim(document_number), '\s+', ' ', 'g')),
    issue_day = (issue_date AT TIME ZONE 'UTC')::date
WHERE ingestion_source IS NULL
   OR external_delivery_id IS NULL
   OR normalized_document_number IS NULL
   OR issue_day IS NULL;

ALTER TABLE invoices ALTER COLUMN ingestion_source SET NOT NULL;
ALTER TABLE invoices ALTER COLUMN external_delivery_id SET NOT NULL;
ALTER TABLE invoices ALTER COLUMN normalized_document_number SET NOT NULL;
ALTER TABLE invoices ALTER COLUMN issue_day SET NOT NULL;
ALTER TABLE invoices ADD CONSTRAINT invoices_ingestion_source_nonempty CHECK (length(ingestion_source) > 0);
ALTER TABLE invoices ADD CONSTRAINT invoices_external_delivery_id_nonempty CHECK (length(external_delivery_id) > 0);
CREATE UNIQUE INDEX invoices_spv_identity_key ON invoices (client_id, spv_reference);
CREATE INDEX invoices_business_identity_lookup_idx
    ON invoices (client_id, normalized_supplier_cui, normalized_document_number, issue_day);
CREATE UNIQUE INDEX invoices_canonical_business_identity_key
    ON invoices (client_id, normalized_supplier_cui, normalized_document_number, issue_day)
    WHERE duplicate_of_invoice_id IS NULL AND normalized_supplier_cui IS NOT NULL;
ALTER TABLE invoices ADD CONSTRAINT invoices_duplicate_not_self
    CHECK (duplicate_of_invoice_id IS NULL OR duplicate_of_invoice_id <> id);
ALTER TABLE invoices ADD CONSTRAINT invoices_duplicate_verification_complete
    CHECK (
        (duplicate_of_invoice_id IS NULL AND duplicate_amount_matches IS NULL AND duplicate_currency_matches IS NULL)
        OR
        (duplicate_of_invoice_id IS NOT NULL AND duplicate_amount_matches IS NOT NULL AND duplicate_currency_matches IS NOT NULL)
    );

ALTER TABLE activity_events ADD COLUMN pipeline_from text;
ALTER TABLE activity_events ADD COLUMN pipeline_to text;
ALTER TABLE activity_events ADD COLUMN trigger text;
ALTER TABLE activity_events ADD COLUMN idempotency_key text;
CREATE UNIQUE INDEX activity_events_idempotency_key ON activity_events (idempotency_key);

CREATE TABLE invoice_lines (
    id text PRIMARY KEY,
    invoice_id text NOT NULL REFERENCES invoices(id),
    position integer NOT NULL CHECK (position > 0),
    description text NOT NULL CHECK (length(description) > 0),
    unit text NOT NULL CHECK (length(unit) > 0),
    vat_rate numeric(20,4) NOT NULL,
    vat_value numeric(20,4) NOT NULL,
    quantity numeric(20,4) NOT NULL,
    unit_price numeric(20,4) NOT NULL,
    net_value numeric(20,4) NOT NULL,
    total_value numeric(20,4) NOT NULL,
    additional_info text,
    CONSTRAINT invoice_lines_invoice_position_key UNIQUE (invoice_id, position)
);

CREATE INDEX invoice_lines_invoice_id_idx ON invoice_lines (invoice_id);

CREATE TABLE outbox_entries (
    id text PRIMARY KEY,
    event_type text NOT NULL CHECK (length(event_type) > 0),
    aggregate_type text NOT NULL CHECK (length(aggregate_type) > 0),
    aggregate_id text NOT NULL CHECK (length(aggregate_id) > 0),
    payload jsonb NOT NULL,
    idempotency_key text NOT NULL UNIQUE CHECK (length(idempotency_key) > 0),
    status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'PROCESSED')),
    attempts bigint NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    created_at timestamptz NOT NULL,
    available_at timestamptz NOT NULL,
    processed_at timestamptz
);

CREATE INDEX outbox_entries_status_available_at_idx ON outbox_entries (status, available_at);
CREATE INDEX outbox_entries_aggregate_idx ON outbox_entries (aggregate_type, aggregate_id);
