# Vânzări V1 — facturi emise

Status: IMPLEMENTAT pe ramura `feature/vanzari`, AȘTEAPTĂ TESTELE UTILIZATORULUI (`Docs/VANZARI_V1_TEST_HANDOFF.md`) (worktree `diana_worktrees/vanzari`). Decizii D-124…D-131.
Migrări: `000039_vanzari_direction.sql`, `000040_vanzari_classification_direction.sql`.

## Scop

Diana procesa doar facturile primite. V1 adaugă facturile **emise** de client:

- import din SPV (lista `T`, pe lângă `P`) și din `spvconnect import-fixture`;
- legare opțională de contractele în care clientul este furnizorul (de exemplu Locator);
- clasificare pe linie cu conturile clasei 7, 167, 419 și 472; creanța 4111 este implicită;
- XML SAGA „Ieșiri” generat, dar neexportabil până la un import acceptat în SAGA C.

Cazul de test este VICTORIA 1881 EVENTS S.R.L. (RO51741718): 19 facturi emise (iunie–august 2026) și 5 contracte de
închiriere de sală, harness în `test-data/victoria1881/` (în afara git).

## În afara V1

- 461 (debitori diverși; facturile IA BILET nu există în e-Factura), încasările, stornările și facturile de avans
  regularizate prin 419;
- împărțirea unei linii pe perioade (70x pentru lunile trecute, 472 pentru cele viitoare): un cont pe linie;
- validarea comercială a prețului pe vânzări (contractele sunt în EUR, facturate în lei la curs BNR; Diana nu are
  sursă de curs);
- exportul efectiv în SAGA Ieșiri, credit note, monedă străină, linii cu categoria E/O la export.

## Model

- **Factura** păstrează `supplier_*` ca fapt UBL. La o factură emisă, furnizorul este clientul însuși. Alături:
  - `direction` INCOMING | OUTGOING (implicit INCOMING pentru toate rândurile existente);
  - `customer_name`, `customer_identifier` (brut, poate fi CNP), `normalized_customer_identifier`,
    `customer_identifier_kind` CUI | CNP | OTHER.
  - Contrapartida (`Invoice.Counterparty()`) este furnizorul la primite și clientul la emise.
  - Indexul de duplicate (client, furnizor, număr, zi) rămâne neschimbat; la emise înseamnă numărul propriu + data.
- **Contractul** are `client_role` BUYER | SUPPLIER (implicit BUYER) și `buyer_name`, `buyer_cui`,
  `normalized_buyer_cui`.
- **CNP**-ul se stochează complet (e nevoie de el pentru SAGA `ClientCIF` și pentru învățarea pe partener) și se
  afișează mascat (`194***`) în API, UI, log-uri și în envelope-ul trimis la AI.

## Flux

1. **Ingestie.** Direcția vine din părțile XML: cumpărătorul = client → INCOMING (inclusiv autofacturarea);
   furnizorul = client → OUTGOING; altfel eroare permanentă, ca înainte. Tipul mesajului SPV rămâne informativ.
   Lista `T` are propriul cursor (`spv_connections.last_successful_sent_sync_at`).
2. **Matching.** Politica `OUTGOING_CONTEXT_V1`: candidați = contractele `SUPPLIER` ale clientului cu
   `normalized_buyer_cui` = clientul facturii; contractele în afara perioadei nu sunt candidați; moneda nu contează.
   Un candidat → legătură. Zero sau mai mulți → fără legătură. Factura nu așteaptă niciodată contract și nu primește
   task MISSING_CONTRACT (eveniment de audit `CONTRACT_NOT_REQUIRED_OUTGOING`).
3. **Validare comercială.** O singură constatare `NOT_APPLICABLE_OUTGOING`, CONFORM.
4. **Clasificare.**
   - `VAT_DEDUCTIBILITY` și `EXPENSE_TAX_TREATMENT` = `NOT_APPLICABLE`, finale, sursa `DIRECTION`.
   - `VAT_TREATMENT`: momentul urmează profilul clientului (fără TVA la încasare → IMMEDIATE/4427). Mențiunea
     „TVA la încasare” de pe factură care contrazice profilul este doar avertisment.
   - `ACCOUNT` = contul creditat: clasa 7, 167, 419 sau 472. 704/708 au analitice (7041/7081) și nu sunt postabile.
   - Învățarea (mapări de cont, cunoștințe aprobate) are cheia direcție + client.
5. **SAGA.** Readiness verifică identitatea (furnizor = client, client = cumpărător), reconcilierea facturii emise și
   liniile, apoi poarta `SAGA_C_DOMAIN_V2_OUTGOING_V1` neaprobată: „Formatul SAGA Ieșiri nu este încă verificat.”
   Factura rămâne în `AWAITING_REVIEW` cu task-ul de clasificare deschis, ca variantele nesuportate din D-109.

## Contracte cu clientul furnizor

Extragerea (`CONTRACT_EXTRACTION_PROMPT_V4_3`, schema `CONTRACT_EXTRACTION_V4_1`) numește explicit rolurile:
furnizor = Furnizor / Prestator / Vânzător / Locator / Sublocator; cumpărător = Beneficiar / Client / Cumpărător /
Locatar / Sublocatar. Extrage și `buyerName`. Rolul clientului se stabilește determinist la confirmare: cumpărătorul
este clientul → BUYER, furnizorul este clientul → SUPPLIER, niciunul → `BUYER_MISMATCH` ca înainte. Anexele au rolul
contractului de bază.

## Interfață

`/invoices` are tab-urile „Primite” și „Emise” (`?kind=issued`). În „Emise” prima coloană este „Client”
(cumpărătorul, CNP mascat), coloana clientului contabil devine „Emitent”, iar căutarea este după client și număr.
„Primite” nu se schimbă. Detaliul facturii emise arată clientul în locul furnizorului.

## Abateri de la plan

- Scenariile mock comune nu primesc o factură emisă, pentru că ar schimba numărătorile din Dashboard și Task Inbox.
  Tab-urile sunt acoperite de `src/features/invoices/IssuedInvoiceList.test.tsx`, care adaugă factura doar în test.
  Nu există suită E2E nouă.
- Golden-ul XML Ieșiri este verificat pe fragmente (`saga/outgoing_readiness_test.go`), nu byte cu byte.
- Task Inbox și Dashboard primesc direcția și clientul facturii, ca să numească partenerul corect.
