# Contract Ingestion Hardening + Service Terms V1 Result

## Status

**IMPLEMENTED — AWAITING USER-RUN TESTS / REVIEW**

All six real-E2E findings have code-level root causes and fixes:

1. CUI: fragmented comparison semantics; replaced with one canonical fiscal-identity package and an additive comparison-data normalization.
2. Confirm edit: stale AI buyer mismatch and universal HTML/backend requirements; confirmation now uses reviewed buyer/period values and explicit blockers.
3. Delete: no lifecycle command; unconfirmed documents can now be audited/discarded while waiting invoices remain waiting.
4. PDF: Cloud `.mjs` MIME was `application/octet-stream` plus `nosniff`; Nginx MIME mapping corrected, protected range delivery and failure actions added.
5. Value model: one total could not express service pricing; typed ordered service terms and extraction/review/persistence were added.
6. Layout: fixed 600px canvas underused the pane; viewer is fit-width, responsive, sticky, controlled, and uses a roughly 55/45 desktop split.

One confirmed contract continues through the existing `ContractAvailable` outbox/resume mechanism. A regression test creates two waiting invoices and verifies both are evaluated; it does not force a match outcome beyond Module 4 policy.

No accounting classification, VAT, deductibility, production rule-pack, or matching-threshold semantics changed.

## Verification performed by Codex

- `gofmt`
- Ent generation
- Atlas migration hash
- Go compile-only test selection for all packages
- integration-tag compile-only selection
- TypeScript typecheck
- `git diff --check`
- read-only Cloud TEST log/header investigation

User-run evidence received on 2026-09-17/18:

- the complete Go runtime unit suite passed;
- the production frontend build passed;
- the complete frontend suite passed: 18 files and 108 tests;
- all 17 migrations applied successfully to the isolated release database and Atlas reported no pending migration;
- the integration gate exposed one obsolete non-numeric Romanian buyer fixture and concurrent Redis database flushing between two Go packages. The fixture now uses structurally valid synthetic Romanian identifiers, and the Redis/Asynq suites run serially on separate Redis databases. The corrected integration gate awaits user rerun.

Playwright, the corrected integration gate, local deployment, live Gemini, and Cloud deployment/retest have not yet been reported as passed. Codex did not execute runtime suites.

Engineering gate ready: **NO** until user-run tests. Ready for user-run tests: **YES**.
