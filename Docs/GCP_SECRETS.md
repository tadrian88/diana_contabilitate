# GCP TEST secrets

Secret values are never committed or pasted into documentation.

| SECRET NAME | USED BY | SOURCE | ROTATION IMPACT |
|---|---|---|---|
| `diana-test-database-url` | API, callback, worker, migration | Generated dedicated `diana_app` Cloud SQL credential | Coordinated DB password and all revisions/jobs |
| `diana-test-redis-url` | API, callback, worker | Memorystore private address and AUTH string | Coordinated Redis auth and all revisions |
| `diana-test-spv-oauth-client-id` | API, callback, worker | ANAF registered application | OAuth initiation/token refresh stops until revisions update |
| `diana-test-spv-oauth-client-secret` | API, callback, worker | ANAF registered application | OAuth/token refresh stops until revisions update |
| `diana-test-spv-token-encryption-key` | API, callback, worker | Existing stable 32-byte hex key | Existing stored tokens become unreadable; requires designed re-encryption/reconnect |
| `diana-test-gemini-api-key` | API, worker | Existing Gemini key | Contract extraction unavailable until revisions update |

Verified on 2026-09-17: the token-encryption key, Gemini key, database URL and
Redis URL use enabled version 1. The ANAF OAuth client ID and client secret use
enabled version 2; their superseded version 1 is destroyed. Cloud Run releases
must pin the current enabled versions explicitly.

To rotate ANAF values, enter replacements locally without shell history or
chat exposure:

```bash
./scripts/gcp/set-secret.sh spv-oauth-client-id
./scripts/gcp/set-secret.sh spv-oauth-client-secret
```

Runtime identities receive accessor only on the secrets they consume. The
migration identity can access only `diana-test-database-url`. The API identity
(also used by the isolated callback service) and worker identity have
`roles/secretmanager.secretAccessor` on the three SPV secrets they consume.
After adding a rotated version, update all three Cloud Run services to the same
new client-ID/client-secret version pair; do not leave OAuth start, callback
exchange and worker refresh on different versions.
