# ANAF / SPV discovery in armqu

## Scope and call graph

The inspected source repository is `armquAI/armqu-be`. Its inbound flow is:

`worker.RegisterAnafSyncHandler` -> `handleAnafSync` -> `syncConnection` ->
`anaf.Client.ListMessagesPaginatedRetry` -> `DownloadInvoiceRetry` ->
`anaf.ParseInvoiceZIP` -> `anafprocess.StoreSourceXML` / `ApplyParsedFields` ->
armqu invoice persistence -> armqu review pipeline.

The relevant components are:

- `internal/anaf/client.go`: OAuth endpoints, received/issued message listing,
  flexible ANAF JSON identifiers, pagination, download and bounded HTTP bodies.
- `internal/anaf/xml.go`: ZIP selection and UBL Invoice/CreditNote parsing.
- `internal/anaf/token.go`, `crypto.go`: JWT metadata inspection and AES-256-GCM
  token encryption.
- `internal/anaf/ratelimit.go`, `transient_retry_test.go`: Redis-backed throttling,
  call pacing, separate transient/rate-limit retry budgets.
- `internal/worker/anaf_sync.go`, `anaf_redrive.go`: hourly overlapping sweeps,
  refresh, download/import, failure cooldown and stored-payload redrive.
- `internal/anafprocess/archive.go`, `failures.go`, `persist.go`, `pipeline.go`:
  best-effort object archival, durable failures and armqu-specific persistence/review.
- `service/api/anaf.go`, `service/admin/anaf.go`: OAuth/manual-sync product entry points.
- `store/schema/anaf_connection.go`, `anaf_platform_config.go`,
  `anaf_import_failure.go`, `invoice.go`, `invoice_item.go`: persistence and uniqueness.
- `internal/anaf/*_test.go`, `internal/anafprocess/*_test.go` and ANAF integration
  tests: sanitized structures, flexible IDs, parser, crypto, retries and uniqueness.

## Actual external behavior

- OAuth2 authorization-code login is performed with a qualified certificate at
  `logincert.anaf.ro`; authorization, token and revoke endpoints use
  `/anaf-oauth2/v1/*`. Access and refresh grants request JWT token content.
- The e-Factura transport uses the test/prod `FCTEL/rest` APIs. Listing uses
  `listaMesajePaginatieFactura` with millisecond start/end, 1-based page, CIF and
  filter. Download uses `descarcare?id=<message-id>`. Incoming invoices use filter
  `P`; armqu also imports `T`, which is outside Diana's scope.
- Numeric message/upload identifiers may arrive as JSON numbers or strings.
  Message ID is mandatory; malformed optional upload ID becomes zero. A benign
  `Nu exista mesaje` response is treated as an empty page.
- A connection belongs to one company/CIF. A certificate can authorize sibling
  CIFs, but armqu requires a separate connection for each legal entity and never
  mixes their messages.
- The scheduled sweep uses an overlapping date window (three days normally,
  up to sixty for initial/manual sync), not an API cursor. `last_sync_at` is
  operational status rather than the query cursor. Unique external message
  identity absorbs overlap and retry.
- The downloaded response is a ZIP. armqu chooses the first non-signature XML,
  then parses root `Invoice` or `CreditNote` as UBL 2.1 / CIUS-RO. It does not
  implement CII parsing.
- Monetary/quantity values use exact decimal parsing. The persisted invoice total
  is UBL `TaxInclusiveAmount` (with a subtotal + tax fallback), not
  `PayableAmount`, so prepayments do not erase invoice value.
- Durable uniqueness is tenant + ANAF message ID. Failures are retained by message
  ID; downloaded ZIP bytes permit DB-only reparse after parser fixes. Network and
  rate-limit failures are retried separately.
- Access/refresh tokens are AES-256-GCM encrypted. Application OAuth credentials
  are platform configuration. JWT claims are decoded only for display/grouping,
  not trusted as local authentication.

## Source -> Diana mapping

| armqu concept | What it does | Diana equivalent | Action |
|---|---|---|---|
| ANAF HTTP/OAuth client | External protocol only | `internal/spv` transport adapter | ADAPT |
| Per-company ANAF connection | Owns CIF and encrypted tokens | Per-`AccountingClient` SPV connection | REIMPLEMENT |
| Overlapping paginated sweep | Avoids missed documents | Durable last-success + configurable overlap | REUSE CONCEPT |
| Tenant + message uniqueness | Technical idempotency | Connection + external message identity | REIMPLEMENT |
| ZIP/UBL parser | Converts structured payload | Explicit Diana document-parser boundary | ADAPT |
| Import-failure row/raw ZIP | Recovery and diagnosis | SPV source-document operational record | ADAPT |
| Object bucket archival | Source preservation | `DocumentStore`, initially PostgreSQL-backed | ADAPT |
| armqu invoice create/review | Persists armqu's broad ledger model | Existing Diana `invoicing.Service.Ingest` | DISCARD / REIMPLEMENT |
| armqu cron and inline sync | Drives import | Module 7 Asynq scheduler/jobs | ADAPT |
| Issued invoices, upload/status/PDF | Outgoing or UI behavior | No inbound Diana equivalent | DISCARD |
| AI/import-rule pipeline | armqu review semantics | Existing Diana contracts/classification pipeline | DISCARD |
| Manual-certificate merge | armqu legacy migration behavior | None | DISCARD |

## Security and robustness findings

No credentials, tokens, certificates, customer XML, production CIFs or logs are
copied. Test documents in Diana are recreated with synthetic identities.

armqu's useful bounds are conceptual, but its ZIP implementation accepts the
first matching XML and its `LimitReader` does not prove the payload stayed under
the limit. Diana must additionally reject traversal-like names, multiple invoice
XML candidates, excessive file count, excessive compressed/uncompressed size and
truncated over-limit bodies. XML is parsed with Go's non-resolving `encoding/xml`
and raw XML must never appear in logs/errors.

## Verification and uncertainties

Official ANAF OAuth documentation confirms qualified-certificate OAuth2, the
`logincert.anaf.ro` endpoints, `api.anaf.ro`, 90-day access tokens, 365-day refresh
tokens and a currently documented global limit of 1000 calls/minute. It warns that
service-specific limits can change. ANAF's official synchronous-service page still
publishes older `webserviceapl.anaf.ro` URLs while its current OAuth guide points
clients to `api.anaf.ro` and a Ministry PDF that was unavailable during discovery.
Therefore base URLs and conservative throttling are configurable.

`data_creare` is formatted `yyyyMMddHHmm` in armqu, but the inspected material does
not establish its timezone. Diana retains the raw value and only stores a parsed
timestamp when a configured source timezone is available.

The armqu comments asserting per-endpoint daily limits and a per-message download
limit are not treated as verified API contract. They are not hard-coded into Diana.

## Certificate onboarding

The complete ARMQU browser call chain is:

`AnafTab.handleConnect` -> `useAnafAuthorize` ->
`POST /anaf/authorize { companyId }` -> `Service.AnafAuthorize` ->
`anaf.AuthorizeURL` -> browser navigation to `logincert.anaf.ro` ->
browser/ANAF qualified-certificate authentication and consent -> ARMQU frontend
`/anaf/callback?code&state` -> `POST /anaf/callback` ->
`Service.AnafCallback` -> authorization-code exchange -> encrypted token persistence.

ARMQU does **not** have a certificate-file picker, multipart certificate endpoint,
PKCS#12/X.509 parser, certificate password field, temporary certificate record or
certificate store in this flow. Its Romanian UI tells the user to connect the USB
token (or install the certificate), restart the browser, press Connect, select the
certificate in the browser prompt and authorize on ANAF. The certificate/private key
and PIN remain in the user's certificate middleware/browser/OS and ANAF boundary.
They are never transmitted to ARMQU.

This agrees with ANAF's published OAuth architecture: `logincert.anaf.ro` is the
identity provider and authenticates the user with a qualified digital certificate;
the application receives an authorization code and exchanges it using its own
registered OAuth application credentials. Consequently Diana must explain and start
this redirect flow, not upload the user's private key material.

### Value inventory

| Value | Source | User input to app? | Sensitive | Persisted by ARMQU | Encrypted | Used after connect | Diana target |
|---|---|---:|---:|---:|---:|---:|---|
| Certificate file/container | USB token, OS/browser certificate store | No | Yes | No | N/A | No by Diana/ARMQU | Never received or stored |
| Certificate PIN/password | Certificate middleware/browser prompt | No | Yes | No | N/A | No by Diana/ARMQU | Never rendered, received, logged or stored |
| Private key | USB token/certificate store | No | Yes | No | N/A | Used only by browser/provider authentication | Never crosses Diana boundary |
| Certificate serial | ANAF access-token claim (`serial`, fallback `sub`) | No | Personal/security metadata | Raw value is not stored as key; may appear in ARMQU display fallback | Hash used for grouping | Display/grouping only | Do not expose or persist until live claims are verified |
| Fingerprint | Not extracted from X.509; ARMQU hashes the first stable token identity claim | No | Security metadata | SHA-256 grouping key | One-way hash, not encryption | Alias/grouping | Not required by Diana onboarding |
| Issuer/subject | Best-effort access-token claims | No | Metadata/possible PII | Display fallback only | No | Display only | Do not claim availability |
| Certificate validity dates | Not available in inspected ARMQU flow | No | Metadata | No | N/A | No | Not displayed; token expiry is not certificate expiry |
| Covered CIFs | Authenticated ANAF list response top-level `cui`, captured during sync | No | Business identity | Comma-separated on connection | No | Informational/sibling hint | Live semantics must be certified before activation claims |
| OAuth application client ID | ARMQU platform configuration | No | Configuration identifier | Platform config | No in ARMQU | Authorization/refresh | Diana server configuration only |
| OAuth application secret | ARMQU platform configuration | No | Yes | Platform config | No explicit field encryption in ARMQU | Exchange/refresh/revoke | Diana server secret only |
| Access token | ANAF token endpoint | No | Yes | Connection | AES-256-GCM | API calls until expiry | Existing encrypted token field |
| Refresh token | ANAF token endpoint | No | Yes | Connection | AES-256-GCM | Automatic token refresh | Existing encrypted token field |
| Token expiry | ANAF token response | No | No | Connection | No | Refresh scheduling/status | Existing connection metadata; not primary UX |
| Capabilities | Access-token `roles`/`role`/`scope` claim | No | No | Normalized string | No | Display only in ARMQU | Do not expose unrelated raw roles in Diana |

### Identity and CIF limitations

ARMQU's “Authorized by” value is not parsed from a supplied certificate. It decodes
the access-token JWT **without signature verification**, tries person/name claims,
then falls back to issuer plus serial. Its stable `authorizer_key` is a SHA-256 hash
of a best-effort identity claim. This is display/grouping metadata and is not an
authorization decision. The screenshot's `certSIGN · <serial>` therefore reflects
token claims/fallback, not an X.509 fingerprint computed by ARMQU.

The screenshot's “Covers CIFs” comes from the top-level `cui` field returned by
`listaMesajePaginatieFactura`, not from certificate bytes, OAuth state or local
company configuration. ARMQU captures this during the first successful sync and
does not validate membership before creating the OAuth connection. The inspected
code does not prove that `cui` is always present for empty/error list responses.
Diana must not claim callback-time CUI verification until this behavior is confirmed
against live ANAF. The frozen buyer-CUI document guard remains authoritative.

### Reconnect and certificate lifetime

Reconnect repeats the same browser/ANAF certificate authorization and replaces the
stored encrypted access/refresh tokens. The certificate and PIN are still not sent
to the application. Normal refresh uses only the refresh token plus application
client credentials, so the certificate is not required again until provider refresh
fails or a user intentionally reconnects.
