# Backend Real SAGA Integration

Status: core file adapter and manual human-confirmed handoff implemented;
awaiting user-run tests and review. Machine acknowledgement remains deferred.

## Implemented boundary

`internal/saga` implements the real SAGA C invoice XML adapter. It validates an
already classified invoice, creates deterministic UTF-8 XML, performs a strict
structural round trip, hashes the bytes with SHA-256 and persists one immutable
attempt for `(invoice_id, invoice_revision, exporter_version)`.

The worker selects the adapter explicitly:

- `SAGA_MODE=file`: real generator and durable artifact;
- `SAGA_MODE=fake`: frozen deterministic adapter for tests/demos.

`SAGA_MODE=fake` is rejected when `APP_ENV=production`. A real adapter error can
never fall back to fake success.

## Attempt and artifact model

Migration `000010_real_saga_export.sql` adds the immutable source
`document_type` and `saga_export_attempts` with:

- attempt, invoice, client and invoice revision identity;
- exporter version and status (`GENERATED` or `FAILED`);
- sensitive payload, filename, content type and SHA-256;
- exact classification snapshot;
- permanent failure category and safe error;
- started/completed timestamps.

The database shape enforces either a complete generated artifact or a complete
failure record. Payload bytes are sensitive and are never logged or sent in an
Asynq job. Audit records capture SAGA generation start/completion/failure.

## Pipeline behavior

The existing `SagaExporter` now returns an explicit result. The fake adapter can
return authoritative test confirmation. The file adapter always returns
`Confirmed=false` because local SAGA import cannot be observed. The worker then
acknowledges the generation job but leaves the invoice at `EXPORTING`; it does
not fabricate `EXPORTED`.

Permanent format/data failures record `SagaStatus=FAILED` without creating a
ValidationTask. Transient infrastructure failures are returned to Asynq for its
existing bounded retry policy. Re-delivery reuses the persisted attempt.

## Validation

Before serialization the adapter enforces:

- exact client ownership and invoice revision;
- `READY_FOR_SAGA`/`EXPORTING` and no active task;
- Invoice-only document support;
- required identities, number/date/currency and lines;
- all three final classifications per line;
- approved direct SAGA account/VAT/deductibility values;
- exact line `net + VAT = total` and invoice `sum(lines) = total` arithmetic;
- safe deterministic filename components.

No XSD validation is claimed because SAGA publishes no XSD for this import
contract.

## Tests added

- documented XML hierarchy/mapping, a synthetic contract-golden fixture,
  filename, UTF-8 diacritics and escaping;
- exact decimals and totals;
- RON and non-RON behavior;
- documented deduction codes;
- pending/invalid classifications and missing account rejection;
- CreditNote rejection;
- idempotent immutable attempt reuse;
- permanent failure persistence;
- pipeline refusal to equate generated files with imported invoices;
- production rejection of the fake adapter;
- UBL CreditNote kind preservation.

## Manual handoff decision

`EXPORTED` now means human-confirmed import of the exact generated artifact.
The contextual Invoice Detail flow and client-scoped API are documented in
`SAGA_EXPORT_UX.md`. Download alone remains operational evidence only. Full
production exposure still requires verified authentication/RBAC; the local
Windows bridge and machine acknowledgement remain deferred.

Remaining inputs:

1. **Format — CreditNote:** provide an accepted sanitized SAGA sample or vendor
   documentation for storno imports.
2. **Accounting configuration:** production rule results must use the explicit
   adapter values documented in `SAGA_FORMAT_DISCOVERY.md`; current demo text is
   intentionally rejected.

The manual file-mode journey is complete through human confirmation; it does
not claim SAGA-side technical acceptance.

## User-run verification commands

Run from the repository root. These commands intentionally use isolated test
databases and synthetic data.

### Fast/static and compile-only

```sh
cd backend
GOCACHE=/private/tmp/diana-go-cache go generate ./ent
atlas migrate hash --dir file://migrations
GOCACHE=/private/tmp/diana-go-cache go list ./...
GOCACHE=/private/tmp/diana-go-cache go test -run '^$' ./...
GOCACHE=/private/tmp/diana-go-cache go test -tags=integration -run '^$' ./internal/platform/postgres
git diff --check
cd ..
```

### SAGA generator, golden format and failure policy

```sh
cd backend
GOCACHE=/private/tmp/diana-go-cache go test ./internal/saga -count=1
GOCACHE=/private/tmp/diana-go-cache go test ./internal/saga -run '^TestGenerateDocumentedSAGAInvoiceXML$' -count=1
GOCACHE=/private/tmp/diana-go-cache go test ./internal/saga -run 'Rejects|PermanentDataFailure|NonRON' -count=1
GOCACHE=/private/tmp/diana-go-cache go test ./internal/invoicing -run 'SAGA|GeneratedFile' -count=1
GOCACHE=/private/tmp/diana-go-cache go test ./internal/platform/config -run '^TestProductionCannotSilentlyUseFakeSAGA$' -count=1
cd ..
```

### PostgreSQL artifact/idempotency/concurrency

```sh
docker compose up -d postgres
docker compose exec -T postgres sh -lc 'dropdb --if-exists -U diana diana_saga_test && createdb -U diana diana_saga_test'
cd backend
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_saga_test?sslmode=disable' atlas migrate apply --env local
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_saga_test?sslmode=disable' GOCACHE=/private/tmp/diana-go-cache go test -tags=integration ./internal/platform/postgres -run '^TestRealSAGA' -count=1
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_saga_test?sslmode=disable' GOCACHE=/private/tmp/diana-go-cache go test -race -tags=integration ./internal/platform/postgres -run '^TestRealSAGA' -count=10
cd ..
```

### Incremental and empty Atlas migration

```sh
docker compose exec -T postgres sh -lc 'dropdb --if-exists -U diana diana_saga_incremental && createdb -U diana diana_saga_incremental'
cd backend
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_saga_incremental?sslmode=disable' atlas migrate apply --env local 9
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_saga_incremental?sslmode=disable' atlas migrate apply --env local 1
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_saga_incremental?sslmode=disable' atlas migrate status --env local
cd ..

docker compose exec -T postgres sh -lc 'dropdb --if-exists -U diana diana_saga_empty && createdb -U diana diana_saga_empty'
cd backend
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_saga_empty?sslmode=disable' atlas migrate apply --env local
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_saga_empty?sslmode=disable' atlas migrate status --env local
cd ..
```

### Full backend, worker, ANAF/SPV and frontend regressions

```sh
docker compose up -d postgres redis
cd backend
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_saga_test?sslmode=disable' TEST_REDIS_URL='redis://127.0.0.1:6382/15' GOCACHE=/private/tmp/diana-go-cache go test -tags='integration redis_integration' ./...
GOCACHE=/private/tmp/diana-go-cache go test ./...
cd ..
npm run typecheck
npm run test
npm run test:e2e
npm run test:e2e:backend1
npm run test:e2e:backend3
npm run test:e2e:backend4
npm run test:e2e:backend5
npm run test:e2e:backend6
npm run test:e2e:backend7
npm run test:e2e:spv-connection
npm run build
```

A real `EXPORTED` transition exists only through the explicit human command. A
live local-SAGA automation command still does not exist. Optional acceptance
testing should use a synthetic file and an isolated, backed-up SAGA test company.

## Real Accounting Rules V1 evidence integration

Real XML generation now additionally rejects automatic demo/unverified decisions, mismatched recorded invoice dates and automatic DEDUCTIBILITY. Verified automatic decisions must retain immutable version/source/period/pack evidence; explicit persisted accountant confirmation may resolve unverified proposals without promoting their rules. Existing VAT numeric agreement, exact totals, account format and TipDeducere guards remain. Classification snapshots include date, pack, eligibility and basis. Synthetic successful fixtures explicitly model accountant confirmation. Runtime regression execution is handed to the user in [Real Accounting Rules test handoff](REAL_ACCOUNTING_RULES_TEST_HANDOFF.md); the approved SAGA module baseline remains frozen.
