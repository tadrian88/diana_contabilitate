# Local → Google Cloud parity

Prepared 2026-09-15 and updated 2026-09-17. The single local `compose.yaml` provides PostgreSQL 17, Redis 7, API, worker, frontend, and one-shot Atlas migrations. The first GCP TEST deployment is now partially executed: images, identities, secrets and private frontend exist, while Compute-dependent resources are blocked by a Google API transition. See `GCP_TEST_DEPLOYMENT.md`.

| CONCERN | LOCAL | GCP TEST | GCP PROD | APPLICATION CODE DIFFERENCE? | OPEN DECISION |
|---|---|---|---|---|---|
| Frontend | Vite hot reload, 5173, same-origin API proxy | compiled frontend + same-origin API routing | compiled frontend + same-origin API routing | NONE (build/routing config) | hosting, gateway/domain and access |
| API | Go cmd/api, 8080 | Cloud Run service | Cloud Run service | NONE for application services | inject HTTP_ADDRESS=:8080 to match container port; auth boundary required |
| Worker | cmd/worker, health 8081, Redis/Asynq + dispatcher | dedicated persistent worker runtime | dedicated persistent worker runtime | NONE for queue/domain logic | runtime/lifecycle choice required |
| Scheduling | embedded SPVScheduler in one worker | same, one scheduler authority | same initially | NONE at single-worker scale | scaling beyond one requires scheduler authority separation |
| Database | PostgreSQL 17, TCP postgres:5432; host 5442 | Cloud SQL PostgreSQL 17 proposed | Cloud SQL PostgreSQL 17 proposed | NONE | region/version availability, HA/backups/TLS/pooling |
| DB connectivity | DATABASE_URL local TCP | injected DATABASE_URL private TCP/proxy/socket | same | NONE | Cloud SQL networking/connector/proxy choice; verify driver DSN |
| Redis | Redis 7, AOF, redis:6379; host 6382 | managed Redis, private network | managed Redis + required HA/persistence | NONE | Memorystore offering/version/auth/TLS/durability and failover policy |
| Redis connectivity | REDIS_URL | REDIS_URL via VPC | same | NONE | supported URI/TLS credentials; validate Asynq and go-redis together |
| Migrations | one-shot Atlas, health gate, latest 000015 | controlled one-shot release step | controlled one-shot release step | NONE | release serialization, backup/recovery; never every API replica |
| Secrets | ignored 0600 .env.local, env injection | Secret Manager injected env | same with IAM/rotation | NONE | service identities, version pinning, backup and rotation |
| Token encryption | persistent SPV_TOKEN_ENCRYPTION_KEY | persistent versioned secret | persistent versioned secret | NONE | explicit future key rotation/migration; never regenerate at startup |
| ANAF | official HTTP adapter, PRODUCTION for real invoices | official TEST or deliberately authorized PRODUCTION | official PRODUCTION | NONE | exact routes and registered callback verification |
| Certificate | browser/USB at ANAF HTTPS IdP | same browser authorization | same | NONE | supported certificate middleware and delegated SPV PJ rights |
| OAuth callback | proposed HTTP loopback, unverified | registered controlled HTTPS callback | registered HTTPS callback | NONE | local acceptance evidence and cloud domain/routing |
| Raw ZIPs | PostgreSQL bytea DocumentStore | KEEP FOR TEST | existing store pending capacity/retention decision | NONE for test | long-term capacity/retention/encryption policies; no GCS migration here |
| Contract extraction | real Gemini adapter; missing key unavailable | same real adapter | same real adapter | NONE | supported model/account permissions and live acceptance |
| Classification | existing V1, no production pack, safe review | same | same | NONE | approved production accounting pack missing; V2 engine out of scope |
| SAGA | file adapter, immutable DB artifact, manual desktop import/confirmation | same | same | NONE | accountant-verified mappings and desktop format acceptance |
| Logs/traces | JSON slog + optional OTLP, runtime metrics | Cloud Logging + OTLP collector/export infrastructure | same | NONE | collector/TLS/export routing, retention/access and alerting |
| Health | API/worker healthz/readyz | startup/liveness probes and operational readiness | same | NONE | expose internal health appropriately; no fiscal permission probe |
| Time | UTC runtime, source timestamp raw; UI Europe/Bucharest | same | same | NONE | unresolved ANAF raw timestamp interpretation retained |
| CORS | same-origin Vite API proxy | same-origin frontend gateway | same-origin gateway | NONE | explicit trusted origins if split deployment is selected |
| Audit operator/auth | explicit private single-operator context injection | trusted authenticated request actor/grants required | same | REQUIRED identity infrastructure integration before public deployment | local attribution is not RBAC; existing demo fallback cannot be exposed |
| Fixtures | no seed in real project | no devseed in real cloud acceptance | none | NONE | APP_ENV=test remains exclusively engineering fixture runs |

Application differences are configuration/infrastructure except the already-deferred production authentication boundary. We do not claim that an unauthenticated API becomes production ready by changing APP_ENV.

## Worker deployment decision required

The existing worker is a **long-lived Redis consumer**, not an HTTP request worker or finite job. `worker.Start` runs Asynq goroutines, dispatcher polls PostgreSQL each second, scheduler polls active SPV connections on startup/hourly, and persistent Redis connections remain active. It requires CPU while no HTTP request is in progress. Redis/private PostgreSQL connectivity must remain available throughout its lifetime; transient failures require retries and observable readiness, not inline execution.

Recommended first test target: one dedicated Cloud Run worker-pool instance, if the selected region/service capabilities meet requirements; otherwise a small persistent Compute Engine/GKE runtime. Worker pools support manually specified instance counts. A finite Cloud Run Job does not match this worker's infinite main loop. No platform migration/Cloud Tasks replacement is justified in this task. This is a deployment recommendation, not an implemented decision. [Google worker-pool overview](https://docs.cloud.google.com/run/docs/overview/what-is-cloud-run), [manual worker-pool scaling](https://docs.cloud.google.com/run/docs/configuring/workerpools/manual-scaling?authuser=1&hl=en), retrieved 2026-09-15.

Cloud Run **service** is an alternative only with instance-based billing/CPU always available, minimum instances >=1, and initially maximum/manual instance count 1 to preserve scheduler authority. Inject WORKER_HTTP_ADDRESS=:8080 and configure the service container port to 8080; health HTTP is not a job trigger. Do not rely on request-driven scaling or a min-instance setting alone to make CPU available. [Google billing/CPU documentation](https://docs.cloud.google.com/run/docs/configuring/billing-settings?authuser=09&hl=en), retrieved 2026-09-15.

SIGTERM cancels dispatcher/scheduler, flips worker readiness false, waits for loops, calls Asynq Shutdown, then shuts down health HTTP. Existing default shutdown timeout is 15s; jobs can take 2m and extraction 90s. Cloud Run services provide a 10s termination grace period, which is shorter than current sequential shutdown bounds. Before selecting that service runtime, validate shutdown ordering/cancellation and use a verified smaller timeout plus margin; changing one timeout alone is not proof all goroutines exit within 10s. Unfinished jobs must be recovered from Redis/DB and tested after termination. Deployments, scale-down and network loss can interrupt jobs; delivery is at-least-once. Keep source-delivery idempotency, command/revision guards, durable outbox and retry recovery unchanged. Redis failover/data-loss guarantees must be selected explicitly; managed Redis is not automatically equivalent to local AOF persistence.

## Exactly one scheduler authority

Local-real starts one worker which owns the existing embedded scheduler. Keep one persistent worker in the first cloud test. Do not simultaneously add Cloud Scheduler/periodic jobs. Asynq deduplication is an overlap defense, not a substitute for a single authority. Multiple worker replicas each start the scheduler today; no independent scheduler-enable flag/separate scheduler executable exists. Before scaling consumers, choose a separate singleton scheduler process or an external scheduling authority with the existing enqueue/service path and an explicit setting to disable embedded scheduling. That future runtime change is a deployment decision, not a local workaround; do not build a second scheduling path now.

## Storage verdict

**KEEP FOR TEST.** PostgreSQL bytea preserves original ZIP/hash and transactional links through the existing abstraction and avoids introducing a different ingestion code path. For a limited, authorized cloud pilot, monitor storage growth, backup/restore time and DB pool/IO pressure; enforce secure transport, encrypted storage/backups and access/retention policy. Production-volume suitability is unproven. GCS selection can remain deferred until capacity/retention evidence justifies an adapter migration.

## Selected TEST deployment decisions — 2026-09-16

Project `diana-508810`, region `europe-central2`, frontend/API hosts
`test.platform.accountingtechco.com` and `api-test.accountingtechco.com`. Cloud Run
service with always-allocated CPU and exactly one instance is selected for the
worker. The embedded scheduler remains the sole authority. Direct VPC egress,
private Cloud SQL PostgreSQL 17, private Memorystore Redis 7, IAP perimeter,
separate public callback backend, global external HTTPS load balancer and
Google-managed TLS are selected. Same-origin `/api` routing on the frontend
host preserves the current frontend repository pattern; the API host remains
the canonical OAuth callback and diagnostic API origin.

`APP_ENV=cloud-test` is deployment hardening: it prohibits fake SAGA and
non-official enabled ANAF endpoints while reusing identical application/domain
code. It is not a new invoice/classification/SAGA implementation.

## Open cloud decisions

Runtime/region and worker lifecycle validation; singleton scheduling when scaling; Cloud SQL connection mode/HA/backups; managed Redis networking/auth/persistence; frontend same-origin routing/HTTPS; trusted identity/RBAC/grants; stable secret versions/rotation; official callback registration; verified ANAF routes and real Gemini/SAGA acceptance; production accounting pack; raw-document retention/capacity. None requires redesigning Accounting Domain V2, classification semantics, contract matching, duplicate policy or SAGA confirmation semantics for local preparation.
