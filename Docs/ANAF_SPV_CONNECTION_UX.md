# ANAF/SPV Connection UX

Status: implemented; awaiting user-run tests and review.

## User journey and location

The additive integration card lives in Client Detail, after the existing SAGA
summary and before the operational client sections. There is no global SPV page and
no setup action in `Toți clienții`. The card always names the selected client.

The normal journey is:

`Client Detail -> review client/CUI and certificate prerequisites -> Conectează
ANAF -> browser/ANAF certificate selection and PIN -> ANAF consent -> Diana callback
-> same Client Detail -> Conectat -> automatic or manual SPV sync`.

There is deliberately no certificate upload or PIN field in Diana. ARMQU and the
official ANAF OAuth model prove that qualified-certificate authentication happens at
`logincert.anaf.ro`, using the certificate exposed to the browser by the OS/USB-token
middleware. Diana never receives a `.p12`, `.pfx`, PEM, private key or PIN. The
not-connected/reconnect card explains this boundary, shows the authoritative Diana
client name/CUI and tells the user where certificate selection and PIN entry happen.

`cmd/spvconnect` remains a development/break-glass tool, not primary onboarding.

## Read model and status

`GET /api/v1/clients/{clientId}/spv` returns a dedicated DTO containing only:
connection status, environment, connection time, last sync timestamps/result,
sanitized error, automatic-import flag, server configuration readiness and explicit
identity-validation capability. It never serializes Ent or credentials.

Connection health is one of `NOT_CONNECTED`, `CONNECTED`,
`NEEDS_REAUTHENTICATION`, `ERROR`, `DISABLED`. Sync result is independently `NEVER`,
`RUNNING`, `SUCCEEDED`, `FAILED`; a temporary sync failure therefore remains
connected. No artificial authorization-pending status is exposed because current
backend state cannot reliably derive it as a user-visible connection state.

## OAuth start and callback security

`POST /api/v1/clients/{clientId}/spv/oauth/start` validates the client, generates 32
cryptographically random bytes and returns the provider URL. PostgreSQL stores only
the SHA-256 digest, bound to client, configured TEST/PRODUCTION environment,
server-selected Client Detail return path, creation time and configurable expiry.

`GET /api/v1/integrations/anaf/callback` hashes the supplied state and atomically
consumes a matching, unexpired, unused row before token exchange. Missing, random,
expired, cross-context and replayed values cannot store credentials. Callback errors
redirect with a small safe reason code; raw provider payloads never enter URLs.

Successful exchange encrypts both tokens using the approved AES-256-GCM boundary and
creates or updates the one client-owned connection. React receives only a safe
redirect. Refresh reads persisted state.

## Identity validation limitation

The inspected OAuth token response provides no proven authenticated CIF identity.
The API reports `identityValidation: NOT_AVAILABLE`; the UI states this limitation
and does not claim verification. The approved ingestion guard remains authoritative:
each downloaded document's buyer CUI must equal the AccountingClient CUI.
Credentialed ANAF certification must determine whether a reliable account/CIF claim
is available.

## Manual sync, reconnect and disconnect

`POST /api/v1/clients/{clientId}/spv/sync` accepts only an active connection and
publishes the existing `spv:sync_account` Asynq task. It returns HTTP 202 without
waiting for ANAF. Publisher uniqueness coalesces repeated one-minute clicks; approved
delivery/document idempotency remains the correctness guarantee.

Reconnection uses OAuth start again and updates the existing connection identity.
`POST /api/v1/clients/{clientId}/spv/disconnect` requires confirmation in React,
marks the connection revoked, clears locally usable token values and stops scheduler
selection. It does not delete source documents or invoices. No provider revocation
is claimed because no verified revocation endpoint exists in the adapter.

## Audit and credential security

Business/admin events are `SPV_CONNECTION_CONNECTED`,
`SPV_CONNECTION_RECONNECTED`, `SPV_CONNECTION_DISCONNECTED` and
`SPV_MANUAL_SYNC_REQUESTED`. Routine refresh/retry is not business audit. Event
payloads contain no tokens.

OAuth client ID/secret, encryption key and provider endpoints remain server config.
Users never enter ANAF passwords, certificate passwords, tokens or application
secrets. No certificate upload, token viewer, raw document viewer, queue UI or
checkpoint editor is introduced.

This is a security property rather than an omitted feature: accepting a private-key
container in Diana would add transport, memory, backup, logging and cleanup exposure
without being part of the proven protocol. There is therefore no certificate size,
MIME/extension or PKCS#12 password validation in Diana; those concerns do not enter
its trust boundary.

## Database migration 000009

The additive migration adds `connected_at`, `last_sync_finished_at` and
`last_sync_status` to `spv_connections`, with a deterministic backfill. The new
`spv_oauth_states` table holds hashed, expiring, single-use OAuth state. Migrations
`000001` through `000008` are unchanged.

New configuration:

| Variable | Default |
|---|---|
| `SPV_AUTHORIZE_URL` | official certificate OAuth authorize URL |
| `SPV_OAUTH_REDIRECT_URI` | `http://127.0.0.1:8080/api/v1/integrations/anaf/callback` |
| `FRONTEND_BASE_URL` | `http://localhost:5173` |
| `SPV_OAUTH_STATE_TTL` | `10m` |

Existing OAuth client/secret, token URL and encryption-key configuration remains
unchanged.

## Tests implemented

- random/hash/TTL/client/environment OAuth state and callback replay unit tests;
- encrypted-only credential persistence, reconnect/disconnect and inactive-sync tests;
- permanent refresh rejection to reauthentication mapping;
- PostgreSQL atomic state consumption, connection lifecycle, audit and document retention;
- Redis/Asynq manual-sync click coalescing;
- explicit frontend HTTP repository endpoints and credential-free DTO assertion;
- component states, callback feedback, redirect, sync, API error and disconnect dialog;
- backend-backed Playwright fake-provider journey through connect, refresh, manual
  import, disconnect/data retention and reconnect.
- no file/password control in the Diana onboarding surface and an explicit synthetic
  provider consent step in the fake-ANAF journey.

## Security boundary still deferred

RequestActor is preserved at every command/audit boundary, but full authentication
and per-client RBAC are still deferred by the approved architecture. Client IDs and
OAuth state binding prevent accidental cross-client persistence; production exposure
must add authorization checks. Live ANAF endpoint/identity semantics also require
credentialed certification.

In particular, live certification must confirm browser/certificate middleware
behavior and whether ANAF's authenticated list response always supplies the complete
top-level `cui` set. Until then Diana does not claim that OAuth callback alone proves
the selected client's CUI.


## Client onboarding entry point — 2026-09-15

Client detail now links derived readiness to the existing certificate/browser/USB onboarding card. No certificate upload/parse/secret storage is added. OAuth cannot verify CUI coverage and no certificate expiry metadata is available. OAuth start requires Idempotency-Key and production Store persists encrypted replay state for the unconsumed attempt; existing hashed/expiring single-use callback remains. Current actor grants are checked before provider token exchange. Safe status distinguishes unusable refresh credentials from refreshable access expiry and shows inactive-client operational pause. Inactive clients cannot start new authorization/manual sync, and scheduler/new queued sync starts stop. Existing source-document completion and disconnect history remain unchanged. See CLIENT_MANAGEMENT_ONBOARDING.md.
