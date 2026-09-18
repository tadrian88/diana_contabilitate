#!/bin/sh
set -eu

docker compose up -d postgres redis
until docker compose exec -T postgres pg_isready -U diana -d diana >/dev/null 2>&1; do sleep 1; done
until docker compose exec -T redis redis-cli ping >/dev/null 2>&1; do sleep 1; done
docker compose exec -T redis redis-cli -n 15 FLUSHDB >/dev/null
docker compose exec -T postgres psql -U diana -d postgres -c "DROP DATABASE IF EXISTS diana_saga_export_e2e WITH (FORCE)" >/dev/null
docker compose exec -T postgres psql -U diana -d postgres -c "CREATE DATABASE diana_saga_export_e2e OWNER diana" >/dev/null

cd backend
export DATABASE_URL="postgresql://diana:diana@127.0.0.1:5442/diana_saga_export_e2e?sslmode=disable"
export REDIS_URL="redis://127.0.0.1:6382/15"
export SAGA_MODE="file"
atlas migrate apply --env local
SEED_MODULE7=false SEED_SAGA_UX=true GOCACHE=/private/tmp/diana-go-cache go run ./cmd/devseed

worker_pid=""
cleanup() {
  if [ -n "$worker_pid" ]; then
    kill "$worker_pid" 2>/dev/null || true
    wait "$worker_pid" 2>/dev/null || true
  fi
}
trap cleanup EXIT
trap 'exit 0' INT TERM

GOCACHE=/private/tmp/diana-go-cache WORKER_HTTP_ADDRESS=127.0.0.1:8081 OUTBOX_POLL_INTERVAL=50ms go run ./cmd/worker &
worker_pid=$!
until curl --fail --silent http://127.0.0.1:8081/readyz >/dev/null 2>&1; do
  kill -0 "$worker_pid" 2>/dev/null || exit 1
  sleep 1
done

GOCACHE=/private/tmp/diana-go-cache HTTP_ADDRESS=127.0.0.1:8080 PIPELINE_DISPATCH_ENABLED=false go run ./cmd/api
