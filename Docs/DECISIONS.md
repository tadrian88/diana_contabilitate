# Decision Log

All decisions below were approved on 2026-09-09. Alternatives are recorded only where the rejected behavior was explicitly discussed.

## D-001 — Missing contract lifecycle

- Status: APPROVED
- Decision: After `SOLICITĂ CONTRACT`, the task remains open/waiting and the invoice remains `AWAITING_CONTRACT`.
- Reason: Processing cannot continue until the external contract condition is resolved.
- Alternatives: Closing the task immediately was rejected.

## D-002 — Classification task granularity

- Status: APPROVED
- Decision: One classification task per invoice contains all uncertain lines and dimensions.
- Reason: The accountant reviews only exceptional items without reviewing the whole invoice.

## D-003 — Single incompatible contract

- Status: APPROVED
- Decision: Surface a contract-match review task instead of rejecting automatically.

## D-004 — Duplicate invoice

- Status: APPROVED
- Decision: `DUPLICATE` is a distinct terminal frontend state visible in invoice list and detail. It never continues to SAGA.

## D-005 — Automatic SAGA export

- Status: APPROVED
- Decision: Eligible invoices transition automatically through `READY_FOR_SAGA`, `EXPORTING`, `EXPORTED`. `EXPORT_FAILED` is supported; retry behavior is not defined.

## D-006 — Global multi-client views

- Status: APPROVED
- Decision: Dashboard, Tasks and Invoices support `TOȚI CLIENȚII`; a persistent selector makes the active scope obvious.

## D-007 — Contextual SPV/SAGA feedback

- Status: APPROVED
- Decision: No standalone Operations page in MVP. Feedback appears on Dashboard, Invoice Detail and workflow states.

## D-008 — Rule scope

- Status: APPROVED
- Decision: Rules comprise global rules and client overrides, with origin clearly displayed. No precedence semantics are defined beyond the approved client-specific variation concept.

## D-009 — Rule versioning

- Status: APPROVED
- Decision: Editing creates a new version and previous versions remain visible.

## D-010 — Contracts and clients scope

- Status: APPROVED
- Decision: Contracts are list/detail/context only, with no create or edit. Clients are list/selection/context only, with no CRUD.

## D-011 — MVP persona and permissions

- Status: APPROVED
- Decision: `CONTABIL` is the only functional persona. Permission denied is only a generic technical completeness state.

## D-012 — Dashboard KPIs

- Status: APPROVED
- Decision: For the current month use exactly: Facturi în procesare, Necesită atenție, Pregătite pentru SAGA, Exportate în SAGA.

## D-013 — Classification corrections

- Status: APPROVED
- Decision: A correction updates invoice review state only and never creates, modifies or proposes a rule.

## D-014 — Source document preview

- Status: APPROVED
- Decision: PDF/XML/document preview is outside the MVP.

## D-015 — Backend architecture mismatch

- Status: PROPOSED
- Decision: None. Resolve the Python versus pgx/Ent/Atlas mismatch only when backend planning begins.

## D-016 — Module 1 approval and freeze

- Status: APPROVED
- Decision: Module 1 is approved and frozen. Its UX or behavior changes only if a later module exposes a genuine, explained conflict.

## D-017 — Task Inbox scope and statuses

- Status: APPROVED
- Decision: Task Inbox contains only contract-match, missing-contract and classification-review tasks, using `OPEN`, `WAITING` and `RESOLVED`.
- Reason: Keep the accountant focused on the three approved human judgment points.

## D-018 — Task Inbox default and filters

- Status: APPROVED
- Decision: The default view is `OPEN`. `WAITING` and `RESOLVED` are accessible. Filters are limited to task type and task status; client scope is controlled by the persistent selector.
- Alternatives: Priority, assignment, SLA, escalation and advanced filters are excluded.

## D-019 — Shared simulated workflow state

- Status: APPROVED
- Decision: Dashboard, Task Inbox and Invoice Detail operate on one mock repository state. Automatic processing continues independently of the active route.
- Reason: Returning to the inbox must not pause or contradict the simulated invoice pipeline.

## D-020 — Module 2 approval and freeze

- Status: APPROVED
- Decision: Module 2 is approved and frozen. Its UX or behavior changes only if a later module exposes a genuine, explained conflict.

## D-021 — Invoice List information model

- Status: APPROVED
- Decision: Invoice List uses the ten approved information categories, derives unresolved issues from shared task state and limits filters to pipeline, attention and SAGA status alongside client scope.
- Alternatives: Saved views, bulk actions, column customization, advanced queries and table export remain excluded.

## D-022 — Complete invoice workspace

- Status: APPROVED
- Decision: Invoice Detail retains the five approved tabs, exposes source invoice and classification context, and reuses the existing contract/classification state transitions. Search, filters and the selected tab use normal URL state.

## D-023 — Operational exception representation

- Status: APPROVED
- Decision: Duplicate is a visible terminal pipeline state with no resolution flow or SAGA progression. Simulated SAGA failure is visible without inventing retry or manual-export behavior.

## D-024 — Module 3 approval and freeze

- Status: APPROVED
- Decision: Module 3 is approved and frozen. Its UX or behavior changes only if a later module exposes a genuine, explained conflict.

## D-025 — Read-only Contracts workspace

- Status: APPROVED
- Decision: Contracts provide list, detail, source context and current invoice associations only. Contract status and all create, edit, delete, upload, lifecycle, renewal, risk and ownership behavior are excluded.

## D-026 — Contract inspection during invoice review

- Status: APPROVED
- Decision: Recommended and alternative candidates may be inspected before selection. Inspection reuses supplied mock reasoning and never resolves the invoice task; selection remains in Invoice Detail.

## D-027 — Shared invoice-contract association

- Status: APPROVED
- Decision: Contract-associated invoices derive from the same current `selectedContractId` used by invoice workflows. Contract views do not maintain a separate association store.

## D-028 — Module 4 approval and freeze

- Status: APPROVED
- Decision: Module 4 is approved and frozen. Its UX or behavior changes only if a later module exposes a genuine, explained conflict.

## D-029 — Dashboard time basis

- Status: APPROVED
- Decision: Dashboard current-month aggregation uses the deterministic frontend application clock for September 2026 and invoice issue timestamps. It does not depend on the runtime system date.

## D-030 — Dashboard attention semantics

- Status: APPROVED
- Decision: Immediate attention derives from `OPEN` validation tasks only. `WAITING` is displayed separately; SAGA failure and duplicate remain operational states and do not create new task categories.

## D-031 — Complete Dashboard structure

- Status: APPROVED
- Decision: Dashboard contains the four approved KPI concepts and exactly the Operational Overview and Needs Attention perspectives. Pipeline, SAGA, SPV and client summaries are dense state-derived views with deep links into existing modules; no analytics or chart model is introduced.

## D-032 — Module 5 approval and freeze

- Status: APPROVED
- Decision: Module 5 is approved and frozen. Its UX or behavior changes only if a later module exposes a genuine, explained conflict.

## D-033 — Rules categories and scope

- Status: APPROVED
- Decision: Rules Administration contains exactly account, VAT and deductibility categories, with explicit global or client-override scope. A selected client sees global rules plus only that client's overrides.

## D-034 — Immutable rule versioning

- Status: APPROVED
- Decision: Rule edits create a new effective-dated version, preserve all earlier versions and preserve the rule's scope. Historical versions are read-only.

## D-035 — Client override creation

- Status: APPROVED
- Decision: A client override can be created only from a global rule, belongs to exactly one client and does not mutate the global parent. No unrestricted create-rule flow exists.

## D-036 — Demonstrative rules and invoice traceability

- Status: APPROVED
- Decision: Rule criteria, results and legal context are fictitious frontend data and do not constitute an executable accounting engine. Existing invoice classifications may link to the exact supplied rule version, but rule changes never recalculate invoices.

## D-037 — Module 6 approval and freeze

- Status: APPROVED
- Decision: Module 6 is approved and frozen. Its UX or behavior changes only if frontend integration exposes a genuine, explained conflict.

## D-038 — Clients as operational context

- Status: APPROVED
- Decision: Clients provide list, detail and contextual navigation using only identity and state-derived operational information. Client CRUD and CRM concepts are excluded.

## D-039 — Cross-client detail safety

- Status: APPROVED
- Decision: When the active selector changes to a different client, a detail belonging to another client is replaced by the corresponding scoped list; client detail moves to the newly selected client. This is UX context safety, not authorization.

## D-040 — Shared frontend terminology

- Status: APPROVED
- Decision: Pipeline, SAGA, task and classification labels use shared canonical Romanian presentation across modules. Domain states and previously approved transitions remain unchanged.

## D-041 — Backend architecture

- Status: APPROVED
- Decision: The backend is a Go modular monolith using PostgreSQL, Ent as the primary persistence abstraction, pgx as the PostgreSQL connectivity layer, and Atlas for versioned migrations. Future asynchronous work uses Redis/Asynq only when required.

## D-042 — Backend dependency direction

- Status: APPROVED
- Decision: Thin HTTP and worker adapters call capability-oriented application/domain packages. Ent and external integrations remain platform adapters. Generic CRUD layers, CQRS, workflow engines, event buses, and premature microservices are excluded.

## D-043 — Money representation

- Status: APPROVED
- Decision: Monetary values use exact decimal representation and PostgreSQL NUMERIC. Module 1 returns JSON numbers at the explicit HTTP transport boundary to preserve the frozen frontend contract.

## D-044 — Parity-first backend delivery

- Status: APPROVED
- Decision: Backend implementation proceeds through small vertical slices. Module 1 persists Client, minimal Invoice, and ActivityEvent, and exposes only client-list and invoice-detail read capabilities. Unimplemented capabilities remain mock-backed during the hybrid period.

## D-045 — Frontend contract protection

- Status: APPROVED
- Decision: The approved frontend is frozen. Backend adapters preserve its visible routes, labels, task semantics, money shape, and client context. Demo controls are not persisted as backend domain data.

## D-046 — Authentication and external integrations

- Status: APPROVED
- Decision: Module 1 establishes only a request-actor boundary. Authentication, RBAC, real SPV/SAGA, OCR, Gemini, MCP, Redis/Asynq, and Cloud Run infrastructure remain deferred.

## D-047 — Technical invoice ingestion identity

- Status: APPROVED
- Decision: A delivery retry is identified by client and SPV reference. It returns the existing invoice and creates no additional lines, audit records, or outbox work.

## D-048 — Business duplicate identity

- Status: PROVISIONAL / ACCEPTED FOR NOW
- Decision: `PROVISIONAL_V1` identifies a business duplicate by client, normalized supplier CUI, normalized invoice number, and issue day. Total and currency are supplementary verification signals and are not identity fields. The policy remains replaceable/versionable and requires final accounting validation.

## D-049 — Duplicate persistence and terminality

- Status: APPROVED
- Decision: A different SPV delivery matching an existing business identity is persisted as a distinct invoice referencing the canonical invoice. Its initial state is terminal `DUPLICATE`, it has no outbox continuation, and it never reaches SAGA.

## D-050 — ValidationTask vocabulary

- Status: APPROVED
- Decision: ValidationTask supports exactly `CONTRACT_MATCH`, `MISSING_CONTRACT`, and `CLASSIFICATION`, with exactly `OPEN`, `WAITING`, and `RESOLVED` statuses.

## D-051 — Active blocking task invariant

- Status: APPROVED
- Decision: Invoice has one-to-many task history but at most one task with status other than `RESOLVED`. PostgreSQL enforces the active blocker invariant.

## D-052 — Missing-contract request

- Status: APPROVED
- Decision: Requesting an `OPEN` missing-contract task changes it to `WAITING`, leaves the invoice in `AWAITING_CONTRACT`, writes audit, and creates no pipeline continuation or external communication.

## D-053 — Task API boundary

- Status: APPROVED
- Decision: Task Inbox has a filtered read endpoint and missing-contract request has a domain-specific invoice endpoint. Arbitrary public task creation and generic status mutation are excluded.

## D-054 — Minimal contract persistence

- Status: APPROVED / FROZEN
- Decision: The MVP persists one read-only Contract record with client, supplier identity, reference, effective period, displayed commercial fields, revision, and source context. Direction and procurement/CRM lifecycle concepts are excluded because the approved invoice flow is supplier-side and does not use them.

## D-055 — Historical invoice association

- Status: APPROVED / FROZEN
- Decision: InvoiceContractAssociation is explicit and unique per invoice. It references the selected contract and match run while storing an immutable minimum snapshot of the contractual fields displayed for the invoice. ContractTerms version tables are deferred until editing/import creates a genuine version lifecycle.

## D-056 — Module 4 baseline matching policy

- Status: PROVISIONAL / DEMO-BASELINE
- Decision: `MODULE4_BASELINE_V1` discovers contracts by exact normalized supplier CUI inside one client, treats effective-period inclusion and exact currency as deterministic compatibility evidence, and orders candidates by reference. It has no numeric score, tolerance, semantic, amount, value, or SKU matching and is replaceable behind MatchingPolicy.

## D-057 — Contract matching outcomes

- Status: APPROVED / FROZEN
- Decision: Matching persists exactly `UNIQUE_COMPATIBLE`, `MULTIPLE_PLAUSIBLE`, `UNIQUE_INCOMPATIBLE`, or `NO_MATCH`. Only unique compatible auto-associates. Multiple and unique-incompatible create CONTRACT_MATCH; no match creates MISSING_CONTRACT.

## D-058 — Expired contract guardrail

- Status: IMPLEMENTED / AWAITING USER-RUN TESTS
- Decision: Module 4 does not map an out-of-period discovered contract to either NO_MATCH or UNIQUE_INCOMPATIBLE. The baseline policy stops that decision explicitly without changing invoice/task state until product semantics are approved.

## D-059 — Human contract confirmation

- Status: APPROVED / FROZEN
- Decision: Confirmation accepts only a persisted candidate from the active match run, validates invoice/task/contract revisions and client ownership, and atomically creates the snapshot association, resolves the task, moves the invoice to DEDUPE_CHECKED, audits, and enqueues continuation.

## D-060 — Backend Module 4 approval and freeze

- Status: APPROVED
- Decision: Backend Module 4 is approved and frozen. Backend Modules 1–4 and the frontend remain approved/frozen.

## D-061 — Line classification identity

- Status: APPROVED / FROZEN
- Decision: Classification is relational state with exactly one current record for each invoice-line and dimension pair. The only dimensions are `ACCOUNT`, `VAT`, and `DEDUCTIBILITY`; each retains proposal, effective decision, opaque confidence, explanation, legal text, review state, policy, and exact rule-version evidence where applicable.

## D-062 — Classification review lifecycle

- Status: APPROVED / FROZEN
- Decision: Explicit policy output, never a numeric confidence threshold, decides review. All pending items for one invoice are grouped into exactly one `CLASSIFICATION / OPEN` task. Partial decisions keep the task and invoice blocked; the final decision atomically resolves the task, moves the invoice to `READY_FOR_SAGA`, audits, and enqueues continuation.

## D-063 — Module 5 baseline policy

- Status: PROVISIONAL / DEMO-BASELINE
- Decision: `MODULE5_BASELINE_V1` uses only explicit technical match kinds (`DESCRIPTION_CONTAINS`, `ALWAYS`, or `NO_AUTOMATION`). No match and same-level ambiguity require review. Confidence is opaque text. This is deterministic parity behavior and contains no validated accounting or tax knowledge.

## D-064 — Stable rules and immutable versions

- Status: APPROVED / FROZEN
- Decision: `ClassificationRule` is stable identity and `RuleVersion` is immutable history. Only version creation and creation of one direct client override from a global origin are public mutations. A direct client override replaces only its referenced global rule for that client; unrelated overlap is review-required rather than arbitrarily ordered.

## D-065 — Human correction is non-learning

- Status: APPROVED / FROZEN
- Decision: Accepting or correcting a proposed classification changes only that line-dimension decision. It never creates or updates rules, suggests rules, trains a model, or reclassifies old invoices. Historical decisions keep the exact rule version or policy version that produced the proposal.

## D-066 — Rule effective dates remain display-only

- Status: PRODUCT DECISION REQUIRED
- Decision: Effective periods are persisted and displayed, but Module 5 does not interpret invoice date versus processing/publication date. The baseline reads the latest immutable version without applying effective-date filtering; production execution semantics require explicit approval.

## D-067 — Explicit runtime data authority

- Status: APPROVED
- Decision: API mode uses the Go/PostgreSQL backend exclusively for every Module 1–5 capability. Mock mode is an explicit alternative for frozen demos and isolated tests. The hybrid adapter and silent not-found/mutation fallback are removed.

## D-068 — Complete invoice-list read for milestone scale

- Status: APPROVED
- Decision: `GET /api/v1/invoices` returns the complete deterministic milestone dataset with optional client filter. Existing Invoice List filters/sort and Dashboard current-month KPI derivation remain in the frontend over this complete backend source. Pagination and a dashboard aggregate endpoint are deferred until scale requires them.

## D-069 — Backend-owned local pipeline runtime

- Status: APPROVED / SUPERSEDED IN EXECUTION TOPOLOGY BY D-071
- Decision: Module 6 proved backend-owned progression with an API-local dispatcher while React only polled persisted state. Module 7 replaces the normal execution topology with the dedicated worker; the approved product semantics remain unchanged.

## D-070 — Module 6 deterministic convergence seed

- Status: APPROVED
- Decision: One reset-oriented devseed path materializes independent backend fixtures for KPI, scope, matching, missing contract, classification, immutable rule changes, duplicate, ready/exported, SAGA failure, history, and a full accountant journey. Pending continuations belonging to already-materialized older demo fixtures are removed before enabling the Module 6 runtime.

## D-071 — Dedicated asynchronous worker

- Status: APPROVED / FROZEN
- Decision: Production-style asynchronous progression belongs to `cmd/worker`, not API replicas. The API commits synchronous commands and transactional outbox intent. Its legacy in-process dispatcher defaults off and exists only for explicit development/test use.

## D-072 — At-least-once outbox delivery

- Status: APPROVED / FROZEN
- Decision: Worker dispatchers claim with PostgreSQL row locks and `SKIP LOCKED`, publish after commit, and record dispatch afterward. A crash between publish and mark may republish after lease expiry. This is intentional at-least-once delivery; idempotent consumers, not an exactly-once claim, protect domain state.

## D-073 — Minimal Asynq vocabulary

- Status: APPROVED / FROZEN
- Decision: One `workflow:continue_invoice` task carries identifiers and correlation metadata only. It invokes the existing pipeline service, which delegates matching, classification, and fake SAGA export to existing capabilities. Human commands are never queued.

## D-074 — Operational retries are not product workflow

- Status: APPROVED / FROZEN
- Decision: Asynq retries transient job failures with bounded configurable backoff and archives exhausted jobs. Outbox publish failures retry with a capped delay and become durable operational `FAILED` rows after a configured limit. No operational failure creates a ValidationTask or a product pipeline status.

## D-075 — Runtime observability boundaries

- Status: APPROVED / FROZEN
- Decision: JSON logs, vendor-neutral OTLP traces, and bounded-cardinality metrics are operational observability. `ActivityEvent` remains business audit. Logs and jobs do not contain invoice XML, contract documents, raw payloads, or sensitive free text.

## D-076 — Backend Module 7 approval and freeze

- Status: APPROVED / FROZEN
- Decision: Backend Module 7 is approved and frozen. The frontend and Backend Modules 1–7 remain approved/frozen.

## D-077 — Inbound SPV technical delivery identity

- Status: APPROVED / FROZEN
- Decision: One SPV connection belongs to one accounting client/CUI. Connection plus ANAF message ID is the immutable technical-delivery identity; content hash is verification only. It remains separate from Module 2 business duplicate policy.

## D-078 — SPV source document lifecycle

- Status: APPROVED / FROZEN
- Decision: Original ANAF ZIP bytes and operational processing state live outside `Invoice.PipelineStatus`. Successful parsing enters the existing invoice pipeline only through `DOWNLOADED`; source failures create no validation task.

## D-079 — SPV structured format boundary

- Status: APPROVED / FROZEN
- Decision: The first adapter supports received UBL 2.1/CIUS-RO Invoice and CreditNote documents. CII, OCR, outgoing submission and ANAF PDF conversion are explicitly deferred.

## D-080 — ANAF/SPV local acceptance gate

- Status: APPROVED / FROZEN
- Decision: On 2026-09-14 the user-run empty/incremental migration, unit, PostgreSQL, Redis/Asynq, concurrency, race, frontend regression, Playwright and local fake-ANAF runtime gates all passed. This verifies the implemented adapter against synthetic protocol fixtures; it does not replace credentialed certification against the live ANAF service.

## D-081 — Durable OAuth state protection

- Status: IMPLEMENTED / AWAITING ANAF-SPV CONNECTION UX APPROVAL
- Decision: Browser OAuth state is a cryptographically random 256-bit value. PostgreSQL stores only its SHA-256 digest with client, environment, return path, expiry and consumption time. Atomic consume-before-exchange makes it single-use across replicas and restarts; invalid, expired and replayed callbacks store no credentials.

## D-082 — One client-owned connection identity

- Status: IMPLEMENTED / AWAITING ANAF-SPV CONNECTION UX APPROVAL
- Decision: The existing one-connection-per-client constraint remains authoritative. First authorization creates the connection; reconnection updates the same identity and credentials. Disconnect marks it revoked and clears locally usable credentials without deleting source documents or imported invoices.

## D-083 — Connection health is separate from synchronization result

- Status: IMPLEMENTED / AWAITING ANAF-SPV CONNECTION UX APPROVAL
- Decision: The backend derives the user-facing connection states NOT_CONNECTED, CONNECTED, NEEDS_REAUTHENTICATION, ERROR and DISABLED. Last synchronization independently exposes NEVER, RUNNING, SUCCEEDED or FAILED. A transient sync failure therefore does not disconnect valid credentials.

## D-084 — Manual sync reuses the approved worker path

- Status: IMPLEMENTED / AWAITING ANAF-SPV CONNECTION UX APPROVAL
- Decision: Manual synchronization returns after enqueueing the existing `spv:sync_account` task. Asynq uniqueness coalesces repeated clicks for one minute, while approved delivery/document idempotency remains the correctness boundary. No ANAF call executes in the HTTP request.

## D-085 — OAuth identity verification limitation

- Status: IMPLEMENTED / EXTERNAL VERIFICATION REQUIRED
- Decision: The inspected token response provides no reliable authenticated CIF claim, so the UI/API explicitly reports identity validation as unavailable and does not pretend OAuth established the client's CUI. The frozen ingestion guard still rejects every downloaded document whose buyer CUI differs from the connection's AccountingClient CUI.

## D-086 — Qualified certificate remains in the browser/ANAF boundary

- Status: IMPLEMENTED / AWAITING ANAF-SPV CONNECTION UX APPROVAL
- Decision: ARMQU and ANAF's OAuth architecture use the qualified certificate through browser/OS/USB-token middleware at `logincert.anaf.ro`. Diana must show certificate prerequisites and the selected client's authoritative CUI, then redirect to ANAF. It must not accept or persist certificate files, private keys or PINs. OAuth application credentials remain server configuration. No certificate model, upload endpoint or migration is justified.

## D-087 — Covered-CIF evidence is post-authorization provider metadata

- Status: EXTERNAL VERIFICATION REQUIRED
- Decision: ARMQU obtains the covered-CIF set from the authenticated `listaMesajePaginatieFactura` response's top-level `cui` field during synchronization, not from certificate bytes or a proven OAuth claim. ARMQU does not enforce callback-time membership, and the inspected code does not establish behavior for empty/error responses. Diana keeps `identityValidation: NOT_AVAILABLE` and the frozen invoice buyer-CUI guard until credentialed ANAF certification can define a reliable activation-time check.

## D-088 — Versioned SAGA C invoice XML adapter

- Status: IMPLEMENTED / AWAITING USER-RUN TESTS
- Decision: `SAGA_C_INVOICE_XML_2026_V1` implements the current SAGA C `Import date` invoice XML hierarchy as an adapter over Diana's domain. It uses exact source decimals and persisted final classifications, never demo accounting defaults. SAGA publishes no XSD or independent schema version, so Diana claims structural validation, not formal schema validation.

## D-089 — Generated is not EXPORTED

- Status: PRODUCT DECISION REQUIRED
- Decision: The verified desktop/manual SAGA workflow has no machine-readable acceptance acknowledgement. A generated durable artifact therefore remains `EXPORTING`. Only explicit HUMAN confirmation of the exact current artifact advances `EXPORTING → EXPORTED`; download alone never does.

## D-090 — No invented cloud-to-desktop bridge

- Status: MANUAL HANDOFF IMPLEMENTED / LOCAL BRIDGE DEFERRED
- Decision: `SAGA_MODE=file` generates and persists an immutable artifact. Diana exposes it through a non-public, client/invoice-scoped API boundary and records every download. The current demo RequestActor is not production authentication, so production exposure remains gated on auth/RBAC. No watched-folder protocol, desktop daemon or SAGA API is claimed.

## D-091 — CreditNote remains blocked at export

- Status: FORMAT VERIFICATION REQUIRED
- Decision: ANAF ingestion now preserves `INVOICE` versus `CREDIT_NOTE`. The SAGA import manual does not define UBL CreditNote/storno mapping, so the real adapter returns a permanent `UNSUPPORTED_DOCUMENT_TYPE` error rather than exporting a credit note as a positive invoice.

## D-092 — Real and fake SAGA selection is explicit

- Status: IMPLEMENTED / AWAITING USER-RUN TESTS
- Decision: `SAGA_MODE=file|fake` selects the adapter without fallback. Fake remains for frozen tests/demos and is forbidden when `APP_ENV=production`. A real adapter failure cannot become fake success.

## D-093 — Real SAGA file-mode completion

- Status: IMPLEMENTED / AWAITING USER-RUN TESTS
- Decision: `EXPORTED` in current real SAGA file-mode means human-confirmed import of the exact generated artifact, not machine-confirmed SAGA acceptance. Confirmation metadata is immutable. Reopening and re-export after confirmation require a future product decision.

## D-094 — Contract availability is an explicit application event

- Status: IMPLEMENTED / AWAITING USER-RUN TESTS
- Decision: A durable Contract becomes a matching trigger only through `contracts.Service.ContractAvailable`. The command reloads the Contract and atomically records business audit plus an identifier-only `CONTRACT_AVAILABLE` outbox event. Future ingestion calls this boundary and does not know invoice/task internals. No contract CRUD API is introduced.

## D-095 — Missing-contract resume reuses Module 4

- Status: IMPLEMENTED / AWAITING USER-RUN TESTS
- Decision: Blocked invoices are reevaluated by the unchanged `MODULE4_BASELINE_V1` policy over the authoritative persisted eligible-contract set. Unique compatible resumes automatically; multiple plausible and unique incompatible replace the missing blocker with one open match-confirmation blocker; no match preserves the same active missing task including `WAITING` status.

## D-096 — Resume concurrency and fan-out boundary

- Status: IMPLEMENTED / AWAITING USER-RUN TESTS
- Decision: Candidate discovery is cursor-bounded by same client, `AWAITING_CONTRACT`, active missing task, and exact normalized supplier CUI. Each invoice commits independently. Invoice status/revision CAS plus existing unique constraints and deterministic keys make duplicate events, concurrent contracts, stale work, and post-commit redelivery converge without duplicate blockers, associations, continuation, or meaningful audit.

## D-097 — AI proposal is not an authoritative Contract

- Status: IMPLEMENTED — AWAITING USER-RUN TESTS / REVIEW
- Decision: Preserve immutable PDF, versioned normalized AI attempts and explicit human-reviewed final values separately. Gemini native PDF runs asynchronously behind ContractExtractor; all existing commercial Contract fields require deterministic validation and explicit user confirmation. Missing currency/value have no defaults. Buyer mismatch blocks tenant-misfiled confirmation. No manual-from-zero flow, amendment merge or AI matching is introduced.

## D-098 — Durable confirmation activation uses the frozen availability boundary

- Status: IMPLEMENTED — AWAITING USER-RUN TESTS / REVIEW
- Decision: Revision-CAS confirmation transaction creates one ordinary Contract, provenance, safe audit and IDs-only activation outbox. Immediate and recoverable asynchronous activation both invoke the existing ContractAvailable with the same stable key. Existing Module 4 and Missing Contract Resume alone determine invoice/task outcomes. Private bytea source storage is the initial implementation behind DocumentStore; production storage/retention/auth hardening is deferred.

## D-099 — Production eligibility is immutable reviewed evidence

- Status: IMPLEMENTED — AWAITING USER-RUN TESTS / ACCOUNTING REVIEW; NOT FROZEN.
- Decision: Add false-by-default production eligibility, structured source provenance and pack metadata to immutable RuleVersion. Existing demo history stays non-production. Public manual configuration creates unverified NO_AUTOMATION versions; no text box verifies a rule. Future verification permission is deferred RBAC. Default runtime uses production policy; explicit baseline remains in guarded demo seed/tests.

## D-100 — Civil-date classification uses one historical snapshot

- Status: IMPLEMENTED — AWAITING USER-RUN TESTS / ACCOUNTING REVIEW.
- Decision: Filter all historical rule/source periods by exact invoice issue date before existing direct-parent override semantics. Overlapping production versions and multiple same-level matches require review. Read invoice/lines/versions in one repeatable-read transaction and retain exact rule FK/date/basis. Adding a version never reclassifies persisted decisions. Database updates cannot rewrite RuleVersion.

## D-101 — Source confirmation is distinct from fiscal treatment

- Status: IMPLEMENTED — AWAITING ACCOUNTING REVIEW.
- Decision: Narrow positive VAT_SOURCE_RATE_EQUALS predicate only confirms declared structured rate with exact decimal comparison. No VAT treatment/rate is seeded without sufficient reviewed applicability. Account-628 vocabulary is deliberately partial and requires explicit CLIENT_OVERRIDE policy/adoption; no generic/global account guess. Stop automatic DEDUCTIBILITY because TipDeducere serialization does not define VAT versus expense tax semantics. Pack RO_INCOMING_ACCOUNTING_V1_REVIEW_ONLY is empty.

## D-102 — Real SAGA requires verified automatic evidence or accountant review

- Status: IMPLEMENTED — AWAITING USER-RUN TESTS / ACCOUNTING REVIEW.
- Decision: Reject demo/unverified/unattributed automatic decisions even when outputs look numerically valid. Explicit persisted human review remains authoritative and preserves the original proposal/rule. Numeric source VAT agreement and exact totals guards remain. Synthetic SAGA fixtures explicitly record fixture-accountant confirmation rather than masquerading as legally verified automation.

## Approved correction: Accounting Domain Model V2 — 2026-09-15

User approved separating ACCOUNT, VAT_TREATMENT, VAT_DEDUCTIBILITY and EXPENSE_TAX_TREATMENT. Legacy VAT is only source-rate confirmation; legacy DEDUCTIBILITY is a SAGA adapter instruction, never a single universal fiscal conclusion. Preserve legacy values/artifacts/model history and existing workflow vocabulary. New source facts retain absence, exact decimal values, declaration/calculation lineage and immutable source provenance.

Use dated accountant-approved client profiles, client chart/acquisition policies and immutable reviewed rule releases. Keep deterministic V1 bounded to explicit ordinary incoming invoice predicates; no invented production mapping and no V2/AI classification. Additive schema correction only. The real production pack stays empty until reviewed inputs and SAGA acceptance arrive.

Use one authoritative readiness validator for V2 auto completion, human review and generation. Four final decisions do not imply readiness. Unsupported fiscal combinations remain one existing CLASSIFICATION task in AWAITING_REVIEW, including mapping-only blockers with zero pending items. SAGA owns TipDeducere derivation; initial approved ordinary full/full omission is the only V2 mapping. Independent VAT/expense deductions must never be collapsed into guessed N50/I instructions.

User authorized bounded frontend/API expansion for typed controls/read-only facts and accurate legacy labels. Private operator profile/rule/pack release tooling is separate from ordinary rule editing, which cannot promote production eligibility. TEST_ONLY engine/exporter opt-ins are constructor-only; production API/worker do not enable them. Runtime tests/migrations/build are explicitly user-run. V1 is implemented but not complete/frozen; V2 is not started. See ACCOUNTING_DOMAIN_V2.md and CLASSIFICATION_ENGINE_V1.md.


## Client Management + Onboarding V1 — 2026-09-15

- Internal client ID remains ownership authority; CUI is display + deterministic normalized business identity, never VAT registration. Retain old raw uniqueness; add nonempty country/normalized identity uniqueness across all lifecycles. Collisions stop migration rather than merge.
- Lifecycle ONBOARDING / ACTIVE / INACTIVE is independent of readiness. Deactivation stops new SPV polling/queued sync starts and new OAuth/manual sync, preserving history and completion of already discovered documents/in-flight work. Reactivation does not reconnect authorization.
- Company/contact defaults preserve old clients; optional registration/address/contact never block first save/readiness. Identity changes are allowed before dependent records and blocked afterwards.
- Reuse exact immutable Accounting V2 profiles. Drafts are immutable unapproved versions; explicit actor/time/evidence approval saves another version with nonoverlapping approved dates. Open-ended approved-period replacement is blocked pending explicit supersession semantics. UNKNOWN remains unknown. No accounting mappings/rules/releases fabricated.
- SAGA is enabled file export, with master name/CIF, unchanged invoice currency authority and explicit human handoff. Existing clients grandfathered enabled; new clients opt in through settings. Mapping release approval and client-specific target acceptance remain separate; no acceptance authority means validation pending.
- Derive all readiness and fixed next actions from actual company/profile/release/SPV/export config, never completion booleans or cached status. Core configuration does not imply production automation or target acceptance; TEST_ONLY never makes production green. Missing pack does not stop ingestion.
- Client commands/audit/idempotency share one transaction, with actor/key scope and optimistic revisions. OAuth replay state is encrypted separately using existing cipher; ordinary read/history DTOs never return credentials or raw event payloads.
- Existing RequestActor grants are enforced on added settings and protected resource routes, including actor-aware OAuth callback. AllClients creates clients until production grant/auth semantics are approved; demo middleware remains unchanged.
- Runtime tests/migrations/build reserved for user execution. Production RBAC, registry enrichment, rule packs/V2, bridge, merge, hard delete and historical reclassification remain deferred.

## D-103 — Diana Authentication V1 owns application identity

- Status: IMPLEMENTED — AWAITING USER-RUN TESTS / REVIEW
- Decision: Replace the development demo actor in production code with a PostgreSQL-backed opaque session. A valid ACTIVE `auth_users` row and its explicit client grants are the only source for RequestActor. Browser headers, query parameters and frontend-selected identity are never trusted. Test-only actor fixtures remain in `_test.go` files only.

## D-104 — Host-only cookie session and explicit CSRF proof

- Status: IMPLEMENTED — AWAITING USER-RUN TESTS / REVIEW
- Decision: Prefer a revocable server-side session over ARMQU's localStorage JWT architecture. Persist only SHA-256 session/CSRF hashes. Use host-only Secure/HttpOnly/Lax session cookies in Cloud TEST, a readable host-only CSRF cookie plus matching header for unsafe methods, and configured-Origin checks. The same-origin `/api/*` load-balancer route avoids a broad parent-domain cookie.

## D-105 — Google IAP is a gated perimeter transition, not Diana login

- Status: CUTOVER NOT EXECUTED
- Decision: The current Google redirect is IAP on the frontend and normal API backend services. Deploy migration/application auth, provision the operator user, pass anonymous-401 and browser gates while IAP remains, and only then disable both IAP backends. Preserve the exact public ANAF callback backend and load-balancer route. Never disable IAP before server-side Diana auth is proven.

## D-106 — Legislative corpus is global; local stacks auto-seed TEST_ONLY drafts
Legislation sources/versions/fragments have no client scope and serve every client. `make dev` runs the idempotent `legislation-seed` compose job (`legislationdraftimport -skip-existing`, effective-from 2016-01-01 as a local retrieval filter only). Cloud/production still require reviewed manifests via `legislationimport`; TEST_ONLY drafts never become production evidence.

## D-107 — Classification uses AI proposals plus accountant/reusable decisions; no production packs for now
Deterministic production rule packs stay empty. Without a pack, readiness accepts only human-reviewed decisions (including accepted reusable knowledge) and profile-derived decisions (D-109). Existing pack validation remains for TEST_ONLY/e2e paths.

## D-108 — The ordinary SAGA mapping is owned by exporter code
Without a pack, `accounting.DefaultSAGAMapping` (`SAGA_C_DOMAIN_V2_ORDINARY_V1`, ordinary/immediate VAT, full deduction, TipDeducere omitted) applies. No separate approval step exists; the reviewed exporter code is the approval. TEST_ONLY profiles are never exported in production.

## D-109 — Microenterprises are supported; unsupported export variants stay classification-only
`MICROENTERPRISE` profiles get `EXPENSE_TAX_TREATMENT=NOT_APPLICABLE` as a final `PROFILE`-sourced decision (no rule, profile evidence only). SAGA export is allowed for OMFP profiles that are PROFIT_TAX or MICROENTERPRISE, ORDINARY_REGISTERED, WITH_DEDUCTION_RIGHT, without client cash accounting and without pro-rata (`Profile.SAGAExportSupported`). Non-VAT-registered, mixed, pro-rata and cash-accounting clients classify normally but export stays blocked with an explicit reason until a SAGA-accepted representation is validated (no guessed N50/I).

## D-110 — Empty profile account list means the whole global OMFP catalog
`Profile.AccountAllowed` returns true for an empty list; the account must still be active and postable in the global catalog. Staleness for such profiles compares a fingerprint of the postable catalog (`Snapshot.AccountCatalogFingerprint`); older runs without a fingerprint are not flagged. Deterministic rule packs/releases stay strict (`Profile.RuleAccountAllowed`): an ACCOUNT rule requires an explicit profile list and is never inferred from the whole catalog.

## D-111 — Unknown supplier cash accounting is confirmed by the accountant
When e-Factura leaves supplier cash accounting UNKNOWN and no pack rule confirms it, every line needs a human-reviewed `VAT_TREATMENT` of ORDINARY/IMMEDIATE. Promoting that decision as reusable knowledge makes later invoices from the same supplier a one-click confirmation.

## D-112 — Changed classification context is a warning, not a lock
Stale context (profile, catalog, policy, contract) shows a warning with „Reanalizează factura”, but manual decisions remain available. Reusable-knowledge promotion stays disabled while stale. Approve-all uses the same readiness authority as per-decision review.

## D-113 — AI analysis envelope is vocabulary-explicit, per-dimension and size-bounded
The provider prompt (`UNIFIED_ACCOUNTING_PROMPT_V3`) and schema enumerate exactly the typed values the deterministic validator accepts. Legislation is retrieved per unresolved dimension, filtered by source kind (LAW for the Fiscal Code, ORDER for OMFP account functions), deduplicated and capped at 20k characters per fragment and 60k in total. Oversized fragments are skipped, never truncated, so citations stay hash-exact. With an empty profile account list, candidates are narrowed by invoice direction: incoming → classes 2, 3, 6 and 471; outgoing → class 7. Validation still accepts any allowed catalog account. Analysis jobs use their own Asynq deadline (provider timeout + 1 min); other jobs keep `WORKER_JOB_TIMEOUT`. Retrieval terms and candidate narrowing only select evidence and options; they never decide a value. When a VAT_TREATMENT proposal omits both `sourceCategory` and `sourceRate`, validation copies them from the e-Factura line facts (source facts, not judgments); values the provider sent are never replaced, so mismatches still fail.

## D-114 — The invoice accounting snapshot mirrors the current classification run
`invoices.accounting_snapshot` used to be frozen at the first classification, so an invoice classified before its client had a fiscal profile (MISSING_FISCAL_PROFILE) could never become ready, even after explicit reanalysis. `classification_runs.snapshot` remains the immutable per-run history. Migration 000034 allows the invoice copy to change only in the same update that moves `current_classification_run_id` to a new run, and only to exactly that run's snapshot. SAGA artifacts keep their own embedded snapshot, and reanalysis stays forbidden once export has started.

## D-115 — Unambiguous VAT and payment-term clauses are confirmed from source text
User approved (2026-09-29) removing the manual confirmation step for clauses whose executable value is stated unambiguously in the cited text, keeping the values editable. Recognition is deterministic (`commercialvalidation.RecognizeSourceTextRule`), not AI, and every automatic confirmation passes the same `reviewedRuleAllowed` / `reviewedRulePreservesProposal` / `reviewedLiteralSupportedBySource` checks as a manual one. Ambiguity keeps the clause in human review. "Cota legală în vigoare" is modelled as `applicable_vat_rate` (the percentage at signing is informative) and resolves from the global `fiscal_vat_rates` table (standard rate only) unless the dossier records its own value. Revisions create a new immutable snapshot and must stay within the cited clause. Automatic confirmations are attributed to the contract confirmer with `actor_kind=SYSTEM`, `automatic=true`.

## D-116 — Commercial review is evidence side by side; service mapping gets deterministic suggestions
User approved (2026-09-29) after BG26000108 showed two identical „Prețul este corect 21 = 21” cards and a service drop-down without context. A contract VAT clause is evaluated as `VAT_RATE_MATCH` / `VAT_RATE_MISMATCH` (engine `COMMERCIAL_VALIDATION_V2`), never as a price; the invoice view shows a VAT rate that matches on every line as one check, and reads V1 runs the same way when the compared value is the line's VAT rate and the clause is about VAT. Every comparison opens a full-screen view with the e-Factura values and XML paths on the left and the original contract page on the right, the cited snippets located in the PDF text layer and highlighted (no coordinates are stored; matching ignores case, whitespace, dashes and ş/ș variants). Uncovered lines receive the contract's priced services ranked by shared wording and UN/ECE unit compatibility (`commercialvalidation.SuggestServices`). **The tariff is never a signal**: a mapping chosen by amount would make the following price check circular. At most one candidate is pre-selected, only when its score is ≥ 0.6 and ≥ 0.25 ahead of the next; a reviewer still confirms every mapping, so D-097 and Service Terms V1 („no fuzzy matching”) still hold for matching itself, which happens only through confirmed aliases. Reuse for the whole dossier is the UI default; learned associations can be revoked and remain as history (migration `000036`). Activating reviewed service prices reads Romanian and English thousands separators (1.800,00 · 1 800,00 · 1,800.00) and reports every service it cannot activate with its reason instead of dropping it; re-activation accepts richer evidence for an unchanged price.

## D-117 — The contract document page is a side-by-side workspace; covered or non-invoice clauses can be closed
User approved (2026-09-29): the contract document page shows what was extracted next to the original PDF, highlighted at the cited words, one item at a time (↑/↓, `?element=` deep links from invoice checks). A confirmed document reports what it enforces now from the dossier's **active snapshot** (`commercialState`: coverage, snapshot version, which rules came from the document, and per confirmed tariff whether invoices are checked against it and otherwise why — computed with the same checks as the activation, before anyone clicks); `confirmedValues.coverage` is never shown as the contract's coverage. A proposed clause can now be closed without becoming a rule, keeping why (migration `000037`: `review_reason_code`, `review_reason`, and `proposal_rule_id` for narrative-only proposals): a contract-reference clause naming the primary reference and an identity clause whose every CUI-like token (`(RO)?\d{6,10}` standing alone) is the dossier's supplier CUI are closed automatically after confirmation (the dossier is already matched by that CUI; a clause naming any other CUI stays in review), and the reviewer may close any other clause as „nu se verifică pe factură” with a 10–500 character reason, recorded as an activity event; the page warns that the terms of a price, VAT or payment clause closed this way are no longer checked. Coverage becomes complete only when no clause of the dossier is left proposed; a conflict found when documents were combined is kept. Every snapshot advance takes the next free version of the dossier (the active snapshot is not always the latest after a revision back to an earlier rule set) and writes nothing when rules and coverage are unchanged. Pre-verification of the review before confirmation is phase 2: only items with confidence HIGH whose snippet is found exactly in the PDF and states the value come pre-ticked; everything else needs the reviewer.

## D-118 — The client's identity and restated tariffs are covered by the dossier, not left to review
User approved (2026-09-30) after invoice inv-31f95fa3f49ffbf729f250c9 (CLEANWAVE → SOFTCO2, contract CWF-0231) stayed in commercial review because of two proposed clauses. Amends D-117. An identity clause is closed automatically when it names at least one CUI and every CUI it names is the dossier's supplier (`COVERED_BY_SUPPLIER_IDENTITY`, unchanged) or the supplier's and the client's (`COVERED_BY_PARTY_IDENTITY`): the client is the buyer checked at contract confirmation (`BUYER_MISMATCH`) and on every SPV invoice (D-085). A clause naming any other CUI still stays in review. A proposed fixed or unit price that only restates an active reviewed service tariff (`service-*`: same plain literal, compatible currency, no applicability of its own, the service named in its text) is closed as `COVERED_BY_SERVICE_TARIFF`, at confirmation, after tariff activation and by the `contractrules close-covered-clauses` backfill; confirming such a clause is refused, and service suggestions never offer it next to the tariff it restates (this also hides restatements confirmed before). The comparison is between two rules of the same contract, never against an invoiced amount, so D-116 still holds. An identity rule may only state the supplier's CUI (the engine compares it with the invoice supplier): `ConfirmProposedRule` refuses another CUI and confirmation readiness reports `IDENTITY_RULE_NOT_SUPPLIER`. The contract page labels identity clauses by the party they name and explains that a clause left „De rezolvat” keeps every invoice of the contract in review; the free-text closing reads „nu influențează facturile”.

## D-119 — An explicit service mapping stays with its invoice; the invoice opens where the action is
User approved (2026-09-30), same invoice. A mapping confirmed with „Ține minte” is stored twice: as the dossier's learned wording and as the invoice's own mapping, so revoking the wording (which now asks for confirmation) affects only future invoices, as its message says. „Serviciul nu apare în contract” offers the real outcomes — ask the supplier for a corrected invoice (`WAIT_FOR_CORRECTION`), accept the line as a reasoned exception (`ACCEPT_EXCEPTION`), or upload the annex that adds the service — instead of a disabled button. A validation result made on an older contract version, or before a learned wording changed, says so and offers to run again; contract-side mutations invalidate the invoice's commercial validation. An invoice opens on the tab of its open task or waiting status (Contract / Clasificare) and pins it in the URL once, so polling never moves the page; Rezumat lists „Ce ai de făcut” with links to each step. A unit-rate tariff whose quantity the contract fixes (`CONTRACT_FIXED_QUANTITY`, unchanged from the cited value) seeds its `unit_quantity_*` variable with source `CONTRACT`; a clause-confirmed unit rate now gets its quantity variable definition too. The coverage finding cites the clauses still open, so its link opens the document that holds them. Frontend changes extend the approved/frozen frontend with the user's explicit approval.

## D-120 — An invoice without a contract may continue on the accountant's reasoned decision
User approved (2026-09-30) while planning the test on real invoices of VICTORIA 1881 EVENTS S.R.L., where none of the 43 purchase invoices has a supplier contract. Amends D-001: a `MISSING_CONTRACT` task, OPEN or WAITING, may be closed with „Continuă fără contract”, which requires a reason of 10–500 characters and records the actor. In one transaction the task becomes RESOLVED (`resolution_metadata.reason = CONTRACT_WAIVED`, with the note and actor), the invoice moves `AWAITING_CONTRACT → DEDUPE_CHECKED` (trigger `CONTRACT_WAIVED`), two audit events are written (`MISSING_CONTRACT_WAIVED`, `CONTRACT_WAIVED`) and the pipeline continuation is enqueued. D-052 is unchanged: requesting a contract still keeps the invoice blocked. Following D-053, the action has its own invoice endpoint, `POST /api/v1/invoices/{id}/contract-waivers` (`taskId`, `expectedRevision`, `reason`, `Idempotency-Key`). No association is created and no migration is needed. Commercial validation of a waived invoice without a contract snapshot does not report missing coverage or uncovered lines; it records one `CONTRACT_WAIVED` finding, `CONFORM`, citing the reason, actor and date, so the invoice goes on to classification. Credit-note checks still apply, and an invoice that has not been waived behaves exactly as before. A contract uploaded later does not re-evaluate a waived invoice, because only invoices still in `AWAITING_CONTRACT` are candidates (D-095). Frontend changes extend the approved/frozen frontend with the user's explicit approval.

## D-121 — Every model call is recorded against its AI run
User approved (2026-09-30). A run is one accounting analysis (`accounting_analysis_runs`, whose Asynq retries reuse the row) or one contract extraction attempt (`contract_extraction_attempts`, extraction plus clause normalization; each Asynq retry is a new attempt). Every `POST /interactions` call, including 429/5xx, invalid envelopes and transport errors, is one append-only row in `llm_usage_events`. The row is recorded by a shared Gemini transport (`internal/platform/gemini`) right after the response body is read and before any status, envelope or schema check, so all error paths are covered; the three copied call sites now use it with unchanged error taxonomy. The owning service puts the run in the context (`llmusage.WithScope`); the client is always taken from the run, never from the caller. Recording is best-effort (`WithoutCancel`, 5 s, log with all counts, failure/unattributed metrics): losing a row is preferred to failing the job, whose retry would bill another call. Thinking, cached and tool-use tokens are captured in addition to input/output; the legacy run-table token columns keep their meaning. The account total is the sum over the accountant's current grants (all clients for `all_clients`); there is no user parameter. The approved/frozen frontend and backend Modules 6–7 are extended additively with the user's explicit approval (sidebar item, client card, routes, worker wiring, metrics).

## D-122 — Costs use versioned, append-only USD prices and are frozen at call time
User approved (2026-09-30). `llm_model_prices` holds USD prices per 1M tokens by provider, model, tier and `effective_from`; a trigger rejects updates and deletes, so a price change is a new row in a new migration (no admin UI). Each call is priced once, at insert, with the price valid at `occurred_at`: `(input − cached) × input + cached × cache + (output + thought) × output`, stored as `numeric(20,10)` so sub-cent calls never round to zero. A missing price gives `NO_PRICE` (excluded from sums, flagged); a response without usage gives `NO_USAGE`. Seed prices come from the Gemini pricing page (2026-09-24): gemini-3.8-flash 0.75 / 0.075 / 3.75 applied to all earlier history, and 1.50 / 0.15 / 7.50 from 2027-01-01. Successful historical runs are backfilled once as `BACKFILL` rows (no thinking tokens, retries or failures), priced by their date and shown as partial history. The tables are raw SQL without Ent, following `accounting_analysis_runs` and `legislation_*`, because they are only read through aggregates.

## D-123 — A line not subject to VAT has a zero source rate
User approved (2026-09-30) during the VICTORIA 1881 EVENTS test. Suppliers that are not VAT payers issue lines with category `O`, and EN 16931 (BR-O-05) forbids a VAT rate on them. Diana required a source rate for every `VAT_TREATMENT` value, so all 16 AI proposals on such lines were invalid and the accountant could not correct them either: the correction dialog refused them with „Sursa nu conține cota/categoria necesară”. For a category `O` line without a rate, the source rate is now 0 by definition, both in AI validation (`sourceTaxRate`: backfill and the `VAT_SOURCE_MISMATCH` check) and in the correction dialog. A missing rate in any other category stays missing and is still refused. Source facts are not changed, and the treatment itself remains a decision for the AI proposal or the accountant. The frontend change extends the approved/frozen frontend with the user's explicit approval.


## D-124 — An issued invoice keeps the UBL supplier as fact; direction and customer are stored beside it
User approved (2026-09-30) for Vânzări V1 (`Docs/VANZARI_V1.md`). `invoices.direction` is INCOMING or OUTGOING (default INCOMING for every existing row); an OUTGOING invoice also stores `customer_name`, the raw `customer_identifier`, its normalized form and its kind (CUI, CNP, OTHER). `supplier_*` keeps its factual UBL meaning, so for an issued invoice it is the client itself and the duplicate index (client, supplier, number, day) means the client's own number and date. The counterparty (supplier or customer) is derived, never stored twice. A CNP is stored in full, because SAGA `ClientCIF` and partner learning need it, but it is masked (first three digits + `***`) in API responses, UI, logs and AI envelopes.

## D-125 — SPV sync lists received (P) and sent (T) invoices; direction comes from the XML parties
User approved (2026-09-30). Each sync lists both ANAF filters, each with its own window; the sent list has its own last-success cursor so the first sync imports the usual initial window of issued invoices. Direction is resolved from the parsed parties, not from the message type: buyer = client → INCOMING (including self-billing), supplier = client → OUTGOING, otherwise the permanent failure of D-085 is kept. The fixture import follows the same rule; fakeanaf emits a sent message only with an explicit flag, so existing suites are unchanged.

## D-126 — The client may be the supplier of a contract; issued invoices link to it optionally, by customer
User approved (2026-09-30). Amends D-054 and D-097. `contracts.client_role` is BUYER or SUPPLIER (default BUYER). Extraction names the parties by role (Locator/Prestator → supplier, Locatar/Beneficiar → buyer) and extracts the buyer name; the client's role is derived deterministically at confirmation, and a contract where neither party is the client is still rejected with `BUYER_MISMATCH`. Purchase matching and missing-contract resume only consider BUYER contracts. Issued invoices use policy `OUTGOING_CONTEXT_V1`: SUPPLIER contracts whose buyer is the invoice customer and whose period covers the invoice date; currency is not compared. One candidate is linked; zero or several leave the invoice unlinked. An issued invoice never waits for a contract and never gets a MISSING_CONTRACT task.

## D-127 — Commercial validation does not apply to issued invoices in V1
User approved (2026-09-30). The contract is context only: sale contracts are priced in EUR and invoiced in RON at the BNR rate, and Diana has no rate source. Validation of an issued invoice records a single `NOT_APPLICABLE_OUTGOING` finding, CONFORM, with or without a linked snapshot.

## D-128 — VAT exigibility of an issued invoice follows the client profile
User approved (2026-09-30). For OUTGOING invoices the expected `VAT_TREATMENT` timing comes from the client profile (`cashAccounting` NO → IMMEDIATE, YES → DEFERRED); the invoice's own „TVA la încasare” note does not decide it. A note that contradicts the profile is shown as a non-blocking warning to be checked with the accountant. Incoming invoices keep D-111 unchanged.

## D-129 — Issued-invoice classification: direction-derived decisions, sales accounts, learning by customer
User approved (2026-09-30). `VAT_DEDUCTIBILITY` and `EXPENSE_TAX_TREATMENT` of an OUTGOING line are final `NOT_APPLICABLE` decisions with the new source `DIRECTION`; reusable knowledge never replaces them. `ACCOUNT` is the credited account, limited to class 7, 167, 419 and 472; the receivable 4111 and the VAT account are implicit. AI retrieval for issued invoices uses the OMFP functions of 704, 706, 708, 167, 419 and 472 and the Fiscal Code VAT exigibility articles. Account mappings and approved knowledge are keyed by direction and counterparty, with purchase keys and hashes unchanged. Issued-invoice reconciliation accepts a customer identified by CNP or by a CUI without RO. One account per line: splitting a line across past and future periods (70x/472) is an open question.

## D-130 — SAGA Ieșiri is generated but not exportable until an accepted import is evidenced
User approved (2026-09-30), following D-102 and D-108. The generator writes the client as `Furnizor*`, the customer as `Client*` (a CNP unmasked), `FacturaTVAIncasare` from the profile, the line account as `Cont` and no `TipDeducere`. Readiness for issued invoices checks identity, reconciliation and lines, and last the code-owned mapping `SAGA_C_DOMAIN_V2_OUTGOING_V1`, which is not approved: the reason is „Formatul SAGA Ieșiri nu este încă verificat.” As for D-109 variants, the invoice stays in `AWAITING_REVIEW` with its classification task open. Purchase XML output is byte-identical.

## D-131 — The invoice list has „Primite” / „Emise” tabs
User approved (2026-09-30) as an additive extension of the approved/frozen frontend. `/invoices?kind=issued` selects „Emise”; without the parameter the page is exactly as before. In „Emise” the first column is „Client” (the customer, CNP masked), the accounting-client column is „Emitent” and search covers customer and number. The invoice detail shows the customer instead of the supplier for issued invoices. `GET /api/v1/invoices` accepts an optional `direction` filter.

## D-132 — The contract review starts pre-verified; each reviewed service keeps the proposal row it came from
User approved (2026-09-29, phase 2 of D-117). Before confirmation the document page is the same side-by-side workspace as a confirmed one. An extracted value comes ticked only when it is `PRESENT`, has confidence `HIGH`, its snippet is found **exactly** in the PDF text layer and the snippet states the value (CUI digits; ISO dates written as `dd.mm.yyyy`, `d.m.yyyy`, with `/` or `-`, or with the Romanian month name; RON as lei/leu, EUR as euro; amounts read with Romanian and English separators exactly as the server reads them; text ignoring case, diacritics and whitespace; the duration as „(ne)determinată”; the document role has no value to state). A proposed service is ticked on the same terms for its description, price, currency and pricing model, with price and currency stated in the cited row. Everything else, every item of a scanned PDF and every item while the PDF text is still being read, waits for the reviewer (Corect / Corectează / Lipsă, Enter and E on the keyboard); the reviewer's decisions live only in the page until confirmation, and the automatic ticks are always derived, never stored. A clause with an executable AI rule is decided explicitly (confirm it, or leave it for after confirmation, which keeps today's partial confirmation); a clause without a formula does not block and is settled on the confirmed page (D-117, D-118). The contract is confirmed only when nothing is left to check and no blocker remains; the blockers and their messages are unchanged, now shown on the item they concern. Each reviewed service carries `sourceIndex`, the proposal row it was read from (`-1` for a service the reviewer added, which has no evidence and is reported „sărit: fără fragment”): the server accepts it on every service or on none, each row at most once, and pairs evidence, price activation and fixed quantities by it. Before, removing a service from the middle of the list handed every later service the evidence of the one before it. Contracts confirmed earlier carry no `sourceIndex` and keep pairing by position.

## D-133 — Calculated line VAT follows the VAT the invoice declares per category
User approved (2026-10-01) after the VICTORIA sales run: VE18810062 declares 4171.89 VAT on 19866.17 at 21%, while net × rate gives 4171.8957, so Diana reported 4171.90 and the invoice could never reconcile to its own total. e-Factura lines carry no VAT amount and the issuer rounds VAT per category (EN 16931), so a line VAT that Diana calculates is rounded to two decimals and the remaining difference to the declared category subtotal is spread one cent per line, largest net first. A group is left unchanged when it has no single declared subtotal in the document currency or when the difference exceeds one cent per calculated line; declared line VAT is never changed. Export reconciliation accepts a line VAT within one cent of net × rate; invoice, VAT and category totals are still reconciled exactly. This extends the protected SAGA reconciliation and applies to received and issued invoices imported from now on; stored invoices are not re-parsed.
