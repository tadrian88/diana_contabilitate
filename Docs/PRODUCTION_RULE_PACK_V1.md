# First real production accounting rule pack V1

Status: ACCOUNTING INPUT REQUIRED. Real pack NOT CREATED. V2 NOT STARTED.
Engineering gate PENDING user-run tests; accounting acceptance PENDING.

## Code prerequisite inspection (2026-09-15)

This inspection used implementation, not only earlier reports:

| Requirement | Implementation verified | Practical limit |
| --- | --- | --- |
| Source facts and lineage | `internal/spv/parser.go`, `spv/service.go`, `invoicing/invoice.go`, migration 000014 | Missing tax evidence is retained; absent ordinary supplier cash evidence needs approved acquisition policy, never inferred from XML absence |
| Four decisions | `internal/accounting/model.go`, `classification/domain_policy.go` | ACCOUNT, VAT_TREATMENT, VAT_DEDUCTIBILITY, EXPENSE_TAX_TREATMENT; strict tagged values |
| Dated client profile | `Profile.Valid`, `Profile.Ordinary`, PostgreSQL loader | UNKNOWN or overlapping profiles prevent ordinary automation |
| Typed RuleVersion | Ent schema/generated code, `InstallReviewedAccountingRule` | Exact immutable payload, provenance, policy and eligibility; public edits stay NO_AUTOMATION |
| Release architecture | `accounting_release.go`, `cmd/accountingrelease` | Private profile → rule → pack installation; only persisted releases are loaded |
| Dating | `accountingdate.Date.Within`, `Pack.Valid`, loader and matching | Inclusive issue-date basis; contained profile/pack/rule/source periods; overlap goes to review |
| Shared readiness | `saga.EvaluateReadiness`, `postgres/accounting_readiness.go`, completion/review/export | Reconciles source, four decisions, vocabulary, evidence and faithful mapping before READY_FOR_SAGA |
| SAGA derivation | `saga/readiness.go`, `domain_generator.go` | Initial mapping only ordinary immediate FULL VAT / FULLY_DEDUCTIBLE expense, with approved TipDeducere omission |
| Legacy handling | model-aware service, migration, legacy generator branch | Legacy ACCOUNT/VAT/DEDUCTIBILITY meanings/history remain separate |
| Production isolation | API/worker service dispatch, release checks, seed guards | TEST_ONLY constructor opt-in cannot activate a production release |
| Frontend review | ClassificationWorkspace, DomainCorrectionDialog, domain-decision-view, HTTP DTOs | Four typed decisions, source/profile display and human reason; no public promotion button |
| Migrations and tests | 000013/000014, domain/parser/readiness/release/concurrency/frontend suites | Code inspected and compile/type checks only; runtime/migration acceptance pending |

No material architectural prerequisite gap was found for the currently supported ordinary profit-tax/full-full subset. This is not an assertion that runtime tests pass. A microenterprise pilot, limited deduction, another fiscal framework, or another SAGA combination would require an explicit supported adapter/domain extension before that family could automate. No workaround is introduced here.

## Input discovered

Repository accounting research, prior implementation reports, SAGA format discovery and synthetic software fixtures exist. No accountant-approved production input package was found. The only XML golden fixture is a software exporter fixture; parser/pipeline XML is TEST_ONLY generated evidence. Devseed and contract-extraction fixtures are not real invoice/accounting approval. No client identifier, account, analytic, entitlement or pack ID is selected.

## Candidate families

None supplied. Target one identified client and preferably two or three narrow recurring families, with two approved sanitized real positives per family. One supplied family is acceptable; do not fabricate another. Scope/predicates/all four decisions/policy/evidence remain ACCOUNTING INPUT REQUIRED. Potential ordinary-domestic-RON families are suggestions for accountant selection, not approved policy.

## Existing release semantics

DRAFT means an offline unapproved input file; ACCOUNTING_APPROVED means externally reviewed evidence still outside executable release storage; AWAITING_REAL_SAGA_VALIDATION means mapping/import evidence is incomplete. These are review-report stages, **not new database/runtime enum states**. RELEASED means the existing controlled operator mechanism has persisted the reviewed immutable pack after checking actual RuleVersions and mapping approval. A draft file is not loaded by the production loader.

Approval actor/time/evidence must describe the exact family scope, four results, rules/pack version, dated profile/policy, sanitized fixture hashes and mapping. Merely writing a name or nonempty strings cannot supply real approval. The CLI validates configuration and records operator-supplied evidence; it does not authenticate professional credentials or verify external signatures. No production commands were executed.

Audit now retains structured native profile, reviewed-rule and pack configuration, including approval references, exact identities/version/period, mapping, rule count and profile version. Family count is not guessed from four-dimensional rules: family IDs/count and fixture approvals belong in the referenced input manifest. Restricted audit records contain no invoice XML; no new metrics or identity labels were added.

Released profiles/packs/RuleVersions cannot be updated; production pack/profile deletion is guarded by migration 000014. Versions require new IDs as well as increasing versions under current schema. Plan explicit finite, nonoverlapping approved periods before release. Existing effective-end periods are honored; an open-ended release cannot be shortened in place. There is no emergency revocation command in this architecture. Overlapping replacement packs safely force review on newly evaluated invoices, but do not constitute revocation. Immediate withdrawal of an open-ended release needs a separately designed append-only revocation mechanism; it is deferred, not claimed implemented. Historical snapshots remain tied to original evidence.

## Safety review

Supplier identity alone is inadequate. Approved item/contract/literal predicates must distinguish capitalization, prepayment, personal use and other treatment exceptions wherever relevant to the selected family. Every predicate needs ACCOUNTING RELEVANCE or SAFETY GUARD justification. No amount/date/invoice-ID overfitting. Missing source identity/category/rate/scheme/country, unknown required profile facts, unsupported document/currency/timing, conflicting rules, or unsupported mapping goes to review. Literal matching follows the engine's existing deterministic normalization, not fuzzy/semantic equivalence; inspect similar descriptions with different accounting meaning.

Mixed invoices retain supported proposals while unresolved lines keep one grouped CLASSIFICATION task; existing persistence implements this. Shadow evaluation does not create that task. Unsupported mapping can block readiness even when all proposals match.

## Deliverables and limits

See [input contract and accountant form](PRODUCTION_RULE_PACK_INPUT.md) and [golden/shadow/acceptance handoff](PRODUCTION_RULE_PACK_ACCEPTANCE.md). Added only offline input/evidence types, discovery checks, shadow evaluation, focused tests and audit evidence detail. No production pack/rules/profile/fixtures/database release, policy inference, new accounting UI, AI, learning or V2.
