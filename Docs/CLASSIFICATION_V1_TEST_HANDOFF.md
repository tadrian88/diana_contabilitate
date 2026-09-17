# Classification V1 — user-run test handoff

Status: implementation and tests prepared; runtime acceptance NOT RUN by Codex. Domain V2 awaits user tests. V1 real production pack EMPTY / NOT APPROVED, real acceptance NOT COMPLETED, NOT FROZEN. V2 NOT STARTED.

Run groups in a disposable local test environment. U/V are real database actions for the user. Integration tests need migrations through 000014 before K–R. Existing legacy suites are regression evidence, not proof of V2 accounting approval. Live ANAF/Gemini/desktop SAGA acceptance is a separate accounting/provider gate.

Shared setup (user-run; PostgreSQL/Redis must be ready before tests):

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate
export GOCACHE=/private/tmp/diana-go-cache
export TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable'
export TEST_REDIS_URL='redis://127.0.0.1:6382/15'
docker compose up -d postgres redis
```

Redis database 15 must be isolated; runtime tests can clear their isolated queue state. Seed-backed Playwright uses the existing demo startup script. V2 fixture is `inv-accounting-v2-test-only`, four visible typed decisions plus an intentionally unapproved mapping. Production has no test-only opt-in.

## A. Static / compile

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
GOCACHE=/private/tmp/diana-go-cache go generate ./ent
atlas migrate hash --dir file://migrations
GOCACHE=/private/tmp/diana-go-cache go test -exec=/usr/bin/true ./...
GOCACHE=/private/tmp/diana-go-cache go test -tags=integration -exec=/usr/bin/true ./internal/platform/postgres
cd /Users/adriantudoran/Projects/diana_contabilitate
npm run typecheck
git diff --check
```

The `-exec=/usr/bin/true` commands compile/link binaries but never execute test bodies. Their Go `ok` output is compile success only.

## B. Domain source facts

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
go test -count=1 ./internal/accounting -run 'TestDomainSource'
```

## C. Parser / UBL

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
go test -count=1 ./internal/spv -run 'TestAccountingUBL|TestUBLParser'
```

## D. Client profiles

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
go test -count=1 ./internal/accounting -run TestDomainClientProfile
go test -count=1 ./internal/classification -run 'TestDomainMissingAndUnsupportedContext|TestDomainMicro'
```

## E. Four decisions / typed values

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
go test -count=1 ./internal/accounting -run TestDomainTypedValues
go test -count=1 ./internal/classification -run 'TestDomainFour|TestDomainAllSpecial'
```

## F. Effective dates

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
go test -count=1 ./internal/accountingdate ./internal/classification -run 'Test.*Date|TestDomainEffective|TestDomainRuleDate'
```

## G. Rule pack / release

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
go test -count=1 ./internal/accounting ./internal/rules
go test -count=1 -tags=integration ./internal/platform/postgres -run 'TestDomainRelease'
```

No real accountant approval is inferred from synthetic engineering approval fields. Operator inputs for profile/rule/pack must be supplied after review; see CLASSIFICATION_ENGINE_V1.md.

## H. Legacy compatibility

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
go test -count=1 ./internal/classification ./internal/rules ./internal/saga
go test -count=1 -tags=integration ./internal/platform/postgres -run 'TestProduction|TestClassification|TestRealSAGAArtifact|TestSaga'
```

## I. Shared readiness

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
go test -count=1 ./internal/saga -run 'TestDomain.*Readiness|TestDomainMappingDisabled'
go test -count=1 -tags=integration ./internal/platform/postgres -run TestDomainHumanTyped
```

## J. SAGA mapping

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
go test -count=1 ./internal/saga -run 'TestDomain|TestGenerate'
```

Validate XML with the accountant's actual desktop SAGA version separately. Supported V2 mapping is only approved ordinary immediate / FULL VAT / FULLY_DEDUCTIBLE expense → TipDeducere omission. All unequal/limited/special/inapplicable/period-ceiling combinations remain blocked by the initial mapping.

## K. PostgreSQL integration

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
go test -count=1 -tags=integration ./internal/platform/postgres
```

## L. Concurrency / race

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
go test -race -count=1 ./internal/accounting ./internal/classification ./internal/saga ./internal/spv
go test -race -count=1 -tags=integration ./internal/platform/postgres -run 'TestDomain|Concurrent|Concurrency'
```

## M. Full deterministic pipeline

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
go test -count=1 -tags=integration ./internal/platform/postgres -run TestDomainFullDeterministicSPVContractClassificationSAGAPipeline -v
```

Fake ANAF source download → raw ZIP/hash persistence → native pipeline → authoritative Contract → four deterministic decisions → readiness → persisted XML. Explicit TEST_ONLY engine/exporter constructors are used; production export rejection is also asserted. Artifact generation remains EXPORTING, preserving human SAGA acknowledgement.

## N. Modules 1–7 regression

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
go test -count=1 ./...
go test -count=1 -tags=integration ./internal/platform/postgres
cd /Users/adriantudoran/Projects/diana_contabilitate
npm run test:e2e:backend1
npm run test:e2e:backend3
npm run test:e2e:backend4
npm run test:e2e:backend5
npm run test:e2e:backend6
npm run test:e2e:backend7
```

## O. ANAF / SPV regression

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
go test -count=1 ./internal/spv
go test -count=1 -tags=integration ./internal/platform/postgres -run 'SPV|ANAF'
cd /Users/adriantudoran/Projects/diana_contabilitate
npm run test:e2e:spv-connection
```

## P. Contract ingestion regression

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
go test -count=1 ./internal/contractingestion/...
go test -count=1 -tags=integration ./internal/platform/postgres -run TestContractIngestion
cd /Users/adriantudoran/Projects/diana_contabilitate
npm run test:e2e:contract-ingestion
```

## Q. Missing Contract Resume regression

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
go test -count=1 ./internal/contracts ./internal/workerruntime
go test -count=1 -tags=integration ./internal/platform/postgres -run 'MissingContract|ContractAvailable|AvailableContract|BlockedInvoices|ContractsUseAuthoritative'
```

## R. SAGA regression

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
go test -count=1 ./internal/saga
go test -count=1 -tags=integration ./internal/platform/postgres -run 'SAGA|Saga'
cd /Users/adriantudoran/Projects/diana_contabilitate
npm run test:e2e:saga-export-ux
```

## S. Frontend components

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate
npm run test -- src/features/invoices/AccountingDomainV2.test.tsx src/features/invoices/InvoiceFlows.test.tsx src/features/invoices/InvoiceWorkspace.test.tsx src/features/rules/RulesWorkspace.test.tsx
npm run test
```

## T. Playwright

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate
npx playwright test --config=playwright.accounting-v2.config.ts
npm run test:e2e
```

The new V2 browser test checks four dimensions, TEST_ONLY/readiness explanation and exact typed limited correction/reload while mapping stays blocked. Existing Module 5 assertions only adopt the authorized historical labels.

## U. Incremental migration

Use the existing local development database with migrations 000001–000013 already applied. Record status, apply only the additive next migration, inspect status and run persistence/legacy checks. No migration was applied by Codex.

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
export DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable'
atlas migrate status --env local
atlas migrate apply --env local
atlas migrate status --env local
TEST_DATABASE_URL="$DATABASE_URL" go test -count=1 -tags=integration ./internal/platform/postgres -run 'TestDomain|TestRealSAGAArtifact|TestProduction'
```

Record counts/model versions and stored XML/hash before/after application: existing invoices/classifications/rule versions must be LEGACY_V1; existing artifact bytes/hashes/exporter versions remain unchanged. Additive migration performs no artifact/data reinterpretation.

## V. Empty database migration

Create a new disposable database name. The following create command intentionally fails if that named database already exists; use a fresh name rather than deleting a database.

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate
docker compose exec -T postgres createdb -U diana diana_accounting_v2_empty_validation
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
export DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_accounting_v2_empty_validation?sslmode=disable'
atlas migrate apply --env local
atlas migrate status --env local
TEST_DATABASE_URL="$DATABASE_URL" go test -count=1 -tags=integration ./internal/platform/postgres
```

## W. Production build

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate
npm run build
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
go build ./cmd/api ./cmd/worker ./cmd/accountingrelease
```

## Accounting input still required

Provide approved client frameworks/analytical chart policies including exact approved accountCodes and effective fiscal profiles; explicit acquisition/business-purpose/right-of-deduction and supplier cash/registration evidence; official legal provenance with applicable dates; reviewed exact predicates and expected four decisions for acceptance invoices; approved mapping/version and actual desktop SAGA import results for every supported combination, especially independent VAT versus expense deductions. No production account/tax outcome was invented.

Engineering gate: NO until user runtime/migration/build acceptance. Real accounting gate: NO until reviewed production pack plus actual acceptance. Ready for user-run tests: YES. V1 complete: NO — ENGINE IMPLEMENTED, REAL APPROVED PRODUCTION PACK / ACCEPTANCE REMAINS.
