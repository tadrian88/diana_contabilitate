#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$ROOT_DIR"
. "$ROOT_DIR/deploy/gcp-test.env"

for command_name in gcloud docker git; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "Missing required command: $command_name" >&2
    exit 1
  fi
done

if [ -n "$(git status --porcelain)" ]; then
  echo "Cloud deployment requires a clean committed working tree." >&2
  echo "Commit or stash all changes, then rerun make deploy-gcp-test." >&2
  exit 1
fi
RELEASE_COMMIT=$(git rev-parse HEAD)

ACTIVE_ACCOUNT=$(gcloud config get-value account 2>/dev/null)
if [ "$ACTIVE_ACCOUNT" != "$GCLOUD_ACCOUNT" ]; then
  echo "Expected gcloud account $GCLOUD_ACCOUNT, found $ACTIVE_ACCOUNT" >&2
  exit 1
fi

if [ "${CONFIRM_GCP_TEST_DEPLOY:-}" != "1" ]; then
  printf 'Deploy committed HEAD to %s in project %s? Type DEPLOY: ' "$RESOURCE_PREFIX" "$PROJECT_ID"
  read -r answer
  if [ "$answer" != "DEPLOY" ]; then
    echo "Deployment cancelled."
    exit 1
  fi
fi

echo "==> Running the complete release gate"
"$ROOT_DIR/scripts/release-check.sh"

if [ "$(git rev-parse HEAD)" != "$RELEASE_COMMIT" ] || [ -n "$(git status --porcelain)" ]; then
  echo "Repository state changed while the release gate was running; refusing Cloud deployment." >&2
  exit 1
fi

IMAGE_TAG="$(printf '%s' "$RELEASE_COMMIT" | cut -c1-12)-$(date -u +%Y%m%dT%H%M%SZ | tr '[:upper:]' '[:lower:]')"
REGISTRY="$REGION-docker.pkg.dev/$PROJECT_ID/$ARTIFACT_REPOSITORY"
BACKEND_IMAGE="$REGISTRY/backend:$IMAGE_TAG"
MIGRATE_IMAGE="$REGISTRY/migrate:$IMAGE_TAG"
FRONTEND_IMAGE="$REGISTRY/frontend:$IMAGE_TAG"

PRE_API_REVISION=$(gcloud run services describe "$API_SERVICE" --region="$REGION" --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --format='value(status.latestReadyRevisionName)')
PRE_CALLBACK_REVISION=$(gcloud run services describe "$CALLBACK_SERVICE" --region="$REGION" --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --format='value(status.latestReadyRevisionName)')
PRE_WORKER_REVISION=$(gcloud run services describe "$WORKER_SERVICE" --region="$REGION" --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --format='value(status.latestReadyRevisionName)')
PRE_FRONTEND_REVISION=$(gcloud run services describe "$FRONTEND_SERVICE" --region="$REGION" --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --format='value(status.latestReadyRevisionName)')

echo "==> Building and pushing immutable images: $IMAGE_TAG"
"$ROOT_DIR/scripts/gcp/build-push.sh" "$IMAGE_TAG"

echo "==> Updating and executing the migration job"
gcloud run jobs update "$MIGRATION_JOB" --image="$MIGRATE_IMAGE" \
  --update-labels="diana-release=$IMAGE_TAG" \
  --region="$REGION" --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --quiet
gcloud run jobs execute "$MIGRATION_JOB" \
  --region="$REGION" --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --wait

echo "==> Verifying migration status with the same immutable image"
gcloud run jobs update "$MIGRATION_STATUS_JOB" --image="$MIGRATE_IMAGE" \
  --update-labels="diana-release=$IMAGE_TAG" \
  --region="$REGION" --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --quiet
gcloud run jobs execute "$MIGRATION_STATUS_JOB" \
  --region="$REGION" --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --wait

echo "==> Rolling out backend services"
for service_name in "$API_SERVICE" "$CALLBACK_SERVICE" "$WORKER_SERVICE"; do
  gcloud run services update "$service_name" --image="$BACKEND_IMAGE" \
    --update-labels="diana-release=$IMAGE_TAG" \
    --region="$REGION" --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --quiet
done

echo "==> Rolling out frontend"
gcloud run services update "$FRONTEND_SERVICE" --image="$FRONTEND_IMAGE" \
  --update-labels="diana-release=$IMAGE_TAG" \
  --region="$REGION" --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --quiet

echo "==> Cloud smoke and anonymous-access checks"
"$ROOT_DIR/scripts/gcp/verify-test.sh"

echo "Cloud TEST release complete: $IMAGE_TAG"
echo "Previous revisions retained for manual rollback:"
echo "  $API_SERVICE=$PRE_API_REVISION"
echo "  $CALLBACK_SERVICE=$PRE_CALLBACK_REVISION"
echo "  $WORKER_SERVICE=$PRE_WORKER_REVISION"
echo "  $FRONTEND_SERVICE=$PRE_FRONTEND_REVISION"
