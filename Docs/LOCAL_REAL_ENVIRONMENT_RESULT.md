### LOCAL REAL ENVIRONMENT RESULT

Prepared 2026-09-15. Live readiness is **NO** pending external callback/route checks and user-run runtime validation.

### EXECUTIVE SUMMARY

Added an isolated one-command Compose runtime using existing frontend, API, worker/dispatcher/scheduler, PostgreSQL and Redis. No seed, fake ANAF/SAGA/extraction fallback, domain redesign, cloud deployment or live fiscal action. Missing production accounting pack remains a valid classification review blocker.

### CURRENT RUNTIME TOPOLOGY

| COMPONENT | LOCAL | CLOUD TARGET | PARITY |
|---|---|---|---|
| frontend | Vite, 5173, API proxy | compiled frontend + gateway | same app, build/routing differs |
| API | cmd/api, 8080 | Cloud Run service | same executable/application |
| worker | cmd/worker, health 8081 | persistent worker pool/VM; decision required | same Asynq consumer/dispatcher |
| scheduler | embedded in singleton worker | same singleton initially | exactly one authority |
| PostgreSQL | 17, container 5432/host 5442 | Cloud SQL 17 proposed | same DSN/Ent/Atlas |
| Redis | 7, container 6379/host 6382 | managed Redis | same Redis/Asynq configuration |
| migration | Atlas one-shot after PG healthy | serialized release step | same migration files |

### CHANGES MADE

New backend Dockerfile, separate real Compose project/volumes, Make targets, full env template and private config preparation tool. Tightened config syntax/SPV environment/key validation, local-real adapter/dispatch/endpoint guards. Added explicit local audit operator binding, API DB+Redis readiness, accurate UI environment labels and frontend real-profile repository guard. Vite loads only VITE names; frontend receives/mounts no secret file. Existing dirty/untracked work preserved.

### LOCAL-REAL CONFIGURATION

Root .env.example → ignored .env.local. APP_ENV=local-real; SPV_ENABLED=true; SPV_ENVIRONMENT=PRODUCTION; SAGA_MODE=file; CONTRACT_EXTRACTOR_MODE=gemini; PIPELINE_DISPATCH_ENABLED=false. Authentication V1 requires a provisioned Diana user and no longer accepts `LOCAL_OPERATOR` impersonation. Engineering APP_ENV=test fixtures remain separate; GCP real test uses APP_ENV=production and official ANAF TEST/PRODUCTION as appropriate.

### REQUIRED SECRETS

SPV_OAUTH_CLIENT_ID, SPV_OAUTH_CLIENT_SECRET, SPV_TOKEN_ENCRYPTION_KEY; DATABASE_URL and REDIS_URL when credentialed. GEMINI_API_KEY for optional real extraction. No certificate/private-key/PIN configuration exists.

### STARTUP COMMAND

```sh
make setup
# Fill .env.local privately, resolve callback/endpoint gates before live authorization.
make dev
```

### LOCAL URLS

UI http://localhost:5173; API http://127.0.0.1:8080; worker health http://127.0.0.1:8081. Postgres host 5442; Redis host 6382. All exposed ports bind loopback.

### DATABASE / MIGRATIONS

PostgreSQL 17 named volume. Latest 000015_client_management_onboarding.sql; old files unchanged. Atlas apply gates startup; make migrate / make migration-status are canonical fresh/incremental/status procedures. No supplied automatic rollback; verified backup or reviewed forward correction.

### REDIS / ASYNQ

Real Redis 7/AOF named volume; Asynq queue workflow; separate worker and durable outbox dispatcher/retries retained. No inline fallback. No Redis connection/queue test executed in preparation.

### API / WORKER

Separate executables from one image, migration/Redis dependency gates, SIGTERM graceful shutdown, Compose 35s stop grace. API inline dispatcher disabled. Worker owns dispatcher and scheduler. Explicit local operator overrides legacy demo attribution before routing; trusted existing context takes precedence.

### REAL ANAF CONFIGURATION

API/worker already construct HTTPClient; fakeanaf is an explicit fixture executable/endpoint, not an automatic provider fallback. local-real now rejects unofficial endpoint overrides and missing SPV config/key. Invalid syntax, unknown APP_ENV or unknown SPV environment no longer silently defaults. Production also rejects API inline dispatch.

### ANAF ENDPOINT VERIFICATION

Retrieved 2026-09-15: official ANAF OAuth procedure verifies logincert.anaf.ro authorize/token and api.anaf.ro token-protected host. Full production FCTEL paths remain PARTIAL: official linked Ministry PDF returned web 502/curl reset including authorized retry. See source URLs and exact configured routes in LOCAL_REAL_E2E.md. No live route certification claimed.

### OAUTH CALLBACK

**ACTION REQUIRED.** Proposed http://127.0.0.1:8080/api/v1/integrations/anaf/callback is implemented, but official inspected instructions do not explicitly verify HTTP loopback acceptance. Verify exact registered callback or official confirmation before authorization. Rejection blocks login; controlled HTTPS domain/proxy/tunnel is a future minimum setup only after evidence. No workaround added.

### CERTIFICATE FLOW

Qualified certificate with SPV PJ rights used at ANAF HTTPS IdP through browser/token middleware. PIN remains in certificate prompt. Diana gets code/state, exchanges token server-side, encrypts access/refresh tokens and persists state hash, environment/client/expiry/sync metadata. Diana does not upload/receive/store certificate file, private key or PIN; no certificate serial/coverage inventory is persisted separately. OAuth alone is not CUI coverage proof.

### FIRST REAL SYNC

After callback gate, create real client/profile; connect via ANAF card; confirm CONNECTED; request Sincronizează acum; inspect connection sync dates/status, worker pages/counts and source SQL. Embedded scheduler also runs on startup/hourly; authorization enables future automatic scheduling. Discovery completion is distinct from document processing. Exact procedure/queries in runbook.

### RAW DOCUMENT STORAGE

**KEEP FOR TEST.** Existing DocumentStore/PostgreSQL bytea; ZIP/hash/identity/parser/version/invoice link retained. No GCS migration. Cloud storage capacity, retention, backups and encryption policy remain decisions.

### REAL INVOICE PIPELINE

DOWNLOADED → ARCHIVED → MATCHING; conditional contract pause/confirmation; DEDUPE_CHECKED → HEADER_READ → LINES_READ → AWAITING_REVIEW where evidence missing. Real data may expose DUPLICATE or parse/review blockers; record actual evidence. Parser handles Invoice/CreditNote and V2 facts without OCR/LLM; downstream CreditNote/storno acceptance remains deferred. Buyer-CUI guard remains authoritative.

### CONTRACT MATCHING EXPECTATION

No contract means AWAITING_CONTRACT plus MISSING_CONTRACT task, valid success at that checkpoint. Upload relevant actual/sanitized PDF through existing ingestion only if continuing. Gemini is real or unavailable, never silently fixture extraction; no live extraction test executed.

### CLASSIFICATION EXPECTATION

Missing approved production pack → safe CLASSIFICATION review/AWAITING_REVIEW when pipeline reaches classification. Existing DomainPolicy AllowTestOnly=false in API/worker rejects TEST_ONLY authority independent of runtime. No production rules created. Optional manual four typed decisions remain testing only and must satisfy shared readiness.

### SAGA CONFIGURATION

SAGA_MODE=file, immutable real XML artifact, exact-byte download, manual desktop import then explicit confirmation. Invalid classification/profile/mapping continues to block generation. No real import attempted or auto EXPORTED marking.

### DEVSEED / MOCK ISOLATION

Real Compose has separate DB/Redis volumes and no seed service. Existing devseed environment guard already rejects local-real; added regression assertion. Demo/test profiles retain explicit fixtures and isolated test scripts. Never run existing E2E reset/seed scripts against this project.

### SECRET / ENCRYPTION PERSISTENCE

Private .env.local created once with exclusive file creation/0600; stable valid 32-byte hex key generated only at preparation. API/worker consume injected env. Key must survive alongside DB; changing it breaks old token decryption. Cloud Secret Manager injection uses the same names. No secret values committed.

### RESTART / DATA PERSISTENCE

`make down` then `make dev` preserves the canonical named volumes and key file. Explicit `make reset` destroys the local volumes after DELETE. User must compare source/invoice/connection/hash counts after restart; not yet executed.

### LOGGING / TRACEABILITY

JSON slog/request IDs/OTel and metrics retained. SPV logs now include job IDs and explicit sync/document start events alongside connection/document/invoice IDs and counts; existing pipeline jobs have correlation/job/invoice IDs. Join message→source→invoice→activity/task/classification→artifact using private metadata queries and UI. Never dump token columns/raw XML into shared diagnostics.

### HEALTH / READINESS

API healthz liveness; readyz now checks DB+Redis with 3s probe bound and preserves outbox metrics Stats delegation. Worker readyz requires started flag and DB+Redis; migrations gate process startup separately. None proves ANAF permission. Added fail-closed readiness test source; runtime probes unexecuted.

### LOCAL → GCP PARITY

Docs/LOCAL_CLOUD_PARITY.md maps concerns/configuration. Business code difference NONE; existing deferred production identity integration is explicitly an exception. No deployment manifests existed in inspected tree; no GCP release prepared/executed.

### CLOUD RUN WORKER ANALYSIS

Long-lived Redis consumer requires persistent CPU and instances. Worker pool/VM preferred pending regional/capability choice. Cloud Run service requires instance-based CPU/billing, min>=1 and singleton initially; finite Job is unsuitable for infinite worker. Existing 15s/sequential shutdown exceeds service 10s grace; lifecycle/cancellation/recovery verification required before choosing service. Queue is at-least-once; Redis durability and DB idempotency remain essential.

### SCHEDULER ANALYSIS

Exactly one embedded scheduler in first local/cloud test worker. Multiple replicas currently each schedule; no separate scheduler command/enable flag. Scaling requires explicit singleton/external authority choice, never duplicate Cloud Scheduler plus embedded authority.

### DEPLOYMENT DECISIONS REQUIRED

Worker runtime/shutdown, scheduler scaling, trusted cloud auth/grants, frontend routing/HTTPS, SQL networking/HA/backup, managed Redis networking/durability, secret rotation, raw retention/capacity and real provider acceptance. No platform migration performed.

### BUGS FOUND / FIXED

| BUG | ROOT CAUSE | FIX | REGRESSION TEST |
|---|---|---|---|
| mistyped SPV environment becomes TEST | catch-all else | explicit enum validation | TestLocalRealFailsClosed |
| invalid runtime strings silently default | parsers discard errors | validate syntax before defaults | TestInvalidRuntimeSyntaxFailsRatherThanUsingDefaults |
| nonhex 64-character key accepted | length-only validation | hex decode and 32-byte requirement | TestLocalRealFailsClosed |
| real local may choose fake SAGA/fixture endpoint | only production fake-SAGA guard | local-real fail-closed checks | TestLocalRealFailsClosed |
| real UI claims fictitious/no real connections | hardcoded demo text | configured public environment label | TypeScript check; browser check pending |
| demo audit actor on real requests | middleware fallback | explicit configured private local operator | TestExplicitOperatorPreservesTrustedIdentity |
| API ready while Redis unavailable | DB-only readiness | DB+Redis probe, retain Stats | TestReadinessFailsForDatabaseOrRedisUnavailable |

The Compose/profile scaffolding and local operator binding are runtime preparation, not domain bug fixes or production authentication.

### TESTS IMPLEMENTED

Focused frontend no-mock-fallback test source, configuration/adapter/key/syntax tests, local-real seed boundary assertion, operator context precedence and DB/Redis unavailable readiness. Existing TEST_ONLY/domain, buyer-CUI, ZIP/UBL, source identity/idempotency, worker health/retry and SAGA tests are preserved. No added test was executed per static/compile-only policy.

### CHECKS ACTUALLY EXECUTED

- npm run typecheck: PASS.
- docker compose config --no-env-resolution --no-interpolate --quiet: PASS (structure only).
- gofmt on changed Go files: PASS.
- GOCACHE=/private/tmp/diana-go-cache go test -run '^$' on config/httpserver/workerruntime/api/worker/devseed: PASS (compile only, no tests run).
- Python preparation-tool AST syntax, Make target dry run and git diff --check: PASS.
- Official documentation browsing and attempted Ministry PDF retrieval: partial evidence, retrieval failure disclosed.

User runs on 2026-09-16 proved image pulls/builds, PostgreSQL 17 and Redis startup/health, and a clean Atlas migration through 000015. The first run correctly failed closed on empty OAuth/operator configuration and exposed a misleading standalone frontend. Follow-up preparation added a redacted preflight, explicit missing-name errors and API/worker health gates. After private configuration was completed, API and worker started successfully and both readiness endpoints returned 200; frontend-to-API returned an empty real client list and the frontend root responded successfully. Probe access-log noise was suppressed in source for the next image rebuild. Restart persistence and live integration operations remain unexecuted.

### REAL E2E RUNBOOK

Docs/LOCAL_REAL_E2E.md.

### EXACT STEPS FOR ME NOW

1. Install Docker/Compose v2, Python 3 and make; ensure listed ports free.
2. Run `make setup`; fill optional integration credentials privately; keep the generated key.
3. Verify exact ANAF callback registration/acceptance and official production FCTEL routes; stop if rejected/unresolved.
4. Run `make dev`; inspect `make status`, health/readiness and `make migration-status`.
5. Create real client, factual profile and review integration readiness.
6. Authorize through browser certificate flow, observe actual selected-CUI sync and raw/source/invoice lineage.
7. Accept missing-contract pause or upload/review actual contract with real Gemini if configured.
8. Confirm classification remains safe review with no TEST_ONLY authority; optional manual/SAGA steps only when readiness valid.
9. Run `make down` then `make dev` and verify data/token state persists.

### EXPECTED SUCCESS CRITERIA

Real client exists; actual ANAF authorization succeeds; real SPV message discovered; ZIP/hash persisted; real UBL parsed; Invoice+lines created; existing pipeline executes; missing contract behaves correctly; no TEST_ONLY rules execute; safe classification review reached when matching permits; restart preserves source/invoice/connection/token state. This is a user acceptance checklist, not completed evidence.

### BLOCKERS

Official loopback callback acceptance remains unverified and full production route documentation was inaccessible here. Runtime startup/readiness now passes, while restart persistence and live ANAF/certificate/CUI authorization remain user-run. Production pack remains absent intentionally; cloud auth and worker lifecycle decisions remain outstanding. Previous onboarding report still lists unclosed engineering gates; this preparation does not overwrite their status.

### FILES MODIFIED

.env.example; .gitignore; Makefile; compose.yaml; backend/Dockerfile; backend/.dockerignore; scripts/prepare-local-env.py; backend/internal/platform/config/config.go and config_test.go; backend/cmd/api/main.go and main_test.go; backend/cmd/devseed/main_test.go; backend/internal/platform/httpserver/operator.go and operator_test.go; src/repositories/http/createAppRepository.ts and createAppRepository.test.ts; backend/internal/workerruntime/spv.go; src/app/AppShell.tsx; vite.config.ts; Docs/LOCAL_REAL_E2E.md; Docs/LOCAL_CLOUD_PARITY.md; Docs/LOCAL_REAL_ENVIRONMENT_RESULT.md.

### READY FOR REAL LOCAL RUN?

**NO** — preparation is reviewable, but callback/route evidence and user-run runtime acceptance remain required. No cloud deployment, Classification Engine V2 or real production accounting rules created.
