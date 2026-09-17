# SAGA Export UX and Manual Handoff

Status: implemented; awaiting user-run tests and review.

## Locked meaning

In real `SAGA_MODE=file`, `EXPORTED` means **a human explicitly confirmed that
the exact generated artifact was imported into SAGA**. It does not mean that
SAGA sent Diana a technical acknowledgement. Generation and download alone
leave the invoice at `EXPORTING`.

The supported journey is:

1. the pipeline generates and persists an immutable SAGA XML artifact;
2. Invoice Detail shows the filename and generation/download context;
3. the accountant downloads the unchanged bytes and imports them manually;
4. the accountant opens a warning dialog and confirms the import;
5. one PostgreSQL transaction records immutable HUMAN confirmation, creates
   `SAGA_IMPORT_CONFIRMED`, and moves `EXPORTING → EXPORTED`.

No download prerequisite is enforced because the same approved artifact may be
obtained through another controlled mechanism. A download creates
`SAGA_EXPORT_DOWNLOADED`, but is not evidence of import.

## API and read model

- `GET /api/v1/clients/{clientId}/invoices/{invoiceId}/saga-export`
- `GET /api/v1/clients/{clientId}/invoices/{invoiceId}/saga-export/artifact`
- `POST /api/v1/clients/{clientId}/invoices/{invoiceId}/saga-export/confirm-import`

The command body contains `attemptId`, `expectedInvoiceRevision`, and an
optional note of at most 500 characters. `Idempotency-Key` is mandatory.
Confirmation accepts only the latest successful attempt belonging to that exact
invoice and client. Failed, stale, guessed, and cross-client attempts are
rejected. Concurrent confirmations converge on one immutable confirmation, one
invoice revision increment, and one success audit event.

Invoice detail JSON exposes only attempt metadata: status, safe filename,
generation/download/confirmation timestamps, confirming actor/type, and source
invoice revision. XML bytes are available only from the artifact route.

## Download security

The server resolves ownership from persistence; it never trusts an artifact
hash or ownership supplied by the browser. The route requires the current
`RequestActor`, scopes lookup by client and invoice, formats
`Content-Disposition` through the standard library, returns the persisted bytes
without reserialization, limits responses to 10 MiB, and sends `no-store`,
`private`, `nosniff`, and `no-referrer` protections. XML is not logged.

The current application still injects its established demo accountant actor.
Therefore these endpoints are architecturally behind the actor/ownership
boundary, but production exposure remains prohibited until the planned
authentication/RBAC module supplies verified identities and client grants.

## UX and aggregate behavior

The workflow is contextual in Invoice Detail, not a global SAGA page. The UI
uses “Import confirmat manual” and explicitly says Diana cannot verify SAGA
automatically. The dashboard and invoice list retain their frozen derivation:
only pipeline `EXPORTED` contributes to exported counts; `GENERATED` and
`DOWNLOADED` do not. No ValidationTask is created.

## Deferred

- a local Windows bridge or watched-folder integration;
- any machine acknowledgement from SAGA;
- reopening or re-export after human confirmation;
- CreditNote/storno export pending vendor evidence;
- production validation of accounting, VAT, and deductibility rule outputs.


## Client setup — 2026-09-15

Client detail now exposes file export opt-in and accurate independent mapping/target acceptance status. New clients must enable file export; preexisting clients are grandfathered. `LoadExportInput` checks explicit opt-in and continues to pass the single master company name/CIF to the unchanged generator. Default company currency never rewrites invoice currency. No technical SAGA connection, client identity override, test invoice or fabricated target validation. Previously generated artifacts and manual HUMAN handoff remain available; pending target validation does not block base company setup. See CLIENT_MANAGEMENT_ONBOARDING.md.
