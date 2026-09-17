# Backend Module 1 — Existing Draft Audit

The uncommitted draft was assessed against the approved Backend Phase 0 architecture.

## KEEP

- exact-decimal value-object intent;
- the exact approved invoice pipeline vocabulary;
- Go as the backend stack;
- capability-oriented package intent.

## ADAPT

- exact decimal: retained internally, with explicit JSON-number serialization;
- invoice model: reduced to the Module 1 read slice;
- Go module and scripts: reconciled around Ent, pgx, Atlas, and real PostgreSQL;
- project documentation: aligned with approved Phase 0 and Module 1 scope.

## DISCARD

- contract import/OCR/extraction UI and repositories;
- contract matching, validation, snapshots, workflow, and outbox implementation;
- the contract-domain migration;
- string-money frontend changes;
- contract routes and visible frontend behavior added by the draft.

These items belonged to later modules and were removed without changing the
approved frontend baseline.
