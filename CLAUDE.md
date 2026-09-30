# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Diana ("SPV-to-SAGA") takes incoming Romanian e-Factura invoices from ANAF SPV, runs them through contract matching, duplicate detection, commercial validation against contracts, and accounting classification, and then produces SAGA C import XML for accountants. The UI, domain vocabulary and much of `Docs/` are in **Romanian**. Keep user-facing strings in Romanian.

## Commands

Local stack (Docker Compose: Postgres on host port 5442, Redis on 6382, API :8080, worker :8081, Vite :5173). Config comes from `.env.local` (template: `.env.example`).

```bash
make dev                 # check env, start postgres/redis, apply migrations, provision demo user, run full stack
make migrate             # atlas migrate apply --env local (in container)
make diagnose            # env report + container status + recent logs
make release-local       # full fail-fast gate (scripts/release-check.sh) then local redeploy + health checks
make deploy-gcp-test     # same gate + build/push + Cloud Run TEST rollout (clean tree, typed confirmation)
```

Frontend (repo root):

```bash
npm run dev
npm run typecheck
npm run build
npm test                                             # vitest (jsdom, MSW)
npx vitest run src/features/invoices/SagaExportCard.test.tsx -t "test name"
```

Backend (Go module lives in `backend/`; scripts use `GOCACHE=/private/tmp/diana-go-cache`):

```bash
npm run test:backend                                 # go test ./... (unit only)
cd backend && go test ./internal/saga -run TestName  # single test
cd backend && go vet ./...
cd backend && go generate ./ent                      # regenerate Ent after editing ent/schema
cd backend && atlas migrate validate --dir file://migrations
```

Integration tests are behind build tags and skip when their env var is unset:
- `-tags=integration` needs `TEST_DATABASE_URL` pointing at an already-migrated isolated DB (never the main `diana` DB). Most live in `internal/platform/postgres`.
- `-tags=redis_integration` needs `TEST_REDIS_URL` (Asynq suites flush the Redis DB, so run them serially on dedicated DB numbers — see `scripts/release-check.sh` for the exact invocations).

E2E (Playwright, Chromium):
- `npm run test:e2e` — base UI suite against the **mock** repository and mock auth; no backend.
- `npm run test:e2e:<suite>` (backend1/3–7, accounting-v2, accounting-workflow, account-learning, spv-connection, saga-export-ux, client-onboarding, authentication, contract-ingestion) — each has its own `playwright.<suite>.config.ts` whose webServer runs a `scripts/start-*-e2e.sh` that **drops and recreates** a dedicated Postgres DB, migrates, runs `cmd/devseed`, provisions a test user and starts the API on a suite-specific port. Keep new suites on their own DB name, Redis DB number and ports.

## Architecture

**Backend: Go modular monolith, two processes** over the same capability packages.
- `backend/cmd/api` — synchronous HTTP (stdlib `net/http` mux, routes in `internal/platform/httpserver/server.go`). Human commands are synchronous.
- `backend/cmd/worker` — async execution: claims PostgreSQL outbox rows (`FOR UPDATE SKIP LOCKED` leases) and publishes to the Asynq `workflow` queue; processes pipeline continuation, SPV sync, contract extraction, accounting analysis.
- Other `cmd/*` are operator/dev tools (e.g. `devseed`, `authuser provision`, `accountingrelease profile|rule|pack`, `spvconnect`, `legislation*`, `contractextract`, `fakeanaf`).

Dependency direction: HTTP handlers (transport/DTO only) → capability packages in `internal/` (`invoicing`, `contracts`, `contractingestion`, `commercialvalidation`, `classification`, `accountinganalysis`, `rules`, `saga`, `spv`, `clients`, `validationtasks`, `authentication`, …) → persistence adapters in `internal/platform/postgres`. Ent (`backend/ent`, generated; schemas in `ent/schema`) is the persistence abstraction; pgx is only the driver. Some concurrency-sensitive code in `platform/postgres` uses SQL directly via the Store (locks, outbox) — follow existing precedent there rather than adding a parallel repository layer.

Key invariants that span many files:
- **Invoice pipeline is an explicit state machine** (`internal/invoicing/state_machine.go`), with optimistic revisions. Every business mutation commits domain change + `ActivityEvent` (audit) + `OutboxEntry` in **one transaction**. Delivery is at-least-once; handlers must be idempotent and treat stale/terminal/blocked jobs as no-ops. Outbox/Asynq payloads carry IDs only, never domain snapshots.
- **ValidationTask**: at most one active blocking task per invoice (DB-enforced); resolved tasks remain as history.
- **Money** is exact decimal internally and `numeric(20,4)` in Postgres (`internal/money`); convert to JSON numbers only at the HTTP boundary.
- **Classification** is per invoice line × dimension. New (V2) invoices use four dimensions `ACCOUNT / VAT_TREATMENT / VAT_DEDUCTIBILITY / EXPENSE_TAX_TREATMENT`; legacy V1 invoices keep three (`ACCOUNT / VAT / DEDUCTIBILITY`) and are never converted. `LineClassification` is the single accounting authority; AI output (`AI_PROPOSAL`) is only ever a proposal pending human review. Rules are immutable versions; human corrections never mutate rules and rule changes never reclassify history.
- **Gemini** is used behind interfaces (`contractingestion.ContractExtractor`, `accountinganalysis`) for PDF contract extraction and accounting proposals. Prompts/schemas are versioned; tests never call live Gemini (a live diagnostic test is opt-in only).
- **SAGA** (`internal/saga`) generates immutable XML artifacts; generation/download does not mean `EXPORTED` — that requires explicit human confirmation. `SAGA_MODE=fake|file`; production rejects fake.
- **ANAF SPV OAuth**: tokens encrypted at rest, OAuth state stored as a digest in Postgres; certificates are never handled by Diana.

**Frontend** (React 19 + Vite + TanStack Query + Tailwind 4, `src/`): feature folders under `src/features/*`. All data access goes through the `InvoiceRepository` interface (`src/repositories/invoiceRepository.ts`), selected by `createAppRepository()`:
- `VITE_BACKEND_READS_ENABLED=true` → `ApiInvoiceReadRepository` (real API; no silent fallback to mocks; pipeline progress observed via React Query polling, never driven by React).
- otherwise → `MockInvoiceRepository` + `AutomaticWorkflowRunner` (mock-only timers) used by the base Playwright suite and component tests.
Query keys are centralized in `src/app/queryKeys.ts`. Vite proxies `/api` to `VITE_API_PROXY_TARGET`.

## Migrations

Atlas, SQL files in `backend/migrations/` (`00000N_name.sql` + `atlas.sum`). Migrations are **additive and forward-only**: never edit an existing migration; add a new numbered file, keep the Ent schema aligned, and re-hash (`atlas migrate hash`). Services never auto-create schema; Cloud applies migrations via a one-shot job.

## Project conventions

- `Docs/PROJECT_STATE.md` is the canonical status log (root `PROJECT_STATE.md` is a short summary); `Docs/DECISIONS.md` is the numbered decision log (D-xxx); each milestone has a design doc plus `*_TEST_HANDOFF.md` / `*_RESULT.md`. Read the relevant doc before changing a capability, and record new milestones/decisions there.
- Areas marked **APPROVED / FROZEN** in `Docs/PROJECT_STATE.md` (frontend routes/behavior, backend modules 1–7, ANAF/SPV ingestion, real SAGA, etc.) must not change behavior without explicit user approval; extend additively.
- Status reporting convention in the docs: distinguish what was compiled/statically checked from what was actually run. Heavy runtime suites (integration, E2E, `make release-local`, live Gemini/ANAF, desktop SAGA import, deployment) are typically run by the user; don't claim they passed unless you ran them.
- Cloud target is GCP project TEST only (`deploy/`, `scripts/gcp/`, `Docs/GCP_*.md`); there is no PROD.

## When writing new code
- DO NOT PERFORM TESTS OF ANY KIND, EXECUTING SCRIPTS, OR INSTALLING NEW PACKAGE - in a word, don't perform task that take more than a minute that I couldn't do. INSTEAD, give me a list of commands to execute for you and I will give the output back to you. This way, we don't consume too much tokens.
- Every command you give me must be complete and copy-pasteable as-is: no placeholders such as `<DB izolată migrată>` or `...`. Include every prerequisite step with real values (e.g. dropping/creating and migrating the isolated integration-test database, exporting `TEST_DATABASE_URL`, `GOCACHE`) and use absolute paths. The only value you may leave for me to type is a secret such as `DIANA_PASSWORD`, and say so explicitly.

## Working with multiple agents (git worktrees)

Several agent sessions may work on this repo at the same time. Apply these rules in every session:

- **One agent = one git worktree + one branch.** Two agents never edit the same working tree.
  - The main checkout `/Users/adriantudoran/Projects/diana_contabilitate` (branch `main`) is for integration, review and running the local stack.
  - Feature work happens in `/Users/adriantudoran/Projects/diana_worktrees/<name>` on branch `feature/<name>`. Create it with `git worktree add ../diana_worktrees/<name> -b feature/<name>` from the main checkout.
  - Remove a worktree only with `git worktree remove`, never `rm -rf`.
- **At the start of a session, check where you are.** Say which worktree and branch you are in and run `git status`. If you see changes you did not make, stop and ask; do not edit files another active agent owns.
- **Commit at every milestone on the agent's branch.** Do not leave work uncommitted between sessions. Merge into `main` only after the user has run the tests.
- **Numbered shared resources collide:**
  - migrations: `backend/migrations/0000NN_*.sql` + `atlas.sum`;
  - decisions: `D-xxx` in `Docs/DECISIONS.md`;
  - the top section of `Docs/PROJECT_STATE.md`.

  Use the number the user assigns in the prompt. Otherwise check `main` and the open branches (`git worktree list`, `git log --all --oneline -- backend/migrations Docs/DECISIONS.md`) and take the next free number. Before merging, rebase on `main`, renumber on conflict, then run `atlas migrate hash` and `atlas migrate validate`.
- **A migration is applied only when finished.** Finished means the file is final and `atlas migrate hash` has been run. Never apply a migration to the shared `diana` database while its author is still working on it: migrations are forward-only, and an applied-then-edited migration forces `make reset`.
- **The local Docker stack and the `diana` database are shared.**
  - Docker Compose derives the project name from the folder unless `compose.yaml` sets `name:`. Until it does, run `docker compose` / `make` only from the main checkout; otherwise a worktree starts a second stack with an empty database and the same host ports.
  - Only one worktree's code runs in the stack at a time: `docker compose up -d --build` builds the worktree you run it from, so say which one.
  - Integration and E2E tests use a database named after the task (for example `diana_<task>_it`), never `diana` and never another agent's test database.
- **Untracked files are missing in a new worktree:** `.env.local` (secrets), `test-data/` (real client data and harnesses), `node_modules`. Symlink `.env.local` and `test-data` from the main checkout and run `npm ci` in the worktree.
- **Run tests only when no agent is mid-change in that worktree.** Report failures in files you did not touch; do not silently fix another agent's work.
