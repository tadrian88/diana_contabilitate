#!/bin/sh
set -eu

docker compose up -d postgres redis
until docker compose exec -T postgres pg_isready -U diana -d diana >/dev/null 2>&1; do sleep 1; done
until docker compose exec -T redis redis-cli ping >/dev/null 2>&1; do sleep 1; done
docker compose exec -T redis redis-cli -n 15 FLUSHDB >/dev/null
docker compose exec -T postgres psql -U diana -d postgres -c "DROP DATABASE IF EXISTS diana_backend7_e2e WITH (FORCE)" >/dev/null
docker compose exec -T postgres psql -U diana -d postgres -c "CREATE DATABASE diana_backend7_e2e OWNER diana" >/dev/null

cd backend
export DATABASE_URL="postgresql://diana:diana@127.0.0.1:5442/diana_backend7_e2e?sslmode=disable"
export REDIS_URL="redis://127.0.0.1:6382/15"
export FRONTEND_BASE_URL="http://127.0.0.1:4179"
atlas migrate apply --env local
GOCACHE=/private/tmp/diana-go-cache go run ./cmd/devseed
DIANA_AUTH_PASSWORD='Diana-E2E-Only-2026!' GOCACHE=/private/tmp/diana-go-cache go run ./cmd/authuser provision --email demo.fixture@accountingtechco.test --all-clients

worker_pid=""
cleanup() {
  if [ -n "$worker_pid" ]; then
    kill "$worker_pid" 2>/dev/null || true
    wait "$worker_pid" 2>/dev/null || true
  fi
}
trap cleanup EXIT
trap 'exit 0' INT TERM

env GOCACHE=/private/tmp/diana-go-cache WORKER_HTTP_ADDRESS=127.0.0.1:8081 OUTBOX_POLL_INTERVAL=50ms go run ./cmd/worker &
worker_pid=$!
until curl --fail --silent http://127.0.0.1:8081/readyz >/dev/null 2>&1; do
  kill -0 "$worker_pid" 2>/dev/null || exit 1
  sleep 1
done

env GOCACHE=/private/tmp/diana-go-cache HTTP_ADDRESS=127.0.0.1:8080 PIPELINE_DISPATCH_ENABLED=false go run ./cmd/api
