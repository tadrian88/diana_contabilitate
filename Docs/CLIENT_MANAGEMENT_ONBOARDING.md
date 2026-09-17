# Client Management + Client Onboarding V1

Status: IMPLEMENTED — AWAITING USER-RUN TESTS / REVIEW. Runtime behavior has not been validated in this implementation session. Classification V1 stays ENGINE READY — REAL PACK MISSING; V2 stays NOT STARTED.

## Discovery: actual Diana baseline

- `backend/internal/clients/client.go`, `service.go`, PostgreSQL `ListClients`, HTTP `listClients`: previously only ID/name/CUI and list access. Ent `schema/client.go` uses `clients`, internal text ID, raw CUI uniqueness, metadata and ownership edges.
- Client list/detail and selectors derive operations from existing repositories. `ApiInvoiceReadRepository` and `MockInvoiceRepository` retain separate runtime authority; API never falls back to demo state. Existing contracts, tasks, invoices and rules remain linked/shared modules.
- `RequestActor` has explicit `AllClients` / `AuthorizedClientIDs`; current HTTP middleware supplies an all-client demo actor when none is provided. This is a development baseline, not production authentication or RBAC.
- Accounting V2 `accounting.Profile`: dated ID/client/version; framework, account codes, chart policy, tax regime, VAT registration, deduction activity, cash accounting, pro-rata, approval (actor/time/evidence), TEST_ONLY. Immutable JSON rows and database triggers protect updates/deletes. Classification selects exactly one applicable approved profile; multiple approved overlaps require review. Existing invoice snapshots are immutable.
- SPV `ConnectionManager` owns OAuth/status/manual sync/disconnect; `Service` owns refresh/sync/source processing. Tokens are encrypted by the existing AES-GCM cipher. OAuth state is durable, hashed, expiring and consumed once. Certificate onboarding is a browser/USB flow: Diana never uploads or stores a certificate, PIN or private key. There is no certificate subject/expiry metadata or provider proof of CUI coverage. OAuth authorization and certificate validity cannot be equated.
- ANAF setup requires environment/application configuration (existing operator configuration) and the user's qualified certificate on their own device. The normal client journey needs no CLI/DB edits once the existing application deployment configuration is present. No new registry or certificate integration is introduced.
- SAGA is XML file export, exact artifact persistence/download and HUMAN confirmation of manual import. `LoadExportInput` reads client name/CUI from master data; generator uses invoice currency, never a duplicated company override. V2 readiness independently requires an approved matching pack/mapping and supported accounting decisions. Legacy XML supports its existing currency behavior. No SAGA company code, target mapping override, API, bridge or test fiscal invoice is invented.
- Audits are `activity_events`; workers consume the existing bounded outbox vocabulary. Client commands have no new asynchronous consumer, so creation/update/audit/idempotency are one transaction without publishing invented outbox events.
- Latest migration at discovery: `000014`; new additive migration: `000015`.

## ARMQU → Diana discovery map

ARMQU source roots inspected read-only: `armquai-be`, `armquai-fe` adjacent to `armquAI`. Diana architecture wins all conflicts.

| ARMQU source | Diana target | Disposition |
| --- | --- | --- |
| `armquai-be/store/schema/company.go` legal identity, registration, address/contact, currency | `clients.Company`, Ent AccountingClient | Adapt the business vocabulary; typed independent fields. Reject tenant-owned facility/manufacturing edges, CAEN enrichment, fiscal-year settings not used by Diana. |
| `armquai-be/service/api/companies.go` explicit list/create/get/update, company ownership checks | Clients service + HTTP client commands | Reuse bounded operations and explicit ownership as concepts; retain Diana RequestActor grants and modular monolith. Reject copying ARMQU admin roles/tenant middleware. |
| `armquai-fe/src/features/company-setup/components/CompanyModal.tsx` persistent create/edit UI and defaults | focused CreateClientPage + CompanyForm | Adapt create/edit form concept and Romanian vocabulary. Reject requiring all address, contact and registration fields before first save; reject accounting-software dropdown implying a SAGA connection. |
| `armquai-be/service/api/anaf.go` company-bound OAuth state, tenant check, platform configuration, encrypted token handling | existing SPV ConnectionManager/Store/Card | Reuse prerequisites and safe ownership concept only. Reject Redis state/tenant architecture migration, provider rewrite and extra credential storage. Diana's durable state and browser certificate flow remain authoritative. |
| `armquai-be/internal/anaf/xml.go` CIF cleaning | deterministic client identifier normalization | Adapt prefix normalization only. Reject inferring VAT registration from `RO`. |
| ARMQU accounting software enum (`saga`) | client file export opt-in | Reject treating a selected software value as connected/validated; use verified Diana file export boundary. |

No verified company registry, ONRC, taxpayer lookup, VIES, SAGA API or local bridge was discovered/introduced. No live provider/registry research was needed for this code-only milestone.

## Domain and company identity

`clients.Client` is the company aggregate boundary, with internal ID, company master data, normalized identifier, lifecycle, revision, created/updated time. Existing direct Name/CUI fields remain computed from the same master data for protected callers. Configuration secrets and accounting policy remain in their existing domains.

Company fields: legal name, display name, CUI/CIF display value, registration number, country (two-letter format), registered address, city, region/county, postal code, email, phone and default currency (three-letter format). Name + valid fiscal identity + country + currency are the first-save minimum; UI supplies editable RO/RON defaults. Optional registration/contact/address fields do not block company readiness. Foreign region text is unrestricted by Romanian county semantics.

Romanian format normalization trims whitespace, uppercases and strips optional RO prefix; requires 2–10 digits, nonzero leading digit. No checksum/registry/tax-registration claim. Foreign identifiers accept bounded uppercase alphanumeric/dot/hyphen format. Display value stays separate. Existing synthetic demo identifiers can be retained verbatim while ordinary metadata is edited. Invalid new identifiers are rejected.

The original raw CUI unique constraint is preserved. A new unique `(country, normalized_identifier)` index prevents RO/non-RO-prefix duplicates across **all** lifecycles; reactivation is preferable to another record for the same company. This is deliberately more conservative than active-only uniqueness. Migration normalizes verified-format existing CUIs and retains other legacy uppercase identifiers; collisions abort migration for human review. No winner, merge or cascade rewrite.

CUI/country changes are allowed only before invoices, contracts, SPV connection/source/OAuth state, contract source documents, accounting profiles or releases exist. Changing display formatting with unchanged normalized identity is allowed. Names/contact metadata can be corrected without rewriting historical sources, classifications or artifacts. Changing legal business identity after dependencies requires a future explicit correction workflow.

## Lifecycle

- ONBOARDING: new saved company; independent setup can progress, including ANAF ingestion when authorized.
- ACTIVE: intended operational lifecycle; it grants no integration/profile/pack approval.
- INACTIVE: retained company/history. Scheduler skips it; newly executing SPV sync jobs check lifecycle before provider access. Manual sync and OAuth start are blocked. OAuth completion cannot activate an inactive client.

Already discovered SPV documents and existing pipeline jobs can finish through their protected boundaries. Deactivation does not revoke or delete credentials/history, cancel leases or rewrite queued work. A sync already in flight can complete; lifecycle is not an unsafe cancellation primitive. Disconnect is the separate existing credential-revocation command. Reactivation changes only lifecycle and recomputes readiness; expired/revoked authorization remains action required.

No hard-delete or merge API exists. Settings/history remain readable for authorized inactive clients. New base invoice discovery stops through SPV scheduling/sync guards; this module does not invent cancellation across all historical pipeline jobs.

## Dated tax/accounting profiles

The UI uses the exact `accounting.Profile` facts. Unanswered facts begin UNKNOWN; account policy/account codes are entered explicitly. New version creation owns ID/client/version in the backend. Draft configuration saves an immutable profile with no approval. An explicit approval checkbox requires evidence references; backend binds actor and time, checks current V2 validation, and inserts a new immutable approved version. It does not promote any accounting rule or pack.

Approval of a draft is another version, leaving the draft unchanged. An approved new period must not overlap any existing approved non-test period. Bounded adjacent historical/future periods are supported. Approved open-ended historical versions cannot be replaced via this module because the existing immutable period has no supersession semantics. A future explicit profile correction/supersession decision is required; this module neither changes classifier selection nor truncates old periods.

Detail shows the uniquely applicable approved profile (or latest applicable draft when none is approved), plus history/future versions. UNKNOWN can be approved as a documented unknown fact; approval does not turn it into an ordinary supported fiscal regime. Existing immutable invoice accounting snapshots and classifications are never reprocessed.

## Derived onboarding read model

`clients.Derive` derives sections; `ConnectionManager.Get` supplies environment-aware safe ANAF status. Reads of company/profile/config/release/history use one repeatable-read snapshot. ANAF enrichment is a separate existing domain read. No readiness/completion flag or derived DTO is stored in the command ledger.

| Section | Authority and meaning |
| --- | --- |
| COMPANY | Saved minimum identity. Optional contact/address data is irrelevant to readiness. |
| ACCOUNTING_PROFILE | One applicable non-TEST_ONLY valid approved V2 profile. Draft/future/history/approved overlap states are distinct. |
| ANAF_SPV | Existing safe connection view and app configuration; missing authorization, refresh expiry, sync failure, disable/inactive are explicit. Certificate stays local and CUI coverage remains unconfirmed by OAuth. |
| SAGA | File export enabled and company identity available. `sagaConfigurationReady` is configuration opt-in, not invoice or fiscal XML eligibility. `sagaMappingApproved` comes from applicable non-test releases; target validation remains VALIDATION_PENDING because no client-specific target acceptance authority exists. |
| CLASSIFICATION | Exactly one applicable approved non-TEST_ONLY nonempty production pack bound to the applicable profile; missing pack is unconfigured capability, not an error. Synthetic test releases never make it green. |

Statuses: NOT_STARTED, INCOMPLETE, READY, ACTION_REQUIRED. Next actions are fixed Romanian strings/section targets, never AI generated. Blockers explain the independent missing facts and operational actions. No percentages.

Overall states: CONFIGURATION_INCOMPLETE; CORE_CONFIGURED_AUTOMATION_PENDING; CORE_AND_AUTOMATION_CONFIGURED_VALIDATION_PENDING; INACTIVE. **CORE_CONFIGURED** means company + approved applicable profile + ANAF authorization/app prerequisites + SAGA export opt-in. It does not mean every invoice can export or all accounting/target acceptance is complete. There is deliberately no “Everything configured” state. Target validation is always shown as pending. Missing real rules never block SPV ingestion; classification retains safe existing review behavior.

## SAGA configuration

Separate `client_saga_configurations` stores only enabled and update time. New clients start disabled; explicit setup enables their existing file exporter input boundary. `LoadExportInput` rejects explicitly disabled configurations before generating new artifacts. Already generated artifacts remain available through existing handoff. Existing clients are grandfathered enabled on migration; direct legacy Ent fixtures without a row retain previous export behavior. Disabling setup does not rewrite classification or trigger reprocessing.

Master name/CIF go straight to existing generator. No company override, extra target identifier, fiscal mapping, fake validation invoice or connection claim. Default currency remains company metadata and does not override invoice currency. Approved account policy facts cannot substitute for mapping release/target validation. Pending target validation does not prevent creation or editing.

## API

All routes use `/api/v1`. Writes require Idempotency-Key; bounded JSON top-level fields are rejected if they belong to another command.

| Method/path | Command/read |
| --- | --- |
| GET /clients | Authorized clients; expanded additive lifecycle/master metadata |
| POST /clients | `{company}`; 201 + Location + full saved client detail |
| GET /clients/{id} | `{client, profiles, sagaEnabled, history, onboarding}` |
| POST /clients/{id}/company | `{company, expectedRevision}` |
| POST /clients/{id}/lifecycle | `{status, expectedRevision}` |
| GET /clients/{id}/onboarding | Derived readiness |
| GET /clients/{id}/accounting-profiles | Immutable profile versions |
| POST /clients/{id}/accounting-profiles | `{profile, approve, evidence, expectedProfileVersion, expectedRevision}` |
| POST /clients/{id}/saga-configuration | `{sagaEnabled, expectedRevision}` |
| Existing /clients/{id}/spv and actions | Safe status, certificate/OAuth start, sync/disconnect; OAuth start now requires Idempotency-Key |

No generic cross-domain patch. New client ID remains authoritative. New client creation currently requires existing AllClients authority because production granting/RBAC is deferred; a client-only actor cannot grant itself a new client. Requests outside a grant return 404. Existing SPV/invoice/contract/task/client override route scope checks were added without changing domain workflow semantics. OAuth callback checks current grants before token exchange when an actor is present.

## Transactions, audit and concurrency

Creation, company/lifecycle/config/profile version, audit and command ledger are one PostgreSQL transaction. Advisory locks serialize identical actor/operation/client/key submissions; normalized payload hash rejects key reuse with changed payload. Client row locks/revisions serialize different settings commands; expected profile max version prevents stale inserts. Replayed commands return a fresh derived read of the same affected client, not cached readiness. No integration credentials are hashed/stored in this client command ledger.

Client events: CLIENT_CREATED, CLIENT_UPDATED, CLIENT_ACTIVATED, CLIENT_DEACTIVATED, CLIENT_REACTIVATED, ACCOUNTING_PROFILE_CREATED/APPROVED, SAGA_CONFIGURATION_UPDATED. Before/after safe company/lifecycle/revision/profile-version/config summaries, actor/time/client/correlation/key are retained. History translates known events and never returns raw detail/snapshot payloads (existing release audit may contain restricted detailed configuration).

OAuth starts use a separate idempotent attempt table, storing encrypted raw state with existing AES-GCM cipher and referencing the existing hashed state. It is not exposed by ordinary reads. Replay is allowed only while unconsumed/unexpired. ANAF_CONFIGURATION_STARTED is safe metadata-only audit; connected/reconnected/disconnected events reuse SPV_CONNECTION_* vocabulary. Disconnect/manual-sync keys are scoped to actor/client/operation. Manual sync keeps the existing publisher and audit/deduplication behavior; no new worker/outbox path or duplicate initial sync. Explicit sync or existing periodic scheduler initiates imports.

## Frontend and operational limits

Focused `/clients/new` page saves identity then redirects. `/clients` supports search/status filter and concise lifecycle/ANAF/SAGA/automation summaries alongside existing operation counts. Detail adds persistent overview, company/profile/ANAF/SAGA/lifecycle/history sections and retains invoice/contract/task/rule links. Romanian primary, desktop-first; no mobile redesign.

Loading/error/retry/missing/configured/action-required states remain explicit. Company/profile dirty drafts survive unrelated configuration updates; submissions retain their editing-base revision. Refresh/OAuth navigation receives native beforeunload protection. SPA Link navigation has no new global router blocker because this app uses BrowserRouter rather than a data router; dirty-state text is visible. Draft unsaved UI fields are not durable until saved. API errors remain visible with no mock fallback.

The pilot list currently reads derived details per visible client; pagination/batched readiness can be added when scale warrants it. No new client-ID/CUI metric labels; existing HTTP tracing/metrics and audit capture operations. No speculative instrumentation added.

## Migration and test review

Only `000015` is new; previous SQL migrations are unchanged. Ent client schema/generated code updated; Atlas checksum regenerated. Actual runtime database contents were not inspected because the user reserved runtime integration. Run the collision preflight in CLIENT_MANAGEMENT_ONBOARDING_TEST_HANDOFF.md before applying an incremental migration. Legacy lifecycle stays ACTIVE, metadata defaults RO/RON/empty, revision 1, SAGA enabled, readiness derives independently.

Implemented tests: client normalization/minimum/legacy edits/grants/profile UNKNOWN/readiness table; HTTP client scoped DTO and command isolation; PostgreSQL concurrency/create/duplicates/stale revisions/legal protection/profile immutability/evidence/periods/SAGA/ANAF OAuth replay/sync/lifecycle/secrets; SPV inactive queued sync/refresh expiry/callback grants; SAGA master-data XML; frontend creation/edit/profile/SAGA/history/duplicate/stale cases; dedicated fake-provider Playwright A–G and denied-response transport case H. Existing integration suites retain their original module semantics. Integration tests that create immutable non-test profiles deliberately retain them: run in an isolated disposable test DB, never a real client DB.

Checks executed: gofmt; Ent go generate; Atlas migrate hash; TypeScript typecheck; Go compile-only across ordinary and integration-tag packages using `-exec /usr/bin/true`; git diff --check. No behavior suite, PostgreSQL migration/runtime, Redis/Asynq runtime, race run, Playwright, production build or live ANAF/certificate/SAGA was executed. Compiler output saying `ok` is not a test pass.

Real browser actor isolation remains unavailable under the existing all-client demo actor. Server grant tests are authoritative; Playwright H explicitly simulates a denied response and does not claim production authorization. New fixture seeding is opt-in `SEED_CLIENT_ONBOARDING=true`, preserving existing default fixture counts; no real credentials or production pack.

## Product decisions / deferred

- PRODUCT DECISION REQUIRED — CLIENT LEGAL IDENTITY CHANGE: only for a future change after dependencies; this module blocks it safely.
- PRODUCT DECISION REQUIRED — CLIENT DELETION / CLIENT MERGE: future workflows; no operations implemented.
- PRODUCT DECISION REQUIRED — APPROVED PROFILE SUPERSESSION: replacing an immutable approved open-ended period requires explicit semantics. Bounded nonoverlapping versions work now.
- SAGA CLIENT OVERRIDE: unnecessary in the verified generator; no conflicting second identity introduced, so no decision is required for this milestone.

Deferred: production RBAC/authentication/automatic new-client grants, ONRC/registry/VIES enrichment, full accounting rule pack, Classification V2, SAGA local bridge/machine acknowledgement, CreditNote/storno, historical reclassification, merge/hard delete, approved-history correction/supersession. App-level ANAF deployment configuration remains the existing operator prerequisite. STOP after this milestone; no release/deployment work.

Engineering gate verified: NO (runtime tests/review outstanding). Ready for user-run tests: YES.
