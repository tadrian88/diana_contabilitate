# Project State

## Vânzări V1: facturi emise, contracte cu clientul Locator, SAGA Ieșiri blocat (D-124…D-131) — 2026-09-30

- IMPLEMENTAT PE RAMURA `feature/vanzari` — AȘTEAPTĂ TESTELE UTILIZATORULUI. Design: `Docs/VANZARI_V1.md`; comenzi:
  `Docs/VANZARI_V1_TEST_HANDOFF.md`.
- Utilizatorul a aprobat explicit extinderea aditivă a zonelor „FROZEN”:
  - ANAF/SPV ingestion (lista `T`, direcția din părțile XML, `fakeanaf -emit-sent`);
  - Module 2/4 (matching `OUTGOING_CONTEXT_V1`: contract opțional, fără task);
  - Module 6 (validare comercială `NOT_APPLICABLE_OUTGOING`);
  - clasificarea (sursa `DIRECTION`, TVA din profil, conturi 7/167/419/472, prompt `UNIFIED_ACCOUNTING_PROMPT_V4`);
  - SAGA (generator Ieșiri, poarta `SAGA_C_DOMAIN_V2_OUTGOING_V1` neaprobată);
  - extragerea contractelor (`CONTRACT_EXTRACTION_PROMPT_V4_3`, `buyerName`, rolul clientului);
  - frontend-ul (tab-uri „Primite”/„Emise”, clientul facturii emise, revizuirea contractului cu clientul Locator).
- Migrări: `000039_vanzari_direction.sql`, `000040_vanzari_classification_direction.sql` (hash recalculat).
- Harness VICTORIA (în afara git): pasul `contracte`, import pe ambele direcții, comparația vânzărilor în `compara.py`.
- Verificare: doar statică de Claude (`go build`, `go vet` inclusiv cu tag-ul de integrare, `atlas migrate validate`,
  `tsc`, `py_compile`). Testele Go unit/integration, vitest, E2E și harness-ul urmează să fie rulate de utilizator.

## Retrieval Cod fiscal + linii neimpozabile (categoria O) — 2026-09-30

- IMPLEMENTAT PE RAMURA `feature/victoria-iteratia-2`, pornită din `feature/victoria-iteratia-1` — AȘTEAPTĂ TESTELE UTILIZATORULUI.
- Ce s-a schimbat:
  - pentru achiziții, AI-ul primește mereu articolele 282, 297–299 și 25 din Codul fiscal, selectate după cheia exactă;
  - limita pe fragment urcă de la 20.000 la 30.000 de caractere; art. 25 (circa 26.100) nu încăpea niciodată.
- Linii cu categoria TVA „O” (D-123):
  - cota sursă este 0 în validarea AI și în dialogul de corecție;
  - înainte, toate cele 16 propuneri pe astfel de linii erau invalide, iar corecția manuală era blocată.
- Frontend „FROZEN” extins cu aprobarea utilizatorului: `DomainCorrectionDialog`. Fără migrări.
- Verificare: doar statică de Claude (`gofmt`, `go build`, `go vet`, `tsc`).

## Retrieval: funcțiunea conturilor OMFP ajunge la AI + metrică pe cont sintetic în harness — 2026-09-30

- IMPLEMENTAT PE RAMURA `feature/victoria-iteratia-1` — AȘTEAPTĂ TESTELE UTILIZATORULUI.
- Prima rulare a harness-ului VICTORIA a dat:
  - TVA-ul (4426/4428) corect pe 20 din 20 de linii;
  - contul de cheltuială exact pe 0 din 28 de linii.
- Cauzele identificate:
  - 628, 622 și 623 sunt nepostabile în catalogul Diana, iar contabilul folosește 628 direct;
  - aproape nicio propunere AI nu primea textul OMFP despre conturi.
- Schimbarea:
  - pentru achiziții, AI-ul primește mereu funcțiunea conturilor 601–628, 231 și 471, selectate după cheia de citare exactă, primele în plan;
  - bugetul de retrieval crește de la 60.000 la 75.000 de caractere, pentru că la prima variantă articolele din Codul fiscal umpleau bugetul și din funcțiuni intra doar „contul 601”;
  - `legislation.Query` are câmpul nou `CitationKeys`.
- Fără migrări. Harness-ul (`test-data/victoria1881/compara.py`, în afara git) raportează acum separat potrivirea exactă, cea compatibilă și cea pe același cont sintetic de grad I.
- Rămas deschis, decizie de produs: planul de conturi al clientului, adică 628 postabil pentru clienții care nu folosesc analitice.
- Verificare: doar statică de Claude (`gofmt`, `go build`, `go vet`, inclusiv cu tag-ul de integrare). Testele și rerularea harness-ului sunt rulate de utilizator.

## Consum AI: tokeni și cost pe rulare, client și cont (D-121, D-122) — 2026-09-30

- IMPLEMENTAT ÎN WORKSPACE — AȘTEAPTĂ TESTELE UTILIZATORULUI. Utilizatorul a aprobat explicit extinderea aditivă a zonelor „FROZEN”: frontend-ul (item „Consum AI” în sidebar, card pe pagina clientului) și backend Module 6 (rute API) și 7 (wiring worker, metrici). Au fost refactorizate și cele trei apeluri Gemini, cu aceeași taxonomie de erori.
- Fiecare apel Gemini (inclusiv retry-uri, 429/5xx și răspunsuri invalide) este un rând în `llm_usage_events`, atribuit rulării (analiză contabilă sau încercare de extragere a unui contract) și clientului ei. Se salvează și tokenii de raționament și cei din cache.
- Costul se calculează în USD cu prețuri versionate și append-only (`llm_model_prices`) și rămâne înghețat la momentul apelului (`numeric(20,10)`).
- Rulările istorice sunt importate ca `BACKFILL` (istoric parțial).
- API:
  - `GET /api/v1/ai-usage` (totalul contului);
  - `GET /api/v1/clients/{clientId}/ai-usage[/runs[/{runKind}/{runId}]]`.
- UI: pagina „Consum AI” și cardul „Consum AI” de pe pagina clientului.
- Migrare: `000038_llm_usage.sql`. Prețurile inițiale pentru `gemini-3.8-flash` (pagina Google din 2026-09-24) trebuie confirmate de utilizator înainte de `atlas migrate hash`.
- Documentație: `Docs/AI_USAGE_COST_TRACKING.md`, `Docs/AI_USAGE_COST_TRACKING_TEST_HANDOFF.md`.
- Verificare: doar statică de Claude (gofmt). Compilarea, testele Go unit/integration, vitest, E2E și migrarea urmează să fie rulate de utilizator.

## Continuare fără contract (D-120) + harness de test pe datele reale VICTORIA 1881 EVENTS — 2026-09-30

- IMPLEMENTAT ÎN WORKSPACE — AȘTEAPTĂ TESTELE UTILIZATORULUI. Utilizatorul a aprobat explicit extinderea aditivă a zonelor „FROZEN”: backend Module 2 (state machine), 3 (task-uri), 4 / Missing Contract Resume și 6 (validare comercială), plus frontend-ul (tab-ul Contract al facturii și Task Inbox).
- „Continuă fără contract”, cu motiv obligatoriu:
  - pe task-ul MISSING_CONTRACT (OPEN sau WAITING);
  - `POST /api/v1/invoices/{id}/contract-waivers`;
  - factura trece `AWAITING_CONTRACT → DEDUPE_CHECKED`, iar validarea comercială dă `CONTRACT_WAIVED` (CONFORM);
  - un contract încărcat ulterior nu mai reevaluează factura.
- Decizie: D-120. Fără migrări noi.
- Harness-ul de test pentru datele reale primite de la contabil stă în `test-data/victoria1881/`, ignorat de git ca tot `test-data/`. Folosește `scripts/clean-client-local.sh` pentru curățarea clientului între iterații. Faza 1 acoperă doar achizițiile, fiindcă Diana nu importă facturi emise.
- Verificare: doar statică de Claude. Testele Go unit/integration, vitest, E2E și rularea harness-ului urmează să fie făcute de utilizator.

## Factura blocată în review comercial: identitatea clientului, tarife dublate, asocieri revocate — 2026-09-30

- IMPLEMENTAT ÎN WORKSPACE — AȘTEAPTĂ TESTELE UTILIZATORULUI. Extinderea frontend-ului „FROZEN” (pagina facturii, pagina contractului, modalul de asociere) a fost aprobată explicit de utilizator.
- Pe inv-31f95fa3f49ffbf729f250c9 (CWF-0231) au fost reparate:
  - clauza IDENTITY a clientului (SOFTCO2) rămânea „De rezolvat” și ținea acoperirea parțială pentru toate facturile contractului; acum se închide automat, acoperită de CUI-urile părților;
  - aceeași linie de tarif apărea de două ori în asociere (tariful din „Servicii și tarife” + clauza extrasă din același rând); clauza care doar repetă tariful se închide automat, nu poate fi confirmată și nu mai apare în sugestii;
  - revocarea formulării învățate ștergea și asocierea facturii curente; acum asocierea confirmată rămâne pe factură, iar revocarea cere confirmare;
  - „Serviciul nu apare în contract” nu ducea nicăieri; acum oferă corecția facturii, excepția motivată sau încărcarea anexei;
  - factura se deschidea pe Rezumat fără nicio indicație; acum se deschide pe tab-ul cu acțiunea, iar Rezumat arată „Ce ai de făcut”.
  - Pe drum: cantitatea fixă din contract se completează automat pentru tariful unitar; o regulă UNIT_RATE confirmată din clauză primește definiția variabilei de cantitate (înainte `PutVariable` răspundea 500); rezultatul vechi arată că există o versiune nouă a contractului.
- Decizii: D-118, D-119. Fără migrări noi.
- Backfill pentru contractele deja confirmate: `go run ./cmd/contractrules close-covered-clauses`.
- Date E2E aliniate: `cmd/devseed` dă contractelor demo CTR-DEMO-100/201/202 un snapshot comercial confirmat și complet, cu tarifele liniilor din parcursurile Module 6 și 7 (numai dacă un contract nu are deja dosar). Facturile trec singure de validarea comercială, iar helper-ul E2E care aproba excepții pentru verificări „neverificabil” a fost eliminat.
- Verificare: doar statică de Claude (`gofmt`, `go build`, `go vet` pe pachetele modificate, inclusiv cu tag-ul de integrare fără `copylocks`, `tsc` pe aplicație). Testele Go unit/integration, vitest și E2E urmează să fie rulate de utilizator.

## Pagina documentului de contract ca spațiu de lucru față în față — 2026-09-29

- FAZA 1 IMPLEMENTATĂ ÎN WORKSPACE — AȘTEAPTĂ TESTELE UTILIZATORULUI. Faza 2 (pre-validarea revizuirii înainte de confirmare, împerecherea serviciu ↔ evidență după `sourceIndex`) urmează după verificarea fazei 1.
- Pagina unui contract confirmat arată acum fiecare valoare lângă PDF-ul evidențiat la textul citat și ce folosește fiecare tarif sau clauză la verificarea facturilor, din snapshot-ul activ.
- Clauzele se pot închide fără să devină regulă:
  - identitate acoperită de CUI-ul furnizorului: automat după confirmare sau cu un click;
  - „nu se verifică pe factură”, cu motiv.
  - Astfel acoperirea poate deveni completă (BG-2025-117 rămânea parțial din cauza clauzei IDENTITY).
- Decizia: D-117. Migrarea aditivă `000037_clause_review_reason.sql` (înainte de aplicare rulează `atlas migrate hash`). Backfill pentru contractele deja confirmate: `go run ./cmd/contractrules close-covered-clauses`.
- Corectat pe drum: versiunea snapshot-ului nou este prima liberă din dosar, nu „activă + 1”; după o revizie care revenea la un set vechi de reguli, următoarea confirmare încălca unicitatea `(dossier_id, version)`.
- Verificare: doar statică de Claude (`gofmt`, `go build`, `go vet` pe pachetul postgres cu tag-ul de integrare, `tsc` pe aplicație și teste). Testele Go unit/integration și vitest urmează să fie rulate de utilizator.

## Validare comercială față în față + asocierea serviciilor — 2026-09-29

- IMPLEMENTAT ÎN WORKSPACE — AȘTEAPTĂ TESTELE UTILIZATORULUI. Schimbarea de UX a fost cerută și aprobată explicit de utilizator; frontend-ul „FROZEN” a fost extins în zona Contract a facturii, a paginii contractului și a filtrului Task Inbox.
- Pe BG26000108 au fost reparate trei probleme:
  - clauza TVA era raportată ca „Prețul este corect” pe fiecare linie;
  - serviciul „Mentenanță IT” (1.800,00 lei) era eliminat fără avertisment la activarea tarifelor;
  - asocierea cerea potrivire exactă a textului.
- Decizia: D-116. Migrarea aditivă `000036_service_alias_revocation.sql` (înainte de aplicare rulează `atlas migrate hash`). Detalii: `CONTRACT_COMMERCIAL_VALIDATION.md`.
- Verificare: doar statică de Claude (inclusiv `gofmt` pe fișierele Go). Testele Go unit/integration, typecheck, vitest și E2E urmează să fie rulate de utilizator.

## Deblocare review V2 + corpus legislativ global + microîntreprinderi — 2026-09-28

- Cauze (din cod): corpusul `legislation_*` era gol local, deci analiza asistată eșua („corpusul legislativ local nu conține fragmente aplicabile”). Bannerul „Planul de conturi s-a modificat” apărea mereu pentru profilurile fără `accountCodes` și bloca editarea. Readiness-ul SAGA cerea un pack per client și `Profile.Ordinary()` (numai PROFIT_TAX), deci microîntreprinderile și clienții fără pack nu puteau ajunge în `READY_FOR_SAGA`.
- Legislație (D-106): corpusul e global. `make dev` rulează acum `make seed-legislation`, care importă idempotent cele două snapshot-uri TEST_ONLY cu `effective-from 2016-01-01`. Cloud-ul nu se schimbă. OMFP 1802/2014 este textul original, neconsolidat.
- Clasificare (D-107..D-112): fără pack-uri de producție; clasificarea se face prin AI plus decizii umane/reutilizabile.
  - Maparea SAGA e implicită în cod: `SAGA_C_DOMAIN_V2_ORDINARY_V1`.
  - Micro plătitor de TVA este exportabil. `EXPENSE_TAX_TREATMENT=NOT_APPLICABLE` e derivat automat din profil (sursa nouă `PROFILE`, migrarea `000033`).
  - `accountCodes` gol înseamnă tot planul OMFP postabil.
  - TVA la încasare necunoscut se confirmă prin `VAT_TREATMENT` revizuit uman.
  - Contextul modificat este doar avertisment.
  - `approve-all` folosește readiness-ul comun.
  - Variantele nesuportate la export (neplătitor TVA, mixt, pro-rata, TVA la încasare client) rămân doar clasificare, cu motiv explicit.
- Corecție analiză AI, după prima rulare reală pe factura FCO 0878 (D-113):
  - Promptul `UNIFIED_ACCOUNTING_PROMPT_V3` conține vocabularul exact pe fiecare dimensiune, iar schema are enum pentru `kind`/`timing`. Gemini trimisese `FULL` + `percentage: 100` și un tip TVA necunoscut, cu categoria în `category`.
  - Căutarea legislativă se face pe fiecare dimensiune nerezolvată: Codul fiscal pentru TVA/impozit, OMFP pentru cont. Buget: maximum 20k caractere pe fragment și 60k în total.
  - La profilurile fără listă de conturi, AI-ul primește doar conturile relevante direcției facturii: primite → clasele 2, 3, 6 și 471; emise → clasa 7.
  - `ACCOUNTING_ANALYSIS_TIMEOUT` are acum implicit 180s, cu un deadline Asynq dedicat analizei (timeout + 60s); celelalte job-uri rămân la `WORKER_JOB_TIMEOUT`.
- Snapshot per rulare (D-114, migrarea `000034`): snapshot-ul facturii urmează rularea curentă de clasificare. Înainte, o factură clasificată fără profil rămânea blocată definitiv cu „Lipsește un snapshot aprobat și coerent de profil/politică/pack.”, chiar și după reanalizare.
- Handoff SAGA:
  - O factură care ajungea la export cu exportul SAGA dezactivat pentru client rămânea tăcut în `EXPORTING`, fără încercare de export.
  - Activarea exportului în setările clientului repune acum în coadă, în aceeași tranzacție, facturile clientului din `READY_FOR_SAGA`/`EXPORTING` fără fișier generat.
  - Pe Rezumat, un avertisment explică exportul dezactivat și trimite la setare.
  - Pagina facturii trece singură pe Rezumat după finalizarea review-ului, iar badge-ul „Confirmă importul SAGA” semnalează acțiunea manuală.
- Verificare: Claude a făcut doar gofmt/editare statică. `go generate ./ent`, `atlas migrate hash`, unit, vitest, integration, E2E și `make dev` urmează să fie rulate de utilizator.

## Clauze TVA / termen de plată recunoscute din text — 2026-09-29

- Problema: clauzele TVA și termen de plată erau extrase corect ca text, dar fără valoare executabilă (normalizarea Gemini întorcea `null` sau eșua fără log), așa că fiecare contract cerea completarea manuală (ex. C7 Valoris, C8 Cleanwave).
- Aprobat explicit de utilizator (D-115): clauzele recunoscute sigur se confirmă automat, iar valorile rămân editabile.
  - `commercialvalidation.RecognizeSourceTextRule`: „N zile … de la emiterea/primirea/acceptarea” și „TVA cota legală/aplicabilă/aferentă” sau o singură cotă fixă. Orice ambiguitate (mai multe valori, zile lucrătoare, bază neclară, scutire, taxare inversă) rămâne la revizuire.
  - Auto-confirmare după confirmarea contractului, prin `ConfirmProposedRule` (aceleași verificări față de sursă ca o confirmare manuală), eveniment `COMMERCIAL_RULE_AUTO_CONFIRMED` (`SYSTEM`, automatic), regulă marcată `origin=SOURCE_TEXT`.
  - Editare: `POST …/commercial-rules/{ruleId}/revise` → snapshot nou, `COMMERCIAL_RULE_REVISED`; valoarea trebuie să apară în clauza citată.
  - Cote TVA legale globale (migrarea `000035`, `fiscal_vat_rates`: 19% până la 31.07.2025, 21% de la 01.08.2025) rezolvă `applicable_vat_rate` la data facturii; o valoare din dosar are prioritate.
  - Backfill pentru contractele confirmate anterior: `go run ./cmd/contractrules autoconfirm-source-text`.
  - Normalizarea Gemini loghează acum motivul (doar categorie + număr de clauze).
- Verificare rulată de Claude: Go unit, suita `-tags=integration` din `internal/platform/postgres` pe o bază izolată (toate trec, cu excepția `TestSagaEnableResumesInvoicesStuckAtHandoff`, care nu are legătură), vitest complet (158), typecheck, build, `atlas migrate validate`. Migrarea și backfill-ul au fost aplicate pe baza locală. E2E și `make release-local` nu au fost rulate.

## Reguli comerciale în limbaj natural — 2026-09-28

- Gap de UX (nu bug de date): review-ul contractului afișa AST-ul regulilor (`{"op":"literal",...}`) și codurile de tip (`FIXED_PRICE`) în 3 locuri: reguli propuse de AI, reguli confirmate în formular și reguli confirmate după confirmarea contractului.
- Fix doar în frontend: `src/features/contracts/commercialRuleText.ts` descrie în română ce compară efectiv motorul (`commercialvalidation/engine.go`); JSON-ul rămâne disponibil doar în „Detalii tehnice (pentru suport)”, închis implicit. Payload-urile, confirmarea și validarea nu se schimbă.
- Verificare: doar statică de Claude; typecheck/vitest urmează să fie rulate de utilizator.

## Contract Intelligence & Invoice Compliance V1 — 2026-09-19

- FUNDAȚIA V1 IMPLEMENTATĂ ÎN WORKSPACE, DAR ACCEPTANȚA PLANULUI ESTE PARȚIALĂ — AȘTEAPTĂ TESTELE UTILIZATORULUI; engineering gate rămâne NO.
- Migrarea aditivă `000018` introduce dosare multi-document, clauze cu proveniență, snapshot-uri comerciale imuabile, variabile/aliasuri, run-uri/findings/excepții și ledger cumulativ.
- Extragerea V4 propune rol/relație și reguli AST; confirmarea umană activează snapshot-ul. Facturile nu reparsază contractul.
- Pipeline-ul rulează validarea comercială înainte de clasificare și folosește `CONFORM`, `NECONFORM`, `NEVERIFICABIL` plus task-ul `COMMERCIAL_REVIEW`.
- API-ul și UI-ul de review/rezultat comercial sunt implementate, inclusiv completarea manuală a variabilelor cu proveniență. Preview-ul retroactiv este read-only; bulk revalidation, sugestiile AI per factură, UI-ul pentru aliasuri, adaptorul live FX/calendar și postarea automată în ledger nu sunt implementate.
- Verificare efectuată de Codex: compilare Go fără execuția testelor pentru pachetele modificate. Unit/runtime/integration/race/Redis/frontend/E2E/Gemini live/release/deployment nu au fost declarate PASS.
- Vezi `CONTRACT_COMMERCIAL_VALIDATION.md` și `CONTRACT_COMMERCIAL_VALIDATION_TEST_HANDOFF.md`.
- Actualizare 2026-09-21: utilizatorul a confirmat migrarea `000018`, `go vet`, frontend typecheck/build și 115 teste frontend PASS; suita Go unit, integrarea PostgreSQL și E2E dedicate/general au raportat eșecuri. Fixture-urile, vocabularul task-urilor, path-ul mock și porturile E2E au fost corectate; testul Go unit țintit trece, dar rerularea completă după modificări este încă în așteptare. `release-check.sh` rămâne FAIL / gate NO.
- A doua rulare 2026-09-21: toate testele Go unit și cele 115 teste frontend au trecut; Atlas este la `000018`. PostgreSQL integration/race au găsit un parametru SQL netipizat la rerulare și coliziuni ale fixture-urilor contractuale într-o bază reutilizată. Codul și cleanup-ul testelor au fost corectate, dar nu există încă rezultat de rerulare. Gate-ul rămâne NO.
- E2E 2026-09-21: contract ingestion 6/6 și mock general 23/23 PASS în outputul utilizatorului. Accounting V2 E2E nu a pornit din cauza fixture-ului `devseed` care încerca să clasifice din `LINES_READ`; corecția la `COMMERCIALLY_VALIDATED` este implementată, dar netestată runtime. Gate-ul rămâne NO.
- Ultima rerulare 2026-09-21: utilizatorul a confirmat PostgreSQL integration PASS, race țintit PASS, Redis integration PASS, contract ingestion E2E 6/6, Accounting V2 E2E 2/2 și E2E mock 23/23. `release-check.sh` a trecut static/build, unit, migrarea izolată și PostgreSQL integration, dar a expirat la 10 secunde în testul complet outbox/Asynq. Testul a primit o limită locală de 25 secunde și diagnostic de stare la eșec; compilarea rapidă a trecut, dar gate-ul complet nu a fost rerulat. Engineering gate rămâne NO; Gemini live și deployment nu sunt verificate.
- Următorul `release-check.sh` din 2026-09-21: PostgreSQL outbox/Asynq, worker/Asynq, E2E mock și backend1/3/4/5 PASS; backend6 testul N FAIL (13/14) fiindcă UI-ul reutiliza un 404 comercial din `COMMERCIAL_VALIDATING` după tranziția în review. Cheia query-ului comercial include acum revizia și starea facturii; typecheck și diff check PASS, dar backend6/backend7 și gate-ul complet rămân de rerulat. Engineering gate NO; nu există verificare Gemini live sau deployment.
- Utilizatorul raportează ulterior că toate testele au trecut pentru versiunea precedentă; logul complet al acestei ultime rulări nu a fost furnizat în acest turn. Schimbarea nouă separă arhivarea unui contract valid de eliminarea din flux a unei încărcări greșite nefolosite. Migrarea aditivă `000019` permite reingestia aceluiași PDF/referințe după `DISCARDED`, fără ștergerea fizică a istoricului; un contract cu facturi/candidați/rulări/ledger nu poate fi eliminat ca greșeală, dar poate fi arhivat. Atlas validate, compilarea PostgreSQL integration, Go unit țintit, frontend typecheck și 10 teste ContractsWorkspace au trecut; integrarea runtime și gate-ul complet nu au fost rerulate. Gemini live/deployment nevalidate.
- Rularea utilizatorului pentru schimbarea `000019`: migrarea bazei dedicate nu avea fișiere restante, iar testele PostgreSQL țintite de arhivare/ștergere au trecut. E2E backend4 a fost blocat de portul `8080` ocupat, iar `make release-local` s-a oprit la un test diagnostic care încerca Gemini live fără endpoint. Backend4 folosește acum portul izolat `8184`; testul diagnostic este opt-in și se sare în gate-ul normal. Verificările rapide Codex (test diagnostic țintit fără opt-in și enumerare E2E) au trecut. E2E backend4 și `make release-local` necesită rerulare de utilizator; migrarea locală/deployment-ul nu au avut loc. Engineering gate rămâne NO; Gemini live nevalidat.
- Utilizatorul a confirmat ulterior PASS pentru `backend4` și `make release-local`, inclusiv instalarea locală a acelei versiuni. Smoke test-ul Gemini live pentru contractul contabil 102 a eșuat semantic la `commercialClauses[0].rule` (`COMMERCIAL_RULE_INVALID`). Promptul a fost clarificat și versionat `V4_1`; diagnosticul indică subcâmpul invalid fără a expune textul contractului. Testul unitar țintit a trecut; după aceste ultime modificări, Gemini live și gate-ul complet nu au fost rerulate. Acceptanța extracției reale rămâne NO.
- Rularea live următoare a izolat lipsa `commercialClauses[0].rule.id`. Adaptorul Gemini generează acum numai ID-ul tehnic absent, stabil per PDF și poziție, fără a altera semantica sau dovezile și fără a permite suprascriere implicită între PDF-uri diferite. Testele unitare rapide țintite și `git diff --check` au trecut; Gemini live și gate-ul complet rămân de rerulat de utilizator. Acceptanța extracției reale rămâne NO.
- Rularea live ulterioară a izolat lipsa `commercialClauses[0].rule.narrative`. Adaptorul completează acum narațiunea/dovada redundante din aceeași clauză numai când lipsesc din regulă; nu repară tipul regulii, tarife sau AST-ul. Testele unitare rapide țintite au trecut; Gemini live și gate-ul complet nu au fost rerulate după schimbare. Acceptanța extracției reale rămâne NO.
- Rularea live următoare a raportat `commercialClauses[0].rule.kind` invalid. Dacă lipsește numai tipul intern, adaptorul poate copia un `clause.kind` exact acceptat; tipurile necunoscute ori contradictorii nu sunt convertite. CLI-ul live afișează acum doar forma structurală sigură a tuturor regulilor la eșec comercial, pentru a reduce apelurile repetate de diagnostic. Testele unitare rapide țintite au trecut; Gemini live și gate-ul complet nu au fost rerulate după această schimbare. Acceptanța extracției reale rămâne NO.
- Rularea live ulterioară a arătat patru clauze comerciale fără nicio expresie AST. Ingestia acceptă acum numai absența expresiei ca review narativ, persistă clauzele ca `PROPOSED` fără regulă normalizată, forțează `PARTIAL` și suprimă fallback-ul de tarif plat ca să nu declare fals un preț universal. UI-ul separă explicit aceste clauze de regulile executabile; validarea facturii rămâne `NEVERIFICABIL` pentru acoperirea lipsă. Testele unitare Go țintite, compilarea testului PostgreSQL integration, frontend typecheck și 17 teste UI de review au trecut; runtime integration, Gemini live și gate-ul complet nu au fost rerulate după schimbare. Acceptanța extracției reale și a prețurilor rămâne NO.
- Utilizatorul a confirmat ulterior PASS pentru testul PostgreSQL țintit al clauzei narative și `make release-local` pe acea versiune. Următoarea implementare face o a doua trecere Gemini, o singură dată la ingestie, doar peste clauzele extrase fără AST, fără PDF; propunerile de formule rămân neautoritare până la selectarea explicită per regulă în UI. Regula normalizată neconfirmată se păstrează ca `PROPOSED`, dar nu intră în snapshot, iar coverage rămâne `PARTIAL`. Promptul este `V4_2`. Verificări rapide Codex: testele unitare țintite Gemini, compilarea PostgreSQL integration fără execuție, frontend typecheck și 18 teste UI de review PASS. Integrarea runtime nouă, `make release-local`, Gemini live și deployment-ul nu au fost rerulate după această schimbare; acceptanța reală a prețurilor rămâne NO.

## Standard release commands — 2026-09-17

- `make release-local`: complete deterministic test/migration/build gate followed by local Docker deployment and health checks.
- `make deploy-gcp-test`: the same gate, immutable image build/push, Cloud TEST migration/status jobs, four-service rollout and post-deploy verification; clean committed tree and explicit confirmation required.
- E2E databases are isolated from the normal local `diana` database. Live ANAF certificate, real Gemini document, desktop SAGA and human Cloud acceptance remain separate manual gates.
- See `Docs/RELEASE_COMMANDS.md`.

## Contract Ingestion Hardening + Service Terms V1 — 2026-09-17

- IMPLEMENTED — AWAITING USER-RUN TESTS / REVIEW; engineering gate remains NO.
- Canonical Romanian CUI comparison, reviewed-value confirmation readiness, indefinite periods, unconfirmed-document discard, authenticated range PDF delivery, Cloud `.mjs` MIME fix, typed service terms, provenance/audit, and two-invoice resume regression are implemented in additive migration `000017`.
- Existing Module 4 policy thresholds and accounting classification remain unchanged. User-run Go unit tests, all 108 frontend tests, production frontend build, and isolated application of all 17 migrations passed on 2026-09-17/18. Integration fixtures/Redis orchestration exposed by the full gate were corrected and await rerun; Playwright/local deployment/Cloud deployment remain outstanding.
- See `CONTRACT_INGESTION_HARDENING_V1.md`, `CONTRACT_SERVICE_TERMS_V1.md`, `CONTRACT_INGESTION_HARDENING_RESULT.md`, and `CONTRACT_INGESTION_TEST_HANDOFF.md`.

- Frontend: APPROVED / FROZEN
- Frontend Modules 1–7: APPROVED / FROZEN
- Backend Phase 0: APPROVED
- Backend Module 1: APPROVED / FROZEN
- Backend Module 2: APPROVED / FROZEN
- Backend Module 3: APPROVED / FROZEN
- Backend Module 4: APPROVED / FROZEN
- Backend Module 5: APPROVED / FROZEN
- Backend Module 6: APPROVED / FROZEN
- Backend Module 7: APPROVED / FROZEN
- Backend Modules 8+: NOT STARTED
- Backend architecture: Go modular monolith; separate API/worker processes; PostgreSQL; Redis/Asynq; Ent; pgx connectivity; Atlas migrations
- Invoice pipeline: explicit state machine, exact invoice lines, revisions, transactional audit/outbox, idempotent ingestion, terminal business duplicates
- Duplicate policy: provisional/versionable `PROVISIONAL_V1`; client + normalized supplier CUI + normalized invoice number + issue day; amount/currency are verification signals
- Validation tasks: exactly three types/statuses; one active blocker per invoice; historical resolved tasks retained
- Contracts: read-only records; versioned match evidence; immutable invoice-association snapshot; provisional `MODULE4_BASELINE_V1`
- Missing Contract Resume: IMPLEMENTED — AWAITING USER-RUN TESTS / REVIEW; durable `CONTRACT_AVAILABLE` outbox trigger, bounded isolated fan-out, unchanged Module 4 policy, automatic normal-pipeline continuation
- Classification: exactly ACCOUNT/VAT/DEDUCTIBILITY per line; provisional `MODULE5_BASELINE_V1`; one grouped task; atomic final review
- Rules: stable GLOBAL/CLIENT_OVERRIDE identities, immutable versions, direct-origin override only, no correction learning or automatic reclassification
- Implemented API: client reads; complete/scoped invoice list and detail; task reads; missing-contract request; contract reads/confirmation; classification decisions; rule reads/version/override; health/readiness
- Frontend integration: explicit API or mock runtime; API mode has no silent mock fallback and observes backend-owned pipeline progression
- ANAF/SPV inbound ingestion: APPROVED / FROZEN
- ANAF/SPV Connection UX: APPROVED / FROZEN
- Real SAGA: APPROVED / FROZEN
- SAGA Export UX / Manual Handoff: APPROVED / FROZEN
- Authentication V1: IMPLEMENTED — AWAITING USER-RUN TESTS / REVIEW; PostgreSQL sessions, Argon2id credentials, Redis login throttling, CSRF, user-derived RequestActor and Romanian login. Cloud IAP cutover not executed.
- Worker runtime: PostgreSQL transactional outbox; SKIP LOCKED lease claims; at-least-once Asynq delivery; bounded retry/archive; idempotent stale-job handling
- Observability: structured logs, vendor-neutral OTLP tracing, operational `/metrics`, distinct API/worker liveness and readiness
- Gemini native PDF contract extraction: IMPLEMENTED — AWAITING USER-RUN TESTS / REVIEW. Standalone OCR, generalized AI, MCP and Cloud Run infrastructure remain DEFERRED.
- Latest approved verification: the complete ANAF/SPV local/fake-adapter gate passed on 2026-09-14; see `Docs/BACKEND_ANAF_SPV.md`.
- Current SAGA implementation: verified XML generation, strict validation, immutable artifact/attempt persistence, client-scoped private download, HUMAN confirmation of the exact artifact, explicit fake/file runtime selection, CreditNote guard and no false EXPORTED transition from generation/download.
- Contract Ingestion + PDF Upload + Gemini AI Extraction: IMPLEMENTED — AWAITING USER-RUN TESTS / REVIEW; see `Docs/CONTRACT_INGESTION_AI.md`. Runtime suites, migrations and live Gemini have not been run by Codex.
- Next required decisions: authenticated SAGA file handoff, the authoritative meaning/confirmation of EXPORTED, and expired-contract matching semantics.

## Real Accounting Rules V1 — 2026-09-15

- Frontend Modules 1–7: APPROVED / FROZEN.
- Backend Modules 1–7: APPROVED / FROZEN.
- ANAF/SPV: APPROVED / FROZEN.
- Real SAGA: APPROVED / FROZEN.
- SAGA Export UX: APPROVED / FROZEN.
- Missing Contract Resume: APPROVED / FROZEN.
- Contract Ingestion + AI Extraction: DO NOT MODIFY / CURRENT STATE PRESERVED.
- Real Accounting Rules: IMPLEMENTED — AWAITING USER-RUN TESTS / ACCOUNTING REVIEW; NOT FROZEN.
- Production pack `RO_INCOMING_ACCOUNTING_V1_REVIEW_ONLY` contains zero accepted mappings. Eligibility, provenance, effective-date selection, consistent rule snapshots, SAGA guard integration and limited UI metadata are implemented.
- Runtime tests/migration application/production build have NOT been run by Codex. Compilation and static checks are separate from user-run engineering/accounting acceptance.
- Required product/accounting decisions: deductibility semantics, reviewed client-account policy and VAT source-confirmation/tax-treatment scope. See REAL_ACCOUNTING_RULES.md, ACCOUNTING_RULES_RESEARCH.md and REAL_ACCOUNTING_RULES_TEST_HANDOFF.md.

## Accounting Domain V2 + deterministic Classification V1 — 2026-09-15

- Accounting Domain Model V2: IMPLEMENTED — AWAITING USER-RUN TESTS.
- Classification Engine V1: ENGINE IMPLEMENTED; REAL PRODUCTION PACK EMPTY / NOT APPROVED; REAL ACCEPTANCE NOT COMPLETED; NOT COMPLETE / NOT FROZEN.
- Classification Engine V2: NOT STARTED.
- New invoices: ACCOUNT / VAT_TREATMENT / VAT_DEDUCTIBILITY / EXPENSE_TAX_TREATMENT, immutable source/profile/pack evidence, shared readiness and bounded typed review controls.
- Historical invoices/artifacts: LEGACY_V1; VAT means historical source-rate confirmation, DEDUCTIBILITY means historical SAGA import instruction. No historical conversion/reclassification.
- Additive migration 000014, Ent regeneration, private reviewed profile/rule/pack tooling and TEST_ONLY fixtures/tests prepared.
- Only static/compile checks are performed by Codex; runtime/race/PostgreSQL/Redis/frontend/Playwright/build/migration acceptance is user-run. Engineering gate remains NO pending that acceptance.
- Pending accounting inputs: approved chart/profile/acquisition policies, exact reviewed rules/source periods, expected four decisions, and real desktop SAGA mapping acceptance. Current mapping supports approved ordinary immediate full/full omission only.
- Frozen Modules 1–7, ANAF/SPV, Contract matching/resume/AI extraction and SAGA handoff retain their existing boundaries. AI classification, learning, advanced VAT/pro-rata/cash/period calculations and historical reclassification remain deferred.

Authoritative details and exact A–W commands: [CLASSIFICATION_V1_TEST_HANDOFF.md](CLASSIFICATION_V1_TEST_HANDOFF.md). This section supersedes earlier descriptions of three dimensions for newly parsed V2 invoices; those descriptions remain historical V1 records.


## Client Management + Client Onboarding V1 — 2026-09-15

- IMPLEMENTED — AWAITING USER-RUN TESTS / REVIEW: focused company creation/editing, lifecycle, dated profile configuration/approval, derived independent company/profile/ANAF/SAGA/classification readiness, operational settings/history and request-actor isolation.
- New additive migration 000015; existing clients remain ACTIVE and grandfathered for existing file export. Collision preflight/manual review required if normalized identity duplicates exist.
- Static/compile checks executed; runtime tests/migrations/build are explicitly user-run. Engineering gate evidence outstanding; ready for user-run tests YES.
- Classification V1 preserved: ENGINE READY — REAL PACK MISSING, NOT FROZEN. Classification V2 NOT STARTED. Production rules, RBAC, deployment and SAGA bridge NOT STARTED by this milestone.
- [Design/discovery/security](CLIENT_MANAGEMENT_ONBOARDING.md); [A–V test commands](CLIENT_MANAGEMENT_ONBOARDING_TEST_HANDOFF.md).

## First GCP TEST deployment — 2026-09-16

- Project `diana-508810`, project number `232660193801`, billing enabled;
  region `europe-central2`; domain `accountingtechco.com`.
- INFRASTRUCTURE DEPLOYED: Artifact Registry, identities, secrets, VPC, private
  Cloud SQL and Redis, successful migrations, frontend/API/callback/worker,
  serverless NEGs and global load balancer exist. Static IP is `136.69.63.17`.
- DNS for `test.platform` and `api-test` resolves to the reserved IP. Managed TLS
  certificate `diana-test-cert-v3` is `ACTIVE`; ANAF OAuth registration/sync has
  not run.
- IAP is enabled on frontend and normal API backends; only the exact isolated
  ANAF callback route is public. External anonymous checks return `401` for the
  protected accounting endpoints.
- `cloud-test` config hardening forbids fake SAGA and unofficial enabled ANAF
  endpoints. Classification V1 production pack remains empty/review-only;
  Classification V2 remains NOT STARTED.
- See `GCP_TEST_DEPLOYMENT.md`, `GCP_INFRASTRUCTURE.md`, `GCP_SECRETS.md`,
  `GCP_DNS_SQUARESPACE.md` and `GCP_ANAF_E2E.md`.
# Authentication V1 — 2026-09-16

Implemented in the workspace and awaiting user-run tests/review. Diana now has
an Armqu-pattern Romanian login, PostgreSQL-backed opaque sessions, Argon2id
credentials, Redis brute-force throttling, CSRF enforcement, protected frontend
shell, 401 handling/logout, user-derived RequestActor grants, an operator CLI,
additive migration `000016`, focused tests and a Playwright journey. The live
Google redirect was confirmed as IAP on the web and API backend services; no GCP
mutation or deployment was performed. See `AUTHENTICATION_V1.md`,
`AUTHENTICATION_V1_RESULT.md` and `AUTHENTICATION_TEST_HANDOFF.md`.
