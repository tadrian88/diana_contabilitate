# Backend ANAF / SPV inbound ingestion

Status: approved / frozen after user-run verification on 2026-09-14.

## Architecture and flow

`internal/spv` owns the external-source application boundary. `HTTPClient` is an
ANAF-only adapter, `InvoiceDocumentParser` isolates structured formats, and
`Service` orchestrates discovery, archival, technical validation and native invoice
ingestion. Ent/pgx persistence remains in `internal/platform/postgres`; Asynq
handlers contain no XML or invoice persistence logic.

The flow is:

`SPV P message -> SPVSourceDocument(DISCOVERED) -> download immutable ZIP ->
UBL parser -> buyer/CUI guard -> invoicing.Service.Ingest -> Invoice(DOWNLOADED)
+ InvoiceLines + ActivityEvent + OutboxEntry -> existing Module 7 worker pipeline`.

No OCR, outgoing submission, ANAF PDF conversion, new `PipelineStatus`, accounting
rule or `ValidationTask` type is introduced.

## Authentication and account ownership

The platform OAuth client ID/secret and token-encryption key are environment
secrets. Access and refresh tokens are encrypted with AES-256-GCM before storage.
Each `SPVConnection` belongs to exactly one `AccountingClient` and copies its CUI;
one environment/CUI cannot belong to two clients. Qualified-certificate
authentication belongs to the browser/ANAF identity-provider boundary; Diana does
not upload certificate files or receive their PIN. The additive Client Detail UX
starts the authorization-code flow without requiring the accountant to use
`cmd/spvconnect`; that command remains development/break-glass only.

`spvconnect authorize` prints the ANAF URL for a caller-supplied, one-time state.
State generation/callback verification belongs to the operator integration; the CLI
does not pretend to be an authenticated web callback. `spvconnect exchange` stores
the returned tokens against an existing accounting client. The worker refreshes an
access token one minute before expiry with the configured OAuth application.

`cmd/fakeanaf` is a synthetic local protocol fixture for development only. It exposes
an explicit synthetic provider authorization/consent page plus token/list/download
shapes without credentials, private keys or customer data.

## Sync and checkpoint

The worker scheduler enqueues one `spv:sync_account` job per active connection.
Listing uses only ANAF filter `P` (received), all pages, and a UTC millisecond
window. The first successful sync covers at most 60 days. Later successful syncs
start three days before `last_successful_sync_at`. A page failure fails the sweep;
the durable checkpoint advances only after every page was accepted. This deliberately
improves armqu's page-skipping behavior, which could leave a silent gap.

ANAF `data_creare` is retained verbatim because its timezone is not established by
the inspected source/official material. Diana's own discovered/downloaded/processed
timestamps are UTC. The query window is authoritative for checkpointing.

API URLs, hourly schedule, windows and 100ms call pacing are configurable. Pacing
is a conservative default, not represented as an official per-service quota.

## Source document and idempotency

`SPVSourceDocument` has a unique `(connection_id, external_message_id)` identity.
Duplicate pages, overlapping windows, job redelivery, restart and concurrent workers
therefore converge on one delivery. Its deterministic `spv_reference` also enters
Module 2's existing technical-ingestion guard. The SHA-256 hash verifies the exact
original ZIP but does not replace the ANAF message ID.

The source record stores provenance, raw ZIP bytes, content type/hash, parser name
and version, timestamps, claim lease, attempts and sanitized failure. A PostgreSQL
implementation of `DocumentStore` is the smallest durable adapter for this module.
Production object storage is deferred; replacing the adapter must preserve immutable
bytes and hash semantics.

The operational states `DISCOVERED`, `PROCESSING`, `DOWNLOADED`, `PROCESSED`,
`FAILED` never enter the invoice state machine. A lease recovers documents left in
processing/downloaded by a crashed worker.

## Parsing and technical validation

`UBLParser` supports UBL 2.1 / CIUS-RO roots `Invoice` and `CreditNote`. CII is
explicitly unsupported. It maps only Diana fields: document identity/dates, supplier,
buyer CUI, currency, gross tax-inclusive total and lines. Exact decimal strings are
used throughout; source values beyond Diana's current four-decimal persistence scale
are rejected rather than rounded silently. Where UBL omits line tax amount, the
parser derives `net * stated VAT rate / 100` to satisfy Diana's required line shape;
this is structural mapping, not a legal VAT validation.

The parser rejects malformed documents, missing required identity, wrong buyer,
unsupported root, traversal-like ZIP entries, excessive file count, multiple invoice
XML candidates and bounded-size violations. Go `encoding/xml` performs no external
entity/network resolution. Errors/logs never contain raw XML.

## Retry, jobs and failures

`spv:sync_account` lists/discovers and enqueues `spv:ingest_document`; job payloads
contain only connection/source IDs. HTTP 429, 5xx, network, Redis and DB failures are
transient and use bounded Module 7/Asynq retry. Malformed ZIP/XML, invalid required
fields, unsupported format, inactive/environment-mismatched connection and wrong
buyer are permanent. Both are durable on the source document; permanent failures
are not retried indefinitely and neither creates a `ValidationTask`.

A downloaded payload is reused on recovery, avoiding another ANAF download. The
successful native ingest is itself idempotent, closing the crash window between
invoice commit and source-document completion.

## Technical versus business duplicate

Technical duplicate means the same connection/message delivery and creates no second
invoice. Two different message IDs remain two source deliveries. If their invoice
headers hit Module 2 `PROVISIONAL_V1`, the second native invoice follows the existing
terminal business-duplicate policy. No fingerprint fields were changed.

## Observability and audit

Structured worker logs use source, connection/source-document/invoice IDs, page/count,
result and duration, never tokens or XML. Metrics add total SPV syncs, discovered
documents, successful/failed ingestions and skipped technical duplicates without
high-cardinality labels. Native ingestion writes `INVOICE_INGESTED` with an ANAF/SPV
source-delivery reference; HTTP retries are not business audit events.

## Configuration

| Variable | Default / requirement |
|---|---|
| `SPV_ENABLED` | `false` |
| `SPV_ENVIRONMENT` | `TEST`; `PRODUCTION` selects production default URL |
| `SPV_API_BASE_URL` | environment-specific `https://api.anaf.ro/.../FCTEL/rest` |
| `SPV_TOKEN_URL` | `https://logincert.anaf.ro/anaf-oauth2/v1/token` |
| `SPV_OAUTH_CLIENT_ID`, `SPV_OAUTH_CLIENT_SECRET` | required when enabled |
| `SPV_TOKEN_ENCRYPTION_KEY` | required 64-character hex AES key |
| `SPV_SYNC_INTERVAL` | `1h` |
| `SPV_INITIAL_WINDOW` | `1440h` (60 days) |
| `SPV_OVERLAP_WINDOW` | `72h` |
| `SPV_DOCUMENT_CLAIM_TTL` | `10m` |
| `SPV_MIN_CALL_INTERVAL` | `100ms` |

## Tests

Unit tests cover sanitized Invoice/CreditNote parsing, exact line mapping, CII
rejection, ZIP attacks/ambiguity, flexible ANAF IDs, incoming filter, HTTP failure
classification, token encryption, native ingestion and wrong-buyer quarantine.
PostgreSQL tests cover concurrent discovery/claim and fake-ANAF-to-native-pipeline.
Redis tests cover sync-to-document job dispatch. Existing Module 2 and Module 7 tests
remain the regression proof for business duplicates, outbox and worker restart.

### User-run verification — 2026-09-14

The complete local/fake-ANAF acceptance gate passed:

- formatting, Ent generation, Atlas checksum and Go package discovery;
- migration from version `000007` and all eight migrations on an empty database;
- focused SPV client, AES-GCM, UBL/ZIP parser and native-ingestion tests;
- complete Go unit, PostgreSQL integration and Redis/Asynq integration suites;
- repeated concurrent discovery/claim verification and Go race checks;
- frontend typecheck, all 68 Vitest tests and production build;
- all Playwright suites: base plus backend modules 1, 3, 4, 5, 6 and 7;
- local fake-ANAF runtime: one discovered document was archived and ingested once,
  then progressed through the existing outbox/worker pipeline. Overlapping later
  syncs rediscovered no second technical delivery.

The build emitted only the existing Rollup annotation and bundle-size warnings.
No live ANAF certification was performed, as intended by this acceptance gate.

## Open decisions

- **EXTERNAL API VERIFICATION REQUIRED:** ANAF's official materials expose a current
  `api.anaf.ro` OAuth model but an older service page still lists
  `webserviceapl.anaf.ro`; confirm the production FCTEL base during credentialed
  certification. It is configuration, not hard-coded policy.
- **PRODUCT / ARCHITECTURE DECISION REQUIRED:** choose production object storage and
  retention/encryption-at-rest policy for original ZIPs. PostgreSQL is durable for
  the current module, not a claim that large binary retention belongs there forever.
- OAuth onboarding and connection management are implemented in the authorized
  additive Connection UX module; see `Docs/ANAF_SPV_CONNECTION_UX.md`. Full
  authentication/RBAC remains required before production exposure.
- **PRODUCT DECISION REQUIRED:** define ANAF `data_creare` timezone before populating
  `source_created_at`; the raw timestamp is preserved now.
- Existing contract, accounting/VAT, duplicate-fingerprint and real SAGA decisions
  remain untouched.
