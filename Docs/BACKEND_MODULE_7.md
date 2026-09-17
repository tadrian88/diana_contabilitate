# Backend Module 7 — Worker Runtime, Outbox Hardening, Observability

Status: APPROVED / FROZEN.

## Runtime topology

- `cmd/api` owns HTTP reads and synchronous human/application commands.
- `cmd/worker` owns PostgreSQL outbox polling and the Asynq worker.
- PostgreSQL is authoritative for domain state, audit, and async intent.
- Redis DB 0 is the development queue; isolated E2E/integration runs use DB 15.
- One `workflow` queue and one `workflow:continue_invoice` job are sufficient.

The API does not require Redis and remains ready when Redis is unavailable.
`PIPELINE_DISPATCH_ENABLED` defaults to false; its old in-process loop is retained
only as an explicit compatibility mode. Worker handlers contain no SQL or
matching/classification rules.
Compatibility mode must not be enabled alongside `cmd/worker`; it is not part of
the multi-instance production topology.

## Transactional outbox lifecycle

Domain mutation, `ActivityEvent`, and `OutboxEntry(PENDING)` commit together.
The worker dispatcher atomically changes eligible rows to `CLAIMED` using
`FOR UPDATE SKIP LOCKED`. Claims have an owner and expiry lease. Publication
success changes the row to `DISPATCHED`; publication failure releases it to
`PENDING` with capped exponential backoff and `last_error`. Exhausted publish
attempts become `FAILED` for operational intervention.

Delivery is at least once. If publication succeeds but marking `DISPATCHED`
fails or the worker crashes, the claim remains until lease expiry and another
dispatcher republishes it. PostgreSQL cannot atomically commit with Redis, so
exactly-once is neither claimed nor simulated. Duplicate jobs are safe because
the worker reloads current PostgreSQL state and existing command identities,
revisions, audit keys, unique constraints, terminal states, and blockers make
old work a no-op.

## Jobs and failures

The job payload contains only `outbox_id`, `invoice_id`, `event_type`,
`idempotency_key`, and `correlation_id`. It never contains invoice lines,
documents, contracts, or mutable snapshots.

- malformed/unsupported and missing-domain jobs are permanent and skip retry;
- terminal, already-advanced, and human-blocked invoices are successful stale no-ops;
- database/network failures are transient and use Asynq retry;
- retry count and backoff are bounded by configuration;
- exhausted Asynq jobs use its standard archive;
- fake SAGA failure preserves `EXPORTING / FAILED` and creates no task.

The final retry policy for a real SAGA adapter remains undecided.

## Health and graceful shutdown

API `/healthz` is process liveness and `/readyz` checks PostgreSQL. Worker
`/healthz` is process liveness; worker `/readyz` requires active acceptance plus
PostgreSQL and Redis. Temporary dependency failure therefore affects readiness,
not liveness.

On SIGINT/SIGTERM the API stops accepting requests and drains within
`SHUTDOWN_TIMEOUT`. The worker marks itself unready, cancels dispatcher polling,
waits for that loop, asks Asynq to drain in-flight work within the same timeout,
then closes its health server and dependencies.

## Observability

Both processes emit JSON logs with process, correlation, outbox/job/invoice IDs,
attempt, duration, result, and error category where relevant. No raw business
documents or free-text payloads are logged.

OpenTelemetry is disabled by default. `OTEL_ENABLED=true` enables the standard
OTLP gRPC exporter configured by `OTEL_EXPORTER_OTLP_ENDPOINT`. Spans delimit
HTTP requests, synchronous application commands, outbox dispatch, job
execution, and fake SAGA adapter calls.

`/metrics` exposes HTTP counts/duration totals, job results/failures/retries,
pipeline progressions and processing duration, stale jobs, dispatched/failed
outbox attempts, current pending/failed outbox counts, and oldest pending age.
Labels use a closed result vocabulary and never invoice/client IDs.
`ActivityEvent` is unchanged business audit.

## Configuration

| Variable | Default |
| --- | --- |
| `DATABASE_URL` | required |
| `REDIS_URL` | `redis://127.0.0.1:6382/0` |
| `WORKER_HTTP_ADDRESS` | `:8081` |
| `WORKER_CONCURRENCY` | `10` |
| `WORKER_QUEUE` | `workflow` |
| `WORKER_MAX_RETRY` | `8` |
| `WORKER_JOB_TIMEOUT` | `2m` |
| `OUTBOX_BATCH_SIZE` | `50` |
| `OUTBOX_MAX_ATTEMPTS` | `20` |
| `OUTBOX_POLL_INTERVAL` | `1s` |
| `OUTBOX_CLAIM_TTL` | `1m` |
| `OUTBOX_RETRY_MIN` / `OUTBOX_RETRY_MAX` | `1s` / `1m` |
| `SHUTDOWN_TIMEOUT` | `15s` |
| `DATABASE_MAX_OPEN_CONNS` / `DATABASE_MAX_IDLE_CONNS` | `20` / `5` |
| `DATABASE_CONN_MAX_LIFETIME` | `30m` |
| `LOG_LEVEL` | `info` |
| `OTEL_ENABLED` | `false` |

## Local startup

Start dependencies:

```sh
docker compose up -d postgres redis
```

Apply migrations and seed once, then use separate terminals:

```sh
cd backend
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' atlas migrate apply --env local
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' GOCACHE=/private/tmp/diana-go-cache go run ./cmd/devseed
```

```sh
cd backend
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' HTTP_ADDRESS=127.0.0.1:8080 GOCACHE=/private/tmp/diana-go-cache go run ./cmd/api
```

```sh
cd backend
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' REDIS_URL='redis://127.0.0.1:6382/0' WORKER_HTTP_ADDRESS=127.0.0.1:8081 GOCACHE=/private/tmp/diana-go-cache go run ./cmd/worker
```

```sh
VITE_BACKEND_READS_ENABLED=true VITE_API_PROXY_TARGET=http://127.0.0.1:8080 npm run dev
```

## Test topology

Unit tests cover dispatcher retry/exhaustion/crash windows, stale/permanent/
transient jobs, trace/log correlation, health, and cancellation. PostgreSQL
integration tests cover concurrent `SKIP LOCKED` claims and lease recovery.
Redis-tagged tests exercise real Asynq duplicate delivery, retry, and restart.
The combined tagged tests connect outbox → Redis → worker → persisted
pipeline, cover the complete automatic ingest → matching → classification →
fake-SAGA path, inject and recover from a transient database-boundary failure,
and republish a completed job to verify one logical mutation. Module 7
Playwright covers the automatic path and each approved blocker/failure using
eventual assertions.

Only a clearly isolated non-zero `TEST_REDIS_URL` may be flushed by tests.
The frozen Module 6 launcher sets `SEED_MODULE7=false` to exclude Module 7's
independently progressing fixtures while still exercising the dedicated worker.

## Deferred decisions

All accounting, contract-expiry, matching/scoring, effective-date,
reclassification, corrected-value, missing-contract resumption, duplicate
fingerprint, and real-SAGA retry decisions remain open. Real SPV/SAGA, OCR,
document import, Gemini, MCP, auth/RBAC, Cloud Run, and push transport remain
outside Module 7.
