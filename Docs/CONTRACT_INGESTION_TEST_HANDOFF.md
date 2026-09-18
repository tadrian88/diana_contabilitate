# Contract Ingestion Hardening V1 — Test Handoff

Run from the repository root. Use an isolated test database for integration/E2E commands.

## Static and compile gates

```bash
npm run typecheck
cd backend
GOCACHE=/private/tmp/diana-go-cache go test ./... -run '^$'
GOCACHE=/private/tmp/diana-go-cache go test -tags=integration ./internal/platform/postgres -run '^$'
atlas migrate hash --dir file://migrations
cd ..
git diff --check
```

## Runtime unit/frontend gates

```bash
cd backend
GOCACHE=/private/tmp/diana-go-cache go test ./...
cd ..
npm test
npm run build
```

## Migration and PostgreSQL gates

```bash
cd backend
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' atlas migrate status --env local
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' atlas migrate apply --env local
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' GOCACHE=/private/tmp/diana-go-cache go test -tags=integration ./internal/platform/postgres
```

## Contract Playwright journey

Follow the existing isolated DB/Redis seed prerequisites, then:

```bash
npm run test:e2e:contract-ingestion
```

The journey covers upload, visible PDF, evidence navigation, reviewed correction, service terms, indefinite duration, confirmation, discard/replacement, and resume.

## Cloud TEST retest

1. Build/deploy one immutable backend/frontend/migration release and apply `000017`; do not alter IAP/session security.
2. Log in as the real test user and open both invoices currently in `AWAITING_CONTRACT`.
3. Upload the real contract once.
4. In DevTools Network verify the worker `.mjs` is `200 application/javascript`; verify the protected PDF file request is authenticated and subsequent range requests return `206`, `Accept-Ranges: bytes`, and `Content-Range`.
5. Verify the PDF renders fit-width, page/evidence navigation works, and retry/download remain authenticated.
6. Verify `21592770`, `RO21592770`, `ro21592770`, and `RO 21592770` do not create a buyer blocker; confirm VAT registration is not inferred or changed.
7. Correct an AI value and verify the blocker disappears from the current reviewed state.
8. Select indefinite duration and verify no end date is required or fabricated.
9. Review the 500 RON accounting fixed fee and 50 RON/employee payroll unit rate; leave unevidenced quantity/frequency unknown.
10. Confirm once, wait for `ContractAvailable`, and verify both invoices are reevaluated by the existing resume path.
11. Separately upload a wrong unconfirmed document, discard it, and confirm the invoices/tasks remain waiting before uploading its replacement.

Expected journey: Invoice A + Invoice B → `AWAITING_CONTRACT` → one upload → PDF renders → AI metadata/service proposal → reviewed corrections → indefinite term → confirm → `ContractAvailable` → both invoices reevaluated.
