# First real local E2E run

Prepared 2026-09-15. **Readiness: NO for live authorization until callback acceptance is verified.** This is a user-run procedure. No live ANAF login, certificate interaction, SPV sync or SAGA import was performed during preparation. Container builds/startup and persistence checks also remain user-run. Approved domain behavior is unchanged.

## Prerequisites and callback gate

Install Docker with Compose v2 (supports health/completion dependencies), make and Python 3. Docker must have internet access to image registries, npm and Go dependencies. Host Go/Node/Atlas are unnecessary for the container workflow. Keep ports 5173, 8080, 8081, 5442 and 6382 free. This is the only local stack; tests and interactive development use the same PostgreSQL and Redis services.

You need an ANAF developer application enrolled for E-Factura, its client ID/secret, and a valid qualified digital certificate registered with SPV PJ rights for the client's CUI (legal representative, designated representative or authorized agent). Install the certificate provider's OS/browser/token middleware. Diana has no certificate upload.

**OAUTH CALLBACK: ACTION REQUIRED.** Code implements `/api/v1/integrations/anaf/callback`. The proposed local address is exactly:

`http://127.0.0.1:8080/api/v1/integrations/anaf/callback`

The official registration instructions require a callback registered for the application, but do not explicitly establish acceptance of HTTP localhost/127.0.0.1. Do not treat the repository default or third-party examples as verification. Confirm this exact callback is accepted in your developer registration, or obtain official ANAF confirmation before authorization. `localhost` and `127.0.0.1` are different registrations. If rejected, stop: obtain provider requirements and choose a controlled HTTPS development hostname/reverse proxy or approved tunnel. No tunnel, TLS bypass or public exposure is implemented here. Changing the callback requires only configuration and matching ANAF registration, but secure routing/access must be designed first.

## Secret preparation

From the repository root:

```sh
make setup
```

This exclusively creates `.env.local` with mode 0600 and a random 32-byte hex encryption key once. It refuses to overwrite an existing file. It prints no key. Edit that file privately in your editor. Fill `SPV_OAUTH_CLIENT_ID` and `SPV_OAUTH_CLIENT_SECRET`. Preserve `SPV_TOKEN_ENCRYPTION_KEY` permanently with the database. Provide `GEMINI_API_KEY` only if testing real contract extraction. Provision the local Diana user with `go run ./cmd/authuser provision --email ... --all-clients`; the command prompts without echo. Do not paste secrets into shell commands, issue trackers or logs. Do not run `docker compose config` with resolved environment output, `docker inspect`, `env`, or token-table dumps when sharing diagnostics.

`.env.local` and certificate/key files are ignored. Backend image context excludes env/key material; frontend mounts only source/configuration and receives only public VITE values. Business code reads injected environment, never secret files. Docker administrators can still inspect container environment; use a private developer machine and protected backups. Cloud uses Secret Manager injection of the same application names.

The template contains public local PostgreSQL credentials for isolated development infrastructure. They are not production credentials. Cloud DATABASE_URL/REDIS_URL are secrets. `SPV_OAUTH_CLIENT_ID` is not a token but treat it as private integration configuration.

## Configuration and profiles

| Profile | Mechanism | Provider behavior |
|---|---|---|
| local | `compose.yaml` + ignored `.env.local`, `APP_ENV=development` | one database/API/frontend; SPV is disabled until credentials are explicitly configured |
| engineering test | APP_ENV=test, isolated database/queue, explicit fixture endpoint/extractor | fixture providers permitted intentionally; never describe as real E2E |
| GCP test / production | APP_ENV=production + injected real configuration | fake SAGA/extractor rejected; official ANAF endpoints when SPV enabled; no seed |

GCP test may deliberately use `SPV_ENVIRONMENT=TEST`; this is ANAF's real test service, not Diana's fake HTTP server. Real client invoices require PRODUCTION. Do not run engineering seed/flush/drop scripts against the real project.

`local-real` requires enabled SPV, complete credentials, stable valid hex key, file SAGA, Gemini adapter, explicit local audit operator, and API inline dispatch disabled. It rejects fake/custom ANAF endpoint overrides. Invalid bool/int/duration syntax and unknown SPV environments fail clearly. Missing Gemini key returns extraction unavailable; there is no fixture fallback.

PUBLIC: VITE_BACKEND_READS_ENABLED, VITE_API_PROXY_TARGET, VITE_ENVIRONMENT_LABEL; only public values belong in VITE variables.
APPLICATION: APP_ENV, HTTP_ADDRESS, WORKER_HTTP_ADDRESS, LOG_LEVEL, FRONTEND_BASE_URL, SPV_ENVIRONMENT/API_BASE_URL/AUTHORIZE_URL/TOKEN_URL/OAUTH_REDIRECT_URI, all worker/outbox and authentication windows and limits, SAGA_MODE, CONTRACT_EXTRACTOR_MODE, GEMINI_CONTRACT_MODEL/API_BASE_URL, OTEL_ENABLED/EXPORTER_OTLP_ENDPOINT.
SECRET: DATABASE_URL and REDIS_URL where credentialed, SPV_OAUTH_CLIENT_ID/SECRET, SPV_TOKEN_ENCRYPTION_KEY, GEMINI_API_KEY. Complete existing defaults are in `backend/internal/platform/config/config.go`; no duplicated alternative application configuration system is added.

The Gemini model in the template is the repository's existing default; availability/quality is not live verified. Verify a supported model for your account before contract extraction. Do not claim a live Gemini test based on compilation.

## Startup, migrations and URLs

```sh
make dev
```

`make dev` creates `.env.local` when absent, performs a redacted preflight, then runs `docker compose up --build`. PostgreSQL and Redis wait for health; a one-shot Atlas service applies pending migrations; API and worker start only after migration success and Redis health. API and worker readiness are required before the frontend starts. For redacted configuration plus current container/log diagnostics use `make diagnose`.

UI: http://localhost:5173
API liveness/readiness: http://127.0.0.1:8080/healthz and /readyz
Worker liveness/readiness: http://127.0.0.1:8081/healthz and /readyz
Metrics: `/metrics` on each runtime.
PostgreSQL host TCP: 127.0.0.1:5442; Redis: 127.0.0.1:6382. Inside Compose use postgres:5432 and redis:6379.

Frontend requests `/api/v1` through the Vite same-origin proxy to API. No permissive CORS is introduced. If running frontend on the host, use:

```sh
VITE_BACKEND_READS_ENABLED=true VITE_API_PROXY_TARGET=http://127.0.0.1:8080 VITE_ENVIRONMENT_LABEL='LOCAL — REAL INTEGRATIONS' npm run dev
```

Stop the container frontend first to free 5173. These are public values only. The badge identifies configured real integrations, not proof that ANAF authorization succeeded. The connection card's state remains authoritative.

Latest migration: **000015_client_management_onboarding.sql**. No old migration changed. Fresh and incremental application use the same Atlas procedure:

```sh
make migrate
make migration-status
```

A fresh empty DB receives 000001–000015; incremental startup applies only pending versions. `make migration-status` invokes Atlas `migrate status --env local`. No auto schema creation, drop or recreation occurs. Take backups before future migrations. There are no supplied down migrations or tested automated rollback; recover using a verified backup or a reviewed additive forward migration. Do not edit applied migrations/checksums or use `atlas migrate set` to conceal a failed migration.

## Client, profile and ANAF authorization

1. Open `/clients`, choose `Adaugă client` and enter the real company/CUI through the existing form. Keep this project empty of demo clients.
2. In client settings, enter only factual fiscal/accounting profile information with effective dates covering invoice issue dates. Unknown remains unknown. Approve only with actual reviewed evidence. Account codes/chart policy must come from your accountant; do not invent an approved production pack.
3. Keep automatic ANAF import enabled for the active client. Review readiness warnings. SAGA enablement is a separate existing setting, not evidence that classifications/mappings are valid.
4. Only after the callback gate is resolved, use the client ANAF/SPV card and `Conectează ANAF`.
5. Diana creates durable hashed OAuth state with expiry/replay protection and returns an authorization URL. The browser navigates to HTTPS logincert.anaf.ro. Select the qualified certificate when requested. Enter PIN only in the certificate provider prompt, never in Diana.
6. ANAF redirects with authorization code/state (or error) to the registered callback. API validates state and exchanges the code using client credentials and JWT token mode. Diana encrypts access/refresh tokens with AES-GCM in PostgreSQL, persists expiry/environment/client association and returns to the client page.
7. Expect CONNECTED only after successful persistence. Diana receives OAuth code/state and token responses, not the certificate file, private key or PIN. Certificate public identity metadata/serial is not separately extracted or persisted by the implemented flow. Diana persists hashed state and encrypted token/connection metadata.

**Successful OAuth is not proof of this CUI's coverage.** Connection identityValidation is NOT_AVAILABLE. Successful SPV list access for the selected CUI without an application-level error is operational evidence; a returned document plus authoritative buyer-CUI validation establishes invoice association. An empty successful list is not a complete certificate coverage inventory. ANAF can issue tokens that do not authorize requested fiscal resources.

## Endpoint evidence — retrieved 2026-09-15

[Official ANAF OAuth procedure](https://static.anaf.ro/static/10/Anaf/Informatii_R/API/Oauth_procedura_inregistrare_aplicatii_portal_ANAF.pdf) confirms authorize/token HTTPS endpoints and api.anaf.ro as the token-protected host. [Official developer registration](https://www.anaf.ro/InregOauth/) explains certificate-bound user identity. The procedure links the [Ministry e-Factura API PDF](https://mfinante.gov.ro/static/10/eFactura/prezentare%20api%20efactura.pdf); that PDF could not be retrieved here (web 502 and curl connection reset, including authorized retry). Therefore full production FCTEL route verification remains **PARTIAL / ACTION REQUIRED**, not live certification.

Configured existing OAuth base: `https://api.anaf.ro/prod/FCTEL/rest`; list path `/listaMesajePaginatieFactura`, download `/descarcare`. TEST substitutes `/test/`. Authorize `https://logincert.anaf.ro/anaf-oauth2/v1/authorize`; token `https://logincert.anaf.ro/anaf-oauth2/v1/token`. The [older official certificate-call service listing](https://static.anaf.ro/static/10/Anaf/Informatii_R/Servicii_web/url_eFactura.html) uses webserviceapl.anaf.ro; do not substitute that host into Diana's OAuth bearer-token adapter. Before live execution inspect the Ministry PDF/official current service material and confirm these route names and pagination parameters. No repository endpoint assumption was promoted to VERIFIED without evidence.

## First sync and source inspection

The worker embeds the periodic scheduler. It enqueues eligible connections immediately at startup and hourly afterward; manual `Sincronizează acum` is also available after connection. After authorizing, real sync can therefore run automatically on the next scheduling pass; do not authorize until ready. Do not add a second scheduler. Initial max window remains 60 days; subsequent overlap 72 hours. Internal timestamps use UTC; raw ANAF data_creare remains preserved without invented timezone. UI connection dates display Europe/Bucharest.

1. In the connection card choose `Sincronizează acum`. Observe started notice, last sync state, last success and safe failures. Reload/revisit the page for current state; no new live progress dashboard is added.
2. Inspect worker logs for safe counts/pages and source-document/invoice identifiers. List completion indicates discovery, not necessarily completed ingestion. Processing may continue afterward in document jobs.
3. Use the SQL diagnostics below to distinguish discovery, raw download, parse/ingestion and queue failures.
4. Open invoice detail and Task Inbox. Compare UBL facts privately with the original in a controlled DB inspection/export; never put full XML in ordinary logs. No new raw download UI was invented.

Raw source identity is unique `(connection_id, external_message_id)`; original ZIP bytes are persisted in bytea through the existing DocumentStore abstraction, with SHA-256, original metadata, parser/version and invoice link. Re-sync the same range and verify counts/identity do not duplicate. This is technical delivery idempotency, distinct from the unchanged provisional business fingerprint. A distinct delivery of the same business invoice can still reach DUPLICATE under existing policy; record sanitized evidence if unexpected.

Parser distinguishes INVOICE/CREDIT_NOTE, maps supplier/buyer CUI, number/date/currency/totals/lines/VAT facts and ACCOUNTING_DOMAIN_V2 source lineage. It rejects unsafe/ambiguous ZIP input and wrong buyer identity. CreditNote parsing is supported, but approved downstream CreditNote/storno accounting/SAGA treatment is deferred: do not claim an end-to-end supported credit-note export. Parser uses neither OCR nor Gemini/LLM.

## Pipeline, contracts and classification

Normal statuses: DOWNLOADED → ARCHIVED → MATCHING. Without a matching contract: AWAITING_CONTRACT plus MISSING_CONTRACT task is a valid stopping point. Multiple candidates may need AWAITING_MATCH_CONFIRM. Once actual contract matching is resolved: DEDUPE_CHECKED → HEADER_READ → LINES_READ → classification review, usually AWAITING_REVIEW with CLASSIFICATION task. Observe actual status/history; do not force transitions or insert fake contracts.

To continue, upload the relevant real/sanitized PDF through existing contract ingestion, inspect actual Gemini extraction, correct it and explicitly confirm the contract. Missing key/model/access or provider failure remains unavailable/failed, never fixture extraction. Existing contract availability/outbox flow resumes matching. No live Gemini test was performed.

**No approved production accounting pack exists.** API/worker use DomainPolicy with AllowTestOnly=false for V2. TEST_ONLY profiles/packs are ineligible for automatic decisions; this guard is independent of environment. Empty real project has no seed authority. Missing evidence must remain review, not invented accounting. ACCOUNT, VAT_TREATMENT, VAT_DEDUCTIBILITY and EXPENSE_TAX_TREATMENT require valid typed decisions/evidence. Optional manual testing uses those four UI decisions and appropriate profile/account mappings; shared readiness may still block XML without all required prerequisites. Such decisions are test inputs, not a production rule pack.

SAGA_MODE=file selects existing real XML artifact generation. Generation does not mark imported/exported. If all shared readiness checks pass, generate immutable artifact, download exact bytes, manually import into SAGA desktop and explicitly confirm only after real import. Stop at safe review when prerequisites are missing. No fake SAGA, auto import or auto confirmation.

## Safe operational diagnostics

```sh
make status
curl --fail http://127.0.0.1:8080/healthz
curl --fail http://127.0.0.1:8080/readyz
curl --fail http://127.0.0.1:8081/readyz
docker compose logs --tail=100 api worker
docker compose exec -T redis redis-cli ping
make migration-status
```

API readiness checks DB and Redis within a bounded probe; worker readiness requires worker-started flag, DB and Redis. Neither verifies ANAF certificate permissions or migration version. Migrations gate startup separately. `/healthz` is liveness, not integration availability. JSON slog, request IDs, OTel (optional) and metrics exist. Outbox/pipeline logs carry job/correlation/invoice identifiers; SPV logs carry connection/document/invoice IDs, page/count/duration. No tokens/PIN/keys/raw XML are deliberately logged. Third-party HTTP errors may include request URLs; do not publish full unreviewed diagnostics. Redis Asynq jobs are at-least-once and retain retries; DB guards remain essential.

Run the following read-only query privately. IDs and hashes are metadata, still avoid public posting of real client identifiers:

```sh
docker compose exec -T postgres psql -U diana -d diana <<'SQL'
SELECT id, lifecycle FROM clients ORDER BY created_at;
SELECT id, client_id, environment, status, last_sync_status,
       last_sync_started_at, last_sync_finished_at, last_successful_sync_at
FROM spv_connections;
SELECT id, client_id, external_message_id, processing_status, failure_kind,
       octet_length(raw_document) AS zip_bytes, content_sha256,
       parser_type, parser_version, invoice_id, attempts
FROM spv_source_documents ORDER BY discovered_at DESC LIMIT 20;
SELECT client_id, model_version, pipeline_status, saga_status, count(*)
FROM invoices GROUP BY 1,2,3,4;
SELECT i.id, i.model_version, i.pipeline_status, count(l.id) AS lines
FROM invoices i LEFT JOIN invoice_lines l ON l.invoice_id=i.id
GROUP BY i.id ORDER BY i.id;
SELECT invoice_id, task_type, status, blocker_code FROM validation_tasks;
SELECT status, count(*) FROM outbox_entries GROUP BY status;
SELECT connection_id, external_message_id, count(*)
FROM spv_source_documents GROUP BY 1,2 HAVING count(*) > 1;
SELECT invoice_line_id, dimension, review_status, model_version
FROM line_classifications;
SELECT invoice_id, event_type, occurred_at, actor_kind, actor_id,
       validation_task_id FROM activity_events ORDER BY occurred_at DESC LIMIT 50;
SQL
```

Follow connection ID → external message ID → source-document ID → invoice ID → invoice activity/task/classification rows → SAGA card/artifact if reached. Inspect invoice.source_facts and line source_facts only privately when comparing buyer/type/date/totals/VAT facts; do not select all token/raw columns. UI detail and source lineage establish the join without a special local ingestion path.

## Restart, persistence, shutdown and reset

```sh
make down
make dev
```

Named volumes preserve database, encrypted tokens/connection state, ZIPs/invoices/audit/artifacts and Redis AOF. Reuse the exact key and same Compose project identity. Record sanitized counts/hash/source link before shutdown; compare after restart and perform repeat sync to verify technical idempotency. Normal down never deletes volumes. `make reset` asks for DELETE and explicitly destroys the single local environment's volumes; it is not startup. Preserve encrypted backups and key separately before any reset. Changing the key without a rotation procedure makes old tokens unreadable.

## Troubleshooting and boundaries

- Empty/incomplete env: edit privately; local-real should fail configuration before serving requests.
- Port conflict: stop conflicting runtime; callback/frontend origin must still match their configuration and registration.
- Atlas failure: API/worker do not start. Inspect migration status, back up and repair with reviewed forward steps; never run E2E reset scripts here.
- Redis outage: readiness fails and enqueue can fail; restore Redis, inspect retry/outbox status before retrying. No synchronous fallback.
- OAuth callback mismatch/rejection: stop authorization and resolve the provider registration/HTTPS requirement; never bypass TLS.
- CONNECTED but SPV denied: OAuth is insufficient CUI proof; inspect SPV PJ certificate rights and E-Factura application scope. No automatic cross-client reassignment.
- Raw stored but FAILED parse: inspect safe failure kind; compare privately with supported UBL/ZIP shapes and buyer-CUI guard. Report sanitized evidence, do not weaken validation.
- No contract: expected valid pause. Real Gemini extraction unavailable: configure/verify provider or remain paused.
- No production pack/profile/mapping: expected review/readiness blocker. Do not run devseed or use TEST_ONLY to remove it.

Authentication V1 now applies locally too: there is no `LOCAL_OPERATOR` impersonation or demo-actor fallback. Every protected API request requires a valid Diana session and derives audit attribution/client grants from that user. No deployment is included.

Success requires a real client, actual verified ANAF authorization, real message/ZIP/hash/parser lineage, Invoice + lines, unchanged pipeline/missing-contract pause, no TEST_ONLY automation, safe classification review when matching permits, and restart persistence. Real-data observations and SAGA desktop acceptance are performed by the user only.
