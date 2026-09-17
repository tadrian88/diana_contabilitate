-- Additive client management. No historical documents/profiles/releases rewritten.
ALTER TABLE clients
 ADD COLUMN display_name text NOT NULL DEFAULT '',
 ADD COLUMN registration_number text NOT NULL DEFAULT '',
 ADD COLUMN country text NOT NULL DEFAULT 'RO',
 ADD COLUMN registered_address text NOT NULL DEFAULT '',
 ADD COLUMN city text NOT NULL DEFAULT '', ADD COLUMN region text NOT NULL DEFAULT '',
 ADD COLUMN postal_code text NOT NULL DEFAULT '', ADD COLUMN email text NOT NULL DEFAULT '',
 ADD COLUMN phone text NOT NULL DEFAULT '', ADD COLUMN default_currency text NOT NULL DEFAULT 'RON',
 ADD COLUMN normalized_identifier text NOT NULL DEFAULT '',
 ADD COLUMN lifecycle text NOT NULL DEFAULT 'ACTIVE' CHECK(lifecycle IN ('ONBOARDING','ACTIVE','INACTIVE')),
 ADD COLUMN revision bigint NOT NULL DEFAULT 1 CHECK(revision>0);
-- Preserve legacy synthetic demo identities; new commands validate deterministic format.
UPDATE clients SET normalized_identifier = CASE
 WHEN btrim(upper(cui)) ~ '^(RO[[:space:]]*)?[1-9][0-9]{1,9}$'
 THEN regexp_replace(btrim(upper(cui)), '^RO[[:space:]]*', '')
 ELSE btrim(upper(cui)) END;
-- A collision intentionally aborts migration; do not choose a winner or merge history.
CREATE UNIQUE INDEX clients_country_normalized_identity ON clients(country,normalized_identifier)
 WHERE normalized_identifier <> '';
CREATE TABLE client_saga_configurations (
 client_id text PRIMARY KEY REFERENCES clients(id), enabled boolean NOT NULL,
 updated_at timestamptz NOT NULL
);
INSERT INTO client_saga_configurations(client_id,enabled,updated_at) SELECT id,true,updated_at FROM clients;
-- File export opt-in only. No invented mapping/target identifier/connection or validation evidence.
CREATE TABLE client_management_commands (
 command_key text PRIMARY KEY, request_hash text NOT NULL,
 client_id text NOT NULL REFERENCES clients(id),
 created_at timestamptz NOT NULL
);

-- Idempotent browser authorization attempts. Raw state is encrypted with existing token cipher.
CREATE TABLE client_oauth_attempts (
 command_key text PRIMARY KEY, state_id text NOT NULL REFERENCES spv_oauth_states(id),
 state_ciphertext text NOT NULL
);
