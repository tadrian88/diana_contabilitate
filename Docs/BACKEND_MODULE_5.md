# Backend Module 5 — Classification and Rules

Status: approved / frozen.

## Classification model

`LineClassification` represents one current decision for an invoice line and
one of exactly three dimensions: `ACCOUNT`, `VAT`, or `DEDUCTIBILITY`. A unique
database constraint prevents competing current records. Each record preserves
the proposal, effective value, opaque confidence display, explanation, legal
basis, review status, source, policy version, exact rule-version reference when
one applied, timestamps, reviewer, and optimistic revision.

The proposal is immutable. Automated items begin `ACCEPTED` with their effective
value. Explicit review items begin `PENDING` without an effective value and can
become only `ACCEPTED` or `CORRECTED`. Audit is the history of human changes;
Module 5 does not add multiple current classification rows.

## Baseline policy and rule matching

`MODULE5_BASELINE_V1` is deterministic integration behavior, not accounting or
tax truth. It recognizes only stored `DESCRIPTION_CONTAINS`, `ALWAYS`, and
`NO_AUTOMATION` technical match kinds. It has no LLM, fuzzy matching, confidence
threshold, or hidden inference. Confidence remains opaque display text.

For each line it must produce all three dimensions independently. One matching
rule proposes a value; no match requires review; multiple matching rules of the
same applicable level require review and are never ordered arbitrarily. A
client override replaces only its directly referenced global origin for that
client. User-authored criteria are stored/displayed as `NO_AUTOMATION`, because
the approved UI text is not a safely defined executable rule language.

All unvalidated rules/classifications use exactly:

`Exemplu demonstrativ — bază legală nevalidată`

## Pipeline and human review

The outbox dispatcher invokes the classification processor at `LINES_READ`.
Classification atomically persists all line-dimension rows and routes:

- no pending item: `LINES_READ → CLASSIFIED → READY_FOR_SAGA`, SAGA ready,
  audit, one continuation outbox, no task;
- one or more pending items: `LINES_READ → CLASSIFIED → AWAITING_REVIEW`, audit,
  exactly one `CLASSIFICATION / OPEN` task for the invoice, no continuation.

`POST /api/v1/invoices/{invoiceId}/classification-decisions` accepts or corrects
one pending item using invoice, task, and classification revisions plus an
idempotency key. Partial review persists the item and advances only the task
revision. The final item atomically resolves the task, advances the invoice to
`READY_FOR_SAGA`, records decision/task/resumption audits, and creates one
continuation outbox entry. Replay produces no duplicate logical effects.

Correction changes only the selected invoice classification. It never creates,
updates, or suggests a rule and never trains an automated system.

## Rules model and APIs

`ClassificationRule` is stable logical identity. `RuleVersion` is immutable and
uniquely numbered per rule. Rules have exactly `GLOBAL` or `CLIENT_OVERRIDE`
scope. A global rule has no client or parent. A client override has one client,
one same-category global parent, and independent version history; at most one
override identity exists per parent/client pair.

The public boundary is intentionally narrow:

- `GET /api/v1/rules` (optional `clientId`);
- `GET /api/v1/rules/{id}`;
- `POST /api/v1/rules/{id}/versions`;
- `POST /api/v1/rules/{id}/client-overrides`.

There is no generic rule creation/update/delete or generic workflow mutation.
Version and override commands use idempotency and PostgreSQL constraints plus
optimistic revision checks. Historical classifications retain their exact
`RuleVersion`; creating a new version does not reclassify them.

Effective periods are persisted and displayed only. The baseline currently
reads the latest immutable version and deliberately performs no effective-date
filtering. Choosing invoice date, processing date, or publication date is a
product decision, not an implementation assumption.

## Product decisions still required

- validated Romanian account mapping;
- validated VAT classification rules;
- validated deductibility rules and legal sources;
- whether numeric confidence thresholds should ever control review;
- precedence beyond direct client override of its global origin;
- exact effective-date execution semantics;
- whether an explicit future operation may reclassify invoices;
- allowed vocabulary and validation for accountant-entered corrected values.

Module 4 questions remain deferred: expired-contract result, final contract
matching, missing-contract resume, and commercial direction.

## Explicitly deferred

Real accounting/tax knowledge, validated Romanian fiscal rules, final chart of
accounts, ML/LLM/Gemini/MCP, automatic learning, rule suggestions, automatic
historical reclassification, OCR, SPV, real SAGA, Redis/Asynq production
runtime, auth/RBAC, and Cloud Run.

## Manual test commands

Run from the repository root. Database gates require Docker and Atlas CLI.

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

### D. Classification concurrency tests

```sh
cd backend
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' GOCACHE=/private/tmp/diana-go-cache go test -tags=integration ./internal/platform/postgres -run 'TestConcurrentFinalClassificationReviewHasOneWinner$' -count=10
cd ..
```

### E. Rules concurrency tests

```sh
cd backend
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' GOCACHE=/private/tmp/diana-go-cache go test -tags=integration ./internal/platform/postgres -run 'TestConcurrentRuleVersionAndOverrideCreationAreSafe$' -count=10
cd ..
```

### F. Atlas incremental migration

```sh
docker compose exec -T postgres dropdb -U diana --if-exists diana_module5_incremental_validation
docker compose exec -T postgres createdb -U diana diana_module5_incremental_validation
cd backend
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_module5_incremental_validation?sslmode=disable' atlas migrate apply --env local 5
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_module5_incremental_validation?sslmode=disable' atlas migrate apply --env local 1
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_module5_incremental_validation?sslmode=disable' atlas migrate status --env local
cd ..
docker compose exec -T postgres dropdb -U diana diana_module5_incremental_validation
```

### G. Atlas empty database migration

```sh
docker compose exec -T postgres dropdb -U diana --if-exists diana_module5_empty_validation
docker compose exec -T postgres createdb -U diana diana_module5_empty_validation
cd backend
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_module5_empty_validation?sslmode=disable' atlas migrate apply --env local
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_module5_empty_validation?sslmode=disable' atlas migrate status --env local
cd ..
docker compose exec -T postgres dropdb -U diana diana_module5_empty_validation
```

### H. Frontend typecheck/tests

```sh
npm run typecheck
npm run test
```

### I. Frozen Playwright suite

```sh
npm run test:e2e
```

### J. Module 5 Playwright

```sh
npm run test:e2e:backend5
```

### K. Production build

```sh
npm run build
docker compose stop postgres
```
