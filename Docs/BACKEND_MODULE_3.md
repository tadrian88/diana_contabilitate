# Backend Module 3 — Validation Tasks / Human in the Loop

Status: implemented; awaiting user-run tests and review.

## Domain vocabulary

ValidationTask has exactly three types:

- `CONTRACT_MATCH`;
- `MISSING_CONTRACT`;
- `CLASSIFICATION`.

It has exactly three statuses: `OPEN`, `WAITING`, and `RESOLVED`. Every task is
created `OPEN`. `RESOLVED` is terminal for that task instance and resolved
instances remain historical; they are never reopened or reused.

The backend models Invoice → many ValidationTasks. PostgreSQL permits at most
one task whose status is not `RESOLVED` for an invoice. The frontend may expose
the current active task singularly without changing the one-to-many backend
model.

## State machine

- `MISSING_CONTRACT`: `OPEN → WAITING` through `RequestMissingContract`;
- `CONTRACT_MATCH`: `OPEN → RESOLVED` is defined but deferred to Module 4;
- `CLASSIFICATION`: `OPEN → RESOLVED` is defined but deferred to Module 5;
- `WAITING → OPEN` is not defined;
- `RESOLVED` has no outgoing transition.

Domain-specific internal capabilities create missing-contract, contract-review,
and classification-review blockers. Each owns its compatible invoice transition
and atomically commits invoice state, task, and ActivityEvent. No public generic
task-creation or status-mutation API exists.

## Missing-contract request

`POST /api/v1/invoices/{invoiceId}/contract-requests` requires the active task
ID, expected task revision, and an `Idempotency-Key`. It accepts only an `OPEN`
`MISSING_CONTRACT` task belonging to an invoice in `AWAITING_CONTRACT` for the
same client.

The transaction changes only the task to `WAITING`, records `waiting_since`,
increments the task revision, and writes a durable ActivityEvent. The invoice
remains `AWAITING_CONTRACT`; its revision does not change and no outbox
continuation is created. There is no email, upload, notification, contract, or
external-request integration behind this action.

## Query and frontend integration

`GET /api/v1/validation-tasks` supports only `clientId`, `invoiceId`, `status`,
and `type`. Its DTO contains the task plus compact invoice/client presentation
context needed by Task Inbox. It does not duplicate the complete invoice.

In hybrid mode, Task Inbox is API-backed and API invoice detail exposes its
active task. Mock-only invoice, contract, classification, rules, and Dashboard
scenarios remain isolated. API-owned contract requests go to Go/PostgreSQL;
mock-owned requests remain in the frozen mock repository.

## Explicitly deferred

- contract entities, candidates, scoring, confirmation, and import;
- `CONTRACT_MATCH` resolution;
- classification items, decisions, rules, and `CLASSIFICATION` resolution;
- missing-contract resumption and `WAITING` exit semantics;
- real SPV/SAGA, Redis/Asynq, authentication/RBAC, OCR, Gemini/MCP, and Cloud Run.

## Manual verification checklist

Run commands from the repository root unless a command explicitly changes into
`backend/`. The PostgreSQL checks expect Docker and the Atlas CLI to be
available.

### Fast static checks

```sh
git diff --check
find backend -name '*.go' -not -path '*/ent/*' -print0 | xargs -0 gofmt -d
npm run typecheck
```

### Go unit/application tests

```sh
cd backend
GOCACHE=/private/tmp/diana-go-cache go test ./...
cd ..
```

### PostgreSQL prerequisite and integration tests

```sh
docker compose up -d postgres
until docker compose exec -T postgres pg_isready -U diana -d diana >/dev/null 2>&1; do sleep 1; done
cd backend
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' atlas migrate apply --env local
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' GOCACHE=/private/tmp/diana-go-cache go test -tags=integration ./internal/platform/postgres
cd ..
```

### Repeated concurrency tests

```sh
cd backend
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' GOCACHE=/private/tmp/diana-go-cache go test -tags=integration ./internal/platform/postgres -run 'Test(ConcurrentBlockingTaskCreationHasAtMostOneWinner|ConcurrentSameTaskCreationCommandIsIdempotent|ConcurrentContractRequestsProduceOneTransition)$' -count=10
cd ..
```

### Atlas incremental and empty-database checks

```sh
docker compose exec -T postgres dropdb -U diana --if-exists diana_module3_incremental_validation
docker compose exec -T postgres createdb -U diana diana_module3_incremental_validation
cd backend
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_module3_incremental_validation?sslmode=disable' atlas migrate apply --env local 3
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_module3_incremental_validation?sslmode=disable' atlas migrate apply --env local 1
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_module3_incremental_validation?sslmode=disable' atlas migrate status --env local
cd ..
docker compose exec -T postgres dropdb -U diana diana_module3_incremental_validation

docker compose exec -T postgres dropdb -U diana --if-exists diana_module3_empty_validation
docker compose exec -T postgres createdb -U diana diana_module3_empty_validation
cd backend
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_module3_empty_validation?sslmode=disable' atlas migrate apply --env local
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_module3_empty_validation?sslmode=disable' atlas migrate status --env local
cd ..
docker compose exec -T postgres dropdb -U diana diana_module3_empty_validation
```

### Frontend tests

```sh
npm run typecheck
npm run test
```

### Frozen Playwright suite

```sh
npm run test:e2e
```

### Backend Module 3 Playwright scenario

```sh
npm run test:e2e:backend3
```

### Production build

```sh
npm run build
```

After all database-backed checks:

```sh
docker compose stop postgres
```
