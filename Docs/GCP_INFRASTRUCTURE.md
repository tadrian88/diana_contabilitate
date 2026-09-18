# GCP TEST infrastructure

## Architecture decision

- Global external Application Load Balancer; no Cloud Run domain mapping.
- Cloud Run services for static frontend, API, public callback and persistent
  Asynq worker; a Cloud Run Job runs Atlas.
- The worker uses instance-based CPU, min/max one instance and port 8080 for
  health. This preserves exactly one embedded SPV scheduler authority.
- Cloud SQL PostgreSQL 17, single-zone TEST sizing, private IP, 10 GiB SSD,
  daily backups retained seven days, no production HA.
- Memorystore Redis 7 Basic 1 GiB with AUTH and private networking.
- Direct VPC egress replaces a billed Serverless VPC Access connector.
- Existing PostgreSQL bytea DocumentStore remains for TEST. Evaluate GCS for
  PROD based on volume, retention and restore evidence.

## Resource names

All exact non-secret names are in [`deploy/gcp-test.env`](../deploy/gcp-test.env).
TEST and future PROD must use separate projects, databases, Redis, secrets,
identities and domains.

## Current resources

| Resource | Name/status |
|---|---|
| Artifact Registry | `europe-central2/diana-test`, immutable tags |
| API identity | `diana-test-api@diana-508810.iam.gserviceaccount.com` |
| Worker identity | `diana-test-worker@diana-508810.iam.gserviceaccount.com` |
| Migration identity | `diana-test-migrate@diana-508810.iam.gserviceaccount.com` |
| Frontend identity | `diana-test-web@diana-508810.iam.gserviceaccount.com` |
| Frontend service | `diana-test-web`, revision `diana-test-web-00001-krj`, load-balancer ingress |
| API service | `diana-test-api`, revision `diana-test-api-00001-tl7`, currently IAP protected; Authentication V1 cutover pending |
| ANAF callback service | `diana-test-anaf-callback`, revision `diana-test-anaf-callback-00001-sbk`, exact public route only |
| Worker service | `diana-test-worker`, revision `diana-test-worker-00001-sj4`, singleton |
| VPC/subnet | `diana-test-vpc` / `diana-test-europe-central2` (`10.20.0.0/24`) |
| Cloud SQL | `diana-test-pg17`, PostgreSQL 17, private IP `172.26.0.3` |
| Memorystore | `diana-test-redis`, Redis 7.2, private IP `172.26.22.35:6379` |
| Load balancer IP | `diana-test-ip` = `136.69.63.17` |
| Managed certificate | `diana-test-cert-v3`, active for `test.platform.accountingtechco.com` and `api-test.accountingtechco.com` |

The canonical Cloud Run application origin is
`FRONTEND_BASE_URL=https://test.platform.accountingtechco.com` on both the API
and ANAF callback services. Do not use the obsolete `app-test` hostname; the
authentication origin check rejects it.

## Images

| Image | Digest |
|---|---|
| `backend:6e4b4b62bc4a-20260916t104203z` | `sha256:d0ea80e5ccde400e10062642ce46693e88d6b9fe0869bc5ee98ecc66d00f791c` |
| `migrate:6e4b4b62bc4a-20260916t111016z` | `sha256:fd0b50340191e66a81952e52d421f76e4478eed1cb600b28f487b9b1e0c3a964` |
| `frontend:6e4b4b62bc4a-20260916t104203z` | `sha256:53a2e2243d6240e4c5d61c62b14e65118e943ed6860054ce603219fa31c322c2` |

## Migration invariant

Latest repository migration is `000016_authentication_v1.sql`.
`backend/Dockerfile.migrate` packages Atlas 0.36.0 plus the checksummed
migration directory. Only `diana-test-migrate` may apply it. A second status
job must run `atlas migrate status --env cloud`; job exit alone is not final
proof.

## Cost profile

Primary recurring TEST costs will be the single always-CPU worker, Cloud SQL,
1 GiB Memorystore and the global load balancer. API/frontend/callback can scale
to zero. Artifact storage, builds, logs, backups and egress are variable. The
approved planning range is approximately USD 120–220/month, not a quote.
