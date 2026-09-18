#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
GO_CACHE=${GOCACHE:-/private/tmp/diana-go-cache}
TEST_DATABASE_NAME=diana_release_gate
TEST_DATABASE_URL="postgresql://diana:diana@127.0.0.1:5442/$TEST_DATABASE_NAME?sslmode=disable"
POSTGRES_TEST_REDIS_URL=redis://127.0.0.1:6382/12
WORKER_TEST_REDIS_URL=redis://127.0.0.1:6382/13

cd "$ROOT_DIR"

for command_name in npm go atlas docker git; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "Missing required command: $command_name" >&2
    exit 1
  fi
done

echo "==> Installing locked frontend dependencies"
npm ci

echo "==> Static checks and production build"
git diff --check
(cd backend && atlas migrate validate --dir file://migrations)
(cd backend && GOCACHE="$GO_CACHE" go vet ./...)
npm run typecheck
npm run build

echo "==> Runtime unit tests"
(cd backend && GOCACHE="$GO_CACHE" go test -count=1 ./...)
npm test

echo "==> Starting isolated PostgreSQL and Redis test dependencies"
docker compose up -d postgres redis
until docker compose exec -T postgres pg_isready -U diana -d diana >/dev/null 2>&1; do sleep 1; done
until docker compose exec -T redis redis-cli ping >/dev/null 2>&1; do sleep 1; done
docker compose exec -T redis redis-cli -n 12 FLUSHDB >/dev/null
docker compose exec -T redis redis-cli -n 13 FLUSHDB >/dev/null
docker compose exec -T postgres psql -U diana -d postgres -c "DROP DATABASE IF EXISTS $TEST_DATABASE_NAME WITH (FORCE)" >/dev/null
docker compose exec -T postgres psql -U diana -d postgres -c "CREATE DATABASE $TEST_DATABASE_NAME OWNER diana" >/dev/null

echo "==> Applying and verifying migrations on the isolated test database"
(cd backend && TEST_DATABASE_URL="$TEST_DATABASE_URL" atlas migrate apply --env test)
(cd backend && TEST_DATABASE_URL="$TEST_DATABASE_URL" atlas migrate status --env test)

echo "==> PostgreSQL integration tests"
(cd backend && TEST_DATABASE_URL="$TEST_DATABASE_URL" TEST_REDIS_URL="$POSTGRES_TEST_REDIS_URL" GOCACHE="$GO_CACHE" go test -count=1 -tags=integration ./...)

echo "==> PostgreSQL outbox/Asynq integration tests"
docker compose exec -T redis redis-cli -n 12 FLUSHDB >/dev/null
(cd backend && TEST_DATABASE_URL="$TEST_DATABASE_URL" TEST_REDIS_URL="$POSTGRES_TEST_REDIS_URL" GOCACHE="$GO_CACHE" go test -count=1 -tags='integration redis_integration' ./internal/platform/postgres -run '^(TestOutboxAsynqWorkerCompletesAndDuplicateJobIsIdempotent|TestOutboxAsynqWorkerCompletesFullAutomaticPipeline|TestContractAvailableOutboxAsynqAutomaticallyResumesMissingInvoice)$')

echo "==> Worker/Asynq integration tests"
docker compose exec -T redis redis-cli -n 13 FLUSHDB >/dev/null
(cd backend && TEST_REDIS_URL="$WORKER_TEST_REDIS_URL" GOCACHE="$GO_CACHE" go test -count=1 -tags=redis_integration ./internal/workerruntime)

echo "==> Stopping local app processes before browser suites"
previously_running=""
for service_name in api worker frontend; do
  if docker compose ps --services --filter status=running | grep -Fx "$service_name" >/dev/null 2>&1; then
    previously_running="$previously_running $service_name"
  fi
done
restore_local_services() {
  for service_name in $previously_running; do
    docker compose start "$service_name" >/dev/null 2>&1 || true
  done
}
trap restore_local_services EXIT
docker compose stop api worker frontend >/dev/null 2>&1 || true

echo "==> Playwright suites"
npm run test:e2e
npm run test:e2e:backend1
npm run test:e2e:backend3
npm run test:e2e:backend4
npm run test:e2e:backend5
npm run test:e2e:backend6
npm run test:e2e:backend7
npm run test:e2e:accounting-v2
npm run test:e2e:spv-connection
npm run test:e2e:saga-export-ux
npm run test:e2e:client-onboarding
npm run test:e2e:authentication
npm run test:e2e:contract-ingestion

echo "==> Release gate passed"
