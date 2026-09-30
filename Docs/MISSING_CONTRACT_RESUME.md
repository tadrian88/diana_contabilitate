# Missing Contract Resume

Status: **IMPLEMENTED — AWAITING USER-RUN TESTS / REVIEW**

## Trigger and future ingestion boundary

`contracts.Service.ContractAvailable(ctx, contracts.AvailableCommand{...})` is the
single application entry point for a durable contract that is now eligible for
matching. The current model has no lifecycle/activation status, so existence of the
persisted `Contract` is the authoritative availability condition.

Future Contract Ingestion must:

1. validate and persist the `Contract`;
2. after that write is durable, call `ContractAvailable` with the persisted contract
   ID and a stable ingestion command ID;
3. never update invoices, match runs, associations, or validation tasks itself.

`ContractAvailable` reloads the contract, records
`CONTRACT_AVAILABLE_FOR_MATCHING`, and creates an identifier-only
`CONTRACT_AVAILABLE` transactional-outbox event. Duplicate command IDs are no-ops.
An existing contract that becomes eligible under a future, separately approved
lifecycle may use this same hook. Contract editing/versioning is not defined here.

## Candidate selection

The worker reloads the contract and scans at most 100 rows per cursor page. Candidates
must belong to the same AccountingClient, currently be `AWAITING_CONTRACT`, have an
active `MISSING_CONTRACT` task (`OPEN` or `WAITING`), and share the exact existing
normalized supplier CUI. The client and supplier predicates are authoritative
identity constraints already used by Module 4; date and currency are deliberately
left to `MatchingPolicy`.

Each candidate invoice is isolated in its own transaction. A failed invoice is
reported after remaining rows in the page are attempted, and an Asynq retry safely
rescans from the beginning.

## Matching re-evaluation and outcomes

Resume calls the unchanged `MatchingPolicy` and persists a new immutable
`ContractMatchRun` plus candidates using exactly `MODULE4_BASELINE_V1`. The complete
eligible contract set from PostgreSQL is evaluated; the event's contract is a trigger,
not a forced selection.

| MATCH OUTCOME | MISSING_CONTRACT | NEW TASK | INVOICE STATE |
|---|---|---|---|
| `UNIQUE_COMPATIBLE` | `RESOLVED` | none | `DEDUPE_CHECKED` / normal continuation |
| `MULTIPLE_PLAUSIBLE` | `RESOLVED` | `CONTRACT_MATCH OPEN` | `AWAITING_MATCH_CONFIRM` |
| `UNIQUE_INCOMPATIBLE` | `RESOLVED` | `CONTRACT_MATCH OPEN` | `AWAITING_MATCH_CONFIRM` |
| `NO_MATCH` | unchanged active | none | `AWAITING_CONTRACT` |

For `NO_MATCH`, the same task and revision remain; in particular `WAITING` is not
reopened. A unique compatible result creates the existing immutable automatic
association snapshot and an `INVOICE_CONTINUE` outbox row. Downstream dedupe, header,
lines, classification, and SAGA logic remain owned by the existing pipeline.

## Idempotency and concurrency

Stable keys cover the availability audit/outbox event, each invoice's resume match
run, audits, review task, association, and continuation. PostgreSQL unique constraints
and invoice revision/status compare-and-set make redelivery and post-commit worker
retry converge without duplicate business effects.

Concurrent contract events reload all currently committed eligible contracts before
evaluating. One transaction wins the invoice revision transition; later/stale work is
a no-op and cannot move the invoice backward. Resolving the existing missing task,
creating any replacement match task, associating a contract, changing invoice state,
writing audit, and emitting continuation all commit atomically. The existing partial
unique index continues to enforce at most one active blocker per invoice.

## Audit, observability, and history

Business audit uses `CONTRACT_AVAILABLE_FOR_MATCHING`,
`MISSING_CONTRACT_REEVALUATED`, `MISSING_CONTRACT_RESOLVED`,
`MISSING_CONTRACT_STILL_WAITING`, and the existing association/task events. Details
are accountant-facing Romanian text and include the outcome/policy or selected
reference without worker jargon.

The worker traces contract availability and logs identifiers/outcome counts only.
Low-cardinality counters cover arrival events, evaluated invoices, automatic resumes,
confirmation transitions, still-missing results, and failures. Contract contents are
never placed in Redis or logs.

## Continuare fără contract (D-120) — 2026-09-30

Pe task-ul `MISSING_CONTRACT`, OPEN sau WAITING, contabilul poate alege „Continuă fără contract”, cu un motiv de 10–500 caractere
(`POST /api/v1/invoices/{id}/contract-waivers`). Task-ul devine RESOLVED cu `resolution_metadata.reason = CONTRACT_WAIVED`,
iar factura trece `AWAITING_CONTRACT → DEDUPE_CHECKED` și continuă în pipeline, în aceeași tranzacție cu auditul și outbox-ul.
Validarea comercială înregistrează o constatare `CONTRACT_WAIVED` (CONFORM), după care factura ajunge la clasificare.

O factură continuată fără contract nu mai este candidată pentru reevaluare. Selecția de mai sus cere `AWAITING_CONTRACT` și
un task MISSING_CONTRACT activ, iar `ApplyResumeDecision` întoarce conflict (tratat ca no-op) pentru orice altă stare.

## Explicit boundaries

At the time of this module, no frontend or API endpoint was added (the D-120 waiver above came later). Contract upload, storage, OCR/parsing, AI
extraction, CRUD, advanced matching, expired-contract semantics, deletion, and
rematching already-associated invoices remain outside this module.

`ErrExpiredContractSemantics` remains unchanged and is a permanent domain outcome for
the worker: **EXISTING PRODUCT DECISION REQUIRED — EXPIRED CONTRACT SEMANTICS**.
