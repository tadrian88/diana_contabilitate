# Accounting Domain Model V2

Status: IMPLEMENTED — AWAITING USER-RUN TESTS. Engineering/accounting acceptance is pending. Classification V1 is NOT FROZEN; Classification V2 is NOT STARTED. This document defines implementation behavior, not approved Romanian accounting advice. The real production pack is empty.

## Separation of responsibilities

`backend/internal/accounting` defines exact source facts, typed fiscal decisions, dated profiles, reviewed predicates and release snapshots. It imports neither SAGA nor AI. `classification.DomainPolicy` evaluates those predicates. The SAGA adapter derives import instructions and owns the shared readiness authority. PostgreSQL calls that authority transactionally before new-model completion and final human review; the generator calls it again before serialization.

| Model | Dimensions | Meaning |
| --- | --- | --- |
| LEGACY_V1 | ACCOUNT, VAT, DEDUCTIBILITY | ACCOUNT is a chart value; VAT confirms a source percentage; DEDUCTIBILITY is a historical SAGA import instruction. |
| ACCOUNTING_DOMAIN_V2 | ACCOUNT, VAT_TREATMENT, VAT_DEDUCTIBILITY, EXPENSE_TAX_TREATMENT | Four separate accounting/fiscal decisions, independent of vendor import codes. |

The old string fields remain compatibility/display carriers. New decisions require typed payloads. An account value is evaluated against the approved client chart policy; the synthetic `628.TEST` example is not production accounting approval.

## Source facts and absence

Invoices retain immutable `source_facts`, and each line retains its own immutable source facts. The archived SPV ZIP and SHA-256 remain the original evidence. Parser version, source document ID/hash and UBL line path identify the interpretation. Missing `Rate` is nil/omitted JSON, whereas explicit zero is the exact decimal string `"0"`. A legacy numeric carrier may still contain zero for compatibility; new classification/readiness never uses that carrier to infer source presence.

Line facts retain source ID/path, category, scheme, exemption code/reason, seller/standard item identifiers, price base, allowances/charges and monetary facts including currency. VAT origin is DECLARED, CALCULATED or UNKNOWN. When a line has no declared VAT amount, the existing exact decimal calculation supplies the carrier with CALCULATED lineage; when both VAT and rate are absent, lineage is UNKNOWN and the source VAT amount is absent. Header totals remain declared evidence; line calculation cannot turn them into source declarations.

Header facts retain all VAT totals and subtotals with category/rate, taxable base, VAT and currency; legal totals remain distinct: line extension, tax exclusive, tax inclusive, payable, prepaid and rounding. Type code, tax currency, tax point, supply period, period description code, preceding invoice reference, fiscal/legal supplier and buyer IDs, countries and document adjustments are separate fields. Incomplete or unsupported facts remain review blockers. Tax IDs are not merged with legal registration IDs.

The bounded reconciliation uses exact rational arithmetic. Ordinary RON invoices require valid domestic identities, ordinary positive-rate category S / VAT scheme, matching source/carrier values, net + VAT = line total, matching invoice and monetary totals, matching category bases/VAT and matching payable. Declared line VAT must match the source base/rate at the supported two/four decimal precision. Category rounding disagreement causes review; no VAT redistribution, hidden rounding tolerance or synthesized header adjustment is applied. Document/line allowances and charges and price bases other than one are retained but blocked by the initial mapping.

A source cash-accounting absence is UNKNOWN, never NO. Initial automation additionally requires the reviewed rule's explicit supplier `SupplierVATRegistration=ORDINARY_REGISTERED` and `SupplierCashAccounting=NO` acquisition evidence; a source YES blocks it. Buyer cash-accounting/pro-rata flags are separately approved profile facts. Absence of a source cash indication is not evidence of ordinary eligibility.

Database UPDATE triggers protect invoice/line source JSON, invoice model version, and an already saved accounting snapshot. There is no implicit historical reparse endpoint. A future versioned reparse/reclassification must retain both interpretations and artifact history.

## Client accounting and tax profile

An immutable approved profile records ID, client, version, inclusive civil effective period, accounting framework, chart policy and explicitly approved `accountCodes` vocabulary, tax regime, VAT registration, deduction activity, cash-accounting and pro-rata flags, approval actor/time/evidence and TEST_ONLY isolation. UNKNOWN is an explicit supported enum. OTHER/UNKNOWN and unsupported regimes cannot acquire ordinary automation through defaults. A blank chart policy or missing approved account vocabulary blocks it. Only exact listed account/analytic codes can become ready; human review cannot add an unapproved analytic.

The initial ordinary profile requires explicitly adopted OMFP_1802_2014 framework, a nonempty approved chart policy and exact approved account/analytic vocabulary, PROFIT_TAX, ORDINARY_REGISTERED, WITH_DEDUCTION_RIGHT, CashAccounting=NO and ProRata=NO. These are reviewed applicability inputs, not conclusions drawn from an invoice or supplier name. MICROENTERPRISE receives a manual NOT_APPLICABLE expense-tax proposal with a reason; it does not use profit-tax deductibility automatically.

Profile and pack selection share one repeatable-read database view. Exactly one approved dated profile and one coherent pack must apply; overlapping versions are ambiguous and do not select the most recent. Profiles/releases are append-only with fresh IDs/versions and explicit effective periods. Existing open-ended periods cannot silently be shortened. Supply/tax-point timing that differs from the approved issue-date basis causes review.

## Typed decisions

| Dimension | Values and required fields |
| --- | --- |
| ACCOUNT | ACCOUNT with a validated account/analytic code. |
| VAT_TREATMENT | ORDINARY or SPECIAL_UNSUPPORTED, with source category/rate and IMMEDIATE, DEFERRED or UNSUPPORTED timing; special treatment needs a reason. |
| VAT_DEDUCTIBILITY | FULL, NONE, LIMITED, NOT_APPLICABLE. LIMITED needs exact 0 < percentage < 100 plus basis; NONE/NOT_APPLICABLE need a reason. |
| EXPENSE_TAX_TREATMENT | FULLY_DEDUCTIBLE, NONDEDUCTIBLE, LIMITED, PERIOD_LIMIT_CATEGORY, NOT_APPLICABLE. Limited needs percentage/basis; period category needs category/basis; non-deductible/inapplicable needs reason. |

Cross-dimension fields, arbitrary kinds and adapter codes are rejected. JSON decoding rejects unknown fields and trailing values. Every new-model human review needs a reason. PERIOD_LIMIT_CATEGORY records the category and evidence; it does not calculate a period ceiling.

Evidence records model, profile/version, client chart policy, pack/version, immutable actual RuleVersion ID, source document/hash/parser/path, issue-date basis and date. The saved snapshot also records only the authoritative associated Contract ID/reference/revision. Contract extraction proposals and Gemini output are never classification evidence until the existing Contract confirmation boundary creates an authoritative Contract.

## Readiness and SAGA

A new-model invoice becomes READY_FOR_SAGA only after four valid final decisions per line, coherent profile/pack/source provenance, exact reconciliation, valid identities/quantities and a tested/approved mapping. A source rate is not a deductible percentage. VAT deductibility and expense-tax treatment remain independent.

The initial mapping supports ORDINARY / IMMEDIATE plus FULL VAT and FULLY_DEDUCTIBLE expense, with an explicitly approved `OrdinaryFullOmission` policy: SAGA `TipDeducere` is omitted. All other combinations, including FULL VAT / NONDEDUCTIBLE expense, stay AWAITING_REVIEW with one existing CLASSIFICATION task and a clear invoice readiness reason. Human confirmation cannot approve a mapping. A mapping-only task includes final automatic decisions so an accountant can revisit them; zero pending items does not mean the task is resolved.

Only the SAGA package owns `TipDeducere`. N50, I, N and SAGA_DEFAULT are never fiscal domain values. Legacy generation keeps its historical validation, exporter version, artifact bytes and import-instruction meaning. New generation uses `SAGA_C_DOMAIN_V2_V1/<mapping version>` and stores typed decisions/evidence in the artifact snapshot. Generated XML still needs the frozen manual SAGA acknowledgement boundary; generation/download does not imply EXPORTED.

## Persistence and compatibility

Migration 000014 is additive: new JSON/model/readiness columns, immutable profile/release tables, expanded categories, and uniqueness by line/dimension/model. Existing rows default to LEGACY_V1; no value, artifact or old migration is rewritten. Ent application immutability is supplemented with database triggers for source/snapshots/releases; actual RuleVersion FKs remain execution references. Test-only profile/release deletion is allowed solely for isolated fixture cleanup; production deletion/update is blocked.

Historical reclassification is deferred. The uniqueness constraint permits separate future model histories, but no conversion job or automatic reprocessing was added. New SPV parses select V2; legacy caller inputs retain V1 unless they explicitly opt into V2.

See [CLASSIFICATION_ENGINE_V1.md](CLASSIFICATION_ENGINE_V1.md) for release tooling and [CLASSIFICATION_V1_TEST_HANDOFF.md](CLASSIFICATION_V1_TEST_HANDOFF.md) for A–W commands and outstanding acceptance.
