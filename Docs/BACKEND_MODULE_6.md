# Backend Module 6 — Frontend/API Integration

Status: approved / frozen.

## Authority map

| Capability | Previous normal source | Module 6 normal source | Remaining mock use |
| --- | --- | --- | --- |
| Clients | API in hybrid mode | Go/PostgreSQL API | frozen/component tests |
| Invoice list, Dashboard, Clients metrics | mock | complete backend invoice list | frozen/component tests |
| Invoice detail, lines, pipeline, history | API with silent invoice fallback | Go/PostgreSQL API only | frozen/component tests |
| Validation tasks and missing-contract request | API with mutation fallback | Go/PostgreSQL API only | frozen/component tests |
| Contracts, matching, confirmation | API with detail/mutation fallback | Go/PostgreSQL API only | frozen/component tests |
| Classification and review | API with mutation fallback | Go/PostgreSQL API only | frozen/component tests |
| Rules, versions, overrides | API | Go/PostgreSQL API only | frozen/component tests |
| Automatic progression | React timer for mock; API runtime inactive | backend outbox in API mode; React timer mock-only | frozen mock journey |

`VITE_BACKEND_READS_ENABLED=true` selects the API repository directly. Any
other value selects the mock repository directly. There is no entity-level
mixing and no API 404-to-mock substitution.

## API and query behavior

Module 6 adds `GET /api/v1/invoices` with optional `clientId`. It intentionally
returns the complete small dataset and complete frontend DTOs. Approved search,
pipeline/SAGA/attention filters, and sorting remain client-side. This is correct
for the milestone and avoids both partial-page KPI errors and premature generic
query infrastructure. Pagination or a narrow aggregate endpoint is a documented
future scaling decision.

Central query keys cover clients, invoice lists/details, tasks, contract
lists/details/associations, and rule lists/details. API invoice lists poll once
per second so Dashboard and cross-screen derived state observe backend changes.
Invoice detail polls faster only in automatic states and stops in human blockers
or terminal states. Successful invoice commands update the exact detail response
and invalidate invoices, tasks, and contracts. Errors, including optimistic
conflicts, refetch the current detail rather than overwriting backend state.

## Backend pipeline authority

Module 6 proved backend ownership using an API-local dispatcher. Module 7
supersedes only this execution topology: normal runtime now uses `cmd/worker`
with Redis/Asynq and the API-local dispatcher defaults off.

The approved backend Modules 1–5 Playwright launcher explicitly sets the flag
to false because those suites assert already-materialized fixture states. The
Module 6 launcher now starts the dedicated worker. `AutomaticWorkflowRunner` can call transitions
only through the explicit `MockWorkflowRepository`; the API implementation does
not expose generic transitions.

## Seed/reset

`go run ./cmd/devseed` is the single development reset path. Module 6 cleanup
deletes its mutable fixture graph in dependency order and recreates it with
deterministic IDs. Earlier demo fixtures are reset by their owning seed slices.
Pending continuations for already-materialized earlier fixtures are discarded
before the live local dispatcher starts, preventing fixture timing from changing
the approved screens.

Dedicated Module 6 fixtures cover a full contract-to-classification-to-export
journey, ready for SAGA, exported, terminal duplicate, stable processing, SAGA
failure, persistent history, and two isolated mutable rule origins. Existing
reset fixtures independently cover multiple/incompatible/no contract and
classification review cases. Re-running devseed resets all mutations.

## Real backend E2E

`playwright.backend6.config.ts` starts PostgreSQL, applies Atlas migrations,
runs devseed, starts the API with dispatch enabled, and starts Vite in explicit
API mode. `e2e/backend-module6.spec.ts` contains scenarios A–N from the approved
brief, including no fallback and a durable full accountant journey. Fixtures are
not reused across mutating scenarios.

Run the complete automated backend journey from the repository root with:

```sh
npm run test:e2e:backend6
```

For manual development, start `./scripts/start-backend6-e2e.sh` in one terminal,
then start the frontend in another:

```sh
VITE_BACKEND_READS_ENABLED=true VITE_API_PROXY_TARGET=http://127.0.0.1:8080 npm run dev
```

## Database and migration at Module 6 approval

No schema or migration change is required. Migration history remains
`000001`–`000006`.

## Remaining mocks and deferred work

`MockInvoiceRepository`, mock scenarios, and the timer runner remain solely for
the frozen 23-scenario Playwright suite and isolated frontend tests. They are not
reachable in explicit API mode. Real SPV, parsers, OCR/import, real SAGA,
Auth/RBAC, final business policies, and
automatic missing-contract resumption remain deferred.
