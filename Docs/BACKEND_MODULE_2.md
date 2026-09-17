# Backend Module 2 — Invoice Pipeline Core

Status: approved and frozen.

## Delivered scope

- explicit invoice pipeline transition table with executable and deferred edges;
- optimistic concurrency through invoice revisions and idempotent command audit keys;
- transactional invoice ingestion, invoice lines, activity audit, and durable outbox;
- technical ingestion idempotency on `(client_id, spv_reference)`;
- terminal business-duplicate detection;
- exact decimal persistence and transport parity for invoice lines;
- deterministic fake SAGA boundary used only to test success/failure state behavior.

No public generic transition endpoint was added. Pipeline commands remain an
internal application capability. Redis/Asynq and real SPV/SAGA integrations are
still deferred.

## Duplicate invariant

The fingerprint below is accepted as `PROVISIONAL_V1`. It is a replaceable,
versionable duplicate-detection policy, not yet a final accounting rule. Only
the invoicing identity boundary and its PostgreSQL lookup/index depend on this
formula; ValidationTask and later domain behavior must not depend on it.

The approved identity is:

`client + normalized supplier CUI + normalized invoice number + issue day`

Normalization is deliberately conservative: trim surrounding whitespace,
collapse internal whitespace, and uppercase. Punctuation remains significant.
The application rejects ingestion without supplier CUI because the approved
business identity would be incomplete.

The first accepted document is canonical. A later document with a different
SPV reference and the same business identity is persisted as a distinct invoice
with status `DUPLICATE`, a reference to the canonical invoice, and no outbox
continuation. `DUPLICATE` has no outgoing transition and never reaches SAGA.

Amount equality and currency equality are persisted as separate verification
signals. They do not participate in duplicate identity and do not change the
terminal verdict. Decimal amount comparison is numeric and ignores scale.

A retry with the same client and SPV reference returns the already persisted
invoice even if delivery metadata differs. It does not create a duplicate,
additional lines, audit records, or outbox work.

## Transaction and concurrency guarantees

Invoice, lines, initial audit, and optional outbox continuation are committed in
one transaction. PostgreSQL partial uniqueness permits any number of duplicate
records while guaranteeing one canonical invoice for a complete business key.
Concurrent deliveries are covered by integration tests: one becomes canonical
and the others become terminal duplicates.

Transition updates compare both expected state and revision. Audit and outbox
records share the transition transaction. Replaying the same command is
idempotent; competing commands produce one winner and a conflict for the other.

## Explicitly deferred

- contract persistence and contract matching decisions;
- automatic choice of a unique compatible contract;
- human review tasks for multiple or incompatible candidates;
- missing/expired-contract resumption;
- classification persistence and rule execution;
- SAGA retry policy and real export;
- Redis/Asynq workers and production outbox claiming;
- real SPV ingestion adapter, authentication, OCR, Gemini, and MCP.
