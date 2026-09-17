-- Inbound ANAF/SPV source accounts and immutable source deliveries.
CREATE TABLE spv_connections (
    id text PRIMARY KEY,
    client_id text NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    cif text NOT NULL CHECK (length(cif) > 0),
    environment text NOT NULL CHECK (environment IN ('TEST', 'PRODUCTION')),
    access_token_ciphertext text NOT NULL,
    refresh_token_ciphertext text NOT NULL,
    access_token_expires_at timestamptz NOT NULL,
    refresh_token_expires_at timestamptz,
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'EXPIRED', 'REVOKED', 'ERROR')),
    last_successful_sync_at timestamptz,
    last_sync_started_at timestamptz,
    last_error text,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT spv_connections_client_id_key UNIQUE (client_id),
    CONSTRAINT spv_connections_environment_cif_key UNIQUE (environment, cif)
);
CREATE INDEX spv_connections_status_idx ON spv_connections (status);

CREATE TABLE spv_source_documents (
    id text PRIMARY KEY,
    connection_id text NOT NULL REFERENCES spv_connections(id) ON DELETE CASCADE,
    client_id text NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    external_message_id text NOT NULL CHECK (length(external_message_id) > 0),
    external_upload_id text,
    external_request_id text,
    message_type text,
    source_created_raw text,
    source_created_at timestamptz,
    discovered_at timestamptz NOT NULL,
    downloaded_at timestamptz,
    raw_document bytea,
    content_type text,
    content_sha256 text,
    parser_type text,
    parser_version text,
    processing_status text NOT NULL DEFAULT 'DISCOVERED' CHECK (processing_status IN ('DISCOVERED', 'PROCESSING', 'DOWNLOADED', 'PROCESSED', 'FAILED')),
    failure_kind text CHECK (failure_kind IN ('TRANSIENT', 'PERMANENT')),
    last_error text,
    attempts bigint NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at timestamptz NOT NULL,
    claim_owner text,
    claimed_at timestamptz,
    invoice_id text UNIQUE REFERENCES invoices(id),
    processed_at timestamptz,
    updated_at timestamptz NOT NULL,
    CONSTRAINT spv_source_documents_external_identity_key UNIQUE (connection_id, external_message_id),
    CONSTRAINT spv_source_documents_raw_immutable_shape CHECK (
        (raw_document IS NULL AND downloaded_at IS NULL AND content_sha256 IS NULL)
        OR (raw_document IS NOT NULL AND downloaded_at IS NOT NULL AND content_sha256 IS NOT NULL)
    )
);
CREATE INDEX spv_source_documents_client_id_idx ON spv_source_documents (client_id);
CREATE INDEX spv_source_documents_status_available_idx ON spv_source_documents (processing_status, available_at);
CREATE INDEX spv_source_documents_content_sha256_idx ON spv_source_documents (content_sha256);
