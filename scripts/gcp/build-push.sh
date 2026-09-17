#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
. "$ROOT_DIR/deploy/gcp-test.env"

IMAGE_TAG=${1:-}
if [ -z "$IMAGE_TAG" ]; then
  echo "usage: $0 IMMUTABLE_TAG" >&2
  exit 2
fi

REGISTRY="$REGION-docker.pkg.dev/$PROJECT_ID/$ARTIFACT_REPOSITORY"
gcloud auth configure-docker "$REGION-docker.pkg.dev" --account="$GCLOUD_ACCOUNT" --quiet

docker build --platform linux/amd64 -t "$REGISTRY/backend:$IMAGE_TAG" "$ROOT_DIR/backend"
docker build --platform linux/amd64 -f "$ROOT_DIR/backend/Dockerfile.migrate" -t "$REGISTRY/migrate:$IMAGE_TAG" "$ROOT_DIR/backend"
docker build --platform linux/amd64 -f "$ROOT_DIR/Dockerfile.frontend" \
  --build-arg VITE_BACKEND_READS_ENABLED=true \
  --build-arg 'VITE_ENVIRONMENT_LABEL=TEST — REAL INTEGRATIONS' \
  -t "$REGISTRY/frontend:$IMAGE_TAG" "$ROOT_DIR"

docker push "$REGISTRY/backend:$IMAGE_TAG"
docker push "$REGISTRY/migrate:$IMAGE_TAG"
docker push "$REGISTRY/frontend:$IMAGE_TAG"
