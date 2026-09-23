# Account Learning Loop

Status: implemented; real SPV acceptance remains pending a user-selected invoice.

## Runtime boundary

Diana owns the `accounts`, `account_mappings`, and `account_mapping_versions` tables. The runtime does not call or read ARMQU. The initial account rows are application configuration supplied for Diana; their presence is not legal evidence that every analytic subaccount is explicitly prescribed by OMFP 1802/2014.

## Postable limitation

The supplied catalogue distinguishes hierarchy through `is_synthetic` but has no operation-specific posting policy. For the MVP, migration `000020_account_learning.sql` derives the technical `postable` selector flag as `NOT is_synthetic`. Backend confirmation additionally requires the account to be active and postable.

The classification input also carries the active/postable catalogue vocabulary loaded from PostgreSQL. A deterministic ACCOUNT rule is not production-eligible merely because its result is lexically shaped like an account code; its result must also be present in that catalogue vocabulary.

This is deliberately narrow: `postable=true` means only that Diana permits selecting the leaf entry. It does not assert that the account is legally or economically appropriate for a particular invoice. That remains part of the accountant's human decision.

## Deterministic identity

Lookup is scoped by client and normalized supplier, then evaluates exact identities in this order:

1. `SELLER_ITEM_ID` with `EXACT_IDENTIFIER_V1`;
2. `STANDARD_ITEM_ID` with `EXACT_IDENTIFIER_V1`;
3. `NORMALIZED_DESCRIPTION` with `NORMALIZED_DESCRIPTION_V1`.

Description normalization applies Unicode NFKC normalization, Unicode case folding, trim/whitespace collapse, and punctuation-as-separator normalization. Numbers, dates, months, years, quantities, SKUs, and identifiers are retained. There is no fuzzy, semantic, embedding, or LLM match.

All exact identities are checked for contradictions. Conflicting mappings, or a learned mapping that conflicts with a deterministic rule, produce an ambiguous pending review rather than a silent winner.

## Review semantics

- The safe UI default is `OCCURRENCE_ONLY`; reusable learning is never preselected.
- Before a reusable action is confirmed, the backend-provided preview shows the client, stable supplier display, exact identity kind/value, and normalizer version.
- `CREATE` creates the reusable mapping and immutable version atomically with the current classification review.
- `VALIDATE` records the unchanged learned proposal as `ACCEPTED` for the current occurrence and leaves the mapping version unchanged.
- `OCCURRENCE_ONLY` applies a changed account only to the current occurrence.
- `CORRECT` appends an immutable `CORRECTION` version using mapping revision CAS.
- `POLICY_CHANGE` requires a reason and appends an immutable `POLICY_CHANGE` version using mapping revision CAS.

Every reused mapping remains a pending proposal. It is never auto-accepted merely because a previous accountant created it.

At every new classification, the referenced account is checked against the active, postable catalogue loaded in the same PostgreSQL read snapshot as the mapping. If that account has since become inactive or non-postable, the historical mapping and all immutable versions remain intact, but the mapping cannot produce a `LEARNED_MAPPING` proposal; the occurrence remains on the existing unresolved/review path.

Mapping suggestions, conflicts, creation, validation, occurrence-only exceptions, corrections, and policy changes emit dedicated activity/audit events in the same transaction as the classification operation.

## Acceptance boundary

Automated integration tests use generic fixtures and execute the real database learning path. They do not claim that the real-invoice acceptance passed. That acceptance must still be performed with two real invoices imported through Diana/SPV.
