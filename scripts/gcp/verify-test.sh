#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
. "$ROOT_DIR/deploy/gcp-test.env"

EXPECTED_IP=$(gcloud compute addresses describe "$GLOBAL_IP_NAME" --global \
  --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --format='value(address)')

echo "Expected global IP: $EXPECTED_IP"
dig +short "$APP_HOST" A
dig +short "$API_HOST" A
curl --fail --silent --show-error --head "https://$APP_HOST/"
curl --fail --silent --show-error "https://$API_HOST/healthz"

expect_401() {
  path=$1
  status=$(curl --silent --output /dev/null --write-out '%{http_code}' "https://$APP_HOST$path")
  if [ "$status" != "401" ]; then
    echo "Expected anonymous 401 for $path, got $status" >&2
    exit 1
  fi
}

expect_401 /api/v1/clients
expect_401 /api/v1/invoices
expect_401 /api/v1/contracts
expect_401 /api/v1/rules
expect_401 /api/v1/validation-tasks
expect_401 /api/v1/clients/security-probe/onboarding
expect_401 /api/v1/clients/security-probe/spv
expect_401 /api/v1/clients/security-probe/invoices/security-probe/saga-export/artifact
gcloud compute ssl-certificates describe "$CERTIFICATE_NAME" --global \
  --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" \
  --format='yaml(name,managed.status,managed.domainStatus,expireTime)'
