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

## Vânzări V1 (D-124…D-131)

- **Linii pe mai multe perioade:** o linie care acoperă luni trecute și viitoare (ex. GASHRI „Iunie–septembrie” facturat pe
  19.08) trebuie împărțită 70x (trecut/curent) și 472 (viitor). Diana decide un singur cont pe linie; împărțirea rămâne
  la contabil.
- **461 și facturile fără e-Factura:** IA BILET (seria EV) apare doar în jurnal, pe 461/708. Nu există sursă XML.
- **Verificarea SAGA Ieșiri:** lipsește un import acceptat în SAGA C. De confirmat:
  - numele fișierului (`F_<CIF emitent>_…`);
  - poziția și valorile `FacturaTVAIncasare`;
  - `ClientCIF` pentru CNP, pentru CUI fără RO și pentru placeholder-ul `0000000000000`;
  - `Cont` = contul de venit.
  Până atunci poarta `SAGA_C_DOMAIN_V2_OUTGOING_V1` rămâne neaprobată.
- **Categoriile E/O la export:** garanția VE18810067 (E, VATEX-EU-O, 167) nu trece de reconcilierea exportului, care
  cere categoria S.
- **704/708 nepostabile:** catalogul are analiticele 7041/7081, deci 704/708 nu pot fi alese direct. Decizie de produs
  comună cu 628 la achiziții (planul de conturi al clientului).
- **Contract încărcat după facturi:** facturile emise nu așteaptă contract, deci un contract confirmat ulterior nu le
  re-leagă.
- **Autofacturare:** o factură în care clientul e și furnizor, și cumpărător e tratată ca primită.
- **Placeholder CNP:** toți cumpărătorii anonimi cu `0000000000000` au aceeași cheie de învățare.
- **Validarea prețului pe vânzări:** contractele VICTORIA sunt în EUR, facturate în lei la cursul BNR; Diana nu are
  sursă de curs BNR.
- **Regimul TVA la încasare al VICTORIA:** facturile emise poartă mențiunea, dar contabilul înregistrează 4427 imediat.
  Diana urmează profilul (D-128). De confirmat cu contabilul.
