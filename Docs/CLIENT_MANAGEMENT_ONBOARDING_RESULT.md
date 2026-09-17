### CLIENT MANAGEMENT + ONBOARDING RESULT

IMPLEMENTED — TARGETED USER RERUN PASSED; REMAINING ACCEPTANCE / REVIEW PENDING.

Follow-up on the user-run results of 2026-09-15: frontend/Go production builds succeeded; runtime suites exposed fixture and selector failures. Corrections now use numeric synthetic ANAF message IDs, the required human-review flag in SAGA fixtures, explicit select labels, scoped/level-specific selectors, and separate identifier text in client rows. The default demo Playwright suite excludes the backend-only accounting spec. Contract E2E setup now includes its dedicated migrated/seeded database and a fail-fast seed check; the upload page distinguishes loading, API failure, and unavailable clients. Worker commands now select `redis_integration` rather than an unrelated tag. No migration or accounting validation rule changed.

Follow-up verification by Codex: `npm run typecheck`, Go compile-only for HTTP/PostgreSQL/Redis workers/devseed using `-tags=integration,redis_integration -exec /usr/bin/true -run '^$'`, gofmt and `git diff --check` succeeded. Runtime tests were not rerun by Codex. The user-run worker output with `[no tests to run]` is not evidence for SPV behavior. The corrected targeted runtime rerun subsequently passed as recorded below; remaining acceptance commands are in CLIENT_MANAGEMENT_ONBOARDING_TEST_HANDOFF.md.

### USER-RUN FOLLOW-UP EVIDENCE — 2026-09-15 18:16–18:17

Evidence supplied in attachment `a1d52fec-01d9-4dbb-875d-194967cad7dc/pasted-text.txt`; these commands were executed by the user, not Codex.

| Group | Reported result |
| --- | --- |
| HTTP ClientManagement | PASS |
| PostgreSQL deterministic domain pipeline + concurrent real SAGA artifact persistence | PASS |
| Redis-tagged SPV worker tests | PASS; no empty-selection warning |
| ClientManagement / ClientsWorkspace / AccountingDomainV2 components | 16 passed |
| Default demo Playwright | 23 passed |
| SPV connection Playwright | 1 passed |
| SAGA manual import UX Playwright | 2 passed |
| Accounting domain V2 Playwright | 2 passed |
| Client onboarding Playwright A–G and simulated denial H | 2 passed |

All failures covered by this targeted rerun are resolved in the supplied output. The dedicated contract-ingestion E2E is absent from this rerun and still needs section L's isolated setup. Previously failing full PostgreSQL/frontend/race groups have not been rerun in this attachment; targeted results do not imply a full-suite PASS.

Asynq emitted one retry-exhausted warning during SPV E2E and two during onboarding E2E, without task types or underlying errors. Code intentionally stops queued sync work for disconnected/inactive clients using ErrPermanent / SkipRetry, which could explain such warnings around those journeys, but this log does not establish their cause. No warning was suppressed or classified as harmless. Underlying task errors remain unverified.

### EXECUTIVE SUMMARY

Company creation/editing, dated profile configuration/approval, lifecycle, independent readiness, existing ANAF certificate/OAuth entry, SAGA file export setup, history and client isolation are implemented. Existing workspace changes were preserved. No subsequent module was started.

### DISCOVERY

#### DIANA

Previously list-only clients with ID/name/CUI; separate immutable V2 profiles, SPV ConnectionManager/Service, real XML exporter/handoff, RequestActor grants, audit/outbox and shared frontend repositories. Actual implementation inspected; details in CLIENT_MANAGEMENT_ONBOARDING.md.

#### ARMQU

Read-only inspection of adjacent armquai-be/armquai-fe company schema/API/form and ANAF code. No architecture copied.

#### ARMQU → DIANA MAP

| Source | Target | Decision |
| --- | --- | --- |
| Company schema | Typed company master data | Adapt legal/contact/currency concepts; reject unused business edges/settings. |
| Company REST API | Bounded client commands | Reuse concept; retain Diana actor grants. |
| CompanyModal | Focused create/edit forms | Adapt; reject mandatory optional fields. |
| Company-bound ANAF OAuth | Existing SPV boundaries | Reuse prerequisite/security concepts; reject integration rewrite. |
| CIF cleaning | Identifier normalization | Adapt prefix handling; reject VAT inference. |
| SAGA software enum | Verified file export setup | Reject connection/validation claim. |

### CLIENT DOMAIN MODEL

Internal ID + typed company data, normalized identity, lifecycle, revision and timestamps; profiles/credentials/integrations remain separate authorities.

### CLIENT LIFECYCLE

ONBOARDING / ACTIVE / INACTIVE. Lifecycle grants no integration or accounting approval.

### COMPANY / LEGAL DATA

Legal/display names, CUI/CIF, registration, country, address/city/region/postal code, email/phone/default currency. Only minimum identity/country/currency required; RO/RON supplied explicitly by UI.

### CUI / BUSINESS IDENTITY

Deterministic Romanian/foreign format normalization, display preserved separately, no registry/checksum/VAT inference. Duplicate normalized identity rejected across all lifecycles; original raw uniqueness retained. Dependent identity changes blocked; no cascade rewrite.

### CLIENT TAX / ACCOUNTING PROFILE

Exact V2 facts, UNKNOWN preserved, immutable draft/new approved versions, actor/time/evidence, history/future periods. Approved overlaps rejected. Open-ended approved-period replacement awaits explicit supersession semantics.

### ONBOARDING READINESS MODEL

COMPANY: minimum saved identity. ACCOUNTING_PROFILE: one applicable approved non-test version. ANAF_SPV: existing safe app/auth/sync state with unverified certificate/CUI coverage distinctions. SAGA: export opt-in + master identity; mapping approval/target validation separate. CLASSIFICATION: applicable nonempty non-test production release. Derived only; no completion flag or fake all-green state.

### CLIENT CREATION FLOW

Clienți → Adaugă client → save identity → persistent client detail/onboarding.

### CLIENT EDIT FLOW

Explicit company form, backend validation, editing-base revision and transactional audit; unsaved drafts retained across unrelated updates.

### CLIENT DETAIL UX

Overview cards + company/profile/ANAF/SAGA/classification/lifecycle/history; existing invoice/contract/task/rule context retained.

### ANAF / SPV ONBOARDING

Existing certificate/browser/USB prerequisites and OAuth/manual sync/disconnect reused. OAuth starts replay idempotently while unconsumed. Current grants checked before callback token exchange. No duplicate initial sync introduced.

### CERTIFICATE / SECRET HANDLING

No certificate/key/PIN upload or metadata invented. Tokens and replay state encrypted using existing cipher; ordinary DTO/history never returns secrets/provider errors. Access expiry may refresh; expired/missing refresh credentials require authorization.

### SAGA SETUP

File export opt-in; master name/CIF go to unchanged XML generator, invoice currency remains authoritative. No connection, company override, fake test invoice or target acceptance claim. Mapping approval derived from releases; target validation pending.

### CLASSIFICATION READINESS

V1 preserved: ENGINE READY — REAL PACK MISSING, NOT FROZEN. TEST_ONLY cannot make production green. Missing rules do not stop ingestion. V2 NOT STARTED.

### DEACTIVATION / REACTIVATION

Stop new scheduler/queued-sync starts/OAuth/manual sync. Retain historical data; existing discovered documents and in-flight work may finish. Reactivation changes lifecycle only.

### API

List/create/detail, bounded company/lifecycle/profile/SAGA writes, derived onboarding/profile reads; existing SPV actions retained. OAuth start now requires Idempotency-Key. Full route/payload table in design document.

### DATABASE / MIGRATIONS

Additive 000015; old SQL unchanged, Ent client generation/checksum updated. Separate export opt-in, command ledger and encrypted OAuth attempt table. Actual DB migration not run.

### AUDIT

Client create/update/activate/deactivate/reactivate, profile configured/approved, SAGA updated, safe ANAF start; existing SPV events reused. Safe company/revision/config summaries and actor/time/client/correlation/key retained. History returns Romanian labels without raw payloads.

### AUTHORIZATION / CLIENT ISOLATION

Existing RequestActor/grants enforced on settings and client-scoped SPV/invoice/contract/task/rule routes/callback. New-client creation requires AllClients until grant/RBAC semantics are implemented. Demo actor stays unchanged.

### FRONTEND

Shared repository API/mock modes, focused create page, search/status summaries, persistent settings, Romanian controls and errors/retries. No API-to-mock fallback.

### IDEMPOTENCY / CONCURRENCY

Actor/operation/client/key scoped ledger + request hash and transaction locks; stale client/profile revisions rejected. Replay returns fresh derived reads. OAuth replay encrypted separately; disconnect/sync keys scoped to actor/client/operation.

### OBSERVABILITY

Existing HTTP tracing/metrics/audit reused; no new high-cardinality labels or speculative metrics.

### TESTS IMPLEMENTED

Domain/readiness tables, HTTP scope/DTO isolation, PostgreSQL create/duplicate/concurrency/revision/identity/profile/SAGA/OAuth/lifecycle/secrets, SPV queued inactive sync/refresh/callback grants, master-data XML, frontend journeys and dedicated Playwright A–G. H is explicitly a denied-response frontend simulation; real grants covered by server tests.

### CHECKS ACTUALLY EXECUTED BY CODEX

- gofmt.
- Ent go generate ./ent.
- Atlas migrate hash --dir file://migrations.
- npm run typecheck.
- Ordinary/integration-tag Go compile-only across ./... using -exec /usr/bin/true; earlier targeted -run '^$' empty runs also performed.
- git diff --check.

No behavior tests, PostgreSQL/Redis runtime, race execution, Playwright, real migration, production build or live ANAF/certificate/SAGA executed. Compile output is not behavior PASS.

### MANUAL USER TEST COMMANDS

Exact commands grouped A–V: [CLIENT_MANAGEMENT_ONBOARDING_TEST_HANDOFF.md](CLIENT_MANAGEMENT_ONBOARDING_TEST_HANDOFF.md). It includes isolated test DB setup, normalized-duplicate preflight, incremental 000014→000015 and empty-DB migration rehearsals. Run runtime checks before approving/freeze.

### EXISTING CLIENT MIGRATION

Keep ACTIVE, RO/RON/empty optional defaults, revision 1 and enabled existing file export. Normalize identities without rewriting historical documents; collisions abort for review. Optional SEED_CLIENT_ONBOARDING=true fixtures preserve default baseline counts and contain no real credentials/pack.

### PRODUCT DECISIONS REQUIRED

Future dependent legal identity correction, delete/merge, and approved open-ended profile supersession. Current module safely blocks those operations. No SAGA override is needed by the verified format.

### FRONTEND CONTRACT CONFLICTS

Additive client fields and repository methods; existing direct id/name/cui and invoice contract retained. Intentional OAuth Idempotency-Key requirement added to frontend/transport regression fixture. Browser actor switching is unavailable under current demo authority; no fake production auth introduced.

### SECURITY REVIEW

Scoped grants, backend validation, protected CUI, no delete, immutable profile approvals, encrypted integration state, secret-free safe DTO/history and transaction/revision protection reviewed in code. Runtime security tests remain user-run.

### KNOWN LIMITATIONS

Open-ended approved-profile replacement blocked; no certificate metadata/coverage proof or client-specific SAGA target acceptance authority. In-flight/history completion is retained on deactivation. Per-visible-client readiness reads suit pilot scale. Native beforeunload protects refresh/OAuth; no new SPA router blocker. Browser real-RBAC E2E remains deferred; immutable integration fixtures require isolated test DB.

### INTENTIONALLY DEFERRED

Production RBAC/auth/grants, registry/ONRC/VIES enrichment, real rule pack, Classification V2, SAGA bridge/machine acknowledgement, CreditNote/storno, historical reclassification, client merge/hard deletion and historical profile correction. No deployment or next module.

### ENGINEERING GATE READY?

NO — targeted rerun passed; dedicated contract E2E, previously failing broader regression reruns, warning classification and final review remain outstanding.

### READY FOR USER-RUN TESTS

YES.

### FILES MODIFIED

The list below identifies this milestone's authored/updated files; preexisting unrelated dirty/untracked work is not part of this report. Generated Ent outputs are included.

Domain/application:

- [backend/internal/clients/client.go](../backend/internal/clients/client.go)
- [backend/internal/clients/service.go](../backend/internal/clients/service.go)
- [backend/internal/clients/management.go](../backend/internal/clients/management.go)
- [backend/internal/clients/readiness.go](../backend/internal/clients/readiness.go)
- [backend/internal/clients/management_test.go](../backend/internal/clients/management_test.go)
- [backend/internal/platform/requestactor/context.go](../backend/internal/platform/requestactor/context.go)

Persistence/API:

- [backend/internal/platform/postgres/client_management.go](../backend/internal/platform/postgres/client_management.go)
- [backend/internal/platform/postgres/client_management_integration_test.go](../backend/internal/platform/postgres/client_management_integration_test.go)
- [backend/internal/platform/postgres/store.go](../backend/internal/platform/postgres/store.go)
- [backend/internal/platform/postgres/spv_store.go](../backend/internal/platform/postgres/spv_store.go)
- [backend/internal/platform/postgres/saga_export.go](../backend/internal/platform/postgres/saga_export.go)
- [backend/internal/platform/httpserver/server.go](../backend/internal/platform/httpserver/server.go)
- [backend/internal/platform/httpserver/client_management.go](../backend/internal/platform/httpserver/client_management.go)
- [backend/internal/platform/httpserver/client_management_test.go](../backend/internal/platform/httpserver/client_management_test.go)
- [backend/internal/platform/httpserver/spv_server_test.go](../backend/internal/platform/httpserver/spv_server_test.go)

Integration/fixtures:

- [backend/internal/spv/connections.go](../backend/internal/spv/connections.go)
- [backend/internal/spv/connections_test.go](../backend/internal/spv/connections_test.go)
- [backend/internal/spv/service.go](../backend/internal/spv/service.go)
- [backend/internal/spv/onboarding_test.go](../backend/internal/spv/onboarding_test.go)
- [backend/internal/saga/client_onboarding_test.go](../backend/internal/saga/client_onboarding_test.go)
- [backend/cmd/devseed/main.go](../backend/cmd/devseed/main.go)

Schema/generated:

- [backend/migrations/000015_client_management_onboarding.sql](../backend/migrations/000015_client_management_onboarding.sql)
- [backend/migrations/atlas.sum](../backend/migrations/atlas.sum)
- [backend/ent/schema/client.go](../backend/ent/schema/client.go)
- [backend/ent/accountingclient.go](../backend/ent/accountingclient.go)
- [backend/ent/accountingclient/accountingclient.go](../backend/ent/accountingclient/accountingclient.go)
- [backend/ent/accountingclient/where.go](../backend/ent/accountingclient/where.go)
- [backend/ent/accountingclient_create.go](../backend/ent/accountingclient_create.go)
- [backend/ent/accountingclient_update.go](../backend/ent/accountingclient_update.go)
- [backend/ent/migrate/schema.go](../backend/ent/migrate/schema.go)
- [backend/ent/mutation.go](../backend/ent/mutation.go)
- [backend/ent/runtime.go](../backend/ent/runtime.go)

Frontend:

- [src/domain/client-management.ts](../src/domain/client-management.ts)
- [src/domain/invoice.ts](../src/domain/invoice.ts)
- [src/features/clients/CreateClientPage.tsx](../src/features/clients/CreateClientPage.tsx)
- [src/features/clients/CompanyForm.tsx](../src/features/clients/CompanyForm.tsx)
- [src/features/clients/ClientSettings.tsx](../src/features/clients/ClientSettings.tsx)
- [src/features/clients/ClientDetailPage.tsx](../src/features/clients/ClientDetailPage.tsx)
- [src/features/clients/ClientsListPage.tsx](../src/features/clients/ClientsListPage.tsx)
- [src/features/clients/ClientManagement.test.tsx](../src/features/clients/ClientManagement.test.tsx)
- [src/features/clients/client-management-hooks.ts](../src/features/clients/client-management-hooks.ts)
- [src/features/clients/SPVConnectionCard.tsx](../src/features/clients/SPVConnectionCard.tsx)
- [src/features/clients/spv-hooks.ts](../src/features/clients/spv-hooks.ts)
- [src/repositories/invoiceRepository.ts](../src/repositories/invoiceRepository.ts)
- [src/repositories/http/ApiInvoiceReadRepository.ts](../src/repositories/http/ApiInvoiceReadRepository.ts)
- [src/mocks/MockInvoiceRepository.ts](../src/mocks/MockInvoiceRepository.ts)
- [src/app/App.tsx](../src/app/App.tsx)
- [src/test/render-app.tsx](../src/test/render-app.tsx)

Browser/config:

- [e2e/client-onboarding.spec.ts](../e2e/client-onboarding.spec.ts)
- [playwright.client-onboarding.config.ts](../playwright.client-onboarding.config.ts)
- [playwright.config.ts](../playwright.config.ts)
- [scripts/start-client-onboarding-e2e.sh](../scripts/start-client-onboarding-e2e.sh)
- [package.json](../package.json)

Documentation:

- [Docs/CLIENT_MANAGEMENT_ONBOARDING.md](CLIENT_MANAGEMENT_ONBOARDING.md)
- [Docs/CLIENT_MANAGEMENT_ONBOARDING_TEST_HANDOFF.md](CLIENT_MANAGEMENT_ONBOARDING_TEST_HANDOFF.md)
- [Docs/CLIENT_MANAGEMENT_ONBOARDING_RESULT.md](CLIENT_MANAGEMENT_ONBOARDING_RESULT.md)
- [Docs/BACKEND_ARCHITECTURE.md](BACKEND_ARCHITECTURE.md)
- [Docs/PROJECT_STATE.md](PROJECT_STATE.md)
- [Docs/DECISIONS.md](DECISIONS.md)
- [Docs/ANAF_SPV_CONNECTION_UX.md](ANAF_SPV_CONNECTION_UX.md)
- [Docs/SAGA_EXPORT_UX.md](SAGA_EXPORT_UX.md)
