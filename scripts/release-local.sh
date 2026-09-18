#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT_DIR"

"$ROOT_DIR/scripts/release-check.sh"

echo "==> Applying local migrations and rebuilding the local deployment"
make migrate
docker compose up -d --build --wait

echo "==> Local deployment health"
curl --fail --silent --show-error http://127.0.0.1:8080/readyz >/dev/null
curl --fail --silent --show-error http://127.0.0.1:8081/readyz >/dev/null
curl --fail --silent --show-error --head http://127.0.0.1:5173/ >/dev/null
docker compose ps

echo "Local release complete: http://127.0.0.1:5173"
