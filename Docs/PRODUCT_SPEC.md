# Product Specification

Status: APPROVED FOR FRONTEND VALIDATION

## Product purpose

SPV-to-SAGA helps an accountant follow incoming e-Factura invoices from a simulated SPV intake through archive, contract matching, duplicate detection, extraction, classification and automatic simulated SAGA export.

The accountant is involved only when:

1. multiple contracts match or a single contract is inconsistent;
2. no contract exists;
3. an invoice contains uncertain classifications.

## Approved workflow states

`DOWNLOADED`, `ARCHIVED`, `MATCHING`, `AWAITING_CONTRACT`, `AWAITING_MATCH_CONFIRM`, `DEDUPE_CHECKED`, `HEADER_READ`, `LINES_READ`, `CLASSIFIED`, `AWAITING_REVIEW`, `READY_FOR_SAGA`, `EXPORTING`, `EXPORTED`, `DUPLICATE`.

`READY_FOR_SAGA` is a frontend operational state. `DUPLICATE` is terminal. SAGA export is automatic after all required processing finishes and no blocking task remains.

## Human tasks

- Multiple/incompatible contract: show recommendation, confidence, fictitious signals and alternatives. The accountant confirms or selects another contract.
- Missing contract: the only MVP action is `SOLICITĂ CONTRACT`. The task remains waiting and the invoice remains `AWAITING_CONTRACT`.
- Classification: one task per invoice contains all uncertain lines and dimensions. Only uncertain items require review. An item may be accepted or corrected.

Classification has three dimensions: accounting account, VAT rate and deductibility. Each uncertain proposal exposes its proposed value, confidence, explanation, legal basis and review status. Demo legal information uses the exact disclaimer `Exemplu demonstrativ — bază legală nevalidată`.

Corrections update only invoice review state. They do not create, change or suggest rules.

## Client context

Dashboard, Tasks and Invoices support `TOȚI CLIENȚII`. A persistent selector controls the active scope, which must remain visually explicit. The only functional persona in the MVP is `CONTABIL`.

## Dashboard

The current-month dashboard uses exactly:

- Facturi în procesare;
- Necesită atenție;
- Pregătite pentru SAGA;
- Exportate în SAGA.

SPV and SAGA feedback appears contextually in Dashboard, Invoice Detail and relevant workflow states. There is no separate Operations page in the MVP.

The complete Dashboard combines exactly two perspectives: `Operational Overview` and `Needs Attention`. All values use the shared invoice/task repository, persistent client scope and a deterministic September 2026 application clock.

- `Facturi în procesare` includes current-month invoices in any non-terminal state and excludes only `EXPORTED` and `DUPLICATE`.
- `Necesită atenție` includes current-month invoices with an `OPEN` validation task. `WAITING`, duplicate and SAGA export failure do not enter this KPI.
- `Pregătite pentru SAGA` counts only `READY_FOR_SAGA`.
- `Exportate în SAGA` counts only `EXPORTED`.

Operational Overview exposes compact counts for every approved pipeline state, separate SAGA readiness/export/failure visibility, contextual simulated SPV intake, and a multi-client table when `TOȚI CLIENȚII` is active. Client rows may switch the existing global scope; no local Dashboard client filter exists.

Needs Attention provides concise deep links to the existing Contract or Classification context for `OPEN` tasks. `WAITING` items are shown separately as external conditions. Dashboard does not duplicate Task Inbox filtering or introduce task priority, escalation, retry, duplicate resolution, analytics, trends or performance metrics.

## Module 1

Module 1 is a core clickable workflow slice with four primary scenarios: happy path, multiple contract matches, missing contract and uncertain classification. It includes a minimal Dashboard, Invoice Detail workspace, pipeline visualization, deterministic mock repositories and automated tests.

Contracts are read-only in the MVP. Clients support list, selection and context only. Document preview is excluded.

The Module 1 UI is Romanian and desktop-first. Additional languages are outside Module 1. All product data is explicitly fictitious. The backend remains locked until `FRONTEND APPROVED`.

## Task Inbox

`/tasks` is the accountant's operational queue. It contains only contract-match review, missing-contract and classification-review tasks.

- The default view contains `OPEN` tasks that require action now.
- `WAITING` contains tasks for which an action was taken but an external condition is still required.
- `RESOLVED` remains available as history and does not dominate the inbox.
- Counts for `OPEN` and `WAITING` derive from the shared repository state.
- The persistent client selector scopes the inbox to all clients or one client.
- MVP filters are task type and task status. No priority, assignment, SLA or automatic prioritization exists.

Each row exposes client, supplier, invoice number/date/value, pipeline state, reason, task status and deterministic created/waiting time. Contract tasks also expose the proposed contract, supplied mock confidence and concise reasoning. Classification tasks expose the count of pending review items.

Task resolution reuses Invoice Detail. The return path preserves the inbox status/type filter and client scope. Missing-contract requests may be made directly from the inbox; the task moves from `OPEN` to `WAITING` and remains visible there.

Dashboard attention data, Task Inbox and Invoice Detail share the same mock repository state.

## Invoice workspace

`/invoices` is the accountant's dense, desktop-first invoice index. It follows the persistent client scope and exposes exactly supplier, invoice number, client, value, date, contract, pipeline status, unresolved issue count, supplied confidence and SAGA status. Search covers supplier and invoice number. Lightweight URL-backed controls cover useful sorting plus pipeline, attention and SAGA filters.

Issue counts derive only from unresolved contract, missing-contract and classification task state. Confidence is shown only when supplied by deterministic mock data; no threshold or confidence category is inferred. Contract ambiguity, missing contracts, external waiting, normal progression, SAGA progression/failure and terminal duplicates use explicit text and icons in addition to semantic color.

`DUPLICATE` remains a terminal, searchable and filterable invoice state. It has no task-resolution workflow and never advances to SAGA. A failed simulated SAGA export is visible, but retry behavior remains undefined and no manual export action exists.

## Complete invoice detail

`/invoices/:invoiceId` preserves the approved `Rezumat`, `Contract`, `Linii factură`, `Clasificare` and `Istoric` tabs. The selected tab is deep-linkable. The persistent header shows invoice, supplier, client, value, date, pipeline, attention and SAGA context.

- Rezumat explains the current processing or blocking state without duplicating the other tabs.
- Contract reuses the approved resolution flow and shows available reference, period, currency, value, unit type and payment terms for an associated mock contract.
- Linii factură contains source fields only: description, unit, VAT label, quantity, unit price, net value, VAT value, total and optional additional information.
- Clasificare shows account, VAT and deductibility for every available line. Confident dimensions are informative only; unresolved dimensions reuse the existing accept/correct workflow.
- Istoric shows deterministic fictitious actor, timestamp, event details and before/after values where available.

Invoice List, Invoice Detail, Task Inbox and Dashboard use one replaceable mock repository. Resolving a task updates all surfaces and automatic processing continues independently of the active route. Missing optional source data is represented as unavailable and is never fabricated.

## Contracts workspace

`/contracts` is a read-only, desktop-first contract index scoped by the persistent client selector. It exposes contract reference, client, supplier, effective period, currency, value, unit/commercial basis, payment terms and available source metadata. Search covers contract reference and supplier; a lightweight supplier filter is URL-backed. Contract status, lifecycle, renewal, risk and ownership concepts are not part of the product.

`/contracts/:contractId` presents the same approved contract data and derives associated invoices from the current invoice-to-contract association in the shared repository. Associated invoice links return to the existing Invoice Workspace and show current pipeline and attention context without duplicating invoice behavior.

Invoice contract review may deep-link to recommended or alternative contract details. The detail reuses only supplied deterministic match confidence and reasoning, labels these as fictitious signals, and explicitly avoids treating them as legal or accounting proof. Viewing a contract never resolves or changes a task. Contract selection remains exclusively in the approved invoice contract-review flow; missing-contract invoices still expose only `SOLICITĂ CONTRACT`.

The mock repository owns the coherent contract and invoice graph. Changing `selectedContractId` through the approved flow updates Invoice Detail, Invoice List, Task Inbox and contract-associated invoice views. Contracts remain read-only: create, edit, delete, upload and document preview are excluded.

## Rules administration

`/rules` is a desktop-first administration workspace for fictitious classification rules. It supports exactly three categories: `ACCOUNT` / Cont contabil, `VAT` / Cotă TVA and `DEDUCTIBILITY` / Deductibilitate. Rules have an explicit `GLOBAL` or `CLIENT_OVERRIDE` scope. In a selected-client context the list contains global rules plus overrides belonging only to that client.

The list exposes rule name, category, scope, client, version, effective period, classification result, legal basis and last update. URL-backed controls cover search across name, criteria and result, plus category, scope, client and current/history filters.

`/rules/:ruleId` shows the selected version's criteria-to-result mapping, effective period, exact demonstrative legal disclaimer and immutable version history. Existing versions are never edited in place. `Creează versiune nouă` preserves both history and rule scope. A client override may be created only from a global rule, is explicitly bound to one client and leaves its global parent unchanged. There is no unrestricted create-rule flow and no rule execution engine.

Invoice classification displays lightweight origin, reference and version traceability for supplied mock rule links. Following a link opens the exact referenced version; viewing or changing rule administration data never recalculates an existing invoice. All rule content remains fictitious and uses `Exemplu demonstrativ — bază legală nevalidată` rather than real accounting or tax logic.

## Clients workspace and integrated frontend

`/clients` provides a concise operational client index, not a CRM. It shows only client identity and state-derived counts for processing invoices, open and waiting tasks, SAGA readiness and exported invoices. The persistent selector scopes the list to all clients or the selected client.

`/clients/:clientId` answers what is currently happening for one client through compact invoice, open-task, waiting-task, contract, client-override and SAGA summaries. Its links reuse the existing Invoice, Task, Contract and Rule workspaces under that client's active scope. Client create, edit and delete behavior is excluded.

The client selector is the shared scope authority across Dashboard, Tasks, Invoices, Contracts, Rules and Clients. All-client context aggregates all client data; selected-client context limits client-owned resources while Rules additionally retain global baseline rules. Switching from an invoice, contract or client-override detail to a different client redirects to the corresponding safely scoped list. Switching between client detail contexts opens the newly selected client. This behavior prevents ambiguous cross-client presentation and is not an authorization model.

The feature-complete frontend uses canonical Romanian labels for pipeline, SAGA, task and classification concepts. Pipeline and task state unions remain unchanged, SAGA export remains automatic, contracts remain read-only, classification corrections do not create rules, and SAGA failures do not create validation tasks or retry actions.
