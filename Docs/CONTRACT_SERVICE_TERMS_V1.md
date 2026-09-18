# Contract Service Terms V1

V1 is deliberately limited to service contracts for accounting-firm clients.

Each confirmed contract may have ordered service terms with:

- service description;
- `FIXED_FEE`, `UNIT_RATE`, or `FIXED_TOTAL`;
- decimal price and ISO currency;
- unit;
- quantity source: fixed by contract, invoice-reported, user-confirmed, future external source, or unknown;
- optional fixed quantity and human-readable quantity driver;
- evidenced billing frequency or `UNKNOWN`;
- immutable extraction evidence.

No formula is executable and no HR integration or fuzzy/LLM invoice-line matching is introduced. Unknown remains unknown. Pricing completeness does not block contract identity/matching unless a submitted term itself is invalid.

The sanitized `service-indefinite` fixture represents:

- Servicii de contabilitate — `FIXED_FEE`, 500 RON, monthly;
- Salarizare și resurse umane — `UNIT_RATE`, 50 RON / `SALARIAT`, quantity driver “numărul efectiv de salariați”, quantity source `UNKNOWN`.

The legacy contract total remains readable and explicitly identified. A new contract without an actual total has `hasLegacyTotalValue=false`; its internal compatibility amount is not presented as a contractual total.

`CommercialVerificationResult` establishes `MATCH`, `MISMATCH`, `NEEDS_INPUT`, and `NOT_APPLICABLE` with deterministic exact-price primitives. It is not connected to accounting classification or automatic invoice blocking.
