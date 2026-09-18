# Contract Ingestion Hardening V1

Status: **IMPLEMENTED — AWAITING USER-RUN TESTS / REVIEW**.

## Root causes and design

- Fiscal identity had four semantics: client onboarding removed `RO`, SPV removed it independently, contract buyer validation had another helper, and invoice/contract supplier matching retained it. The old contract-specific helper itself already considers the exact pair `21592770` / `RO21592770` equal, so the VAT prefix alone does not reproduce the live `buyerMismatch`; the persistent UI blocker came from validating the immutable proposal rather than reviewed state. `internal/fiscalidentity` is now the canonical comparison boundary across all flows. Romanian whitespace and optional `RO` are ignored for identity only; raw source values and VAT meaning remain unchanged. Foreign identifiers use conservative case/whitespace normalization.
- Confirmation used `Document.BuyerMismatch`, derived from the immutable AI proposal, before validating the reviewed command. Confirmation readiness now evaluates current reviewed values. The proposal, evidence, confidence, correction, and confirmed values remain separate.
- Every blocker is derived and displayed. Backend `ConfirmationReadinessFor` is authoritative and is re-run during confirmation; the frontend mirrors it for immediate UX.
- End date was universally required. `period_type` is now `FIXED_TERM` or `INDEFINITE_TERM`; indefinite contracts persist a null upper bound and matching treats it as open-ended.
- Unconfirmed uploads can transition to `DISCARDED`. Reads and file access exclude discarded documents; bytes remain retained for audit/retention policy. Waiting invoices/tasks are untouched. Confirmed contracts are not deletable; `ACTIVE/ARCHIVED` groundwork exists without adding an unsafe hard-delete workflow.

## PDF Cloud TEST root cause

Read-only Cloud Logging showed the protected file route returned HTTP 200 with the complete 139,616-byte body. The worker asset also returned HTTP 200. A direct header check showed the exact defect: `.mjs` was served as `application/octet-stream` while `X-Content-Type-Options: nosniff` was enabled. Chrome rejected the PDF.js ES-module worker.

`deploy/nginx.frontend.conf` now maps `.mjs` to `application/javascript`. The private file endpoint uses `http.ServeContent`, preserving authentication/client isolation and adding standard `Range`, `Accept-Ranges`, `206`, `Content-Range`, length, and conditional-delivery behavior. PDF.js now loads the authenticated URL directly with credentials rather than buffering the entire PDF first.

The viewer provides fit-width rendering, zoom, page controls, evidence-page navigation, sticky desktop layout, retry, and authenticated download. No storage URL is exposed.

## API changes

- `POST /api/v1/clients/{clientId}/contract-documents/{documentId}/discard`
- `GET .../file` now supports authenticated byte ranges.
- Contract-document DTO includes the authorized client's CUI for reviewed buyer readiness.
- Confirmation accepts period type, reviewed buyer CUI, and typed service terms.
- Contract detail includes period type, legacy-total presence, and service terms.

## Audit and security

Upload/extraction/confirmation events remain. Added document discard, reviewed correction, and service-terms confirmation events. Source evidence is copied from the immutable extraction attempt, never trusted from browser input. Cross-client metadata, file, and discard access return not found. PDFs remain private and `no-store`.

## Migration and compatibility

Migration `000017_contract_ingestion_hardening.sql` is additive after `000016`. It canonicalizes only valid Romanian numeric comparison columns (not raw values), adds open-ended period/lifecycle fields, records whether the legacy total is semantically present, expands document lifecycle with `DISCARDED`, and creates `contract_service_terms`.

Existing contracts remain fixed-term, active, and marked as having a legacy total. Module 4 thresholds and outcomes are unchanged; only the legitimate null end bound and canonical Romanian supplier identity affect candidate discovery.
