# Contract Intelligence & Invoice Compliance V1

Status: utilizatorul a raportat trecerea testului de integrare pentru clauze narative și a `make release-local` pentru versiunea anterioară. A doua trecere Gemini și confirmarea individuală a formulelor necesită încă testele runtime și gate-ul complet; nu reprezintă aprobare de producție, verificare Gemini live sau deployment reușit.

## Obiectiv și limite de autoritate

Un PDF contractual este trimis extractorului o singură dată pentru perechea SHA-256 + versiune de extractor. Rezultatul revizuit de om devine un snapshot comercial imuabil. Validarea facturilor citește exclusiv snapshot-ul, variabilele cu proveniență, aliasurile confirmate și factura; nu retrimite contractul la Gemini.

AI-ul produce propuneri neautoritative. Documentul este tratat ca date neîncrezătoare, nu ca instrucțiuni. Regula executabilă este un AST închis și validat; nu se evaluează cod sau formulă text generată de provider. Mapping-ul determinist folosește descrieri/SKU-uri/aliasuri confirmate. Pentru liniile neacoperite, motorul ordonează serviciile contractului după cuvintele comune și compatibilitatea unității de măsură (niciodată după preț) și pre-selectează cel mult o sugestie neambiguă; utilizatorul confirmă asocierea din ecranul facturii (D-116). Până la confirmare, linia rămâne `NEVERIFICABIL`.

## Înlocuire și corectarea unei încărcări greșite

Detaliul contractului oferă două acțiuni distincte, ambele cu confirmare și control optimist prin `expectedRevision`:

Documentele încă neconfirmate folosesc în continuare acțiunea existentă „Renunță la acest document” din ecranul de review; acțiunile de mai jos se aplică unui contract deja confirmat.

- „Scoate din utilizare” → `POST /api/v1/contracts/{id}/archive`: contractul și dosarul devin `ARCHIVED`. Dispar din lista activă și din matching, dar PDF-urile, snapshot-urile, rulările și legăturile facturilor rămân consultabile. Este potrivit pentru contracte valide expirate/înlocuite. Un contract nou/reînnoit cu aceeași referință poate fi confirmat într-un dosar nou, dar același PDF nemodificat rămâne deduplicat până la o operație explicită de corectare a încărcării.
- „Șterge încărcarea greșită” → `POST /api/v1/contracts/{id}/discard`: este permisă numai dacă nu există asocieri de facturi, candidați de matching, rulări comerciale sau intrări ledger. Contractul și dosarul sunt arhivate, iar documentele dosarului trec în `DISCARDED` și dispar din lista de documente. PDF-ul, clauzele și snapshot-urile rămân în baza de date pentru audit, nu sunt șterse fizic. Aceeași copie PDF și aceeași referință pot fi încărcate și confirmate din nou, cu document și dosar noi. Dacă există utilizare istorică, API-ul returnează `409 CONTRACT_IN_USE`; utilizatorul trebuie să aleagă arhivarea.

Migrarea aditivă `000019_contract_replacement.sql` înlocuiește cheile unice globale cu indexuri unice parțiale: referința doar pe contracte `ACTIVE`, SHA-256 doar pe documente nediscardate și referința dosarului doar pe dosare nearchivate. Rulările/snapshot-urile vechi nu sunt rescrise. Acțiunile sunt limitate la clientul autorizat, înregistrate în audit și idempotente la repetare; o revizie învechită produce `409`.

Pentru validare locală, rulează comenzile țintite din `CONTRACT_COMMERCIAL_VALIDATION_TEST_HANDOFF.md`, apoi `make release-local` (include gate-ul complet, migrarea bazei locale și reconstruirea serviciilor). În UI: deschide Contracte → contractul vizat → alege una dintre cele două acțiuni. Pentru încărcare greșită, confirmă eliminarea, apoi încarcă din nou același PDF. Pentru contract valid înlocuit, arhivează-l și încarcă documentul nou sau anexa aplicabilă; un act adițional trebuie legat de un contract încă activ. Nu folosi `make reset` pentru acest flux: șterge toate datele locale, nu doar un contract.

## Model persistent

Migrarea aditivă `000018_contract_commercial_validation.sql` adaugă:

- `contract_dossiers`: dosar per client/referință, cu `DRAFT`, `ACTIVE_PARTIAL`, `ACTIVE_COMPLETE` sau `ARCHIVED`;
- relația 1:N dintre dosar și `contract_source_documents`, rolul documentului și părintele explicit;
- `contract_clause_candidates`: text narativ, regulă normalizată, pagină/citat și starea review-ului;
- `contract_commercial_snapshots`: versiuni imuabile, interval efectiv, coverage și hash al regulilor;
- definiții/valori de variabile cu interval și proveniență;
- aliasuri de servicii confirmate și datate;
- run-uri, findings și excepții comerciale;
- ledger cumulativ pentru limite și reversări.

Migrarea creează pentru contractele legacy dosare `ACTIVE_PARTIAL`, fără a inventa reguli. Ele rămân disponibile matching-ului, iar validarea comercială produce `NEVERIFICABIL` până la confirmarea acoperirii. Snapshot-urile și run-urile istorice sunt numai inserate; un act adițional produce o versiune nouă.

## Ingestie și consolidare

Schema `CONTRACT_EXTRACTION_V4` / promptul `CONTRACT_EXTRACTION_PROMPT_V4_2` extrag rolul documentului, referința părinte, identități divergente, clauze narative, tabele/tier-uri, discounturi, tranșe, formule, variabile, TVA, FX, frecvență, scadență și dovada pagină/citat.

Dacă providerul omite `rule.id`, adaptorul îi atribuie un ID tehnic determinist din hash-ul PDF-ului și poziția clauzei. Dacă omite copia lui `rule.narrative` sau lista `rule.evidence`, adaptorul le preia numai din narațiunea/dovada deja prezentă în aceeași clauză. `rule.kind` se copiază numai când lipsește și `clause.kind` este exact un tip executabil acceptat; un tip necunoscut nu este convertit, iar două tipuri acceptate dar contradictorii sunt respinse. Nu completează tarife sau formule și nu elimină proprietăți necunoscute: restul regulii trece prin validarea strictă. ID-urile generate pentru PDF-uri diferite nu coincid, deci un act adițional nu suprascrie implicit o regulă anterioară; relația/suprascrierea necesită review explicit.

Pentru clauzele fără expresie AST, extractorul face **o a doua cerere Gemini în aceeași încercare de ingestie**, trimițând numai ID-ul, tipul, narațiunea, dovada și metadatele comerciale ale clauzelor extrase (maximum 32), niciodată PDF-ul. Răspunsul poate adăuga doar `expression` pentru ID-uri cunoscute; AST-ul este decodat strict și validat de motor. Necunoscutul, un calcul nevalid sau eroarea providerului lasă clauza narativă, nu oprește ingestia. Costul celor două apeluri este adunat în usage-ul încercării. Deduplicarea documentului/extractorului înseamnă că validarea facturilor nu repetă niciunul dintre apeluri.

Toate formulele AI, inclusiv cele produse în prima trecere, sunt inițial **neconfirmate**. UI-ul afișează separat formula, pagina și citatul, iar utilizatorul apasă „Confirmă regula” pentru fiecare regulă acceptată. O regulă neconfirmată cu AST valid este stocată `PROPOSED` cu `normalized_rule` păstrată; una fără AST este stocată `PROPOSED` cu `normalized_rule = NULL`. Niciuna nu intră în lista executabilă a snapshot-ului. Dacă rămâne cel puțin o clauză neconfirmată, serverul forțează `PARTIAL`, chiar dacă browserul trimite `COMPLETE`. Facturile aferente acoperirii lipsă rămân `NEVERIFICABIL`. Nu se construiesc automat reguli de preț din tarifele plate legacy când propunerea conține clauze comerciale: 500 RON nu devine tarif universal în locul unei grile. Confirmarea formulei cere și verificarea aplicabilității, monedei, perioadei și dovezii față de PDF; validitatea sintactică a AST-ului nu demonstrează corectitudinea economică.

Confirmarea unui `BASE_CONTRACT` creează contractul și dosarul. `ANNEX`, `AMENDMENT`, `SOW`, `ORDER` și `PRICE_LIST` cer `relatedReference` și cel puțin o regulă comercială cu identitate explicită; se atașează contractului existent din același client. O listă plată de tarife fără identificarea regulilor nu poate suprascrie automat contractul de bază. Un document suplimentar nu creează încă un contract de matching. Regulile cu același ID sunt singurele suprascrise; nu există regula implicită „cel mai nou câștigă”. Reguli suprapuse pentru același serviciu, dar cu identități diferite, marchează snapshot-ul `CONFLICTED`.

Dovada regulilor este reatașată server-side din propunerea imuabilă înainte de confirmare. Browserul nu poate fabrica pagina/citatul unei reguli. Divergențele dintre preambul, corp și semnături sunt cerute extractorului ca ambiguități/clauze `IDENTITY`; nu rescriu automat identitatea fiscală.

Coverage:

- `COMPLETE`: numai după confirmare umană explicită; regulile confirmate pot decide conformitatea;
- `PARTIAL`: matching-ul continuă, dar acoperirea lipsă produce `NEVERIFICABIL`;
- `CONFLICTED`: regulile conflictuale rămân vizibile și blochează decizia automată.

## Motorul determinist

AST-ul acceptă numai `literal`, `variable`, `add`, `multiply`, `percent`, `tier`, `min`, `max`, `prorate`, `fx` și `round`, cu zecimale exacte. Sunt modelate reguli de tarif fix/unitar, tier, discount, tranșă, prorata, minim/maxim, cost-plus, curs, TVA, scadență, frecvență, identitate, referință contractuală și storno.

Nu există toleranță procentuală implicită. Moneda prețului de linie este comparată cu regula; moneda totalului facturii nu este substituită într-o factură cu monede mixte. Dacă dovada monedei de linie lipsește, rezultatul este `NEVERIFICABIL`. O variabilă absentă, perioada absentă, milestone-ul/tranșa absentă ori o bază temporală indisponibilă produc `NEVERIFICABIL`, nu fallback. Ratele de curs pot fi introduse numai ca variabile cu sursă și referință; conectarea la un provider oficial live și calendarul de sărbători nu sunt configurate în această versiune, deci acele cazuri rămân review până la furnizarea datelor.

Pentru credit note/storno, motorul caută factura originală în același client și furnizor după referința UBL. Lipsa ei produce `ORIGINAL_INVOICE_UNAVAILABLE`; când există, sunt verificate moneda, semnul și faptul că valoarea absolută nu depășește originalul. Reconcilierea linie-cu-linie a unei corecții parțiale necesită mapping determinist al liniilor; altfel rezultatul rămâne `NEVERIFICABIL`.

Outcome-ul run-ului este:

- `CONFORM`: toate findings neexceptate sunt conforme;
- `NECONFORM`: există cel puțin o abatere;
- `NEVERIFICABIL`: nu există abatere certă, dar lipsesc date/reguli.

`NECONFORM` are prioritate față de `NEVERIFICABIL` pentru a nu ascunde o abatere certă.

## Pipeline și review

Fluxul este `LINES_READ → COMMERCIAL_VALIDATING → COMMERCIALLY_VALIDATED → CLASSIFIED`. Un rezultat neconform sau neverificabil mută factura în `AWAITING_COMMERCIAL_REVIEW` și creează task-ul `COMMERCIAL_REVIEW`.

Acțiunile sunt:

- completarea variabilei sau confirmarea aliasului, apoi `RERUN`;
- `ACCEPT_EXCEPTION` pentru un finding, obligatoriu cu motiv și actor;
- `WAIT_FOR_CORRECTION`, care păstrează factura blocată.

O factură fără contract, pentru care contabilul a ales „Continuă fără contract” (D-120), nu primește `CONTRACT_COVERAGE_INCOMPLETE` și nici `SERVICE_LINE_UNCOVERED`. Primește o singură constatare `CONTRACT_WAIVED`, `CONFORM`, care citează motivul, autorul și data deciziei. Verificările de storno rămân. Decizia contează numai cât timp factura nu are un snapshot contractual asociat.

Clasificarea este pornită numai după `CONFORM` sau după exceptarea explicită a tuturor findings blocante. Comenzile au idempotency key, optimistic revision și verificare de ownership pe client.

Ecranul facturii afișează outcome, valoare actuală/așteptată, calcul, date lipsă, document/pagină/citat și acțiunile disponibile. Review-ul documentului afișează rolul, referința părinte, coverage și regulile comerciale propuse.

## API

- `POST|GET /api/v1/clients/{clientId}/contract-dossiers`
- `GET /api/v1/clients/{clientId}/contract-dossiers/{dossierId}`
- endpoint-urile existente de upload/review/confirmare document; rolul și relația sunt în payload-ul confirmat;
- `POST /api/v1/clients/{clientId}/contract-dossiers/{dossierId}/variables`
- `POST /api/v1/clients/{clientId}/commercial-service-aliases`
- `GET /api/v1/clients/{clientId}/invoices/{invoiceId}/commercial-validation`
- `POST /api/v1/clients/{clientId}/invoices/{invoiceId}/commercial-validation/resolve`
- `GET /api/v1/clients/{clientId}/commercial-snapshots/{snapshotId}/revalidation-preview`

Preview-ul retroactiv este read-only. Revalidarea în masă nu pornește automat la activarea unui act adițional; comanda operațională bulk trebuie adăugată numai după stabilirea politicii pentru facturi deja exportate.

## Cazurile Beciul Domnesc

Asocierea autoritativă se face după client/CUI, nu după textul liber al liniei:

- COL30 aparține GRUP / contract 19. Prețurile de 750 EUR și 625 EUR corespund discountului de 75%, iar textul care indică contractul 20 produce `CONTRACT_REFERENCE_MISMATCH`;
- COL31 aparține SA / contract 20. Prețurile de 2.250 EUR și 1.875 EUR corespund discountului de 25%, iar referința la contractul 19 produce aceeași abatere;
- dacă documentele sunt storno, ambele cer factura originală; fără aceasta apare și `ORIGINAL_INVOICE_UNAVAILABLE`.

## Revizuire față în față și asocierea serviciilor — 2026-09-29

- **TVA separat de preț.** O clauză `VAT` produce `VAT_RATE_MATCH` / `VAT_RATE_MISMATCH` pe fiecare linie, cu sursa legală a cotei în `calculation`; `PRICE_*` rămâne doar pentru tarife. Versiunea motorului devine `COMMERCIAL_VALIDATION_V2`; rulările vechi rămân neschimbate până la `RERUN`.
- **Card grupat.** Verificările pe factură apar sus, cele pe linii sub antetul liniei; o cotă TVA conformă pe toate liniile apare o singură dată. Valorile sunt formatate ro-RO: „Pe factură” / „În contract”.
- **Verificare față în față** (`?tab=contract&verifica=<id>`). În stânga, extrasul din e-Factura cu rândul și valoarea comparată marcate și calea XML. În dreapta, PDF-ul original la pagina evidenței, cu fragmentele localizate în stratul de text pdf.js și evidențiate. O sumă care apare de mai multe ori este aleasă pe rândul serviciului. Navigarea se face cu ← →; excepțiile pentru `NECONFORM` se aprobă tot aici.
- **Asocierea serviciilor** (`?asociere=`). Toate liniile neacoperite apar într-un singur ecran, cu serviciile ca opțiuni (nu drop-down), semnalele de potrivire și rândul din PDF.
  - Sugestia este pre-selectată doar când e neambiguă; prețul nu este niciodată semnal.
  - „Ține minte pentru acest contract” este implicit bifat.
  - Confirmarea trimite toate asocierile, cu chei derivate din rulare (reîncercarea nu dublează), apoi `RERUN`.
- **Formulări învățate.** `GET /api/v1/clients/{clientId}/contract-dossiers/{dossierId}/service-aliases` (sau `.../contract-documents/{documentId}/service-aliases`) și `POST /api/v1/clients/{clientId}/commercial-service-aliases/{aliasId}/revoke`. Revocarea păstrează istoricul (migrarea `000036`), scrie `COMMERCIAL_ALIAS_REVOKED` în audit și permite reasocierea formulării.
  - Din 2026-09-30 (D-119), o asociere confirmată cu „Ține minte” se salvează de două ori: ca formulare a dosarului și ca asociere a facturii. Revocarea formulării (cu confirmare) afectează doar facturile viitoare.
  - „Serviciul nu apare în contract” oferă corecția facturii (`WAIT_FOR_CORRECTION`), excepția motivată pe linie (`ACCEPT_EXCEPTION`) sau încărcarea anexei.
  - Rularea afișată spune când contractul are între timp o versiune nouă (`activeSnapshotVersion`, citit la `GET`, nestocat) sau când formulările s-au schimbat, și oferă „Rulează din nou”.
  - Un tarif unitar cu cantitate fixă în contract (`CONTRACT_FIXED_QUANTITY`, neschimbată față de valoarea citată) primește automat valoarea variabilei `unit_quantity_*` cu sursa `CONTRACT`.
- **Activarea tarifelor.** Sumele cu separator de mii (1.800,00 · 1 800,00 · 1,800.00) sunt recunoscute. Răspunsul `{activated, skipped[]}` explică fiecare serviciu neactivat. Butonul de activare apare cât timp există tarife confirmate nefolosite, chiar dacă alte tarife sunt active. Regulile noi citează și rândul serviciului, nu doar suma, și poartă unitatea de măsură.

## Pagina documentului de contract și închiderea clauzelor — 2026-09-29

- **Spațiu de lucru.** Pagina `/contracts/documents/{clientId}/{documentId}` afișează valorile extrase grupate: părți, contract, plată, servicii și tarife, clauze.
  - Fiecare valoare apare lângă PDF-ul original, evidențiat la textul citat.
  - Navigarea: ↑/↓, `?element=field:<cheie>|service:<index>|rule:<ruleId>|group:services|group:clauses`.
  - Link-urile din factură duc direct la regula verificată.
- **Starea comercială a documentului.** `GET .../contract-documents/{documentId}` întoarce `commercialState` pentru un document confirmat, citit din snapshot-ul activ:
  - `coverage` și `snapshotVersion`;
  - `activeRuleIds`: regulile documentului prezente în snapshot;
  - `services[]`: pentru fiecare tarif confirmat, `active` sau `skipReason`, calculat cu aceleași verificări ca activarea, inclusiv `PRICED_BY_CONTRACT_CLAUSES`;
  - `clauses[]`: starea fiecărei clauze propuse (`PROPOSED`/`CONFIRMED`/`REJECTED`), cu motiv, cine și când.

  `confirmedValues.coverage` rămâne valoarea de la confirmare și nu descrie acoperirea curentă.
- **Clauze închise fără regulă** (migrarea `000037`: `review_reason_code`, `review_reason`, `proposal_rule_id`):
  - `COVERED_BY_CONTRACT_REFERENCE`: o a doua clauză de referință care conține referința principală;
  - `COVERED_BY_SUPPLIER_IDENTITY`: o clauză de identitate în care **toate** CUI-urile citate sunt CUI-ul furnizorului dosarului;
  - `COVERED_BY_PARTY_IDENTITY` (D-118): o clauză de identitate care citează CUI-ul clientului (singur sau alături de al furnizorului). Clientul este cumpărătorul verificat la confirmare și pe fiecare factură din SPV. Dacă apare un al treilea CUI, clauza rămâne în review;
  - `COVERED_BY_SERVICE_TARIFF` (D-118, doar sistemul): o clauză de preț fix/unitar care doar repetă un tarif activ din „Servicii și tarife” (același literal, aceeași monedă, fără aplicabilitate proprie, serviciul numit în text). O astfel de clauză nu mai poate fi confirmată, iar sugestiile de asociere nu o mai oferă lângă tarif.

    Se închid automat după confirmare (`CloseCoveredClauses`), la fiecare confirmare de regulă și după activarea tarifelor. Pentru contractele confirmate anterior: `go run ./cmd/contractrules close-covered-clauses`.
  - O regulă `IDENTITY` poate cita doar CUI-ul furnizorului (motorul o compară cu furnizorul facturii): altfel confirmarea este refuzată (`IDENTITY_RULE_NOT_SUPPLIER`).
  - `NOT_INVOICE_VERIFIABLE`: se închide cu `POST .../contract-documents/{documentId}/commercial-rules/{ruleId}/dismiss`, `{reasonCode, reason}`.
    - Motivul are 10–500 caractere.
    - Evenimentul `COMMERCIAL_CLAUSE_DISMISSED` intră în audit.
    - Reluarea e idempotentă. O clauză deja confirmată întoarce 409.
- **Avansarea snapshot-ului** (`advanceSnapshot`):
  - Acoperirea devine completă numai când dosarul nu mai are clauze propuse; `CONFLICTED` se păstrează.
  - Versiunea nouă este prima liberă din dosar.
  - Nu se scrie nimic dacă regulile și acoperirea nu se schimbă.

## Limitări explicite V1

- Nu s-a executat Gemini live și calitatea V4 pe PDF-urile reale nu este revendicată.
- Nu există încă adaptor live pentru BNR/ECB sau calendar oficial de zile nelucrătoare; valorile cu proveniență pot fi introduse prin API.
- Preview-ul retroactiv există, dar execuția bulk nu este implementată; istoricul nu se rescrie automat.
- Ledger-ul este pregătit de schemă, dar postarea și verificarea cumulativă automată pentru plafoane/consum contractual nu sunt implementate.
- Nu există mapping AI per factură. Sugestiile deterministe și confirmarea asocierilor (cu `RERUN` automat) sunt în ecranul facturii; asocierile învățate se pot revoca din pagina contractului. Variabilele lipsă pot fi completate în ecranul facturii cu valoare, perioadă și referință de proveniență.
- Validarea de TVA este limitată la comparația ratei de pe linie cu expresia confirmată; tratamentul fiscal complet și cursul oficial nu sunt deduse automat.
- Confirmarea unei anexe este disponibilă prin fluxul de document, dar conflictele de clauze nu au încă un editor dedicat de reconciliere; `CONFLICTED` blochează validarea automată.
