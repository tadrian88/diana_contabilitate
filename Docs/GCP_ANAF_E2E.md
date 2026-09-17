# GCP TEST ANAF / SPV end-to-end

## Exact registration value

Register this callback only after DNS resolves and the managed certificate is
ACTIVE:

```text
https://api-test.accountingtechco.com/api/v1/integrations/anaf/callback
```

In the ANAF developer portal, authenticate as an application developer, open
**Editare profil OAuth → Gestionare aplicații**, select the e-Factura service,
and enter the exact Callback URL above. Store the resulting client ID and
client secret with `scripts/gcp/set-secret.sh`; never paste them into source,
documentation or chat.

## Authoritative endpoint verification

Retrieved 2026-09-16 from ANAF's official OAuth instructions and developer
registration page:

- Authorize: `https://logincert.anaf.ro/anaf-oauth2/v1/authorize`
- Token: `https://logincert.anaf.ro/anaf-oauth2/v1/token`
- Protected e-Factura base configured by Diana:
  `https://api.anaf.ro/prod/FCTEL/rest`
- Diana operations: `/listaMesajePaginatieFactura` and `/descarcare`

The official OAuth document confirms Authorization Code, registered callback,
client ID/client secret, request-header authorization and the authorize host.
The application preserves random 256-bit state, hashed-at-rest state,
single-use consumption, expiry, client/environment binding, idempotent OAuth
start, safe callback errors and an explicit configured redirect URI; it does
not derive callback URLs from Host or forwarded headers.

## Enable SPV on the three Cloud Run services

Run these commands from the repository root after the exact callback above is
registered in the ANAF application. Secret payloads must never be placed in the
command line: the services consume the already-provisioned Secret Manager
versions. Pinning versions rather than using `latest` keeps the release
reproducible.

Although this is the Diana GCP TEST deployment, `SPV_ENVIRONMENT=PRODUCTION`
deliberately means the real ANAF e-Factura environment and official production
FCTEL endpoint. Do not change it to `TEST` merely because the Diana hostname
contains `test`.

Capture the current revisions first so traffic can be rolled back without
guessing revision names:

```bash
cd /Users/adriantudoran/Projects/diana_contabilitate
. deploy/gcp-test.env

PRE_SPV_API_REV=$(gcloud run services describe "$API_SERVICE" \
  --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --region="$REGION" \
  --format='value(status.latestReadyRevisionName)')
PRE_SPV_CALLBACK_REV=$(gcloud run services describe "$CALLBACK_SERVICE" \
  --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --region="$REGION" \
  --format='value(status.latestReadyRevisionName)')
PRE_SPV_WORKER_REV=$(gcloud run services describe "$WORKER_SERVICE" \
  --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --region="$REGION" \
  --format='value(status.latestReadyRevisionName)')
printf 'API=%s\nCALLBACK=%s\nWORKER=%s\n' \
  "$PRE_SPV_API_REV" "$PRE_SPV_CALLBACK_REV" "$PRE_SPV_WORKER_REV"
```

Enable SPV and mount OAuth credential versions 2 on the API, callback and
worker. Setting the canonical frontend URL on all three also removes the old
`app-test.accountingtechco.com` value from the worker:

```bash
for SERVICE in "$API_SERVICE" "$CALLBACK_SERVICE" "$WORKER_SERVICE"; do
  gcloud run services update "$SERVICE" \
    --project="$PROJECT_ID" \
    --account="$GCLOUD_ACCOUNT" \
    --region="$REGION" \
    --update-env-vars="SPV_ENABLED=true,FRONTEND_BASE_URL=https://$APP_HOST" \
    --update-secrets="SPV_OAUTH_CLIENT_ID=diana-test-spv-oauth-client-id:2,SPV_OAUTH_CLIENT_SECRET=diana-test-spv-oauth-client-secret:2"
done
```

The services already mount the stable
`SPV_TOKEN_ENCRYPTION_KEY=diana-test-spv-token-encryption-key:1`; do not replace
or regenerate that key. Each update creates a new Cloud Run revision while
preserving the existing image, networking, scaling and unrelated environment
configuration.

Verify all new revisions are ready and the public API remains healthy:

```bash
for SERVICE in "$API_SERVICE" "$CALLBACK_SERVICE" "$WORKER_SERVICE"; do
  gcloud run services describe "$SERVICE" \
    --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --region="$REGION" \
    --format='table(metadata.name,status.latestReadyRevisionName,status.conditions[0].status)'
done

curl --fail --silent --show-error "https://$API_HOST/healthz"
curl --fail --silent --show-error "https://$API_HOST/readyz"
./scripts/gcp/verify-test.sh
```

After signing in, open the created client. The ANAF card must no longer display
"Integrarea ANAF nu este configurată pentru acest mediu", and the **Conectează
ANAF** button must be enabled. Starting OAuth is the final proof that the
registered callback and credential pair agree.

If one of the new revisions is unhealthy, route that service back to the
captured revision. Run only the affected command(s), from the same shell in
which the `PRE_SPV_*` variables were captured:

```bash
gcloud run services update-traffic "$API_SERVICE" --to-revisions="$PRE_SPV_API_REV=100" \
  --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --region="$REGION"
gcloud run services update-traffic "$CALLBACK_SERVICE" --to-revisions="$PRE_SPV_CALLBACK_REV=100" \
  --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --region="$REGION"
gcloud run services update-traffic "$WORKER_SERVICE" --to-revisions="$PRE_SPV_WORKER_REV=100" \
  --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --region="$REGION"
```

## Gates

1. IAP and URL-map security gate passes anonymously.
2. Exact callback is registered and cloud secrets have enabled versions.
3. API/callback/worker revisions run with `APP_ENV=cloud-test`,
   `SPV_ENABLED=true`, `SPV_ENVIRONMENT=PRODUCTION`, official endpoints and
   `SAGA_MODE=file`.
4. Create the real client/profile/SAGA configuration through IAP.
5. Start OAuth; the user performs the qualified certificate/PIN step.
6. Verify callback returns to
   `https://test.platform.accountingtechco.com/clients/<client-id>` and status is
   CONNECTED.
7. Request one manual sync and observe Asynq processing.

Success requires a real message to be discovered, downloaded, raw-archived,
parsed, persisted and linked, with Invoice and InvoiceLines created and the
pipeline started. `AWAITING_CONTRACT` + `MISSING_CONTRACT` is correct when no
contract exists. With no approved production accounting pack, review-only /
`AWAITING_REVIEW` is correct; TEST_ONLY rules must never execute.
