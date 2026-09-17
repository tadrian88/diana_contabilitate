# Go backend

Backend Module 1 proves the first production-shaped read slice:

`React -> HTTP -> Go application services -> Ent -> PostgreSQL`

## Implemented scope

- persisted `Client`, minimal `Invoice`, and durable `ActivityEvent`;
- exact decimal amounts in the domain and PostgreSQL `numeric(20,4)`;
- explicit JSON-number conversion at the HTTP boundary;
- `GET /api/v1/clients` and `GET /api/v1/invoices/{id}`;
- `/healthz`, `/readyz`, request IDs, structured logs, and graceful shutdown;
- deterministic development seed used by the real-backend Playwright slice.

The API never runs schema auto-creation. Atlas migrations in `migrations/` are
the deployment source. Ent generated code lives in `ent/` and uses pgx only as
the PostgreSQL driver; there is no parallel direct-SQL application layer.

## Local run

From the repository root:

```sh
docker compose up -d postgres
cd backend
export DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable'
atlas migrate apply --env local
go run ./cmd/devseed
go run ./cmd/api
```

The explicit SAGA adapter mode is `SAGA_MODE=fake` for deterministic local
demos/tests or `SAGA_MODE=file` for the real SAGA C XML artifact generator.
Production rejects fake mode. File generation does not claim that desktop SAGA
imported the artifact; see `../Docs/BACKEND_SAGA_REAL.md`.

The frontend enables the hybrid read adapter only when
`VITE_BACKEND_READS_ENABLED=true`. All capabilities outside Module 1 continue
to use the approved mock repository.

## Verification

```sh
npm run test:backend
npm run test:backend:integration
npm run test:e2e:backend1
```

The integration test requires a migrated PostgreSQL database in
`TEST_DATABASE_URL`.
