# Client Management + Onboarding V1 — user-run test handoff

No runtime tests/migrations/build in this file were executed by Codex. Run from the project root. Commands below use synthetic/fake providers only. PostgreSQL tests create immutable profile history and deliberately retain it; use the dedicated disposable test DB. The E2E launcher creates/resets only `diana_client_onboarding_e2e` and Redis DB 13; do not run multiple backend Playwright configs concurrently on their shared ports.

Initial environment (run once in the terminal used for B–Q):

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate
export GOCACHE=/private/tmp/diana-go-cache
export TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_client_management_test?sslmode=disable'
export TEST_REDIS_URL='redis://127.0.0.1:6382/12'
```

Set up N first before PostgreSQL groups C/D/F/H/I/K/L/M. H/O require running Redis and the `redis_integration` tag; `integration` alone selects none of these worker tests. L has a separate contract E2E database and seed. B/E and ordinary Go/frontend unit checks do not require services.

## Follow-up rerun after reported failures (2026-09-15)

User-supplied rerun: all three Go groups, 16 component tests and all 30 Playwright tests in this block passed. The contract E2E was not included; run section L next. Previously failing broader groups N/Q/P still need their corrected rerun. SPV/onboarding emitted retry-exhausted warnings without underlying errors; retain task error evidence if they recur. Do not repeat this passing block unless a new change or failure warrants it.

Use the initial environment above. Run the isolated contract setup in L separately. This targeted block covers the reported regressions; it does not replace acceptance groups below:

```sh
(cd backend && go test ./internal/platform/httpserver -run 'TestClientManagement' -count=1)
(cd backend && go test -tags=integration ./internal/platform/postgres -run 'TestDomainFullDeterministicSPVContractClassificationSAGAPipeline|TestRealSAGAArtifactPersistenceIsIdempotentUnderConcurrency' -count=1)
(cd backend && go test -tags=redis_integration ./internal/workerruntime -run 'SPV' -count=1)
npm run test -- src/features/clients/ClientManagement.test.tsx src/features/clients/ClientsWorkspace.test.tsx src/features/invoices/AccountingDomainV2.test.tsx
npm run test:e2e
npm run test:e2e:spv-connection
npm run test:e2e:saga-export-ux
npx playwright test --config=playwright.accounting-v2.config.ts
npm run test:e2e:client-onboarding
# Then run the isolated contract E2E setup and command in L.
```

## A. Static / compile

```sh
npm run typecheck
(cd backend && go generate ./ent && atlas migrate hash --dir file://migrations && go test -exec /usr/bin/true -run '^$' ./... && go test -tags=integration,redis_integration -exec /usr/bin/true -run '^$' ./...)
git diff --check
```

`-exec /usr/bin/true` compiles test binaries and replaces execution; `ok` output is not a behavior-test PASS.

## B. Client domain

```sh
(cd backend && go test ./internal/clients -run 'TestClient(Minimum|Identity|Grants)' -count=1)
```

## C. Client persistence/API

```sh
(cd backend && go test ./internal/platform/httpserver -run 'TestClientManagement' -count=1)
(cd backend && go test -tags=integration ./internal/platform/postgres -run 'TestClientManagement' -count=1)
```

## D. Accounting profile

```sh
(cd backend && go test ./internal/clients -run 'TestClientProfile' -count=1)
(cd backend && go test -tags=integration ./internal/platform/postgres -run 'TestClientProfilePersistence|TestDomain' -count=1)
```

## E. Onboarding readiness

```sh
(cd backend && go test ./internal/clients -run 'TestClientOnboarding|TestClientReadiness' -count=1)
```

## F. ANAF integration regression

```sh
(cd backend && go test ./internal/spv -count=1)
(cd backend && go test ./internal/platform/httpserver -run 'SPV' -count=1)
(cd backend && go test -tags=integration ./internal/platform/postgres -run 'SPV|TestClientOnboardingANAF' -count=1)
```

## G. Certificate onboarding (synthetic browser flow)

```sh
(cd backend && go test ./internal/spv -run 'OAuth|TestClientOnboarding' -count=1)
npm run test -- src/features/clients/SPVConnectionCard.test.tsx
npm run test:e2e:spv-connection
```

There is no server-side certificate upload/parser; existing fake ANAF page uses the synthetic certificate authorization action. No real certificate is needed.

## H. SPV worker regression

```sh
(cd backend && go test -tags=redis_integration ./internal/workerruntime -run 'SPV' -count=1)
(cd backend && go test ./internal/spv -run 'TestClientOnboardingQueuedSync|TestSPV' -count=1)
(cd backend && go test -tags=integration ./internal/platform/postgres -run 'SPV' -count=1)
```

## I. SAGA configuration

```sh
(cd backend && go test ./internal/saga -run 'TestClientOnboarding' -count=1)
(cd backend && go test -tags=integration ./internal/platform/postgres -run 'TestClientProfilePersistence' -count=1)
npm run test -- src/features/clients/ClientManagement.test.tsx
```

## J. SAGA exporter regression

```sh
(cd backend && go test ./internal/saga -count=1)
(cd backend && go test -tags=integration ./internal/platform/postgres -run 'Saga|SAGA' -count=1)
npm run test:e2e:saga-export-ux
```

## K. Classification V1 regression

```sh
(cd backend && go test ./internal/accounting ./internal/accountingdate ./internal/classification ./internal/rules ./internal/accountingacceptance -count=1)
(cd backend && go test -tags=integration ./internal/platform/postgres -run 'Classification|TestDomain|Production' -count=1)
npx playwright test --config=playwright.accounting-v2.config.ts
```

Synthetic engine fixtures are TEST_ONLY; no production pack is installed by onboarding.

## L. Contracts regression

The contract E2E requires its own migrated, seeded database; the ordinary devseed does not create `client-contract-ingestion`. Prepare and run it in a subshell so its overrides do not affect later suites. A fresh database name makes CREATE fail safely on reuse:

```sh
(
 export DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_client_mgmt_contract_e2e?sslmode=disable'
 export REDIS_URL='redis://127.0.0.1:6382/14'
 export WORKER_QUEUE='client-mgmt-contract-e2e'
 docker compose exec -T postgres createdb -U diana diana_client_mgmt_contract_e2e &&
 (cd backend && atlas migrate apply --env local && APP_ENV=test go run ./cmd/contractingestionseed) &&
 npm run test:e2e:contract-ingestion
)
```

```sh
(cd backend && go test ./internal/contracts ./internal/contractingestion ./internal/contractingestion/fixtures -count=1)
(cd backend && go test -tags=integration ./internal/platform/postgres -run 'Contract' -count=1)
# Run the isolated contract E2E setup and command in L above.
```

## M. Invoice pipeline regression

```sh
(cd backend && go test ./internal/invoicing ./internal/validationtasks -count=1)
(cd backend && go test -tags=integration ./internal/platform/postgres -run 'Pipeline|Invoice|ValidationTask' -count=1)
```

## N. PostgreSQL integration

Fresh disposable DB creation (if the database already exists from a previous run, omit the CREATE DATABASE statement):

```sh
docker compose up -d postgres redis
docker compose exec -T postgres psql -U diana -d postgres -c 'CREATE DATABASE diana_client_management_test OWNER diana'
(cd backend && DATABASE_URL="$TEST_DATABASE_URL" atlas migrate apply --env local)
(cd backend && go test -tags=integration ./internal/platform/postgres -count=1)
```

Wait for `docker compose exec -T postgres pg_isready -U diana -d diana` and `docker compose exec -T redis redis-cli ping` to succeed if services are still starting.

## O. Redis/Asynq

```sh
(cd backend && go test -tags=redis_integration ./internal/workerruntime -count=1)
```

Uses TEST_REDIS_URL and TEST_DATABASE_URL exported above; existing integration helpers select their own test queues/DBs as implemented. No live provider needed.

## P. Concurrency/race

```sh
(cd backend && go test -race ./internal/clients ./internal/spv ./internal/saga ./internal/platform/httpserver -count=1)
(cd backend && go test -race -tags=integration ./internal/platform/postgres -run 'TestClientManagement|TestClientProfile|TestClientOnboardingANAF' -count=1)
```

## Q. Frontend component tests

```sh
npm run test -- src/features/clients/ClientManagement.test.tsx src/features/clients/ClientsWorkspace.test.tsx src/features/clients/SPVConnectionCard.test.tsx src/repositories/http/ApiInvoiceReadRepository.test.ts
npm run test
```

## R. Playwright client onboarding

```sh
npm run test:e2e:client-onboarding
```

A–G use real application/API/PostgreSQL/worker + fake ANAF browser certificate flow. H simulates an HTTP denied response to check frontend error handling, because the browser still has the existing all-client demo actor. Actual cross-grant read/mutation/callback isolation is exercised in Go HTTP/service tests. This is explicitly not a production RBAC browser test.

## S. Full Playwright regression

Run these sequentially (shared backend ports):

```sh
npm run test:e2e
npm run test:e2e:backend1
npm run test:e2e:backend3
npm run test:e2e:backend4
npm run test:e2e:backend5
npm run test:e2e:backend6
npm run test:e2e:backend7
npm run test:e2e:spv-connection
npm run test:e2e:saga-export-ux
# Run the isolated contract E2E setup and command in L above.
npx playwright test --config=playwright.accounting-v2.config.ts
npm run test:e2e:client-onboarding
```

## T. Incremental migration

Collision preflight against an existing local 000014 database (read-only):

```sh
docker compose exec -T postgres psql -U diana -d diana -c "SELECT CASE WHEN btrim(upper(cui)) ~ '^(RO[[:space:]]*)?[1-9][0-9]{1,9}$' THEN regexp_replace(btrim(upper(cui)), '^RO[[:space:]]*', '') ELSE btrim(upper(cui)) END AS normalized_identifier, count(*) FROM clients GROUP BY 1 HAVING count(*) > 1"
```

A returned collision requires review; do not merge/delete/change history to force migration. Fresh disposable incremental rehearsal:

```sh
docker compose exec -T postgres psql -U diana -d postgres -c 'CREATE DATABASE diana_client_mgmt_incremental OWNER diana'
(cd backend && DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_client_mgmt_incremental?sslmode=disable' atlas migrate apply --env local 14)
docker compose exec -T postgres psql -U diana -d diana_client_mgmt_incremental -c "INSERT INTO clients(id,name,cui,created_at,updated_at) VALUES('migration-client-legacy','Client legacy SRL','RO-DEMO-MIGRATION',now(),now()),('migration-client-normal','Client normal SRL','RO12345678',now(),now())"
(cd backend && DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_client_mgmt_incremental?sslmode=disable' atlas migrate apply --env local)
(cd backend && DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_client_mgmt_incremental?sslmode=disable' atlas migrate status --env local)
```

The 000014 rehearsal uses original-schema client rows; new devseed is run only after 000015. Verify existing rows stay ACTIVE/revision 1, preserve display identity, receive normalized identity and enabled SAGA export.

## U. Empty DB migration

```sh
docker compose exec -T postgres psql -U diana -d postgres -c 'CREATE DATABASE diana_client_mgmt_empty OWNER diana'
(cd backend && DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_client_mgmt_empty?sslmode=disable' atlas migrate apply --env local)
(cd backend && DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_client_mgmt_empty?sslmode=disable' SEED_CLIENT_ONBOARDING=true go run ./cmd/devseed)
(cd backend && DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_client_mgmt_empty?sslmode=disable' atlas migrate status --env local)
```

## V. Production build

```sh
npm run build
(cd backend && go build ./cmd/api ./cmd/worker)
```

These commands build artifacts locally; they do not deploy or enable production RBAC/rules.
