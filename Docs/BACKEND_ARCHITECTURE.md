# Backend Architecture

Real SAGA file mode uses a contextual manual-handoff boundary: generated XML
remains immutable in PostgreSQL, authenticated HTTP streams exact bytes, and a
separate domain command atomically records HUMAN confirmation plus the terminal
invoice transition. See `SAGA_EXPORT_UX.md`. A local bridge is not part of this
architecture.

Status: Backend Phase 0, Backend Modules 1–7 and ANAF/SPV ingestion approved/frozen; real SAGA core file adapter implemented with handoff/acknowledgement decisions outstanding.

## Shape and dependency direction

The backend is a Go modular monolith with two executable processes over the
same capability packages: `backend/cmd/api` for synchronous HTTP commands and
`backend/cmd/worker` for asynchronous workflow execution.

Dependencies flow from transport and platform adapters into capability-oriented
application packages and domain values. HTTP handlers contain transport logic,
not Ent queries. Ent is the primary persistence abstraction. pgx supplies the
PostgreSQL driver and is not a competing repository layer. Atlas migrations are
the schema deployment source.

## Module 1 slice

Module 1 persists only Client, the Invoice fields required by the first Invoice
Detail read, and durable ActivityEvent records. Money is exact internally and
stored as PostgreSQL NUMERIC; the HTTP DTO explicitly emits a JSON number for
the frozen frontend contract.

Backend DTOs do not contain `DemoScenario`, `primaryDemo`, `autoRun`, or
`pipelinePath`; the frontend adapter isolates presentation compatibility values.

## Module 2 slice

Module 2 adds the native invoice pipeline core: explicit transitions, invoice
lines, optimistic revisions, transactional activity events, and a durable
outbox. Ingestion has separate technical and business identities. A technical
retry returns the existing record; a business duplicate is a distinct terminal
invoice and has no continuation toward SAGA.

The outbox is a persistence boundary, not a generic event bus. Its in-process
dispatcher and deterministic fake SAGA adapter prove state behavior only.
Redis/Asynq, production claiming/retry, and real integrations remain later work.

## Module 3 slice

Module 3 adds the ValidationTask capability with a one-to-many invoice history
and a PostgreSQL-enforced maximum of one active blocker per invoice. Internal
domain commands own task creation and compatible invoice blocking transitions.
The only public mutation is the domain-specific missing-contract request.

Task Inbox uses a compact API query. Invoice Detail exposes only the current
active task, while resolved instances remain queryable history. Task mutation,
invoice state, durable audit, and outbox decisions share one transaction.

## Module 4 slice

Module 4 adds read-only Contracts, immutable match runs/candidates, and an
explicit invoice-contract association. The association stores a minimum
contract snapshot so later changes to the current contract record cannot alter
the historical context used by an already-processed invoice. A separate terms
version hierarchy is intentionally not introduced before contract editing or
import exists.

`contracts.MatchingPolicy` isolates candidate evaluation. The initial
`MODULE4_BASELINE_V1` policy is deterministic integration behavior, not a final
accounting policy: discovery uses exact normalized supplier CUI within a client;
period inclusion and currency equality are objective compatibility signals; it
contains no numeric score, tolerance, semantic, amount, or SKU logic.

Pipeline outbox dispatch calls a narrow contract-matching processor at
`MATCHING`. The matching transaction persists its run and evidence, then either
associates the unique compatible contract and continues, or creates exactly one
Module 3 blocker. Human confirmation validates the active run/candidate and
both optimistic revisions before atomically resolving the task, snapshotting
the association, moving to `DEDUPE_CHECKED`, auditing, and enqueuing continuation.
Through Module 6, the in-process dispatcher proved the application boundary but
was not a production worker runtime. Module 7 supersedes that execution detail
with the dedicated worker, leased outbox claiming, and bounded retry behavior
described below.

## Module 5 slice

Module 5 adds `LineClassification` as one current relational decision per
invoice-line and dimension. The dimension vocabulary is closed to `ACCOUNT`,
`VAT`, and `DEDUCTIBILITY`. Proposal evidence is immutable; an accepted or
corrected effective value, reviewer identity, and optimistic revision capture
human resolution without creating a second competing current row.

`classification.Policy` isolates proposal behavior from HTTP, React, Ent, and
future inference. `MODULE5_BASELINE_V1` is temporary demonstrative behavior: it
uses only explicit stored technical match kinds, no numeric thresholds, and no
accounting or fiscal claims. No match or ambiguous same-level matches require
review. A direct client override replaces only its referenced global origin.

`ClassificationRule` is stable identity and `RuleVersion` is immutable history.
Global rules have no client; client overrides belong to one client and one
global origin. Effective periods are stored/displayed but deliberately not
interpreted by the baseline. User-created textual criteria are stored as
`NO_AUTOMATION` rather than silently becoming an invented executable DSL.

Classification persistence, invoice routing, the single grouped task, audit,
and continuation outbox share one transaction. Partial review changes one item
and task revision only. Final review atomically resolves the task, advances the
invoice to `READY_FOR_SAGA`, and enqueues continuation. Human corrections never
mutate rules and rule changes never reclassify historical invoices.

## Module 6 integration

Normal API-backed runtime now has one authority for all Module 1–5 capabilities:
Go/PostgreSQL. `ApiInvoiceReadRepository` implements the complete frontend
capability interface, including the complete, optionally client-scoped invoice
list. The former hybrid adapter and its silent API-to-mock fallbacks were
removed. An API `404` remains a not-found result and a transport failure remains
an error; neither substitutes an unrelated demo entity.

The mock repository remains an explicit alternative runtime only when
`VITE_BACKEND_READS_ENABLED` is not `true`. It supports frozen Playwright and
isolated component tests. `AutomaticWorkflowRunner` and its timer-driven
transitions are statically narrowed to that mock-only capability. API invoices
are observed through React Query polling and are never transitioned by React.

`GET /api/v1/invoices` returns the complete small milestone dataset, optionally
filtered by `clientId`, in deterministic issue-date order. Invoice List,
Dashboard, Clients, and global/client scope are therefore backend-backed without
pagination or an analytics subsystem. Dashboard retains its approved
current-month date basis and four KPI definitions.

Module 6 originally proved backend ownership with an API-local dispatcher.
Module 7 supersedes only that execution topology: normal runtime now uses the
dedicated worker and `PIPELINE_DISPATCH_ENABLED` defaults to false. The legacy
loop remains an explicit development/test compatibility mode.

React Query keys are centralized by capability and identity. Successful human
commands update the returned detail and invalidate affected lists. Conflicts
refetch current persisted state rather than overwriting it.

## Module 7 runtime

PostgreSQL remains the source of domain state and asynchronous intent. A
business transaction atomically commits the domain mutation, `ActivityEvent`,
and `OutboxEntry`. The worker-owned dispatcher claims eligible rows using
PostgreSQL `FOR UPDATE SKIP LOCKED`, publishes a minimal identity payload to the
single Asynq `workflow` queue, then records dispatch.

Delivery is at least once. If Redis publication succeeds and the process dies
before PostgreSQL records dispatch, the claim lease expires and another
dispatcher republishes the row. This deliberate crash window cannot lose work
but can duplicate transport. The original outbox identity plus persisted state,
command idempotency, optimistic revisions, unique constraints, and terminal or
blocker checks make duplicate execution a harmless stale no-op.

The only task type is `workflow:continue_invoice`; its payload contains outbox,
invoice, idempotency, event, and correlation identifiers, never mutable domain
snapshots. Matching, classification, and fake SAGA behavior remains in the
approved services/adapters. Human commands remain synchronous HTTP commands.

Asynq retries are finite and archived after exhaustion. Outbox publication uses
capped exponential backoff and becomes operational `FAILED` after a configured
limit. Neither path creates a `ValidationTask`. Stale, terminal, and
human-blocked jobs are acknowledged.

The API liveness endpoint is process-only and API readiness depends on
PostgreSQL, not Redis. Worker liveness is process-only; worker readiness requires
the worker to accept work plus PostgreSQL and Redis connectivity. Shutdown stops
new claims before draining Asynq within the configured timeout.

Both processes use structured JSON logs. OpenTelemetry is disabled by default
and supports a standard OTLP exporter without a vendor dependency. Trace
boundaries cover HTTP, outbox dispatch, job processing, and fake SAGA export.
`/metrics` exposes bounded-cardinality HTTP, worker, retry/failure, and outbox
backlog measures. `ActivityEvent` remains separate business audit.

## ANAF/SPV connection UX

Connection configuration remains inside the existing `internal/spv` capability.
Client Detail calls explicit read/command endpoints; React never receives Ent rows,
tokens, ciphertext, OAuth secrets, state digests, checkpoints or queue metadata.
The API process owns OAuth start/callback and publishes manual synchronization to
the same Asynq path used by the approved worker.

OAuth CSRF/replay state is durable PostgreSQL data because callbacks must survive
replica changes and process restarts. Only a SHA-256 digest of a random 256-bit
browser value is stored. State has a configurable short TTL, is bound to client,
environment and a server-selected Client Detail return path, and is consumed
atomically before token exchange. Callback URLs contain only safe result categories.

`SPVConnection` remains one identity per AccountingClient. Reauthorization updates
that row; disconnect revokes it and clears locally usable credentials while source
documents and invoices remain untouched. The user-facing connection health model is
derived centrally and remains distinct from the last synchronization result.
Permanent refresh rejection maps to reauthentication; transient ANAF failure leaves
the connection active.

Manual sync acknowledges queue acceptance with HTTP 202. It uses
`spv:sync_account`, and the publisher coalesces identical pending tasks for one
minute. Connection events are business/admin audit; routine token refresh and HTTP
retry remain infrastructure behavior.

Qualified-certificate handling remains outside Diana's process boundary. The browser
and ANAF `logincert` identity provider select/use the installed or USB-token-backed
certificate and request its PIN. Diana handles only OAuth state, authorization code,
encrypted tokens and safe connection metadata. There is intentionally no certificate
upload endpoint, certificate table, private-key payload or Redis job material.

## Protected boundaries

- The approved frontend behavior and routes are frozen except for the authorized
  additive ANAF/SPV card inside Client Detail.
- Client scope is product context, not authorization.
- A development request actor establishes the future auth boundary without JWT
  or RBAC.
- Structured operational logs and durable business audit records are separate.
- Runtime schema auto-creation is not a deployment mechanism.

## Real SAGA file adapter

SAGA remains an external adapter over the Diana invoice/classification model.
`internal/saga` implements the current documented SAGA C invoice-import XML and
does not own contract matching, duplicate detection, classification or rules.
It consumes only persisted final line decisions and exact decimal source values.

An immutable `SagaExportAttempt` stores the generated sensitive artifact, its
hash, exporter version, source invoice revision and classification trace. The
unique revision/version key provides Diana-side idempotency under repeated
outbox/Asynq delivery. Infrastructure errors retain the Module 7 bounded retry
path; permanent format errors create no ValidationTask.

The verified handoff is manual import into local desktop SAGA and exposes no
acknowledgement. Consequently `GENERATED` is operational attempt state, not a
new pipeline state, and does not prove `EXPORTED`. The file adapter deliberately
leaves the invoice at `EXPORTING`. An authenticated download endpoint and/or a
local bridge remain blocked; request-actor context is not authorization and
must not expose stored accounting payloads.

## Deferred

Expired-contract outcome semantics,
final matching/scoring, validated accounting/tax knowledge, rule effective-date
execution, general correction-value vocabularies, SAGA local handoff/acknowledgement,
OCR, Gemini, MCP, and Cloud Run infrastructure remain deferred. Full authentication/RBAC still gates production
exposure of connection commands, and provider-side token revocation is not invented.

## Missing-contract automatic resume

The internal `contracts.Service.ContractAvailable` command is the contract-ingestion
handoff. It persists a narrow `CONTRACT_AVAILABLE` outbox event only after reloading a
durable Contract. The worker uses cursor-bounded same-client/same-normalized-supplier
candidate discovery and reevaluates each blocked invoice independently through the
unchanged `MODULE4_BASELINE_V1` policy.

Per-invoice PostgreSQL transactions preserve the single-active-blocker invariant and
atomically persist the new match run/candidates, resolve the existing missing task,
create any contract-review replacement, create the immutable association, move the
invoice, write business audit, and enqueue normal pipeline continuation. Revision CAS
and deterministic keys make duplicate or concurrent arrival delivery converge. See
`Docs/MISSING_CONTRACT_RESUME.md` for the outcome matrix and integration contract.

## Contract ingestion AI boundary

ContractSourceDocument is an immutable private PDF source, not an authoritative Contract.
ContractExtractionAttempt holds versioned normalized AI proposals and safe attempt history;
human-reviewed final values are persisted separately. IDs-only transactional extraction and
activation outbox events route through existing Asynq. Explicit confirmation alone creates
the ordinary Contract, then uses the frozen ContractAvailable service/arrival processor.
No ingestion code writes invoice/task decisions or introduces AI matching. Gemini native PDF
Interactions adapter lives behind ContractExtractor; PostgreSQL bytea lives behind DocumentStore.
See `Docs/CONTRACT_INGESTION_AI.md` for lifecycle, tenant boundaries, field matrix and user-run gates.

## Real Accounting Rules V1 boundary

Classification now defaults to an explicitly production-safe local policy. Immutable RuleVersion adds production eligibility, rule-pack identity and structured reviewed provenance; all existing versions default to non-production. Exact civil issue dates select all applicable historical versions, with overlap routed to existing CLASSIFICATION review. A read-only repeatable-read snapshot loads invoice/lines/rules together. No production mapping is seeded pending the accounting/data/policy gates; automatic deductibility is stopped. SAGA requires reviewed production evidence or explicit accountant confirmation while preserving numeric VAT/arithmetic guards. See [architecture](REAL_ACCOUNTING_RULES.md), [research](ACCOUNTING_RULES_RESEARCH.md) and [user-run checks](REAL_ACCOUNTING_RULES_TEST_HANDOFF.md). Contract ingestion and approved pipeline/task/dimension vocabulary remain preserved.

## Accounting Domain V2 / Classification V1 — 2026-09-15

Implemented, awaiting user-run runtime/migration/build tests; NOT FROZEN. Real production pack remains empty. Accounting V2 introduces vendor-independent immutable source facts, typed four-dimensional decisions, dated approved profiles and exact reviewed release snapshots. Legacy V1 retains three dimensions and its historical SAGA instruction semantics/artifacts.

The deterministic classification service selects V2 by invoice model version. Profile/pack/Contract reads share repeatable-read; invoice/decision/task writes use existing transactional CAS and outbox boundaries. Shared `saga.EvaluateReadiness` guards new-model automatic completion, final human review and generation. Unsupported combinations stay on the existing grouped CLASSIFICATION task. New operator executable `cmd/accountingrelease profile|rule|pack` is private reviewed configuration installation; ordinary Rule Administration remains NO_AUTOMATION and ineligible.

Migration 000014 is additive; migrations 000001–000013 are unchanged. New SPV parses retain source absence/origins/totals/identities/timing. Existing Contract AI extraction, matching/resume and SAGA acknowledgement remain separate. No AI classification or V2 engine is implemented. See [ACCOUNTING_DOMAIN_V2.md](ACCOUNTING_DOMAIN_V2.md), [CLASSIFICATION_ENGINE_V1.md](CLASSIFICATION_ENGINE_V1.md), [CLASSIFICATION_V1_TEST_HANDOFF.md](CLASSIFICATION_V1_TEST_HANDOFF.md).


## Client Management + Onboarding V1 — 2026-09-15

Implemented pending user-run tests/review. `clients` now owns typed company master data, lifecycle and bounded commands/readiness, retaining immutable `accounting.Profile` versions and existing SPV/SAGA services. `postgres/client_management.go` uses the existing Store DB adapter for transaction-local row/advisory locks, revision checks, audit and durable command keys; this follows existing worker/outbox SQL concurrency precedent and introduces no transport persistence logic or competing repository. Ent client schema remains aligned. Client command events have no asynchronous consumer, so no unrecognized outbox jobs are published.

Only additive migration 000015: company/lifecycle/revision/normalized identity, separate file export opt-in, command ledger and encrypted OAuth replay state. SAGA input checks explicit export opt-in, with existing clients grandfathered. Derived readiness is never persisted; read snapshots are repeatable-read, with safe existing ConnectionManager enrichment. Inactive clients skip periodic/new SPV sync while already discovered documents can finish. Profile approval inserts immutable nonoverlapping versions; no profile/rule/pack promotion/reclassification rewrite. RequestActor grant checks guard settings and existing client-scoped routes; demo actor remains development-only semantics, with production RBAC deferred.

See [CLIENT_MANAGEMENT_ONBOARDING.md](CLIENT_MANAGEMENT_ONBOARDING.md) and [test handoff](CLIENT_MANAGEMENT_ONBOARDING_TEST_HANDOFF.md).
