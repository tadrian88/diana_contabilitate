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
