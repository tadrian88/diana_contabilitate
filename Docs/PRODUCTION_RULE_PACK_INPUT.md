# Production rule pack V1 — accountant input

ACCOUNTING INPUT REQUIRED. This form authorizes nothing until the accountant explicitly approves the completed scope and every decision. Account approval alone does not approve VAT or expense treatment.

## Fill once for the pilot client

- Client legal name, CUI and existing Diana client ID:
- Approved profile/version and effective start/end:
- Accounting framework and adopted chart/accounting policy reference/version:
- Exact approved account/subaccount/analytic codes:
- Tax regime for that period:
- VAT registration for that period:
- Activity deduction rights / mixed or excluded activities:
- Client TVA la încasare applicability:
- Pro-rata applicability:
- Approved by, approval date and retained evidence/reference:

Unknown values stay UNKNOWN. They do not become NO, FULL or ordinary by default.

## Repeat for each family

- Family name / stable family reference:
- Client / period:
- Supplier(s), exact tax identity, approved ordinary VAT and TVA la încasare facts/evidence:
- What purchases does this cover? Business purpose and recognition policy:
- How can Diana identify it reliably? Exact seller item code, confirmed contract and/or approved literal description:
- Why each identifying condition matters (accounting relevance or safety guard):
- Supported invoice type, currency, source VAT category/rate and timing:
- ACCOUNT — account and analytic/subaccount, with explicit client policy:
- VAT TREATMENT — treatment and timing, with source category/rate and basis:
- VAT DEDUCTION — FULL / NONE / LIMITED percentage / NOT_APPLICABLE, with entitlement basis and required client-use facts:
- EXPENSE TAX TREATMENT — FULLY_DEDUCTIBLE / NONDEDUCTIBLE / LIMITED percentage / PERIOD_LIMIT_CATEGORY / NOT_APPLICABLE, with tax-regime basis and required facts:
- Exceptions and purchases that must NOT match:
- Supporting dated client/acquisition policies:
- Legal/accounting basis and applicability period where relevant:
- At least two positive real e-Factura examples if available (one permitted initially), sanitized XML and original source reference:
- Negative boundary examples, expected unresolved decisions and reasons:
- Multiline and mixed supported/unsupported example if available:
- Target SAGA company/product/version, proposed mapping and expected result:
- Approved by / date / evidence covering scope and all four decisions:

## SAGA evidence (separate explicit approval)

Supply the generated XML hash, corresponding sanitized source hash, isolated target-company import record, SAGA product/version/company configuration and accountant verification of resulting account/analytic, VAT treatment, deduction and expense result. Explicitly verify omitted TipDeducere for the supported full/full mapping. XML shape or synthetic test success is insufficient. Missing evidence means AWAITING_REAL_SAGA_VALIDATION, not production acceptance.

## Machine-readable contract

The JSON format is `PRODUCTION_RULE_PACK_INPUT_V1`. Start with [blank JSON](production-rule-pack-input.v1.template.json). The authoritative offline shape is `backend/internal/accountingacceptance/contract.go`: Input, Family, Decision, Example, ExpectedLine. It reuses accounting.Profile, Predicate, Value, Approval, MappingPolicy and civil Date; there is no second executable domain/rule-pack model. `MissingInput()` is a sorted discovery checklist, not a production eligibility validator or proof of approval authenticity. No runtime engine imports this package or golden outcomes.

| Object | Required contents |
| --- | --- |
| profile | Existing domain profile id/clientId/version/effectiveFrom/effectiveTo/framework/accountCodes/chartPolicy/taxRegime/vatRegistration/deductionActivity/cashAccounting/proRata/approval; testOnly false |
| mapping | Existing mapping version/approved/testOnly/ordinaryFullOmission/approval, with target-SAGA evidence |
| families[] | familyId/clientId/name/effective period/scope/predicate/predicateReasons/requiredFacts/clientPolicy/acquisitionPolicy/decisions/exceptions/approval; exact ruleVersionIds after operator preparation |
| decisions | All four dimension keys, each with native value, policyReference, basis, requiredClientFacts |
| examples[] | Source fixture/hash/original reference, client/profile version, each line's four expected native values (null = unresolved), readiness, mapping compatibility/version, kind/boundary reason and approval |

Framework enums: OMFP_1802_2014 / OTHER / UNKNOWN. Tax regime: PROFIT_TAX / MICROENTERPRISE / OTHER / UNKNOWN. VAT registration: ORDINARY_REGISTERED / NOT_REGISTERED / SPECIAL_REGISTERED / UNKNOWN. Deduction activity: WITH_DEDUCTION_RIGHT / MIXED / WITHOUT_DEDUCTION_RIGHT / UNKNOWN. Cash accounting and pro-rata: YES / NO / UNKNOWN. These record facts, not suggested answers.

Native ACCOUNT value uses `account` for the complete approved code, including analytic suffix; do not invent a separate analytic field. Native VAT_TREATMENT supports ORDINARY or SPECIAL_UNSUPPORTED with timing/sourceCategory/sourceRate. Percentage is an exact decimal string only for LIMITED, with basis. NOT_APPLICABLE/NONE/NONDEDUCTIBLE/SPECIAL_UNSUPPORTED require reason. The current automatable adapter supports only ORDINARY/IMMEDIATE, FULL VAT and FULLY_DEDUCTIBLE expense on the approved ordinary PROFIT_TAX profile. MICROENTERPRISE can record NOT_APPLICABLE for expense but remains review-only in this adapter.

## Required authoritative facts for the implemented predicate

Exact client/profile and dated chart/account whitelist; exact source supplier/buyer VAT identity matching the client; document Invoice/type 380, RON, domestic party countries; source category S, explicit positive rate/scheme VAT and no exemption; complete source amounts, tax breakdown/totals and line quantitative reconciliation; no credit reference, unsupported adjustment/base quantity or special tax-point timing; approved supplier ordinary-registration/cash-NO acquisition policy when XML leaves cash UNKNOWN; confirmed contract identity/reference if used; exact seller item and/or approved literal description if used. Each family must identify its additional business-use, tax-entitlement and expense-policy facts. Missing required facts means review.

Only the actual allowed V1 fields can become predicates. Unsupported standard-item IDs, allocations, unconfirmed extraction proposals or period-ceiling facts are not silently approximated by descriptions.

## Evidence and privacy

Sanitize names, identifiers and commercial data consistently; retain all accounting-relevant structure, categories/rates/totals and relationships. The accountant approves the exact sanitized XML bytes (SHA-256), expected decisions and negative cases. Keep original source lineage in access-controlled references. Do not confuse sanitized XML hash with the ingestion source ZIP hash: both can exist and are different byte sequences. Do not copy raw invoices into audit logs or public reports. No TEST_ONLY, demo, seed or Codex-created example is approval evidence.
