# Deterministic Classification Engine V1

ENGINE IMPLEMENTED — AWAITING USER-RUN TESTS. Real approved production pack: EMPTY. Real accounting/SAGA acceptance: NOT COMPLETED. V1: NOT COMPLETE / NOT FROZEN. V2: NOT STARTED.

## Bounded implementation

Production service dispatches ACCOUNTING_DOMAIN_V2 invoices to `DomainPolicy`; legacy invoices retain their old policy and three-decision path. The engine is deterministic: exact supplier/client/date/category/rate/currency/type and literal normalized description or exact item/confirmed Contract reference. There are no scores, AI interpretations, history learning, embeddings or a generic rules DSL.

Each line gets exactly four proposals. Missing applicability/evidence or no matching rule produces NO_MATCH and manual review. Multiple matches or overlapping effective versions produce AMBIGUOUS; recency is not a tiebreaker. A client override directly replaces its global parent for that dimension/period; it cannot override another override. Other client policies cannot apply. The whole pack must be valid against the selected profile before evaluation.

The only predicate version is ORDINARY_INCOMING_V1: exact approved supplier ID, reviewed supplier ORDINARY_REGISTERED status and cash-accounting NO, category S, positive exact rate, RON, type 380, and at least one specific description/item/confirmed Contract condition. Profile/date/source applicability is a separate guard. This shape does not implement advanced VAT, cash-accounting, credit notes, allocation or pro-rata computation.

## Flow and authoritative readiness

1. Load invoice, lines, client profile, release and authoritative Contract association in one repeatable-read snapshot.
2. Evaluate immutable pack rules at INVOICE_ISSUE_DATE and produce typed proposals plus execution evidence.
3. Persist the selected profile/pack/Contract snapshot, proposals, actual RuleVersion references and audit records atomically under invoice status/revision compare-and-swap.
4. If any proposal needs review, create one existing CLASSIFICATION task. Otherwise run shared `saga.EvaluateReadiness` inside the same transaction. Failure also creates that task, preserving a clear readiness reason.
5. New-model human review validates the typed value and reason, revises the decision/task atomically, and runs the same authority when no pending decisions remain. Failure persists the review but keeps the task OPEN and invoice AWAITING_REVIEW. Mapping-only tasks permit correction of final automatic decisions.
6. Only readiness success resolves the task and advances to READY_FOR_SAGA with an outbox continuation. SAGA independently rechecks readiness and automatic predicates immediately before generating XML.

Pipeline statuses, task types/statuses, Contract matching/resume, contract ingestion AI and desktop SAGA human acknowledgement semantics are unchanged. The legacy completion path remains deliberately separate to preserve frozen artifacts and tests; shared new readiness prevents V2 from inheriting the old all-items-final shortcut.

## Private reviewed release tooling

`backend/cmd/accountingrelease` consumes strict reviewed JSON from stdin. It has no HTTP endpoint and no TEST_ONLY mode. DATABASE_URL selects the operator target. No production configuration is installed by migration 000014 or devseed.

The concrete operator sequence, after accountant review, is:

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
export DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable'
GOCACHE=/private/tmp/diana-go-cache go run ./cmd/accountingrelease profile < /private/tmp/reviewed-profile.json
GOCACHE=/private/tmp/diana-go-cache go run ./cmd/accountingrelease rule < /private/tmp/reviewed-rule.json
GOCACHE=/private/tmp/diana-go-cache go run ./cmd/accountingrelease pack < /private/tmp/reviewed-pack.json
```

These files are inputs to be provided later; there are deliberately no invented production JSON examples. Repeat the rule command for every reviewed RuleVersion in the release. A reviewed rule input has `rule`, `clientId`, `profileId`, `packId`, `packVersion`, `provenance`, `approval`. `rule` is the typed accounting.Rule, `provenance` uses the existing official-source provenance schema, and `approval` records actor/time/evidence. ACCOUNT provenance must explicitly agree with the adopted framework and client chart policy. Predicate/acquisition evidence, legal basis, dates, scope/parent and exact result are mandatory.

Profile installation rejects test-only/unapproved/invalid payloads. Rule installation validates the existing approved profile, specific acquisition policy, official-source provenance, exact dates and direct override identity. The rule is immutable and production-eligible but remains inactive until a released pack references it. Ordinary public Rule Administration creates NO_AUTOMATION, ineligible versions without domain payload; it cannot promote them.

Pack release rejects synthetic data and requires a coherent dated profile, accountant approval, nonempty reviewed rule set and approved mapping/version/evidence. It checks every actual immutable RuleVersion FK for eligibility, model, pack ID/version, typed payload, source provenance/effective period, category/scope/client/parent. The release stores exact rules/profile identity/mapping policy and emits an approval audit event. Duplicate IDs/versions fail; release updates/deletion are refused. Corrections use a new reviewed release and explicit future effective dates. Overlapping releases force review for newly evaluated invoices; saved invoice snapshots do not change.

The approval mechanism records evidence supplied by an authorized operator; it does not verify a person's accounting credentials or obtain an external signature. Authentication/RBAC remains the existing deferred boundary. Real approved rules and SAGA acceptance are the missing production inputs, not something the CLI invents.

## Source and timing scope

Missing source rate stays absent; explicit zero is retained but outside automation. Category AE and all other special/unknown categories are retained and reviewed. Source cash UNKNOWN requires separate explicit reviewed supplier NO evidence in the rule. Source cash YES or unknown buyer cash/pro-rata eligibility never passes ordinary guards. Invoice/line/header/currency reconciliation and positive quantity/preprice checks occur before READY and again at generation. Retained adjustments or non-unit price bases cause review.

The adopted date basis is inclusive civil INVOICE_ISSUE_DATE. Tax-point changes and advanced timing require review; supply period is retained but no alternative period calculation is added. The explicit approved profile/rule periods determine applicability. MICROENTERPRISE proposes a reasoned manual NOT_APPLICABLE expense decision; initial SAGA mapping still blocks unsupported combinations.

## Audit, idempotency and concurrency

Approval/profile/rule/pack events, automatic execution/routing, human accept/correct and accounting readiness evaluations are persisted with actor/time/correlation and immutable evidence. Human corrections do not overwrite proposals or source facts. Invoice snapshots and artifact snapshots make the exact model/profile/pack/rule/date/source/mapping explainable.

Classification command audit keys provide replay detection; invoice CAS provides one winner for concurrent execution. Review CAS covers invoice, task and decision revisions; a stale task update rolls the decision back. Saved snapshot selection survives a later release during evaluation. Artifact attempts retain invoice revision plus exporter/mapping version in their idempotency identity. Existing artifacts are not regenerated by migration. Asynq remains at-least-once with the frozen PostgreSQL outbox/stale-job boundary.

Metrics retain bounded dimension labels for the legacy and new categories and report rule evaluations, matches, review demand and automatic completion. `accounting_readiness_evaluations_total{result="ready"|"blocked"}` covers application completion/review checks. No supplier/client/invoice identifiers are metric labels. Generator rejection also uses the existing SAGA failure metrics.

## Synthetic fixtures and testing

`internal/accountingtest` is conspicuously TEST_ONLY. `DomainPolicy{AllowTestOnly:true}`, `GenerateTestOnly` and `NewTestOnlyFileExporter` are explicit fixture-only constructors; production API/worker never enable them, and there is no environment fallback. The full synthetic integration fixture runs fake ANAF ZIP ingestion, native archive/matching/dedupe/header/line transitions, authoritative Contract association, four deterministic decisions and durable SAGA XML. It remains EXPORTING until human desktop acknowledgement; production export rejects its test-only release.

Devseed additionally creates `inv-accounting-v2-test-only` with four visible synthetic decisions and deliberately unapproved mapping, leaving a grouped mapping review task. Existing demonstration invoices remain LEGACY_V1. Seed cleanup is scoped to seed-owned legacy catalog/decisions and does not delete V2 history or a production catalog. Seed is a development/test utility, not a migration or a production pack loader.

New tests cover typed values, profile/date/policy isolation, missing/zero rates, declared/calculated origin, source categories/exemptions/totals, four decisions, special/unknown/cash/pro-rata cases, micro inapplicability, direct overrides and ambiguity, confirmed Contract context, shared readiness and generation rejection, mapping approval, PostgreSQL persistence/replay/concurrency/review and immutable releases/snapshots. Existing legacy suites remain for compatibility; only authorized legacy display-label assertions changed.

All runtime tests and real migrations are user-run. Successful compilation is not test PASS. See [CLASSIFICATION_V1_TEST_HANDOFF.md](CLASSIFICATION_V1_TEST_HANDOFF.md).

## Freeze gate and deferred work

Engineering gate requires A–W runtime/static acceptance with incremental and empty database migration checks. Accounting gate requires accountant-approved rules/profile/acquisition evidence, official source periods, exact expected four decisions and real SAGA import acceptance of every represented combination. V1 cannot be declared complete/frozen from synthetic fixtures.

Deferred: AI classification, supplier/historical correction learning, embeddings, semantic matching, Gemini classification, pro-rata calculations, cash-accounting automation, advanced VAT, period ceiling calculation, CreditNote/storno and historical reclassification. Existing Gemini Contract extraction remains separate and unchanged.
