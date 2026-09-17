# Authentication V1 test handoff

The code is implemented but behavior suites are intentionally user-run.

## Static/compile gate

```bash
cd /Users/adriantudoran/Projects/diana_contabilitate
npm run typecheck
cd backend
GOCACHE=/private/tmp/diana-go-cache go list ./...
GOCACHE=/private/tmp/diana-go-cache go test -run '^$' ./...
atlas migrate hash --dir file://migrations
cd ..
git diff --check
```

## Focused authentication behavior

```bash
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
GOCACHE=/private/tmp/diana-go-cache go test ./internal/authentication ./internal/platform/httpserver
```

## PostgreSQL integration

Apply migration `000016`, then:

```bash
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' \
  TEST_REDIS_URL='redis://127.0.0.1:6382/15' \
  GOCACHE=/private/tmp/diana-go-cache \
  go test -tags=integration ./internal/authentication
```

## Frontend unit and real browser journey

```bash
cd /Users/adriantudoran/Projects/diana_contabilitate
npm test -- --run src/features/auth/AuthFlow.test.tsx
npm run test:e2e:authentication
```

## Manual security checks

Before removing IAP, run against the candidate through its restricted validation path and confirm each response is 401 without cookies:

```bash
for path in \
  /api/v1/clients \
  /api/v1/invoices \
  /api/v1/contracts \
  /api/v1/rules \
  /api/v1/validation-tasks \
  /api/v1/clients/CLIENT_ID/spv \
  /api/v1/clients/CLIENT_ID/invoices/INVOICE_ID/saga-export/artifact
do
  curl -i "https://test.platform.accountingtechco.com${path}"
done
```

Also verify: wrong password and unknown email have the same 401 body; refresh preserves login; logout invalidates the session; browser back cannot use the app; a grant-only fixture cannot access another client; the ANAF callback still reaches the callback service with a valid OAuth state.
