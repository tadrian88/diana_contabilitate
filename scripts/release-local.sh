#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT_DIR"

"$ROOT_DIR/scripts/release-check.sh"

echo "==> Applying local migrations and rebuilding the local deployment"
make migrate
docker compose up -d --build --wait

wait_for_http() {
  service_name=$1
  url=$2
  attempt=1
  max_attempts=30

  while ! curl --noproxy '*' --fail --silent --max-time 3 "$url" >/dev/null 2>&1; do
    if [ "$attempt" -ge "$max_attempts" ]; then
      echo "Local health check failed for $service_name: $url" >&2
      curl --noproxy '*' --fail --silent --show-error --max-time 3 "$url" >/dev/null || true
      docker compose ps >&2
      docker compose logs --tail=100 "$service_name" >&2 || true
      exit 1
    fi
    attempt=$((attempt + 1))
    sleep 1
  done
}

echo "==> Local deployment health"
wait_for_http api http://127.0.0.1:8080/readyz
wait_for_http worker http://127.0.0.1:8081/readyz
wait_for_http frontend http://127.0.0.1:5173/
docker compose ps

echo "Local release complete: http://127.0.0.1:5173"
