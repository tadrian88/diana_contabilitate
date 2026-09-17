# Contract Ingestion + PDF + Gemini + Human Confirmation

Status: IMPLEMENTED — AWAITING USER-RUN TESTS / REVIEW. This is not approval or a claim that runtime tests passed.
Modules 1–7, ANAF/SPV, Real SAGA + export UX, and Missing Contract Resume remain APPROVED / FROZEN. No Real Accounting Rules work is included.

## User journey

Select an authorized client → upload PDF → persisted processing state → review original beside normalized AI proposal → correct/complement fields → explicitly confirm → authoritative Contract → existing ContractAvailable → existing Missing Contract Resume.
The missing-contract invoice screen links to this same flow with its client locked. API-backed waiting invoice detail refreshes every 3 seconds so asynchronous ContractAvailable results become visible without manual reload; matching decisions remain unchanged. Proposal availability alone never creates a Contract or resumes an invoice. Failed extraction offers re-extraction, not a manual-from-zero form. Duplicate client/hash upload opens the existing document.

## Architecture and storage

`internal/contractingestion` owns upload, provider-independent proposal validation, review confirmation and ports (`DocumentStore`, `Store`, `ContractExtractor`, `ContractAvailability`). `GeminiContractExtractor` is an HTTP infrastructure adapter, not domain matching logic. PostgreSQL supplies the current `DocumentStore` implementation.

Immutable `ContractSourceDocument` identity, client, sanitized original filename, PDF MIME, size, SHA-256, uploader/time and sensitive bytea preserve original bytes. Lifecycle/status/revision/latest attempt and confirmation metadata are separate. Metadata list/detail selects exclude PDF bytes. Scoped download serves only the preserved bytes, inline PDF, `private, no-store`, `nosniff`.

Extraction attempts retain provider/model/schema/prompt versions, start/end, normalized proposal, safe failure category and token counts. Proposal JSON is sensitive in generated Ent string/JSON representations; explicit authorized DTO mapping is the only intended exposure. Terminal attempt changes are guarded by STARTED status. Final user values and confirmation fingerprint are persisted separately from AI proposal. Contract source document/extraction IDs provide provenance. No deletion or retention mutation exists.

## ARMQU CONTRACT EXTRACTION DISCOVERY

| Concept | Source file/function in armqu | Diana target | Decision | Rationale |
|---|---|---|---|---|
| Native PDF extraction | `armqu-be/internal/contracts/parser.go`, `ExtractContractTermsFromPDFStream` | Gemini adapter | Adapt concept | Send actual PDF, including scans; no text-only assumption. |
| Missing fields | Same parser, pointer-valued extraction | V1 Field.value | Adapt | Null is distinct from default/inferred value. |
| Storage port | `armqu-be/internal/storage/storage.go` | DocumentStore | Adapt boundary only | No new GCS dependency or deployment requirement. |
| Upload/poll/review | `armqu-fe/src/features/inventory/hooks/usePartnerDocuments.ts`, `PartnerDocumentUploadDialog.tsx` | Ingestion hooks/pages | Adapt lifecycle | Actual asynchronous persisted states, no simulated progress. |
| Existing contract preview | `armqu-fe` contract `DocumentPreview.tsx` | PDF.js canvas preview | Do not copy WYSIWYG | Original PDF, not a reconstructed contract document. |
| Commercial float values / amendment merge | armqu parser and partner-document UI | Existing Contract model | Do not copy | Exact decimal money; no implicit amendments or new matching semantics. |

## PDF / scanned PDF handling

Upload requires nonzero bounded bytes, PDF header and EOF marker, .pdf filename, allowed MIME (PDF/octet-stream). Default maximum is 20 MiB via CONTRACT_MAX_PDF_BYTES. Filename is basename-only, control-character stripped and bounded. Size/MIME/signature checks are admission checks, not a complete malware/PDF parser. Before provider invocation, source size/hash are rechecked.

Provider receives the entire inline base64 native PDF, not locally extracted text. Gemini native vision supports image-only scanned PDFs; no separate OCR dependency is introduced. The deterministic `scanned` fixture is a readable image-only synthetic contract rendered with a dependency-free bitmap font; it proves the byte transport/state boundary, NOT real-provider OCR accuracy. Live OCR accuracy is intentionally not claimed.

PDF.js `6.2.108` is used with a locally bundled worker and canvas-only rendering: no scripting manager, annotation actions, links, embedded attachments or public document URL. Loading/render tasks are cancelled on replacement/unmount. Page evidence navigates the original. Patched version selected after [the PDF.js security advisory](https://github.com/advisories/GHSA-hq66-cqwq-w95j); unrelated existing moderate Vitest audit findings were not upgraded in this module.

## GEMINI INTEGRATION

Verified mechanism: POST `https://generativelanguage.googleapis.com/v1beta/interactions`, API key in `x-goog-api-key` header, configurable model (default `gemini-3.8-flash`), `store:false`, `system_instruction`, native inline `document` plus extraction instruction. `response_format` requests JSON text with a strict schema; temperature 0 and thinking summaries disabled. Only completed model-output text is decoded; reasoning/thought steps are ignored. Input/output token counts are retained, never raw provider payload or credentials.

Official references: [Interactions API](https://ai.google.dev/api/interactions-api), [structured output](https://ai.google.dev/gemini-api/docs/structured-output), [native PDF processing](https://ai.google.dev/gemini-api/docs/document-processing), [Interactions overview](https://ai.google.dev/gemini-api/docs/interactions-overview). Provider/model availability should be rechecked before deployment; no live request was made by Codex.

Configuration: GEMINI_API_KEY (backend secret only), GEMINI_CONTRACT_MODEL, GEMINI_API_BASE_URL, CONTRACT_EXTRACTION_TIMEOUT (90s), CONTRACT_MAX_PDF_BYTES (20 MiB), CONTRACT_EXTRACTOR_MODE (`gemini`, or `fake-fixtures` only with APP_ENV=test). Fake mode recognizes ONLY exact generated fixture hashes and rejects arbitrary user PDFs. API uploads do not require a live key; actual Gemini extraction does. Existing WORKER_JOB_TIMEOUT defaults to 2m and extraction lease to 3m; keep provider timeout below job timeout and lease when customizing.

## EXTRACTION SCHEMA / FIELD MATRIX

Schema `CONTRACT_EXTRACTION_V1`; prompt `CONTRACT_EXTRACTION_PROMPT_V1`. Each field has nullable value, PRESENT/MISSING/AMBIGUOUS status, AI-declared HIGH/MEDIUM/LOW/UNKNOWN confidence, optional 1-based page + snippet, and alternatives. There is no invented calibrated confidence score and no persisted chain-of-thought.

| Field | AI extracts? | Required for Contract? | Normalization | Deterministic validation | Evidence | User edits? |
|---|---|---|---|---|---|---|
| supplierName | Yes | Yes | Trim | Nonempty, bounded | Page/snippet | Yes |
| supplierCui | Yes | Yes | Trim; frozen matching normalization separately | Romanian structural identifier, nonzero; no checksum claim | Page/snippet | Yes |
| reference | Yes | Yes | Trim | Nonempty, bounded | Page/snippet | Yes |
| effectiveFrom | Yes | Yes | ISO YYYY-MM-DD | Real date, ordered interval | Page/snippet | Yes |
| effectiveTo | Yes | Yes | ISO YYYY-MM-DD | Real date, ordered interval | Page/snippet | Yes |
| totalValue | Yes | Yes | Exact decimal string | Existing signed decimal policy, numeric(20,4), no float | Page/snippet | Yes |
| currency | Yes | Yes | Uppercase | Supported ISO code, no RON default | Page/snippet | Yes |
| unitType | Yes | Yes | Trim | Nonempty, bounded | Page/snippet | Yes |
| paymentTerms | Yes | Yes | Trim | Nonempty, bounded | Page/snippet | Yes |
| buyerCui | Yes | No, safety guard | Uppercase/remove whitespace/optional RO comparison only | When present, must match owning client | Page/snippet | No tenant reassignment |

Supplier matching continues to use the frozen `invoicing.NormalizeBusinessIdentifier`; its punctuation behavior is unchanged. Buyer comparison does not remove arbitrary punctuation or rewrite supplier matching semantics.

## Hallucination guards / evidence

Document text is untrusted data, never instructions. Prompt forbids invented parties, reference, dates, value/currency, inferred default terms, external lookup or matching. Missing fields are null; ambiguities expose alternatives. Strict JSON schema + decoder rejects unknown/malformed fields, invalid enums, contradictory MISSING/non-null values, invalid money/currency/date proposals and trailing output. PRESENT requires value + evidence snippet. Prompt injection fixture and transport tests verify the application boundary; deterministic fake tests are not proof that a live model is immune to injection. Human review remains mandatory. Provider evidence itself is not independently verified/calibrated by the application.

## Lifecycle / failure model

UPLOADED → EXTRACTING → READY_FOR_REVIEW or EXTRACTION_FAILED → CONFIRMED after explicit human command.
Re-extraction from failed/ready state returns UPLOADED with incremented revision and a new event; attempts remain in history. Confirmed documents cannot be re-extracted. Extraction success/confirmed states are no-op on redelivery. Revision CAS protects extraction claims. Active lease returns retryable busy; expired STARTED attempt becomes FAILED/LEASE_EXPIRED before a replacement attempt starts.

429/5xx/transport failures are retryable; timeout persistence uses a bounded cancellation-independent context. Invalid output/auth/provider rejection is permanent with generic category, never provider body in UI/logs. Asynq retries are bounded by existing runtime settings. A worker crash relies on queued redelivery/lease recovery; exhausted queue archives and operational remediation remain visible through the existing worker system. No fake progress or silent manual fallback.

## Review / authoritative confirmation / Contract creation

Responsive original/fields layout; persisted states survive refresh. Ready proposals prefill all existing commercial fields, show missing/ambiguous markers, AI-declared confidence, snippets/page links and correction markers. Buyer mismatch blocks confirmation. Native required-field validation complements backend deterministic validation. Stale 409 responses refetch latest proposal and block the stale form. User-facing copy explicitly distinguishes proposal from authoritative data.

Confirmation transaction checks owning client, latest successful attempt, document revision/state and buyer guard; claims revision; creates ordinary Contract with final user-reviewed exact fields; stores final values/reviewer/time/fingerprint; marks CONFIRMED; writes safe audit and durable IDs-only activation outbox. Source/proposal are never overwritten. Only the authoritative Contract participates in the frozen candidate set.

Idempotency key is scoped by client/document and fingerprint of reviewed values. Exact replay returns same Contract without mutation, even with original revision; changed payload/key/attempt after confirmation conflicts. CAS + unique provenance indexes prevent two reviewers producing two Contracts. Frontend key stays stable for unchanged review values across retry. No automatic confirmation, correction learning, or AI matching.

## CONTRACTAVAILABLE / Missing Contract Resume E2E

After commit, service calls frozen `contracts.Service.ContractAvailable` with stable `contract-ingestion:<documentId>:available`. Transactional CONTRACT_ACTIVATION_REQUESTED provides crash recovery through a dedicated Asynq handler using exactly that same command. Frozen ContractAvailable idempotency emits one logical arrival and normal resume outbox. No direct invoice/task mutation in ingestion.

The existing Module 4 policy alone decides: unique compatible → resume; multiple plausible → match-confirmation task; irrelevant → preserve same missing task including WAITING. Integration tests cover all three paths after upload/extraction/human confirmation and consume the actual frozen arrival processor. Playwright covers backend-connected upload/edit/confirm/provenance and the missing-contract invoice journey using actual outbox/Redis worker, not mock repository continuation.

## Data model / migrations / audit / observability

Additive migration `backend/migrations/000012_contract_ingestion_ai.sql`; prior migrations are not rewritten. Client/hash uniqueness, client/status, latest/confirmed lookup, attempt history and Contract provenance indexes are included. `atlas.sum` regenerated. Both incremental and empty paths require user execution below, not claimed as run.

Safe business events cover upload, extraction start/success/failure, final human confirmation and frozen availability/resume. Full source and exact proposal/final values remain in authorized provenance storage, not duplicated in generic activity snapshots. Actor grant is required at domain and scoped HTTP boundaries; injected RequestActor is preserved. Existing demo AllClients fallback remains demo-only, NOT production authentication/RBAC. Private backend bytea is not a new at-rest encryption claim; production backup/encryption/access policies still apply.

Counters for document uploads, extraction completion/failure/duration and confirmation, plus existing outbox statistics. Worker trace/log metadata is IDs, correlation/job/attempt/duration only. Extraction counters count worker invocations including retries/no-op deliveries, not billing-grade unique-document analytics. No PDFs, snippets, commercial snapshots or API keys in new logs.

## Tests implemented / checks actually executed

Domain: upload validation/duplicate/safe filename, fixture states/no autoactivation, corrections preserve proposal, transient/permanent failure, tenant isolation, required fields/buyer mismatch. Provider: mock HTTP request/schema/store:false/PDF bytes/injection boundary, safe categories, malformed output/missing fields. Fixtures: Romanian, English, scanned image-only, missing, ambiguous, injection + golden values. PostgreSQL: duplicate/provenance/idempotency, retry/stale review, buyer/cross-client, concurrent confirmation, unique/multiple/irrelevant resume, actual HTTP upload/download/confirmation/auth boundary. Asynq: IDs-only routing/malformed/retry categories and real Redis duplicate/retry transport. Frontend: proposal boundary, explicit corrections, evidence navigation, missing currency, mismatch, failed/processing states. Dedicated backend-connected Playwright file and isolated test-only seed.

Actually executed by Codex: gofmt, Ent generation, Atlas hash, git diff --check, compile-only Go (`-run '^$'`), integration compile-only, Redis integration compile-only (also combined tags), npm typecheck, direct TypeScript no-emit checking of the new Playwright config/spec, npm dependency install/audit. No runtime unit/integration/Redis/race/frontend/Playwright tests, Docker startup, production build, migration apply or live Gemini call were executed. Compilation is not runtime test PASS.

## MANUAL TEST COMMANDS FOR USER

Run from repository root. Integration prerequisites: PostgreSQL/Redis available; migrations applied to a dedicated test database; never point Redis integration at shared data (existing harness FlushDB requires a dedicated nonzero DB).

### A. Fast/static checks

```sh
npm run typecheck
git diff --check
cd backend
GOCACHE=/private/tmp/diana-go-cache go generate ./ent
atlas migrate hash --dir file://migrations
GOCACHE=/private/tmp/diana-go-cache go test -run '^$' ./...
GOCACHE=/private/tmp/diana-go-cache go test -tags=integration -run '^$' ./internal/platform/postgres
GOCACHE=/private/tmp/diana-go-cache go test -tags=redis_integration -run '^$' ./internal/workerruntime
cd ..
```

### B–E. Document / schema / mock Gemini / PDF tests

```sh
# B
cd backend
GOCACHE=/private/tmp/diana-go-cache go test ./internal/contractingestion -run '^TestContractDocument'
# C
GOCACHE=/private/tmp/diana-go-cache go test ./internal/contractingestion -run '^TestContractExtractionSchema'
# D: deterministic httptest transport only, no credentials
GOCACHE=/private/tmp/diana-go-cache go test ./internal/contractingestion -run '^TestGeminiContractExtractor'
# E: actual synthetic image-only PDF boundary; not live OCR accuracy
GOCACHE=/private/tmp/diana-go-cache go test ./internal/contractingestion/fixtures ./internal/contractingestion -run 'TestContractPDF|TestContractGolden|TestContractExtractionFixtures'
cd ..
```

### F–I. PostgreSQL persistence / lifecycle / confirmation / concurrency

```sh
export TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_ci_test?sslmode=disable'
cd backend
# F
GOCACHE=/private/tmp/diana-go-cache go test -tags=integration ./internal/platform/postgres -run 'TestContractIngestionPersistence|TestContractIngestionHTTP'
# G
GOCACHE=/private/tmp/diana-go-cache go test -tags=integration ./internal/platform/postgres -run 'TestContractIngestionExtractionFailure|TestContractIngestionBuyerMismatch'
# H
GOCACHE=/private/tmp/diana-go-cache go test ./internal/contractingestion -run 'TestContractExtractionHumanCorrection|TestContractConfirmation'
GOCACHE=/private/tmp/diana-go-cache go test -tags=integration ./internal/platform/postgres -run '^TestContractIngestionPersistence'
# I
GOCACHE=/private/tmp/diana-go-cache go test -race -tags=integration ./internal/platform/postgres -run '^TestContractIngestionConcurrentConfirmation' -count=10
cd ..
```

### J–O. Queue / resume / frozen regressions

```sh
export TEST_REDIS_URL='redis://127.0.0.1:6382/14'
cd backend
# J: /14 must be dedicated: harness FlushDB before/after
GOCACHE=/private/tmp/diana-go-cache go test ./internal/workerruntime -run '^TestContractExtraction'
GOCACHE=/private/tmp/diana-go-cache go test -tags=redis_integration ./internal/workerruntime -run '^TestRealAsynqContractExtraction'
# K
GOCACHE=/private/tmp/diana-go-cache go test -tags=integration ./internal/platform/postgres -run '^TestContractIngestionMissingContractResumeE2E'
# L
GOCACHE=/private/tmp/diana-go-cache go test ./internal/contracts
GOCACHE=/private/tmp/diana-go-cache go test -tags=integration ./internal/platform/postgres -run 'Contract|Match'
# M
GOCACHE=/private/tmp/diana-go-cache go test ./...
GOCACHE=/private/tmp/diana-go-cache go test -tags=integration ./internal/platform/postgres
GOCACHE=/private/tmp/diana-go-cache go test -tags=redis_integration ./internal/workerruntime
# N
GOCACHE=/private/tmp/diana-go-cache go test ./internal/spv
GOCACHE=/private/tmp/diana-go-cache go test -tags=integration ./internal/platform/postgres -run 'SPV|Spv'
# O
GOCACHE=/private/tmp/diana-go-cache go test ./internal/saga
GOCACHE=/private/tmp/diana-go-cache go test -tags=integration ./internal/platform/postgres -run 'Saga|SAGA'
cd ..
```

### P–R. Frontend / dedicated ingestion E2E / frozen Playwright

```sh
# P
npm run test -- src/features/contracts/ContractDocumentReviewPage.test.tsx
# Q: use a fresh dedicated database; createdb fails safely if it already exists
docker compose up -d postgres redis
docker compose exec -T postgres createdb -U diana diana_ci_e2e
export DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_ci_e2e?sslmode=disable'
export REDIS_URL='redis://127.0.0.1:6382/13'
export WORKER_QUEUE='contract-ingestion-e2e'
cd backend
atlas migrate apply --env local
APP_ENV=test GOCACHE=/private/tmp/diana-go-cache go run ./cmd/contractingestionseed
cd ..
npm run test:e2e:contract-ingestion
# R: clear the E2E overrides before running existing configurations
unset DATABASE_URL REDIS_URL WORKER_QUEUE
npm run test:e2e
npm run test:e2e:backend1
npm run test:e2e:backend3
npm run test:e2e:backend4
npm run test:e2e:backend5
npm run test:e2e:backend6
npm run test:e2e:backend7
npm run test:e2e:spv-connection
npm run test:e2e:saga-export-ux
```

E2E /13 must be dedicated and initially empty; this configuration does not flush/reset it or delete database rows. Repeat Q with a new isolated database name and empty Redis namespace. Seed refuses an existing fixture instead of deleting anything. Existing frozen configuration scripts retain their existing behavior.

### S–T. Atlas incremental / empty migrations

```sh
# S: create dedicated database and advance exactly the first 11 migrations
docker compose exec -T postgres createdb -U diana diana_ci_incremental
export DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_ci_incremental?sslmode=disable'
cd backend
atlas migrate apply --env local 11
atlas migrate status --env local
atlas migrate apply --env local
atlas migrate status --env local
cd ..
# T: fresh empty database, no reset/drop
docker compose exec -T postgres createdb -U diana diana_ci_empty
export DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_ci_empty?sslmode=disable'
cd backend
atlas migrate apply --env local
atlas migrate status --env local
cd ..
```

For F–O preparation, create `diana_ci_test` with the same createdb command, set TEST_DATABASE_URL, then `cd backend && atlas migrate apply --env test`; do not use a production/shared database.

### U. Production build

```sh
npm run build
```

### V. OPTIONAL LIVE GEMINI SMOKE TEST

Explicit opt-in only: incurs provider processing/cost and sends the chosen PDF to Google. Use a non-sensitive synthetic PDF or an approved document. Set the real key securely in your shell; never commit it. Model must be explicitly selected.

```sh
export GEMINI_CONTRACT_MODEL='gemini-3.8-flash'
# Set GEMINI_API_KEY securely before invoking this command.
cd backend
GOCACHE=/private/tmp/diana-go-cache go run ./cmd/contractextract -file /absolute/path/to/approved-contract.pdf
cd ..
```

## Decisions / conflicts / intentionally deferred

No new product decision blocks the human-confirmed baseline. Existing Contract requires all nine commercial fields; missing values must be filled from the original during review, never invented. Structurally invalid or entirely missing extraction is failed rather than opening a manual-from-zero form. No frozen matching normalization or contract-value meaning has been changed.

Deferred: full amendment/versioning semantics (no implicit merge/replacement), deletion/retention, generalized document AI platform, auto-confirmation, AI invoice-contract matching, learning from corrections, production auth/RBAC, production object storage migration, malware scanning/encrypted-storage operational policy, independent evidence verification, calibrated confidence, real-provider quality/OCR benchmark. These are not silently asserted as implemented.
