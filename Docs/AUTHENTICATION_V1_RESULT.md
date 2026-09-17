# Authentication V1 result

Status: **IMPLEMENTED — AWAITING USER-RUN TESTS / REVIEW**.

Authentication V1 adds the Romanian Diana login, server-enforced PostgreSQL sessions, Argon2id credentials, Redis login throttling, CSRF enforcement, authenticated RequestActor derivation, operator-only provisioning/reset, logout, session-expiry handling, security tests and a real Playwright journey.

The Google redirect root cause is confirmed as GCP IAP on both `diana-test-web-backend` and `diana-test-api-backend`; it is not React, the Diana backend, Cloud Run IAM redirect logic, or ANAF OAuth. The live URL map sends application-host `/api/*` to the API and only the exact API-host ANAF callback path to the public callback backend.

No Cloud deployment or IAP mutation was performed. Migration `000016_authentication_v1.sql` is additive. No real demo password was chosen, stored or generated. See `AUTHENTICATION_TEST_HANDOFF.md` and `GCP_TEST_DEPLOYMENT.md` for the gated rollout.

Engineering gate remains NO until the user-run behavior, integration, browser and Cloud TEST security gates pass.
