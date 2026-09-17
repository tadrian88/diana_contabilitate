### REAL ACCOUNTING RULES RESULT

IMPLEMENTED — AWAITING USER-RUN TESTS / ACCOUNTING REVIEW. **Architecture implemented; zero production mappings seeded; NOT FROZEN.**

### EXECUTIVE SUMMARY

Default production execution excludes demo/unverified rules, evaluates exact historical rule/source periods, retains exact immutable provenance, uses one consistent database snapshot and routes unresolved cases through existing CLASSIFICATION review. SAGA now checks verified automatic evidence or explicit accountant confirmation. Source/data/policy gaps prevented a defensible production mapping pack; none was invented.

### ACCOUNTING / TAX RESEARCH PERFORMED

Official Romanian accounting/VAT/deduction source discovery plus mandatory Diana model/engine/SAGA semantic audit and read-only armqu discovery. VERIFIED facts, NOT VERIFIED applicability, OUT OF SCOPE treatments and PRODUCT POLICY REQUIRED choices are separated in [research](ACCOUNTING_RULES_RESEARCH.md).

### AUTHORITATIVE SOURCES

- [OMFP 1802/2014, Ministry of Public Finance](https://static.anaf.ro/static/10/Anaf/legislatie/OMFP_1802_2014.pdf): art.2/5/10, annex point3, chapter14 point593 and account628; original start 2015-01-01 subject to the regulation's fiscal-year provision. Supports partial vocabulary, entity scope, analytics and client-policy distinction; current client adoption/consolidation is not verified.
- [ANAF VAT change material](https://static.anaf.ro/static/10/Anaf/AsistentaContribuabili_r/Cotele_de_TVA_09.2025.pdf): fiscal art.291(1)-(2), 21/11 change from 2025-08-01; supports research/boundary fixtures, no arbitrary invoice tax-treatment rule.
- [ANAF Brașov VAT material](https://static.anaf.ro/static/10/Brasov/Brasov/cote_TVA.pdf): art.291 and conditional Legea141/2025 art.III transition 2025-08-01 through 2026-07-31; supports refusal to infer broad reduced-rate treatment.
- [ANAF Brașov deduction material](https://static.anaf.ro/static/10/Brasov/Brasov/limite_2025.pdf): art.25(3)(l), VAT art.298/norms point68; informative snapshot updated 2025-04-09. Supports separating concepts and requiring contextual evidence; no fiscal percentage mapping implemented.

All retrieved/verified 2026-09-15. Full historical/current-to-2026 legislative consolidation remains NOT VERIFIED after portal/large-HTML retrieval limits; no rule is fabricated to bypass this limitation.

### DEDUCTIBILITY SEMANTIC AUDIT

Current DEDUCTIBILITY is a SAGA TipDeducere instruction (`SAGA_DEFAULT`, `N50`, `I`). It does not independently calculate VAT deduction entitlement or expense tax deductibility. Its intended fiscal/product semantics remain ambiguous. **PRODUCT DECISION REQUIRED — DEDUCTIBILITY SEMANTICS.** Automatic deductions are stopped; manual decisions remain authoritative.

### EXISTING DATA MODEL GAPS

Missing persisted tax category, exemption reason/code, legally relevant tax-point/advance context, client accounting-regime adoption, tax regime and business-use evidence. Exact line VAT rate/amount and invoice issue date are present.

### RULE ARCHITECTURE CHANGES

Reuse existing three dimensions, tasks, statuses, direct GLOBAL/CLIENT_OVERRIDE precedence and manual flow. Add immutable production eligibility/pack/provenance, exact date selection and VAT_SOURCE_RATE_EQUALS. No AI/DSL/new API hierarchy.

### PRODUCTION ELIGIBILITY

False by default for every existing/new manually configured version. Database enforces required provenance and rejects the demo placeholder; runtime validates source structure, predicate/value semantics, client policy and periods. Public creation cannot verify or promote a rule. Future verification authorization remains deferred RBAC under the existing actor/audit boundary.

### EFFECTIVE-DATE MODEL

Strict civil YYYY-MM-DD values, inclusive rule/source periods, exact invoice date trace. All historical versions loaded; no newest-row winner. Overlap is intentionally review-only. Existing rule administration date inputs normalize calendar dates, avoiding instant/timezone comparisons.

### RULE PACK

Exact identifier: **RO_INCOMING_ACCOUNTING_V1_REVIEW_ONLY**. Explicit empty production pack; no automatic fiscal mapping seeded.

### PRODUCTION RULE INVENTORY

| Rule ID | Dimension | Condition | Output | Effective period | Authority | Automatic? | Reason |
| --- | --- | --- | --- | --- | --- | --- | --- |
| No accepted production rule | — | — | — | — | See source research | No | Missing reviewed applicability/policy/context |

### ACCOUNT RULES

Partial sourced vocabulary OMFP_1802_2014_628_V1. Future narrow client overrides require explicit reviewed policy/adoption. No generic/global/ALWAYS mapping. Manual legitimate SAGA accounts/subaccounts remain supported.

### VAT RULES

Positive supplier-declared rate confirmation uses existing exact structured rate/amount and configured exact decimal parameter. It does not conclude tax-treatment correctness. No historical/current rate or treatment mapping is seeded.

### DEDUCTIBILITY RULES

None eligible for automatic execution while semantic/context gates remain unresolved. No generic vehicle-to-N50 or ALWAYS SAGA_DEFAULT rule.

### RULES REJECTED FROM AUTOMATION

Generic service-to628, global/ALWAYS accounts, description VAT inference, blanket historical/current rates, generic zero VAT, vehicle N50, default deductions, reverse charge/intra-community/import/exemption inference and CreditNote treatment. See research's explicit rejected-rule table.

### SOURCE DATA GAPS

| Desired classification | Required fact | Present in Diana? | Source | Consequence |
| --- | --- | --- | --- | --- |
| Source VAT rate confirmation | Line percent/amount | Yes, exact | InvoiceLine/e-Factura parser | Narrow predicate; no shipped treatment mapping |
| VAT category/exemption | Category and exemption evidence | No persisted fields | UBL tax data | VAT SOURCE DATA GAP; review |
| Historical VAT treatment | Relevant supply/tax-point context | Issue date only | Applicable fiscal provisions | No blanket rule |
| ACCOUNT policy | Client regime/adopted account policy | No client model; future immutable policy citation possible | OMFP1802/2014 | Reviewed CLIENT_OVERRIDE required |
| VAT/expense deductibility | Tax regime, purpose/use, exceptions | No | Separate fiscal provisions | Review |
| TipDeducere | Approved product meaning/instruction | Manual choice only | SAGA contract/accountant | PRODUCT DECISION REQUIRED |

### CLIENT POLICY DEPENDENCIES

Accounting-regime adoption, recognition/current-expense decisions and account analytics; VAT source-confirmation policy; intended deduction/import behavior. None encoded as universal global law.

### LEGAL PROVENANCE

Immutable structured source/instrument/provision/URL/effective-period/verification/notes fields. Separate creator, scope, condition, result and pack. Runtime has no legislation HTTP dependency.

### RULE VERSIONING

New versions insert history; Ent immutable fields plus database BEFORE UPDATE trigger prevent rewriting. Decisions retain original version FK/date/basis. No historical reclassification. Open-ended historical production periods are not silently closed by edits; ambiguous new overlap remains review-only.

### CLASSIFICATION FLOW

Default empty pack: LINES_READ → CLASSIFIED → AWAITING_REVIEW. Accountant completion may reach READY_FOR_SAGA using existing semantics. Fully automatic fiscal completion is deliberately unavailable pending deduction semantics.

### REVIEW BEHAVIOR

CLASSIFICATION groups unresolved dimensions; no fabricated fallback, numeric confidence or arbitrary winner. Corrections preserve proposal/actor/timestamp/history and do not create rules.

### SAGA INTEGRATION

Verified automatic rule/source/date evidence or explicit persisted accountant review is required. Demo/unverified/unattributed automatic results rejected. Exact source-rate equality, totals, account format, TipDeducere vocabulary and CreditNote guard remain. Production-source-VAT plus accountant-account/deduction fixture reaches real XML generation.

### DEMO / PRODUCTION ISOLATION

Explicit BaselinePolicy only in compatibility tests and development/test-only devseed. API/worker use production policy. Synthetic successful SAGA fixtures record fixture-accountant review; they are not legally verified rules.

### DATABASE / MIGRATION CHANGES

Additive 000013_production_accounting_rules.sql: false-by-default eligibility, immutable pack/JSON provenance, immutable invoice-date-used, predicate/provenance constraints, update-rejection trigger and production-period index. Original migrations untouched; hash regenerated. Migration application remains user-run.

### FRONTEND IMPACT

Limited rule/version eligibility and source display, safe official external links, source VAT/evaluated-date context and accurate human-confirmation labels. Existing date editing and administration flow remain; no global redesign.

### AUDIT / EXPLAINABILITY

Exact rule version, source period/provenance/pack, policy, proposed/effective values, issue date used, explanation and system actor activity. Manual review retains original evidence and actor/time. Historical rule links are available in classification display.

### IDEMPOTENCY / CONCURRENCY

One read-only repeatable-read input snapshot plus existing revision CAS/command/audit/task keys. New version insertions neither mix snapshots nor mutate persisted classifications. Concurrent snapshot, overlap and replay tests implemented.

### OBSERVABILITY

Requested four bounded-dimension counters; no rule/client/account labels or sensitive description logging.

### TESTS IMPLEMENTED

Production/demo isolation; verified provenance; no-match/ambiguity/overlap; exact civil dates; law-change/future/expiry boundaries; historical versions; temporal overrides; source VAT exactness/conflicts/zero; account policy/analytics; deduction insufficient context; CreditNote refusal; safe default/replay/snapshot convergence; database immutability/provenance constraints; concurrent update snapshot; review lifecycle/manual trace; verified-source-VAT plus manual accounting to real SAGA boundary; SAGA rejection/acceptance; source-link safety; bounded metrics; devseed environment safety. Frozen demo unit/integration services now select BaselinePolicy explicitly. Successful SAGA fixtures now represent human decisions.

### CHECKS ACTUALLY EXECUTED BY CODEX

- gofmt for edited Go sources.
- go generate ./ent.
- atlas migrate hash --dir file://migrations.
- Go compile-only: go test -exec=/usr/bin/true ./....
- Integration compile-only: go test -exec=/usr/bin/true -tags=integration ./internal/platform/postgres ./internal/workerruntime.
- npm run typecheck.
- git diff --check.

No runtime suite, race/integration execution, Playwright, full frontend tests, migration apply or production build was run. Go's compile-only command output is **not a test PASS claim**.

### MANUAL TEST COMMANDS FOR USER

[Exact copy-paste commands grouped A–V](REAL_ACCOUNTING_RULES_TEST_HANDOFF.md): static, unit, effective dates, isolation, ACCOUNT, VAT, DEDUCTIBILITY, version/provenance, PostgreSQL, concurrency, tasks, production SAGA, pipeline, Modules1–7, ANAF/SPV, contract/missing contract, SAGA, frontend, Playwright, incremental/empty migrations and production build.

### ACCOUNTING REVIEW CHECKLIST

Approve the deliberately empty production pack; define TipDeducere versus VAT/expense semantics; approve target-client regime/account628 policy/trigger; decide VAT source-confirmation scope and legal periods; approve exact source facts/provisions for every future version; confirm unsupported zero/vehicle/advanced treatments stay review-only. Full checklist is in the handoff.

### PRODUCT / ACCOUNTING DECISIONS REQUIRED

Deductibility semantics; target-client accounting policy/regime; VAT source-confirmation versus treatment boundary and sufficient historical evidence; full applicable legislative versions before production seeding. No historical bulk reclassification decision is exercised here.

### FRONTEND CONTRACT CONFLICTS

None identified by typecheck; runtime UI tests remain user-run.

### INTENTIONALLY DEFERRED

AI classification fallback; historical bulk reclassification; complex rule DSL; CreditNote/storno accounting; unsupported advanced VAT; automatic learning from corrections; verification RBAC. Live Gemini remains deferred. Contract ingestion preserved.

### ENGINEERING GATE READY?

**NO — runtime suites and migration application unexecuted.** Implementation/static compilation is ready for the user's gate execution, not engineering approval.

### ACCOUNTING GATE READY?

**NO — policy/semantic/source-applicability decisions remain; no mappings approved.**

### READY FOR USER TEST EXECUTION

**YES.**

| Module | Completion state |
| --- | --- |
| Frontend Modules1–7 | APPROVED / FROZEN |
| Backend Modules1–7 | APPROVED / FROZEN |
| ANAF/SPV | APPROVED / FROZEN |
| Real SAGA | APPROVED / FROZEN |
| SAGA Export UX | APPROVED / FROZEN |
| Missing Contract Resume | APPROVED / FROZEN |
| Contract Ingestion + AI Extraction | DO NOT MODIFY / CURRENT STATE PRESERVED |
| Real Accounting Rules | IMPLEMENTED — AWAITING USER-RUN TESTS / ACCOUNTING REVIEW; NOT FROZEN |

STOP. Classification Engine V2 was not started.
