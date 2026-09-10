# Open Questions

These questions do not block Frontend Acceptance. Module 7 introduces no new unresolved product decision.

## Frontend validation

- Does the completed desktop Dashboard density need adjustment after accountant usability validation?
- What visual confidence format works best for accountants? The current frontend displays supplied mock confidence directly and defines no thresholds.
- Which generic permission-denied placements are useful for component completeness?
- Should failed simulated SAGA exports eventually expose a retry action? No retry is implemented in the feature-complete frontend.

## Accounting and legal validation

- What are the real contract-matching tolerances?
- What confidence thresholds are acceptable for each classification dimension?
- Who owns and maintains global rules and client overrides?
- Which accounting, VAT and deductibility rules and legal bases are valid?

## Backend-only

- Confirm the current ANAF SPV API contract before integration.
- Obtain and validate the official SAGA import schema.
- Resolve the Python backend versus pgx/Ent/Atlas mismatch.
- Define authentication, persistence, storage, queues, observability, Gemini and MCP only after `FRONTEND APPROVED`.
