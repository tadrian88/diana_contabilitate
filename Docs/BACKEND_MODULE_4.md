# Backend Module 4 — Contracts and Contract Matching

Status: implemented; awaiting user-run tests and review.

## Domain and persistence

The MVP Contract is read-only and contains stable identity, client, supplier
identity, reference, effective period, displayed commercial context, source
context, timestamps, and an optimistic revision. No public create/update/delete
API exists.

ContractMatchRun records the immutable outcome and `policy_version`.
ContractMatchCandidate records ordered evidence, compatibility, opaque display
confidence, recommendation, reasons, and the exact contract revision evaluated.
InvoiceContractAssociation is the explicit queryable relationship and stores an
immutable snapshot of the selected contract's reference, supplier, period,
value/currency, unit type, and payment terms.

This snapshot is the smallest justified historical strategy. A separate
ContractTerms/ContractPriceItem version hierarchy is deferred because the MVP
has no editing or import lifecycle. Future changes to the current Contract row
must not rewrite existing association snapshots.

## Matching boundary and temporary policy

`MatchingPolicy` owns evaluation and is independent of HTTP, frontend, Ent, and
future LLM integrations. `MODULE4_BASELINE_V1` is a replaceable demo/baseline
policy, not a final business rule. The application validates the structural
invariants of every policy result before any persistence occurs, so a future
policy cannot persist an incomplete outcome, duplicate candidates, invalid
ranks, or an ambiguous recommendation.

It uses only:

- exact normalized supplier CUI within the same client for discovery;
- invoice date inside the effective period;
- exact currency equality;
- reference ordering for deterministic recommendation.

It does not use numeric scores, thresholds, tolerances, semantic text, total
value, SKU, quantities, or legal/accounting inference. Confidence is opaque
display text and not a decision threshold.

## Result flows

- `UNIQUE_COMPATIBLE`: snapshot association, `MATCHING → DEDUPE_CHECKED`, audit,
  continuation outbox, no task.
- `MULTIPLE_PLAUSIBLE`: persisted candidates and recommendation,
  `MATCHING → AWAITING_MATCH_CONFIRM`, one `CONTRACT_MATCH / OPEN`, no continuation.
- `UNIQUE_INCOMPATIBLE`: same review flow; candidate count alone never auto-matches.
- `NO_MATCH`: `MATCHING → AWAITING_CONTRACT`, one `MISSING_CONTRACT / OPEN`.
- out-of-period discovered contract: no outcome is chosen. Processing returns
  `ErrExpiredContractSemantics` and leaves state unchanged pending a product decision.

Module 3 request behavior remains unchanged: `MISSING_CONTRACT OPEN → WAITING`
without invoice progression or outbox.

## Human confirmation

`POST /api/v1/invoices/{invoiceId}/contract-confirmations` requires task ID,
selected candidate ID, expected invoice revision, expected task revision, and
`Idempotency-Key`. Only a candidate belonging to the task's active match run is
accepted. Contract revision and same-client ownership are checked.

Success atomically creates the immutable association, resolves the task, moves
the invoice to `DEDUPE_CHECKED`, writes durable audit, and creates one continuation
outbox entry. Recommended and alternative candidates have the same lifecycle.

## API and frontend

- `GET /api/v1/contracts` with optional `clientId` and `q`;
- `GET /api/v1/contracts/{id}`;
- `GET /api/v1/contracts/{id}/invoices`;
- `POST /api/v1/invoices/{id}/contract-confirmations`.

Contract workspace reads and API-owned confirmation use Go/PostgreSQL in hybrid
mode. No generic match endpoint or contract CRUD is exposed.

## Product decisions still required

- expired-contract mapping: `NO_MATCH` versus `UNIQUE_INCOMPATIBLE` review;
- final discovery, compatibility, ranking, score, tolerance, value, and SKU rules;
- how a new/imported contract triggers rematching;
- `MISSING_CONTRACT WAITING` resolution/resumption semantics.

## Explicitly deferred

OCR/extraction, contract import and public CRUD, automatic missing-contract
resume, final matching policy, classification/rules, real SPV/SAGA, Redis/Asynq
production runtime, auth/RBAC, Gemini/MCP, and Cloud Run. When contract writes
or imports are introduced, their concurrency design must also prevent a
contract revision changing between match evaluation and commit; the current
MVP exposes contracts as read-only and validates persisted revisions.

## Manual test commands

Run from the repository root. Database checks require Docker and Atlas CLI.

### A. Fast/static checks

```sh
git diff --check
find backend -name '*.go' -not -path '*/ent/*' -print0 | xargs -0 gofmt -d
cd backend && GOCACHE=/private/tmp/diana-go-cache go generate ./ent && atlas migrate hash --dir file://migrations && cd ..
npm run typecheck
```

### B. Go unit/application tests

```sh
cd backend
GOCACHE=/private/tmp/diana-go-cache go test ./...
cd ..
```

### C. PostgreSQL integration tests

```sh
docker compose up -d postgres
until docker compose exec -T postgres pg_isready -U diana -d diana >/dev/null 2>&1; do sleep 1; done
cd backend
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' atlas migrate apply --env local
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' GOCACHE=/private/tmp/diana-go-cache go test -tags=integration -count=1 ./internal/platform/postgres
cd ..
```

### D. Concurrency tests

```sh
cd backend
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' GOCACHE=/private/tmp/diana-go-cache go test -tags=integration ./internal/platform/postgres -run 'Test(ConcurrentContractMatchingHasOneConsistentOutcome|ConcurrentContractConfirmationHasOneWinner|ConcurrentBlockingTaskCreationHasAtMostOneWinner|ConcurrentSameTaskCreationCommandIsIdempotent|ConcurrentContractRequestsProduceOneTransition)$' -count=10
cd ..
```

### E. Atlas incremental migration

```sh
docker compose exec -T postgres dropdb -U diana --if-exists diana_module4_incremental_validation
docker compose exec -T postgres createdb -U diana diana_module4_incremental_validation
cd backend
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_module4_incremental_validation?sslmode=disable' atlas migrate apply --env local 4
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_module4_incremental_validation?sslmode=disable' atlas migrate apply --env local 1
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_module4_incremental_validation?sslmode=disable' atlas migrate status --env local
cd ..
docker compose exec -T postgres dropdb -U diana diana_module4_incremental_validation
```

### F. Atlas empty-database migration

```sh
docker compose exec -T postgres dropdb -U diana --if-exists diana_module4_empty_validation
docker compose exec -T postgres createdb -U diana diana_module4_empty_validation
cd backend
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_module4_empty_validation?sslmode=disable' atlas migrate apply --env local
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_module4_empty_validation?sslmode=disable' atlas migrate status --env local
cd ..
docker compose exec -T postgres dropdb -U diana diana_module4_empty_validation
```

### G. Frontend typecheck/tests

```sh
npm run typecheck
npm run test
```

### H. Frozen Playwright 23 scenarios

```sh
npm run test:e2e
```

### I. Module 4 Playwright

```sh
npm run test:e2e:backend4
```

### J. Production build

```sh
npm run build
docker compose stop postgres
```
