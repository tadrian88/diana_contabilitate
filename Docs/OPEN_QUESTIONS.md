# Open Questions

These questions do not block execution of the ANAF/SPV Connection UX test handoff.

## Frontend validation

- Does the completed desktop Dashboard density need adjustment after accountant usability validation?
- What visual confidence format works best for accountants? The frontend displays backend-supplied opaque confidence and defines no thresholds.
- Which generic permission-denied placements are useful for component completeness?
- Should failed simulated SAGA exports eventually expose a retry action? No retry is implemented in the feature-complete frontend.

## Accounting and legal validation

- Validate or replace the provisional `PROVISIONAL_V1` business duplicate fingerprint before treating it as a final accounting rule.
- What are the real contract-matching tolerances?
- Should a discovered contract outside its effective period produce MISSING_CONTRACT or CONTRACT_MATCH review?
- Which objective signals should replace `MODULE4_BASELINE_V1`, and should value/SKU ever participate?
- Should numeric confidence thresholds ever control review, and if so what are they for each classification dimension?
- Who owns and maintains global rules and client overrides?
- What are the validated account mappings, VAT rules, deductibility rules, and legal bases?
- How should overlapping rules be ordered beyond a direct client override replacing its global origin?
- Which values may an accountant enter when correcting ACCOUNT, VAT, or DEDUCTIBILITY?

## Backend-only

- Confirm the configured production FCTEL hostname during credentialed ANAF certification; official materials still expose conflicting current/legacy hostnames.
- Choose production object storage and retention/encryption-at-rest policy for immutable ANAF ZIP documents.
- Define the timezone of ANAF `data_creare` before converting the preserved raw timestamp.
- Confirm during credentialed ANAF certification whether the authenticated list response always exposes the complete top-level `cui` set, including an empty message window, and whether it can safely gate activation. The access-token claims inspected in ARMQU are not a proven CIF authorization source; the current UX explicitly reports identity verification as unavailable.
- Define production authentication/RBAC permission checks before exposing client integration commands beyond local development.
- Obtain an official SAGA XSD if SAGA Software publishes one; the current manual
  documents the element structure but no XSD or independent schema version.
- Define missing-contract `WAITING` resumption and expired-contract semantics before the relevant workflow module.
- Validate real contract-matching tolerances before replacing the Module 4 baseline policy in production.
- Define rule effective-date execution semantics before a production rule engine.
- Decide whether any future rule change may trigger explicit reclassification; Module 5 never reclassifies automatically.
- Define retry behavior for any future SAGA transport; file generation currently
  retries only transient infrastructure failures through the bounded worker path.
- Define contract OCR/import as a separate post-parity product phase.
- Define authentication and authorization before exposing the backend outside local development.
## Real SAGA integration

- **PRODUCT DECISION REQUIRED — REEXPORT AFTER CONFIRMATION:** Define a new
  workflow before an already human-confirmed artifact may ever be reopened or
  replaced. This module intentionally provides no unconfirm operation.
- **ARCHITECTURE DECISION REQUIRED — OPTIONAL LOCAL BRIDGE:** Manual,
  client-scoped download is implemented. A separately secured Windows bridge
  remains optional and deferred; no SAGA API/watched-folder contract was found.
- **SAGA FORMAT VERIFICATION REQUIRED — CreditNote:** Provide vendor guidance or
  a sanitized accepted storno import XML. Credit notes are safely rejected now.
- **SAGA EXPORT DATA GAP:** Replace demonstrative ACCOUNT/VAT/DEDUCTIBILITY rule
  results with explicit approved adapter values (`Cont`, numeric VAT confirmation,
  and `SAGA_DEFAULT`/`N50`/`I`) before real export can succeed.
