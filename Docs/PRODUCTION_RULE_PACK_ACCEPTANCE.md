# First production pack V1 — acceptance handoff

Real acceptance PENDING. No real dataset or released production pack exists.

## Golden dataset

[Golden JSON structural schema](production-rule-pack-golden.v1.schema.json), `accountingacceptance.Example` and `ExpectedLine` define JSON acceptance evidence in the input manifest's `examples` array. Each entry has exampleId, kind POSITIVE/NEGATIVE_BOUNDARY, sourceFixture, fixtureSha256 (64 lowercase hexadecimal characters), originalSourceReference, clientId/profileId/profileVersion, lines, expectedReadiness READY_FOR_SAGA/AWAITING_REVIEW, expectedSagaMappingCompatibility, mappingVersion, optional boundaryReason, approval actor/at/evidence. Each line uses the XML sourceLineId, familyId (absent for unsupported lines), and all four native decision keys; null asserts unresolved. For negative boundaries, familyId identifies the family being challenged, not a successful match.

Expected values are tests only, never runtime lookup/learning data. Prefer two approved real positives per family, one multiline and one mixed invoice where possible. Include actual relevant wrong client/supplier/category/rate evidence/currency/type/profile/policy/rule period/contract/description/conflict boundaries. Do not manufacture real positives or claim that synthetic perturbations were accountant-reviewed real invoices. Approved sanitized negative fixtures or explicitly approved boundary modifications must be identified accurately.

No real acceptance runner/fixtures were invented. When inputs arrive, tests must parse the exact archived/sanitized e-Factura, retain source lineage and persist source facts through the existing ingestion flow, evaluate exact reviewed versions, compare all four outcomes and call shared readiness plus XML generation. At least one supported example must prove the existing SPV → parser → persistence → deterministic rules → readiness → SAGA XML path with no classification review. Real SPV/target-SAGA evidence remains separate from fake-ANAF engineering fixtures.

## Shadow evaluation

`internal/saga.EvaluateShadow(invoice, clientIdentity, candidateSnapshot, reviewedRuleCandidates)` is an offline Go entry point. It reuses DomainPolicy and EvaluateReadiness. Supply an accounting-domain-v2 invoice and candidate native profile/pack snapshot with actual approvals; no dummy approval is injected. RuleCandidates supply exact reviewed immutable provenance/eligibility for production readiness. Missing or duplicate references cannot establish eligibility. The result contains per-line/dimension proposals (typed results, source NO_MATCH/AMBIGUOUS/RULE, explanation/legal basis and exact rule/profile/source evidence) and shared readiness including mapping version/reason. Separate MappingVersion and MappingCompatibility fields retain the proposed mapping identity even when readiness is blocked; compatibility is COMPATIBLE only after shared validation, otherwise BLOCKED_OR_UNRESOLVED. Join exact RuleVersion IDs to manifest familyId to display matched families. An unapproved candidate stays unresolved; a reviewed but unmapped candidate can show proposals with blocked readiness.

The function deep-copies invoice, snapshot and supplied RuleCandidates. It has no store, observer, task resolver, pipeline transition or artifact generator. It does not resolve tasks, alter authoritative decisions/status, write audit events, emit metrics or generate XML. Existing non-classification tasks still block readiness. Shadow readiness assesses the candidate; it is not production release authorization. Only tests access the unexported synthetic capability.

Accountant review report for each example:

| Field | Fill from actual evidence |
| --- | --- |
| Example | Sanitized source reference/hash and client/profile period |
| Source facts | Supplier/item/contract, category/rate/currency/document/timing and relevant totals |
| Family | Exact matched family, or no match/conflict |
| Decisions | Account/analytic, VAT treatment, VAT deduction, expense-tax treatment separately |
| Basis | Approved client/acquisition policy and legal/accounting reference for each result |
| SAGA | Mapping/version/compatibility, readiness blocker if any |
| Expected | Accountant's four outcomes and readiness; identify any mismatch |
| Approval | Identity/date/reference, exact hashes and scope |

## Tests added in this milestone

- Input discovery: absent profile/mapping/family, TEST_ONLY exclusion, UNKNOWN profile facts and account-only approval cannot stand in for other decisions.
- Engine: offline draft (no loaded pack), missing approval, wrong supplier and ended pack fail closed.
- Shadow: deep-copy/no mutation and no returned-pointer aliasing; synthetic candidate rejected by public production function; missing approval; mixed supported/unsupported lines; conflicts.
- Audit: actual structured approval actor/evidence/client retained for native profile/pack/reviewed rule.

Existing V1 suites already contain typed value/source/parser boundaries, client/profile/cash/pro-rata/currency/CreditNote/contract/effective dates, overlapping versions, readiness/SAGA compatibility, persisted one-task routing, replay/concurrency/release-during-evaluation, snapshot/source/released-pack immutability and synthetic fake-SPV-to-XML proof. These are engineering fixtures, not accounting acceptance.

## Checks actually executed

- gofmt on modified Go files.
- Targeted Go compile/link only using `-exec=/usr/bin/true` for accountingacceptance/accounting/classification/saga/postgres/accountingrelease.
- PostgreSQL integration-tag compile/link only using `-exec=/usr/bin/true`.
- npm run typecheck.
- git diff --check and blank JSON syntax check.

No runtime test suite, race, PostgreSQL or Redis integration, frontend suite, Playwright, production build or real Atlas migration apply was run. No migration/schema change was needed, so go generate and atlas migrate hash were unnecessary. Compile-only `ok` output is not runtime PASS.

## Manual user commands

From repository root:

```sh
cd backend
GOCACHE=/private/tmp/diana-go-cache go test ./internal/accountingacceptance ./internal/accounting ./internal/classification ./internal/saga ./internal/platform/postgres
GOCACHE=/private/tmp/diana-go-cache go test ./...
GOCACHE=/private/tmp/diana-go-cache go vet ./...
GOCACHE=/private/tmp/diana-go-cache go test -race ./internal/classification ./internal/saga
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' GOCACHE=/private/tmp/diana-go-cache go test -tags=integration ./internal/platform/postgres
cd ..
npm run test
npm run test:e2e -- --config=playwright.accounting-v2.config.ts
npm run build
```

Use [existing V1 test handoff](CLASSIFICATION_V1_TEST_HANDOFF.md) for migration validation on incremental/empty isolated databases, Docker/Redis setup and Asynq/infrastructure/regression groups. Review database targets before any migration command. Do not run accountingrelease with the blank template. Operator activation is a later step only after exact accountant/mapping evidence is supplied.

## Gates

ENGINEERING: PENDING user runtime/migration/concurrency/frontend/regression results. ACCOUNTING: PENDING identified/approved pilot profile, nonempty released real pack, real approved fixtures, zero-touch proof, negative boundaries, mapping approval and isolated real-SAGA import validation. V1 status: ENGINE READY — REAL PACK MISSING (architecture implemented; engineering gate still pending). NOT APPROVED/FROZEN. V2 NOT STARTED.
