#!/bin/sh
set -eu

docker compose up -d postgres redis
until docker compose exec -T postgres pg_isready -U diana -d diana >/dev/null 2>&1; do sleep 1; done
until docker compose exec -T redis redis-cli ping >/dev/null 2>&1; do sleep 1; done
docker compose exec -T redis redis-cli -n 13 FLUSHDB >/dev/null
docker compose exec -T postgres psql -U diana -d postgres -c "DROP DATABASE IF EXISTS diana_client_onboarding_e2e WITH (FORCE)" >/dev/null
docker compose exec -T postgres psql -U diana -d postgres -c "CREATE DATABASE diana_client_onboarding_e2e OWNER diana" >/dev/null

cd backend
export DATABASE_URL="postgresql://diana:diana@127.0.0.1:5442/diana_client_onboarding_e2e?sslmode=disable"
export REDIS_URL="redis://127.0.0.1:6382/13"
export SPV_ENABLED=true
export SPV_ENVIRONMENT=TEST
export SPV_API_BASE_URL="http://127.0.0.1:8090"
export SPV_TOKEN_URL="http://127.0.0.1:8090/token"
export SPV_AUTHORIZE_URL="http://127.0.0.1:8090/authorize"
export SPV_OAUTH_CLIENT_ID="fake-app"
export SPV_OAUTH_CLIENT_SECRET="fake-secret"
export SPV_TOKEN_ENCRYPTION_KEY="0101010101010101010101010101010101010101010101010101010101010101"
export SPV_OAUTH_REDIRECT_URI="http://127.0.0.1:8080/api/v1/integrations/anaf/callback"
export FRONTEND_BASE_URL="http://127.0.0.1:4180"
export SPV_SYNC_INTERVAL="1s"
atlas migrate apply --env local
GOCACHE=/private/tmp/diana-go-cache go run ./cmd/devseed

fake_pid=""; worker_pid=""
cleanup() {
  [ -z "$worker_pid" ] || kill "$worker_pid" 2>/dev/null || true
  [ -z "$fake_pid" ] || kill "$fake_pid" 2>/dev/null || true
  [ -z "$worker_pid" ] || wait "$worker_pid" 2>/dev/null || true
  [ -z "$fake_pid" ] || wait "$fake_pid" 2>/dev/null || true
}
trap cleanup EXIT
trap 'exit 0' INT TERM

GOCACHE=/private/tmp/diana-go-cache go run ./cmd/fakeanaf -buyer-cui RO12345678 &
fake_pid=$!
until curl --fail --silent http://127.0.0.1:8090/token >/dev/null 2>&1; do kill -0 "$fake_pid" 2>/dev/null || exit 1; sleep 1; done

env GOCACHE=/private/tmp/diana-go-cache WORKER_HTTP_ADDRESS=127.0.0.1:8081 OUTBOX_POLL_INTERVAL=50ms go run ./cmd/worker &
worker_pid=$!
until curl --fail --silent http://127.0.0.1:8081/readyz >/dev/null 2>&1; do kill -0 "$worker_pid" 2>/dev/null || exit 1; sleep 1; done

env GOCACHE=/private/tmp/diana-go-cache HTTP_ADDRESS=127.0.0.1:8080 PIPELINE_DISPATCH_ENABLED=false go run ./cmd/api
