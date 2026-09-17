# Implementation report — 2026-09-15

### ACCOUNTING DOMAIN V2 + CLASSIFICATION ENGINE V1 RESULT

Accounting Domain Model V2: IMPLEMENTED — AWAITING USER-RUN TESTS. Classification V1: ENGINE IMPLEMENTED, REAL PRODUCTION PACK NOT APPROVED, REAL ACCEPTANCE NOT COMPLETED, NOT FROZEN. Classification V2: NOT STARTED.

### EXECUTIVE SUMMARY

Implemented immutable source facts, four independent typed decisions, dated approved profiles and rule releases, deterministic matching, typed review and shared readiness. Existing history and workflow vocabulary remain separate legacy behavior.

### ARCHITECTURE

The accounting package owns vendor-independent values; classification evaluates bounded reviewed predicates; PostgreSQL persists exact snapshots under transactional CAS; SAGA owns mapping/readiness and thin serialization. See ACCOUNTING_DOMAIN_V2.md and CLASSIFICATION_ENGINE_V1.md.

### SOURCE FACT MODEL

Header/line category/rate/scheme/exemption, currencies, tax breakdowns/totals, distinct monetary totals, type/party/country/timing/period facts and raw ZIP/parser/hash/path lineage. Missing rate is nil; zero is explicit. Source JSON/model and saved snapshots are protected from UPDATE.

### E-FACTURA PARSER CHANGES

New parses select ACCOUNTING_DOMAIN_V2. Declared versus calculated VAT is explicit; missing rate plus missing VAT retains UNKNOWN source lineage. Header/subtotal facts are never synthesized from line calculations. Unsupported adjustments/base quantities/timing remain retained review evidence.

### CLIENT TAX / ACCOUNTING PROFILE

Immutable client/version/effective period, approval/evidence, framework, chart policy and exact accountCodes, fiscal/VAT/deduction/cash/pro-rata fields. UNKNOWN is explicit. Multiple applicable profiles are ambiguous. No chart/registration applicability is inferred.

### DOMAIN DECISIONS

ACCOUNT, VAT_TREATMENT, VAT_DEDUCTIBILITY, EXPENSE_TAX_TREATMENT. Strict kind/dimension checks; exact limited percentage plus basis; period category plus basis without ceiling calculation; reasoned NOT_APPLICABLE. Human review cannot add an unapproved account analytic.

### LEGACY VAT / DEDUCTIBILITY STRATEGY

LEGACY_V1 keeps ACCOUNT/VAT/DEDUCTIBILITY strings and history. VAT is source percentage confirmation; DEDUCTIBILITY is historical SAGA import instruction. Legacy generator/export version/idempotency identities remain preserved.

### RULE MODEL

Existing actual immutable RuleVersion FK plus additive domain model/payload, reviewed client/acquisition policy, structured predicate, typed result, explanation/legal evidence, scope/parent and dated applicability. Ordinary UI edits stay ineligible NO_AUTOMATION.

### DETERMINISTIC PREDICATES

Only ORDINARY_INCOMING_V1: exact supplier/client/profile/date/currency/type/category/rate, explicit reviewed supplier ordinary registration/cash NO, and literal description or exact item/confirmed Contract reference. No DSL, scores, semantic matching or history learning.

### RULE PACK / RELEASE MODEL

Private accountingrelease profile|rule|pack tooling consumes strict reviewed JSON. Installs approved profile and exact reviewed immutable rule versions, then validates actual eligible references/payload/provenance/period/scope and stores approved release/mapping evidence. TEST_ONLY operator promotion is refused; demo seed identity is not promotable.

### EFFECTIVE DATING

Inclusive civil INVOICE_ISSUE_DATE basis; profile/pack/rule containment. Overlaps create review rather than choosing recency. Repeatable-read selection is retained unchanged even if a newer release appears during evaluation.

### CLASSIFICATION FLOW

Four proposals per line → immutable snapshot/evidence → atomic persistence → one grouped review task or shared readiness → READY_FOR_SAGA/outbox only on success. Legacy completion remains its frozen separate path.

### HUMAN REVIEW

Typed correction and explicit reason/confirmation controls, immutable proposal/source facts and per-command typed before/proposal/final/reason/evidence audit. Pending and mapping-only tasks use existing CLASSIFICATION. Unsupported confirmed values persist but do not resolve the task.

### SHARED READINESS VALIDATOR

saga.EvaluateReadiness guards new-model auto completion, final review, exporter cache reuse and XML generation. Checks four final values, approved snapshots/vocabularies/source provenance/identities/currencies/quantities/reconciliation and representable mapping; automatic predicates are rechecked.

### SAGA DERIVATION

Thin V2 serializer uses validated mapped lines. Only approved ordinary immediate FULL VAT / FULLY_DEDUCTIBLE expense mapping is implemented. V2 exporter version includes mapping version and artifact snapshot includes full accounting snapshot plus typed evidence. XML generation remains unconfirmed/EXPORTING.

### TIPDEDUCERE

SAGA adapter ownership only. The approved initial V2 full/full policy omits the tag. Fiscal enums never contain N50, I, N or SAGA_DEFAULT. Historical codes retain their legacy meaning and bytes.

### UNSUPPORTED SAGA COMBINATIONS

FULL VAT / NONDEDUCTIBLE expense, limited/unequal deductions, special/inapplicable/period-limit/cash/pro-rata/credit-note treatments are blocked by the initial mapping. Human confirmation cannot approve a missing/unsupported mapping.

### HISTORICAL DATA / ARTIFACT PRESERVATION

No conversion/reclassification/reparse job. Existing rows default LEGACY_V1; old values, XML/hash/version and attempt identities are not rewritten. Separate future histories can use line/dimension/model uniqueness. Synthetic cached artifacts cannot be reused by the production exporter.

### DATABASE MIGRATIONS

Additive 000014_accounting_domain_v2.sql and regenerated Ent. Old 000001–000013 files unchanged. Source/snapshot/release protection and model-aware review/uniqueness constraints. Atlas checksum regenerated; no real database migration applied.

### FRONTEND CHANGES

Bounded four-dimension workspace, read-only source/profile/timing/origin, typed controls and explicit human justification, accurate legacy labels, new rule categories and existing provenance links. Existing review flows remain compatible.

### AUDIT / EXPLAINABILITY

Profile approval, rule review, pack release and readiness evaluation events supplement existing classification audit. Decision/source/profile/pack/rule/date/policy evidence and typed human correction history are retained. No learning side effect.

### IDEMPOTENCY / CONCURRENCY

Command audit replay keys, invoice CAS, task/decision review CAS, immutable selected snapshots and revision/export-version attempt uniqueness. Stale review rolls back atomically. Synthetic tests cover replay, concurrent execution and release between evaluation/application.

### OBSERVABILITY

Bounded legacy/new dimension counters plus ready/blocked readiness evaluation counters. API/worker observers configured at startup. Existing SAGA failure metrics cover generator rejection. No client/supplier/invoice IDs as labels.

### TESTS IMPLEMENTED

New accounting/source/profile/value tests, parser absence/origin/exemption/date tests, four-decision/policy/isolation/special/cash/pro-rata/micro/override/date/Contract tests, shared readiness/mapping/generator tests, PostgreSQL persistence/replay/concurrency/review/immutability/release tests, full fake ANAF→Contract→V2→SAGA pipeline, frontend component tests and V2 Playwright. Existing label assertions updated only for authorized historical labels.

### CHECKS ACTUALLY EXECUTED

gofmt; Ent go generate; atlas migrate hash; Go compile/link only with -exec=/usr/bin/true (all packages and integration-tag PostgreSQL tests); npm run typecheck; git diff --check. Runtime unit/integration/race/Redis/frontend/Playwright/build tests and real migrations were NOT RUN. Go ok lines from compile-only commands are not runtime PASS.

### MANUAL TEST COMMANDS

Exact copy-paste groups A–W, shared PostgreSQL/Redis setup and incremental/empty database instructions are in CLASSIFICATION_V1_TEST_HANDOFF.md. Run U/V before database integration groups. Operator reviewed input commands are in CLASSIFICATION_ENGINE_V1.md; no real inputs are fabricated.

### REAL PRODUCTION PACK STATUS

EMPTY. No approved production mappings were seeded or released. Synthetic engineering approval data is TEST_ONLY and not real accounting approval.

### ACCOUNTING INPUT STILL REQUIRED

Approved dated client framework/chart/analytic accountCodes and tax profiles; acquisition/business-purpose/deduction/supplier registration and cash evidence; official-source legal applicability periods; exact approved predicates/results and expected four decisions; approved mapping/version and real desktop SAGA import acceptance for every represented combination.

### ENGINEERING GATE READY?

NO — static preparation complete; user runtime/migration/build acceptance remains.

### CLASSIFICATION ENGINE V1 COMPLETE?

NO — ENGINE IMPLEMENTED, REAL APPROVED PRODUCTION PACK / ACCEPTANCE REMAINS. NOT FROZEN.

### FRONTEND CONTRACT CONFLICTS

Historical display labels intentionally corrected within authorization. Four typed decisions are additive model-aware API/UI changes. Frozen status/task vocabulary and existing legacy command fields remain. No additional conflict discovered during compile checks; runtime regressions await user tests.

### PRODUCT DECISIONS REQUIRED

Accountant-approved production inputs and accepted SAGA representations for independent VAT/expense outcomes. Future explicit reparse/reclassification policy must address already saved missing-profile/pack snapshots; no silent mutation or migration is introduced. Existing authentication/RBAC deferral is unchanged.

### INTENTIONALLY DEFERRED

AI classification; supplier history learning; historical correction learning; embeddings; semantic matching; Gemini classification; pro-rata calculation; cash-accounting automation; advanced VAT; period ceiling calculation; CreditNote/storno; historical reclassification.

### READY FOR USER-RUN TESTS

YES. Implementation/test handoff ready; engineering and real accounting acceptance remain pending. Classification V2 NOT STARTED.
