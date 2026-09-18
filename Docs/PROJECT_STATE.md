# Project State

## Standard release commands — 2026-09-17

- `make release-local`: complete deterministic test/migration/build gate followed by local Docker deployment and health checks.
- `make deploy-gcp-test`: the same gate, immutable image build/push, Cloud TEST migration/status jobs, four-service rollout and post-deploy verification; clean committed tree and explicit confirmation required.
- E2E databases are isolated from the normal local `diana` database. Live ANAF certificate, real Gemini document, desktop SAGA and human Cloud acceptance remain separate manual gates.
- See `Docs/RELEASE_COMMANDS.md`.

## Contract Ingestion Hardening + Service Terms V1 — 2026-09-17

- IMPLEMENTED — AWAITING USER-RUN TESTS / REVIEW; engineering gate remains NO.
- Canonical Romanian CUI comparison, reviewed-value confirmation readiness, indefinite periods, unconfirmed-document discard, authenticated range PDF delivery, Cloud `.mjs` MIME fix, typed service terms, provenance/audit, and two-invoice resume regression are implemented in additive migration `000017`.
- Existing Module 4 policy thresholds and accounting classification remain unchanged. User-run Go unit tests, all 108 frontend tests, production frontend build, and isolated application of all 17 migrations passed on 2026-09-17/18. Integration fixtures/Redis orchestration exposed by the full gate were corrected and await rerun; Playwright/local deployment/Cloud deployment remain outstanding.
- See `CONTRACT_INGESTION_HARDENING_V1.md`, `CONTRACT_SERVICE_TERMS_V1.md`, `CONTRACT_INGESTION_HARDENING_RESULT.md`, and `CONTRACT_INGESTION_TEST_HANDOFF.md`.

- Frontend: APPROVED / FROZEN
- Frontend Modules 1–7: APPROVED / FROZEN
- Backend Phase 0: APPROVED
- Backend Module 1: APPROVED / FROZEN
- Backend Module 2: APPROVED / FROZEN
- Backend Module 3: APPROVED / FROZEN
- Backend Module 4: APPROVED / FROZEN
- Backend Module 5: APPROVED / FROZEN
- Backend Module 6: APPROVED / FROZEN
- Backend Module 7: APPROVED / FROZEN
- Backend Modules 8+: NOT STARTED
- Backend architecture: Go modular monolith; separate API/worker processes; PostgreSQL; Redis/Asynq; Ent; pgx connectivity; Atlas migrations
- Invoice pipeline: explicit state machine, exact invoice lines, revisions, transactional audit/outbox, idempotent ingestion, terminal business duplicates
- Duplicate policy: provisional/versionable `PROVISIONAL_V1`; client + normalized supplier CUI + normalized invoice number + issue day; amount/currency are verification signals
- Validation tasks: exactly three types/statuses; one active blocker per invoice; historical resolved tasks retained
- Contracts: read-only records; versioned match evidence; immutable invoice-association snapshot; provisional `MODULE4_BASELINE_V1`
- Missing Contract Resume: IMPLEMENTED — AWAITING USER-RUN TESTS / REVIEW; durable `CONTRACT_AVAILABLE` outbox trigger, bounded isolated fan-out, unchanged Module 4 policy, automatic normal-pipeline continuation
- Classification: exactly ACCOUNT/VAT/DEDUCTIBILITY per line; provisional `MODULE5_BASELINE_V1`; one grouped task; atomic final review
- Rules: stable GLOBAL/CLIENT_OVERRIDE identities, immutable versions, direct-origin override only, no correction learning or automatic reclassification
- Implemented API: client reads; complete/scoped invoice list and detail; task reads; missing-contract request; contract reads/confirmation; classification decisions; rule reads/version/override; health/readiness
- Frontend integration: explicit API or mock runtime; API mode has no silent mock fallback and observes backend-owned pipeline progression
- ANAF/SPV inbound ingestion: APPROVED / FROZEN
- ANAF/SPV Connection UX: APPROVED / FROZEN
- Real SAGA: APPROVED / FROZEN
- SAGA Export UX / Manual Handoff: APPROVED / FROZEN
- Authentication V1: IMPLEMENTED — AWAITING USER-RUN TESTS / REVIEW; PostgreSQL sessions, Argon2id credentials, Redis login throttling, CSRF, user-derived RequestActor and Romanian login. Cloud IAP cutover not executed.
- Worker runtime: PostgreSQL transactional outbox; SKIP LOCKED lease claims; at-least-once Asynq delivery; bounded retry/archive; idempotent stale-job handling
- Observability: structured logs, vendor-neutral OTLP tracing, operational `/metrics`, distinct API/worker liveness and readiness
- Gemini native PDF contract extraction: IMPLEMENTED — AWAITING USER-RUN TESTS / REVIEW. Standalone OCR, generalized AI, MCP and Cloud Run infrastructure remain DEFERRED.
- Latest approved verification: the complete ANAF/SPV local/fake-adapter gate passed on 2026-09-14; see `Docs/BACKEND_ANAF_SPV.md`.
- Current SAGA implementation: verified XML generation, strict validation, immutable artifact/attempt persistence, client-scoped private download, HUMAN confirmation of the exact artifact, explicit fake/file runtime selection, CreditNote guard and no false EXPORTED transition from generation/download.
- Contract Ingestion + PDF Upload + Gemini AI Extraction: IMPLEMENTED — AWAITING USER-RUN TESTS / REVIEW; see `Docs/CONTRACT_INGESTION_AI.md`. Runtime suites, migrations and live Gemini have not been run by Codex.
- Next required decisions: authenticated SAGA file handoff, the authoritative meaning/confirmation of EXPORTED, and expired-contract matching semantics.

## Real Accounting Rules V1 — 2026-09-15

- Frontend Modules 1–7: APPROVED / FROZEN.
- Backend Modules 1–7: APPROVED / FROZEN.
- ANAF/SPV: APPROVED / FROZEN.
- Real SAGA: APPROVED / FROZEN.
- SAGA Export UX: APPROVED / FROZEN.
- Missing Contract Resume: APPROVED / FROZEN.
- Contract Ingestion + AI Extraction: DO NOT MODIFY / CURRENT STATE PRESERVED.
- Real Accounting Rules: IMPLEMENTED — AWAITING USER-RUN TESTS / ACCOUNTING REVIEW; NOT FROZEN.
- Production pack `RO_INCOMING_ACCOUNTING_V1_REVIEW_ONLY` contains zero accepted mappings. Eligibility, provenance, effective-date selection, consistent rule snapshots, SAGA guard integration and limited UI metadata are implemented.
- Runtime tests/migration application/production build have NOT been run by Codex. Compilation and static checks are separate from user-run engineering/accounting acceptance.
- Required product/accounting decisions: deductibility semantics, reviewed client-account policy and VAT source-confirmation/tax-treatment scope. See REAL_ACCOUNTING_RULES.md, ACCOUNTING_RULES_RESEARCH.md and REAL_ACCOUNTING_RULES_TEST_HANDOFF.md.

## Accounting Domain V2 + deterministic Classification V1 — 2026-09-15

- Accounting Domain Model V2: IMPLEMENTED — AWAITING USER-RUN TESTS.
- Classification Engine V1: ENGINE IMPLEMENTED; REAL PRODUCTION PACK EMPTY / NOT APPROVED; REAL ACCEPTANCE NOT COMPLETED; NOT COMPLETE / NOT FROZEN.
- Classification Engine V2: NOT STARTED.
- New invoices: ACCOUNT / VAT_TREATMENT / VAT_DEDUCTIBILITY / EXPENSE_TAX_TREATMENT, immutable source/profile/pack evidence, shared readiness and bounded typed review controls.
- Historical invoices/artifacts: LEGACY_V1; VAT means historical source-rate confirmation, DEDUCTIBILITY means historical SAGA import instruction. No historical conversion/reclassification.
- Additive migration 000014, Ent regeneration, private reviewed profile/rule/pack tooling and TEST_ONLY fixtures/tests prepared.
- Only static/compile checks are performed by Codex; runtime/race/PostgreSQL/Redis/frontend/Playwright/build/migration acceptance is user-run. Engineering gate remains NO pending that acceptance.
- Pending accounting inputs: approved chart/profile/acquisition policies, exact reviewed rules/source periods, expected four decisions, and real desktop SAGA mapping acceptance. Current mapping supports approved ordinary immediate full/full omission only.
- Frozen Modules 1–7, ANAF/SPV, Contract matching/resume/AI extraction and SAGA handoff retain their existing boundaries. AI classification, learning, advanced VAT/pro-rata/cash/period calculations and historical reclassification remain deferred.

Authoritative details and exact A–W commands: [CLASSIFICATION_V1_TEST_HANDOFF.md](CLASSIFICATION_V1_TEST_HANDOFF.md). This section supersedes earlier descriptions of three dimensions for newly parsed V2 invoices; those descriptions remain historical V1 records.


## Client Management + Client Onboarding V1 — 2026-09-15

- IMPLEMENTED — AWAITING USER-RUN TESTS / REVIEW: focused company creation/editing, lifecycle, dated profile configuration/approval, derived independent company/profile/ANAF/SAGA/classification readiness, operational settings/history and request-actor isolation.
- New additive migration 000015; existing clients remain ACTIVE and grandfathered for existing file export. Collision preflight/manual review required if normalized identity duplicates exist.
- Static/compile checks executed; runtime tests/migrations/build are explicitly user-run. Engineering gate evidence outstanding; ready for user-run tests YES.
- Classification V1 preserved: ENGINE READY — REAL PACK MISSING, NOT FROZEN. Classification V2 NOT STARTED. Production rules, RBAC, deployment and SAGA bridge NOT STARTED by this milestone.
- [Design/discovery/security](CLIENT_MANAGEMENT_ONBOARDING.md); [A–V test commands](CLIENT_MANAGEMENT_ONBOARDING_TEST_HANDOFF.md).

## First GCP TEST deployment — 2026-09-16

- Project `diana-508810`, project number `232660193801`, billing enabled;
  region `europe-central2`; domain `accountingtechco.com`.
- INFRASTRUCTURE DEPLOYED: Artifact Registry, identities, secrets, VPC, private
  Cloud SQL and Redis, successful migrations, frontend/API/callback/worker,
  serverless NEGs and global load balancer exist. Static IP is `136.69.63.17`.
- DNS for `test.platform` and `api-test` resolves to the reserved IP. Managed TLS
  certificate `diana-test-cert-v3` is `ACTIVE`; ANAF OAuth registration/sync has
  not run.
- IAP is enabled on frontend and normal API backends; only the exact isolated
  ANAF callback route is public. External anonymous checks return `401` for the
  protected accounting endpoints.
- `cloud-test` config hardening forbids fake SAGA and unofficial enabled ANAF
  endpoints. Classification V1 production pack remains empty/review-only;
  Classification V2 remains NOT STARTED.
- See `GCP_TEST_DEPLOYMENT.md`, `GCP_INFRASTRUCTURE.md`, `GCP_SECRETS.md`,
  `GCP_DNS_SQUARESPACE.md` and `GCP_ANAF_E2E.md`.
# Authentication V1 — 2026-09-16

Implemented in the workspace and awaiting user-run tests/review. Diana now has
an Armqu-pattern Romanian login, PostgreSQL-backed opaque sessions, Argon2id
credentials, Redis brute-force throttling, CSRF enforcement, protected frontend
shell, 401 handling/logout, user-derived RequestActor grants, an operator CLI,
additive migration `000016`, focused tests and a Playwright journey. The live
Google redirect was confirmed as IAP on the web and API backend services; no GCP
mutation or deployment was performed. See `AUTHENTICATION_V1.md`,
`AUTHENTICATION_V1_RESULT.md` and `AUTHENTICATION_TEST_HANDOFF.md`.
