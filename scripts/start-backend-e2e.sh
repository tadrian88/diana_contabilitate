#!/bin/sh
set -eu

docker compose up -d postgres redis
until docker compose exec -T postgres pg_isready -U diana -d diana >/dev/null 2>&1; do
  sleep 1
done
until docker compose exec -T redis redis-cli ping >/dev/null 2>&1; do
  sleep 1
done
docker compose exec -T redis redis-cli -n 10 FLUSHDB >/dev/null
docker compose exec -T postgres psql -U diana -d postgres -c "DROP DATABASE IF EXISTS diana_backend_e2e WITH (FORCE)" >/dev/null
docker compose exec -T postgres psql -U diana -d postgres -c "CREATE DATABASE diana_backend_e2e OWNER diana" >/dev/null

cd backend
export DATABASE_URL="postgresql://diana:diana@127.0.0.1:5442/diana_backend_e2e?sslmode=disable"
export REDIS_URL="redis://127.0.0.1:6382/10"
export FRONTEND_BASE_URL="${E2E_FRONTEND_BASE_URL:?E2E_FRONTEND_BASE_URL is required}"
atlas migrate apply --env local
GOCACHE=/private/tmp/diana-go-cache go run ./cmd/devseed
DIANA_AUTH_PASSWORD='Diana-E2E-Only-2026!' GOCACHE=/private/tmp/diana-go-cache go run ./cmd/authuser provision --email demo.fixture@accountingtechco.test --all-clients
exec env GOCACHE=/private/tmp/diana-go-cache "HTTP_ADDRESS=${E2E_BACKEND_HTTP_ADDRESS:-127.0.0.1:8080}" PIPELINE_DISPATCH_ENABLED=false go run ./cmd/api
