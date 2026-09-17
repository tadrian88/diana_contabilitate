# User-run verification: Real Accounting Rules V1

Codex ran **no runtime tests**. Compile-only commands use `-exec=/usr/bin/true`, so Go never executes the test binaries. Engineering gate requires the user-run suites; accounting gate separately requires reviewed evidence and client policy. No production mapping was seeded. The full-pipeline fixture includes accountant review because automatic DEDUCTIBILITY is deliberately stopped.

Commands below assume the existing local compose services. Run migration group T before database integration tests. Dedicated Redis database 14 is for tests only. Existing Playwright setups own their API/worker lifecycles; stop unrelated local listeners or use the intended existing fixture servers.

### A. Fast/static checks

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
export GOCACHE=/private/tmp/diana-go-cache
gofmt -l internal/accountingdate internal/classification internal/rules internal/platform/postgres/production_rules_integration_test.go internal/saga/production_classification_test.go internal/platform/observability/classification.go
go generate ./ent
go test -exec=/usr/bin/true ./...
go test -exec=/usr/bin/true -tags=integration ./internal/platform/postgres
atlas migrate hash --dir file://migrations
cd /Users/adriantudoran/Projects/diana_contabilitate
npm run typecheck
git diff --check
```

### B. Rule engine unit tests

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
GOCACHE=/private/tmp/diana-go-cache go test -count=1 ./internal/classification ./internal/rules ./internal/accountingdate ./internal/platform/observability
```

### C. Effective-date tests

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
GOCACHE=/private/tmp/diana-go-cache go test -count=1 ./internal/classification -run 'TestProduction(EffectiveDate|HistoricalVersions|ClientOverrideTemporal)'
GOCACHE=/private/tmp/diana-go-cache go test -count=1 ./internal/accountingdate
```

### D. Production/demo isolation tests

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
GOCACHE=/private/tmp/diana-go-cache go test -count=1 ./internal/classification ./internal/rules -run 'TestProduction(DemoIsolation|DefaultService|PackExplicitly)'
GOCACHE=/private/tmp/diana-go-cache go test -count=1 ./cmd/devseed
```

### E. ACCOUNT tests

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
GOCACHE=/private/tmp/diana-go-cache go test -count=1 ./internal/classification ./internal/rules -run 'TestProductionAccount'
```

### F. VAT tests

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
GOCACHE=/private/tmp/diana-go-cache go test -count=1 ./internal/classification -run 'TestProduction(VAT|VerifiedRule|HistoricalVersions)'
GOCACHE=/private/tmp/diana-go-cache go test -count=1 ./internal/money
```

### G. DEDUCTIBILITY tests

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
GOCACHE=/private/tmp/diana-go-cache go test -count=1 ./internal/classification -run TestProductionDeductibility
```

### H. Rule-version/provenance tests

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
GOCACHE=/private/tmp/diana-go-cache go test -count=1 ./internal/rules ./internal/classification -run 'TestProduction(Provenance|VerifiedRule|DemoIsolation|EvaluationSnapshot)|Test.*Version'
```

### I. PostgreSQL integration

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate
docker compose up -d postgres redis
cd backend
export TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable'
export GOCACHE=/private/tmp/diana-go-cache
go test -count=1 -tags=integration ./internal/platform/postgres
```

### J. Concurrency/rule-snapshot tests

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' GOCACHE=/private/tmp/diana-go-cache go test -race -count=1 -tags=integration ./internal/platform/postgres -run 'TestProduction(ConcurrentRuleUpdate|HistoricalProvenance|OverlappingVersions)'
GOCACHE=/private/tmp/diana-go-cache go test -race -count=1 ./internal/classification ./internal/platform/observability
```

### K. Classification task lifecycle

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' GOCACHE=/private/tmp/diana-go-cache go test -count=1 -tags=integration ./internal/platform/postgres -run 'TestProductionDemoIsolationClassificationTaskLifecycle|TestClassificationReview'
```

### L. SAGA production classification regression

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
GOCACHE=/private/tmp/diana-go-cache go test -count=1 ./internal/saga -run 'TestSAGAProduction|TestSAGADemo|TestGenerateRejects'
```

### M. Full invoice pipeline classification test

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' GOCACHE=/private/tmp/diana-go-cache go test -count=1 -tags=integration ./internal/platform/postgres -run 'TestProductionDemoIsolationClassificationTaskLifecycleAndFullPipeline|TestProductionHistoricalProvenance|TestProductionVerifiedVATAndManualAccountingReachSAGABoundary'
```

This proves conservative LINES_READ → CLASSIFIED → AWAITING_REVIEW → accountant resolution → READY_FOR_SAGA → XML generation. It deliberately does **not** claim a shipped all-automatic accounting treatment.

### N. Modules 1–7 regression

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
GOCACHE=/private/tmp/diana-go-cache go test -count=1 ./...
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' TEST_REDIS_URL='redis://127.0.0.1:6382/14' GOCACHE=/private/tmp/diana-go-cache go test -count=1 -tags=integration ./internal/platform/postgres ./internal/workerruntime
```

### O. ANAF/SPV regression

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
GOCACHE=/private/tmp/diana-go-cache go test -count=1 ./internal/spv
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' TEST_REDIS_URL='redis://127.0.0.1:6382/14' GOCACHE=/private/tmp/diana-go-cache go test -count=1 -tags=integration ./internal/platform/postgres ./internal/workerruntime -run 'TestSPV|Test.*SPV'
```

### P. Contract/Missing Contract regression

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
GOCACHE=/private/tmp/diana-go-cache go test -count=1 ./internal/contracts ./internal/contractingestion
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' GOCACHE=/private/tmp/diana-go-cache go test -count=1 -tags=integration ./internal/platform/postgres -run 'Test.*Contract|Test.*Missing'
```

Live Gemini remains deferred; these commands use existing fake/synthetic boundaries.

### Q. SAGA regression

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
GOCACHE=/private/tmp/diana-go-cache go test -count=1 ./internal/saga
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' GOCACHE=/private/tmp/diana-go-cache go test -count=1 -tags=integration ./internal/platform/postgres -run 'Test.*SAGA|Test.*Saga'
```

### R. Frontend tests

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate
npm run test -- src/features/rules/LegalSourceLink.test.tsx src/features/rules/RulesWorkspace.test.tsx src/repositories/http/ApiInvoiceReadRepository.test.ts
npm run test
```

### S. Playwright regressions for touched rule/classification UI

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate
npm run test:e2e
npm run test:e2e:backend1
npm run test:e2e:backend3
npm run test:e2e:backend4
npm run test:e2e:backend5
npm run test:e2e:backend6
npm run test:e2e:backend7
npm run test:e2e:spv-connection
npm run test:e2e:saga-export-ux
npm run test:e2e:contract-ingestion
```

No Playwright files were changed. Guarded devseed explicitly uses the demo policy for frozen demonstration flows; production service defaults remain safe.

### T. Atlas incremental migration

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate
docker compose up -d postgres
cd backend
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' atlas migrate apply --env local
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' atlas migrate status --env local
```

### U. Atlas empty database migration

Run against a new database name; this command does not drop an existing database.

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate
docker compose exec -T postgres createdb -U diana diana_accounting_rules_empty_validation
cd backend
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_accounting_rules_empty_validation?sslmode=disable' atlas migrate apply --env local
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_accounting_rules_empty_validation?sslmode=disable' atlas migrate status --env local
```

### V. Production build

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate
npm run build
cd backend
GOCACHE=/private/tmp/diana-go-cache go build ./cmd/api ./cmd/worker
```

### Accounting review checklist

- Confirm there are **zero** automatically seeded fiscal/account mappings; approve the intentionally empty pack scope.
- Confirm source-percent confirmation versus tax-treatment semantics before approving any VAT seed.
- Approve client adoption of the relevant chart and account-628 policy/trigger before a CLIENT_OVERRIDE becomes eligible.
- Resolve VAT-deduction versus expense-deductibility semantics and the meaning of SAGA_DEFAULT/N50/I.
- For any proposed future production version, approve exact source provision, required available facts, trigger, output and rule/source periods; no demo placeholder.
- Confirm unsupported zero-tax/vehicle/advanced VAT cases stay in CLASSIFICATION review.

Engineering suite execution does not satisfy this accounting checklist. Both gates are required before freezing.
