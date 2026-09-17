-- Diana Authentication V1. Credentials and sessions are additive and do not
-- rewrite accounting, ANAF, SAGA, contract or invoice data.
CREATE TABLE auth_users (
 id text PRIMARY KEY,
 email text NOT NULL UNIQUE CHECK(email = lower(btrim(email)) AND position('@' in email) > 1),
 password_hash text NOT NULL CHECK(password_hash LIKE '$argon2id$%'),
 status text NOT NULL CHECK(status IN ('ACTIVE','DISABLED')),
 persona text NOT NULL DEFAULT 'CONTABIL' CHECK(persona IN ('CONTABIL')),
 all_clients boolean NOT NULL DEFAULT false,
 credential_version bigint NOT NULL DEFAULT 1 CHECK(credential_version > 0),
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL
);

CREATE TABLE auth_user_client_grants (
 user_id text NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
 client_id text NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(user_id,client_id)
);

CREATE TABLE auth_sessions (
 id text PRIMARY KEY,
 user_id text NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
 token_hash bytea NOT NULL UNIQUE,
 csrf_hash bytea NOT NULL,
 created_at timestamptz NOT NULL,
 last_seen_at timestamptz NOT NULL,
 expires_at timestamptz NOT NULL,
 revoked_at timestamptz,
 CHECK(expires_at > created_at)
);
CREATE INDEX auth_sessions_active_user ON auth_sessions(user_id,expires_at) WHERE revoked_at IS NULL;

CREATE TABLE auth_events (
 id text PRIMARY KEY,
 user_id text REFERENCES auth_users(id) ON DELETE SET NULL,
 event_type text NOT NULL CHECK(event_type IN ('LOGIN_SUCCESS','LOGIN_FAILURE','LOGOUT','USER_DISABLED','PASSWORD_CHANGED','GRANT_CHANGED')),
 occurred_at timestamptz NOT NULL
);
CREATE INDEX auth_events_user_time ON auth_events(user_id,occurred_at DESC);
