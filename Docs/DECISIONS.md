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
