# Romanian incoming supplier invoice rules: source discovery

Verification/retrieval date: **2026-09-15**. This is a source snapshot and an implementation scope assessment, not a guarantee of legal compliance.

### Scope

ACCOUNT, VAT and DEDUCTIBILITY only. Incoming Invoice documents; no CreditNote/storno, AI classification, new tasks or pipeline states. Research preceded any production seeding. **No production mapping was accepted for seeding.** The delivery is a production eligibility/effective-date architecture with an explicit empty, review-only pack.

We distinguish **VERIFIED** source facts, **NOT VERIFIED** applicability, **OUT OF SCOPE** treatments and **PRODUCT POLICY REQUIRED** choices. A source-rate confirmation is distinct from an assessment of the supplier's legally correct tax treatment.

### Authoritative Sources

| ID | Title / issuer | Provision | Relevant effective period | Verified fact / implementation supported |
| --- | --- | --- | --- | --- |
| S1 | [OMFP 1802/2014, Ministry of Public Finance](https://static.anaf.ro/static/10/Anaf/legislatie/OMFP_1802_2014.pdf) | Art. 2, 5, 10; annex point 3; chapter 14 point 593; account 628 function, PDF p.223 | Original regulation starts 2015-01-01, subject to its fiscal-year provision; current client applicability and subsequent consolidated amendments NOT VERIFIED | VERIFIED: regulation has a defined entity scope, entity accounting policies and analytical accounts; 628 is in its vocabulary. Supports a deliberately partial versioned vocabulary, never a universal description-to-account mapping. |
| S2 | [ANAF, Principalele modificări ale cotelor de TVA reglementate prin Legea 141/2025](https://static.anaf.ro/static/10/Anaf/AsistentaContribuabili_r/Cotele_de_TVA_09.2025.pdf) | Codul fiscal art. 291(1)-(2), PDF p.2 | Change starts 2025-08-01 | VERIFIED: new standard 21 and reduced 11 rates at this change. Supports research and software boundary fixtures; does not prove the correct rate for an arbitrary invoice. |
| S3 | [ANAF Brașov, Modificări ale cotelor de TVA](https://static.anaf.ro/static/10/Brasov/Brasov/cote_TVA.pdf) | Art. 291; Legea 141/2025 art. III, PDF p.3 | 2025-08-01 change; stated conditional housing transition 2025-08-01 through 2026-07-31 | VERIFIED: reduced-rate categories and conditional transitions exist. Supports refusing broad rate/category inference. No housing treatment implemented. |
| S4 | [ANAF Brașov, Cheltuieli cu deductibilitate limitată, updated 2025-04-09](https://static.anaf.ro/static/10/Brasov/Brasov/limite_2025.pdf) | Art. 25(3)(l); VAT art. 298 and norms point 68, PDF pp.9-11 | Informative source snapshot at 2025-04-09; complete historical/current applicability NOT VERIFIED | VERIFIED: expense and VAT deduction are distinct provisions and vehicle treatment requires contextual evidence. Supports review-only behavior, no percentage mapping. |

These are official issuer publications. S2-S4 are explanatory material; they are not substituted for a full effective-dated legislative consolidation. Portal Legislativ law links were discovered: [Legea 141/2025](https://legislatie.just.ro/Public/DetaliiDocument/300022) and [historical Codul fiscal](https://legislatie.just.ro/Public/DetaliiDocument/186620). Full law content retrieval was unsuccessful/insufficient; ANAF consolidated HTML retrieval exceeded the browser limit or failed. Therefore full current-to-2026 consolidation and historical standard/reduced-rate histories are **NOT VERIFIED** in this module. Do not turn these retrieval limitations into guessed rules.

No large legislative text is reproduced. URLs are static provenance metadata; runtime evaluation performs no HTTP requests.

### DEDUCTIBILITY Semantic Audit

**VERIFIED FROM CODE:** `backend/internal/saga/generator.go` maps DEDUCTIBILITY values `SAGA_DEFAULT` to omission of `TipDeducere`, and `N50`/`I` to their exact SAGA serialization codes. The classification model has one undifferentiated string value. Neither invoice nor client schema defines a separate VAT deduction entitlement, expense tax deductibility, client tax regime, usage determination or evidence for legal exceptions.

**IMPLEMENTATION:** DEDUCTIBILITY is currently a **SAGA import instruction**. It does not independently establish either VAT deductibility or corporate-income-tax expense deductibility. The code does not calculate either entitlement; descriptions/UI that imply a unified fiscal conclusion would overstate its model. Existing manual correction remains available and original proposals remain recorded.

**PRODUCT POLICY REQUIRED:** `PRODUCT DECISION REQUIRED — DEDUCTIBILITY SEMANTICS`. The accounting/product owner must define what each supported import code means for Diana's target clients and whether a single dimension is sufficient to represent intended decisions. No fourth dimension is introduced here. All automatic deductions are stopped, even with structurally populated provenance. Passing a SAGA serialization test is not legal evidence.

### ACCOUNT Rules Investigated

**VERIFIED:** account 628 exists in S1 and has a function for other third-party services. S1 also requires entity policies and permits analytical accounts.

**NOT VERIFIED:** Diana stores client name/CUI, not a reviewed accounting-regulation adoption record or approved account-selection policy. A generic service description does not prove recognition as a current expense, distinguish a more specific account, capitalization/prepayment, or resolve a client's analytics.

**PRODUCT POLICY REQUIRED:** account choice and precise deterministic description predicates must be reviewed for each client. No GLOBAL ACCOUNT mapping or ALWAYS ACCOUNT mapping is seeded. `OMFP_1802_2014_628_V1` is a partial vocabulary for future explicitly reviewed CLIENT_OVERRIDE configurations, not Diana's universal chart of accounts. Manual legitimate codes/subaccounts retain the existing SAGA structural boundary.

### VAT Rules Investigated

**VERIFIED:** Diana persists exact line VAT rate and amount. e-Factura parsing reads a line's tax percent; the invoice model does not retain tax-category identity, exemption reason/code, tax point, or header tax-category breakdown as classification fields. Source rate is supplier declaration, not Diana's conclusion of fiscal correctness.

**IMPLEMENTATION:** `VAT_SOURCE_RATE_EQUALS` compares persisted source rate and configured positive rate numerically using existing exact decimal values. Condition spelling versions the narrow predicate; `match_value` and immutable RuleVersion carry its parameter. Its output is only a confirmation of the source percent. Description/ALWAYS VAT mappings and all generic zero-rate automation are ineligible. Rate/result disagreement cannot automatically reach SAGA.

**NOT VERIFIED:** complete historical rate applicability, advance/supply/tax-point context and a reviewed policy allowing source confirmation to satisfy Diana's VAT dimension for these clients. **No 19/21/11 or other VAT rule is seeded.** 21/11 are researched source facts; 19 normalization and historical engine fixtures are engineering assumptions. No code switch defines Romanian rates.

### DEDUCTIBILITY Rules Investigated

| Proposed treatment | Required facts | Diana facts | Decision |
| --- | --- | --- | --- |
| Vehicle-related N50 | Approved meaning of SAGA code; VAT versus expense tax question; client regime; vehicle specifications; use; exceptions/evidence | Description and money only; no reviewed usage/regime/evidence | Reject automatic `car → N50`; CLASSIFICATION review |
| I | Approved product semantics and context establishing the corresponding accounting import instruction | No explicit reviewed semantic/context model | Review only |
| SAGA_DEFAULT | Explicit accountant/client decision that default SAGA import behavior is appropriate | No universal client policy | No ALWAYS fallback; review only |

S4 demonstrates why vehicle description alone is insufficient. We do not assert current percentage treatment for any invoice based on this informative snapshot.

### Rules Accepted for Automation

**Production-seeded inventory: empty.** This is a deliberate result of the source/data/policy gates, not a missing fallback.

| RULE ID | DIMENSION | TRIGGER | OUTPUT | LEGAL/ACCOUNTING BASIS | EFFECTIVE FROM | EFFECTIVE TO | PRODUCTION ELIGIBLE | AUTOMATIC/REVIEW |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| No accepted production rule | — | — | — | Source and policy gates unresolved | — | — | No | Review |

Only engineering fixtures configure reviewed positive-source-rate confirmation and explicitly adopted client account-628 policy. These are clearly TEST_ONLY and do not enter `ProductionPack()` or migrations. No automatic deductibility fixture is claimed.

### Rules Rejected for Automation

| Candidate | Status | Reason |
| --- | --- | --- |
| Generic services → 628 | PRODUCT POLICY REQUIRED | Source establishes vocabulary, not a deterministic universal recognition/account policy |
| ALWAYS ACCOUNT / ALWAYS SAGA_DEFAULT | Rejected | No universally safe classification justified |
| Description → VAT rate | Rejected | Structured source percent exists; category/context does not |
| 19/21/11 source-rate production seed | NOT VERIFIED / PRODUCT POLICY REQUIRED | Source confirmation policy and full intended period not approved |
| 0 → generic VAT treatment | VAT SOURCE DATA GAP | Legally distinct tax categories/exemptions absent |
| Vehicle → N50 | PRODUCT POLICY REQUIRED / data gap | VAT versus expense distinction and required business facts missing |
| Reverse charge, intra-community, imports, exemptions | OUT OF SCOPE / VAT SOURCE DATA GAP | Representation and structured evidence insufficient |
| CreditNote/storno | OUT OF SCOPE | Existing SAGA treatment deferred |

For any unsubstantiated proposed mapping, the result remains **ACCOUNTING RULE SOURCE REQUIRED**.

### Missing Source Data

| Desired classification | Required fact | Present in Diana? | Source | Consequence |
| --- | --- | --- | --- | --- |
| Source VAT percent confirmation | Line percent and VAT amount | Yes, exact decimals | InvoiceLine / e-Factura parser | Narrow predicate supported; no seeded treatment rule |
| VAT category / reverse charge | UBL tax category and supporting context | No persisted classification field | e-Factura tax category; S2/S3 context | VAT SOURCE DATA GAP; review |
| Exemption treatment | Exemption reason/code, applicable provision | No persisted field | UBL exemption evidence | VAT SOURCE DATA GAP; review |
| Rate law applicability | Relevant supply/tax-point/advance context | Issue date only | Effective-dated fiscal source needed | No blanket historical rate validation |
| ACCOUNT mapping | Adopted regulation and client recognition/account policy | No client adoption model; future version provenance can explicitly cite reviewed client policy | S1 art.2/5 and point593 | No GLOBAL seed; reviewed client policy required |
| VAT deduction entitlement | Client VAT regime, purpose, use and exceptions | No | S4 / underlying art.298,297,299 | Review only |
| Expense deductibility | Tax regime, business purpose and statutory context | No | S4 / underlying art.25 | Review only |
| SAGA TipDeducere selection | Approved code semantics and accountant instruction | Manual decision available; no universal automatic policy | Existing verified SAGA format docs | Manual correction; semantics decision required |

### Client Policy Dependencies

Regulatory vocabulary is distinct from a client-account policy. Future reviewed ACCOUNT versions require CLIENT_OVERRIDE, a cited policy reference and explicit `OMFP_1802_2014` adoption in immutable provenance. This limited binding is not a general client policy platform or evidence of universal law. The existing public creation API cannot verify or enable executable production rules.

### Historical / Effective-Date Considerations

Evaluation uses `invoice.issueDate` calendar components and inclusive domain dates; it never uses today's date to choose a version. Rule and source periods both apply. No newest-row selection. Future/expired overrides are removed before approved GLOBAL-parent replacement. Multiple effective versions of a logical rule cause review, even if one trigger alone matches. No bulk reclassification or retrospective historical rewrite is implemented.

### Open Accounting Decisions

1. Approve DEDUCTIBILITY semantics and separation of VAT versus expense questions.
2. Confirm target-client accounting regime and deterministic account policies.
3. Decide whether source-percent confirmation alone may satisfy the existing VAT dimension, and define fiscal periods/evidence necessary for a seeded version.
4. Review full applicable official legislative versions before any fiscal mapping is seeded.

Engineering readiness and accounting acceptance are separate gates. **This module is NOT FROZEN.**

### armqu Discovery

| ARMQU RULE/CONCEPT | SOURCE | LEGAL BASIS PRESENT? | DIANA APPLICABILITY | DECISION |
| --- | --- | --- | --- | --- |
| Ordered contract-line recognition classification | `armqu-be/internal/revenuerecognition/classification.go`, identification and policy code | No Romanian incoming supplier ACCOUNT/VAT/DEDUCTIBILITY authority in discovered rules | Separate revenue-recognition domain; first-match order conflicts with Diana ambiguity handling | Do not port mappings or precedence |
| Policy rules as stored data / validation | Same package | Implementation concept only | Useful architectural pattern | Retain Diana immutable RuleVersion and deterministic validation |

armqu was read only; no armqu files were changed.
