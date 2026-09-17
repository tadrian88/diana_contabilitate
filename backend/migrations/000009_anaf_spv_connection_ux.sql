-- Durable OAuth CSRF/replay protection and minimal connection/sync read metadata.
ALTER TABLE spv_connections
    ADD COLUMN connected_at timestamptz,
    ADD COLUMN last_sync_finished_at timestamptz,
    ADD COLUMN last_sync_status text NOT NULL DEFAULT 'NEVER'
        CHECK (last_sync_status IN ('NEVER', 'RUNNING', 'SUCCEEDED', 'FAILED'));

UPDATE spv_connections
SET connected_at = created_at,
    last_sync_status = CASE
        WHEN last_error IS NOT NULL THEN 'FAILED'
        WHEN last_successful_sync_at IS NOT NULL THEN 'SUCCEEDED'
        WHEN last_sync_started_at IS NOT NULL THEN 'RUNNING'
        ELSE 'NEVER'
    END;

CREATE TABLE spv_oauth_states (
    id text PRIMARY KEY,
    state_hash text NOT NULL UNIQUE,
    client_id text NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    environment text NOT NULL CHECK (environment IN ('TEST', 'PRODUCTION')),
    return_path text NOT NULL CHECK (return_path LIKE '/clients/%'),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL
);

CREATE INDEX spv_oauth_states_expires_at_idx ON spv_oauth_states (expires_at);
CREATE INDEX spv_oauth_states_client_id_idx ON spv_oauth_states (client_id);
