# Google Cloud TEST deployment

Status on 2026-09-16: **PARTIALLY DEPLOYED / BLOCKED BY GOOGLE COMPUTE API TRANSITION**.

## Fixed environment

| Setting | Value |
|---|---|
| Project | `diana-508810` (`232660193801`) |
| Account | `adrian@co2later.com` |
| Region | `europe-central2` (Warsaw) |
| Root domain | `accountingtechco.com` |
| Frontend | `https://test.platform.accountingtechco.com` |
| API | `https://api-test.accountingtechco.com` |
| ANAF callback | `https://api-test.accountingtechco.com/api/v1/integrations/anaf/callback` |
| Release tag | `6e4b4b62bc4a-20260916t104203z` |

This is TEST, never PROD. `APP_ENV=cloud-test`, `SPV_ENVIRONMENT=PRODUCTION`,
`SAGA_MODE=file`, and production classification policy preserve the local code
paths while refusing fake adapters. No devseed is run.

## Current checkpoint

Created: Artifact Registry, dedicated identities, Secret Manager containers,
VPC/private service access, Cloud SQL, Memorystore, successful Atlas migration
and status jobs, frontend/API/callback/worker Cloud Run services, serverless
NEGs and the global HTTPS load balancer. IAP is enabled on frontend and normal
API backends; the isolated ANAF callback backend is intentionally public.

Current user action: add the two A records from `GCP_DNS_SQUARESPACE.md`.
Static IP is `136.69.63.17`; replacement managed certificate `diana-test-cert-v2` is
`PROVISIONING`. Do not register the ANAF OAuth callback until DNS resolves and
the certificate is `ACTIVE` for both hostnames.

## Build and release

Run all checks, select a new immutable tag containing the Git SHA, then:

```bash
./scripts/gcp/build-push.sh "$(git rev-parse --short=12 HEAD)-$(date -u +%Y%m%dT%H%M%SZ | tr A-Z a-z)"
```

Release order is fixed: infrastructure, database credentials, migrations,
migration status, API/callback, singleton worker, frontend, load balancer,
DNS, certificate ACTIVE, ANAF registration, then real OAuth and SPV sync.
API/worker startup never applies migrations.

## Access boundary (Authentication V1 candidate)

The currently deployed frontend and normal API backends have IAP enabled and
grant `roles/iap.httpsResourceAccessor` only to approved `co2later.com` testers.
The URL map exposes only the exact ANAF callback path through a separate public
backend pointed at `diana-test-anaf-callback`. Cloud Run ingress is
`internal-and-cloud-load-balancing`, preventing direct public `run.app`
bypass. The callback service must not receive a wildcard route.

Authentication V1 removes the demo RequestActor fallback. The candidate must be
deployed and proven while IAP remains in place before IAP is disabled. Do not
reverse that order. Before real SPV sync, anonymous requests to clients,
invoices, artifacts, settings and contract documents must be verified as 401.

## Authentication V1 deployment and provisioning

Build a new immutable backend/frontend/migration release, apply migration
`000016_authentication_v1.sql`, run Atlas status, deploy API/callback/worker and
frontend, and keep IAP enabled. In an IAP-authenticated browser, verify `/login`,
login/session/logout, and that a request without a Diana cookie reaches the app
but receives 401.

The API and callback services must use the exact public frontend origin below.
Authentication compares the request `Origin` against this value; the obsolete
`https://app-test.accountingtechco.com` value causes `403 ORIGIN_REJECTED`:

```bash
gcloud run services update "$API_SERVICE" \
  --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --region="$REGION" \
  --update-env-vars=FRONTEND_BASE_URL=https://test.platform.accountingtechco.com
gcloud run services update "$CALLBACK_SERVICE" \
  --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --region="$REGION" \
  --update-env-vars=FRONTEND_BASE_URL=https://test.platform.accountingtechco.com
```

Create the TEST user only after the migration and candidate API are healthy.
The following uses a temporary Secret Manager version so the password is never
in shell history, command arguments, an image or source. Replace `BACKEND_IMAGE`
with the immutable candidate image digest/tag:

```bash
cd /Users/adriantudoran/Projects/diana_contabilitate
. deploy/gcp-test.env
read -r -s -p 'Diana demo password: ' DIANA_DEMO_PASSWORD; printf '\n'
printf '%s' "$DIANA_DEMO_PASSWORD" | gcloud secrets create diana-test-demo-bootstrap-password \
  --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --replication-policy=automatic --data-file=-
unset DIANA_DEMO_PASSWORD
gcloud secrets add-iam-policy-binding diana-test-demo-bootstrap-password \
  --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" \
  --member="serviceAccount:diana-test-migrate@${PROJECT_ID}.iam.gserviceaccount.com" \
  --role=roles/secretmanager.secretAccessor

BACKEND_IMAGE='europe-central2-docker.pkg.dev/diana-508810/diana-test/backend:REPLACE_WITH_IMMUTABLE_TAG'
gcloud run jobs deploy diana-test-authuser \
  --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --region="$REGION" \
  --image="$BACKEND_IMAGE" --command=authuser \
  --args=provision,--email,demo@accountingtechco.com,--all-clients \
  --set-secrets=DATABASE_URL=diana-test-database-url:1,DIANA_AUTH_PASSWORD=diana-test-demo-bootstrap-password:latest \
  --service-account="diana-test-migrate@${PROJECT_ID}.iam.gserviceaccount.com" \
  --network="$VPC_NAME" --subnet="$SUBNET_NAME" --vpc-egress=private-ranges-only \
  --max-retries=0 --task-timeout=10m
gcloud run jobs execute diana-test-authuser \
  --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --region="$REGION" --wait
```

Expected output is `created`. A repeated provision prints `already exists`,
exits nonzero and does not overwrite the credential. After successful login is
verified, remove the one-shot job and destroy the temporary password secret:

```bash
gcloud run jobs delete diana-test-authuser --quiet \
  --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT" --region="$REGION"
gcloud secrets delete diana-test-demo-bootstrap-password --quiet \
  --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT"
```

For an explicit password reset, repeat the temporary-secret procedure but use
`--args=reset-password,--email,demo@accountingtechco.com`. Reset revokes all
existing sessions. It never silently overwrites through `provision`.

## IAP cutover after the security gate

Before: anonymous browser -> IAP Google login -> frontend/API -> demo actor.

After: anonymous browser -> Diana `/login` -> PostgreSQL session -> authenticated
RequestActor -> existing client authorization. ANAF callback remains on the
separate exact public path.

Only after all local/integration/browser gates and IAP-protected candidate checks
pass, allow load-balancer invocation and disable IAP on both normal backends:

```bash
gcloud run services add-iam-policy-binding "$FRONTEND_SERVICE" --member=allUsers --role=roles/run.invoker \
  --region="$REGION" --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT"
gcloud run services add-iam-policy-binding "$API_SERVICE" --member=allUsers --role=roles/run.invoker \
  --region="$REGION" --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT"
gcloud compute backend-services update diana-test-web-backend --global --iap=disabled \
  --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT"
gcloud compute backend-services update diana-test-api-backend --global --iap=disabled \
  --project="$PROJECT_ID" --account="$GCLOUD_ACCOUNT"
```

Cloud Run ingress remains `internal-and-cloud-load-balancing`, so `run.app`
cannot bypass the load balancer. Immediately verify anonymous root shows Diana
login and every sensitive anonymous API request returns 401. If either fails,
re-enable IAP before any further testing; do not leave a partially cut-over
environment.

## Rollback

List revisions and move traffic to the prior revision; never automatically
roll back database migrations:

```bash
gcloud run revisions list --service=diana-test-api --region=europe-central2 --project=diana-508810
gcloud run services update-traffic diana-test-api --to-revisions=REVISION=100 --region=europe-central2 --project=diana-508810
gcloud run revisions list --service=diana-test-web --region=europe-central2 --project=diana-508810
gcloud run services update-traffic diana-test-web --to-revisions=REVISION=100 --region=europe-central2 --project=diana-508810
```

Database rollback is a separate reviewed restore/forward-fix decision.
