-- Preserve the source document kind and generated SAGA artifacts. A generated
-- file is not evidence that the local SAGA installation imported it.
ALTER TABLE invoices
    ADD COLUMN document_type text NOT NULL DEFAULT 'INVOICE'
        CHECK (document_type IN ('INVOICE', 'CREDIT_NOTE'));

CREATE TABLE saga_export_attempts (
    id text PRIMARY KEY,
    invoice_id text NOT NULL,
    client_id text NOT NULL REFERENCES clients(id) ON DELETE RESTRICT,
    invoice_revision bigint NOT NULL CHECK (invoice_revision > 0),
    exporter_version text NOT NULL,
    status text NOT NULL CHECK (status IN ('GENERATED', 'FAILED')),
    filename text,
    content_type text,
    payload bytea,
    payload_sha256 text,
    classification_snapshot jsonb,
    failure_category text CHECK (failure_category IN ('DATA_INVALID', 'SERIALIZATION_FAILED', 'UNSUPPORTED_DOCUMENT_TYPE')),
    safe_error text,
    started_at timestamptz NOT NULL,
    completed_at timestamptz NOT NULL,
    CONSTRAINT saga_export_attempts_invoice_client_fk
        FOREIGN KEY (invoice_id, client_id) REFERENCES invoices(id, client_id) ON DELETE RESTRICT,
    CONSTRAINT saga_export_attempts_result_shape CHECK (
        (status = 'GENERATED' AND filename IS NOT NULL AND content_type IS NOT NULL AND payload IS NOT NULL AND payload_sha256 IS NOT NULL AND failure_category IS NULL AND safe_error IS NULL)
        OR
        (status = 'FAILED' AND payload IS NULL AND payload_sha256 IS NULL AND failure_category IS NOT NULL AND safe_error IS NOT NULL)
    ),
    CONSTRAINT saga_export_attempts_invoice_revision_version_key UNIQUE (invoice_id, invoice_revision, exporter_version)
);

CREATE INDEX saga_export_attempts_client_id_completed_at_idx ON saga_export_attempts (client_id, completed_at);
CREATE INDEX saga_export_attempts_payload_sha256_idx ON saga_export_attempts (payload_sha256);
