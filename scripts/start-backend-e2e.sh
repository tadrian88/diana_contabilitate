#!/bin/sh
set -eu

docker compose up -d postgres
until docker compose exec -T postgres pg_isready -U diana -d diana >/dev/null 2>&1; do
  sleep 1
done

cd backend
export DATABASE_URL="postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable"
atlas migrate apply --env local
GOCACHE=/private/tmp/diana-go-cache go run ./cmd/devseed
exec env GOCACHE=/private/tmp/diana-go-cache HTTP_ADDRESS=127.0.0.1:8080 PIPELINE_DISPATCH_ENABLED=false go run ./cmd/api
