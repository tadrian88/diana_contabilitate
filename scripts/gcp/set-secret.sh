#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
. "$ROOT_DIR/deploy/gcp-test.env"

case "${1:-}" in
  spv-oauth-client-id|spv-oauth-client-secret|spv-token-encryption-key|gemini-api-key)
    SECRET_NAME="diana-test-$1"
    ;;
  *)
    echo "usage: $0 {spv-oauth-client-id|spv-oauth-client-secret|spv-token-encryption-key|gemini-api-key}" >&2
    exit 2
    ;;
esac

SECRET_FILE=$(mktemp /private/tmp/diana-secret.XXXXXX)
trap 'rm -f "$SECRET_FILE"' EXIT HUP INT TERM
chmod 600 "$SECRET_FILE"
printf 'Enter value for %s: ' "$SECRET_NAME" >&2
stty -echo
IFS= read -r SECRET_VALUE
stty echo
printf '\n' >&2
printf '%s' "$SECRET_VALUE" > "$SECRET_FILE"
unset SECRET_VALUE
test -s "$SECRET_FILE"

gcloud secrets versions add "$SECRET_NAME" \
  --data-file="$SECRET_FILE" \
  --project="$PROJECT_ID" \
  --account="$GCLOUD_ACCOUNT"
