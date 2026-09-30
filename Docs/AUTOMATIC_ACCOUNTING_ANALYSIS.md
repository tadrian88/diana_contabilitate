# Diana — Automatic Accounting Analysis & Learning

## Capitolul B — contractul contabil unificat (2026-09-25)

Pentru analizele noi, `LineClassification` este singura autoritate contabilă. Nu mai există un rezultat AI paralel care să poată afirma o monografie diferită de clasificarea facturii.

Contractul canonic este, pentru fiecare `InvoiceLine`:

```text
InvoiceLine
  ├─ ACCOUNT
  ├─ VAT_TREATMENT
  ├─ VAT_DEDUCTIBILITY
  └─ EXPENSE_TAX_TREATMENT
```

Fiecare clasificare poate păstra independent valoarea propusă și valoarea efectivă, `source` pentru propunere, `effectiveSource` pentru rezultatul final, explicația, citările legale verificate, rezultatele validării, starea review-ului, revizia și provenance-ul provider/model/prompt. `AI_PROPOSAL` este o sursă de propunere, niciodată o aprobare. După corecția contabilului, propunerea rămâne `AI_PROPOSAL`, iar valoarea efectivă are sursa `MANUAL`. O propunere validă tehnic rămâne `PENDING` până la decizia contabilului.

Gemini folosește schema `UNIFIED_ACCOUNTING_PROPOSAL_V2` și returnează valori `accounting.Value` pentru dimensiunile încă nerezolvate. Primește facts, direcția facturii, profilul din snapshotul `classification_run`, dimensiunile deja rezolvate, fragmentele legislative recuperate și numai conturile active, postabile și permise de profil. Catalogul complet rămâne în snapshotul validatorului backend și nu este înlocuit de constrângerile promptului.

Validarea este independentă per `InvoiceLine × Dimension`. De exemplu, `ACCOUNT=628` este păstrat ca propunere AI, dar primește `ACCOUNT_NOT_POSTABLE` și sugestiile analitice disponibile; o propunere validă `VAT_TREATMENT` din același răspuns rămâne validă și reviewable. Statusul aggregate al artefactului poate fi `PROPOSED`, `PARTIAL_VALIDATION` sau `VALIDATION_FAILED`, dar nu este autoritatea deciziilor individuale.

Citările sunt returnate per dimensiune și sunt validate față de fragmentele exacte ale rulării: `fragmentId`, `versionId`, `citationKey` și `contentHash` trebuie să coincidă. Citarea invalidă este păstrată ca evidence neverificată și produce o eroare granulară; Diana nu inventează o citare înlocuitoare. Corpusul legislativ rămâne evidence/retrieval, nu regulă executabilă.

`accounting_analysis_runs.raw_structured_response` rămâne artefactul imuabil al providerului și sursa de audit pentru provider/model/tokeni. Pentru V2, review-ul se face prin infrastructura existentă a clasificărilor; cardul legacy nu mai aprobă întregul JSON V2. Analizele V1 istorice (`ACCOUNTING_ANALYSIS_V1`) nu sunt convertite euristic și rămân artefacte legacy/audit. Monografia V1 nu mai este autoritară pentru analize noi.

Outputul V2 nu conține evenimente de plată sau încasare. Facturile primite și emise sunt diferențiate prin identitățile fiscale existente, iar modelul descrie exclusiv deciziile evenimentului facturii. Payment/collection matching, fallback-ul AI automat prin pipeline și promovarea în knowledge rămân în afara Capitolului B. Capitolul C va orchestra regulile deterministe → dimensiuni nerezolvate → AI → review fără un nou redesign de domain.

Migrarea aferentă este `000028_unified_accounting_contract.sql`. Ea adaugă pe `line_classifications` citările, validările și provenance-ul propunerii, sursa `AI_PROPOSAL`, starea `REJECTED` și statusul aggregate `PARTIAL_VALIDATION`. Nu rescrie analizele istorice.

Secțiunile de mai jos documentează și istoricul vertical slice-ului V1; orice referire la monografie separată sau la aprobarea JSON-ului complet se aplică numai artefactelor legacy.

## Stare locală confirmată (2026-09-23)

Confirmat de utilizator:

- migrarea `000024` a fost aplicată cu succes în baza separată `diana_contract_commercial_test`;
- au fost importate acolo cele două versiuni `TEST_ONLY`: Codul fiscal cu 638 fragmente și OMFP 1802/2014 cu 831 fragmente;
- testele Go pentru `accountinganalysis`, `legislation`, `httpserver`, `postgres`, `workerruntime` și `config` au trecut;
- `npm run typecheck` a trecut;
- în `.env.local` au fost configurate `APP_ENV=development`, `ACCOUNTING_ANALYSIS_ENABLED=true` și cheia Gemini.

Verificarea Codex asupra mediului Docker activ a găsit:

- containerele API și worker sunt sănătoase, dar au fost create înaintea vertical slice-ului curent;
- `docker compose restart api worker` nu a recreat containerele și nu a recitit `.env.local`: în containerul API, `ACCOUNTING_ANALYSIS_ENABLED` este încă gol;
- `.env.local` și `compose.yaml` trimit aplicația Docker în baza `diana`, nu în `diana_contract_commercial_test`;
- baza Docker `diana` este la migrarea `000024`, are `000025` pending și are zero versiuni/fragmente legislative;
- baza `diana` are 37 facturi `ACCOUNTING_DOMAIN_V2`, datate `2026-09-04`–`2026-09-21`, 7 profiluri contabile și 270 conturi active/postabile.

Migrarea incrementală `000025_accounting_analysis_test_only.sql` a fost adăugată pentru a păstra `000024` imutabilă și pentru a adăuga sigur marcajul `test_only` plus unicitatea review-ului per analiză. `atlas migrate status` vede corect `000025` pending atât în `diana`, cât și în `diana_contract_commercial_test`.

### Următorii pași pentru primul test end-to-end

1. Aplică `000025` în baza folosită de Docker:

```bash
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' atlas migrate apply --env local
```

2. Snapshoturile se importă automat în baza `diana` (D-106): `make dev` rulează `make seed-legislation`, care execută `legislationdraftimport -skip-existing` pentru ambele snapshoturi, cu `-effective-from 2016-01-01`. Aceasta e exclusiv o alegere de test, nu o dată juridică de aplicabilitate. Importul e idempotent; o versiune existentă cu alt conținut produce eroare. Poate fi rulat și separat:

```bash
make seed-legislation
```

Versiunile importate manual anterior (de ex. `-TEST_ONLY-2026-09-01`) rămân imuabile și coexistă cu cele noi.

3. Reconstruiește și recreează API-ul și workerul. `restart` nu este suficient pentru cod sau variabile noi:

```bash
cd /Users/adriantudoran/Projects/diana_contabilitate
docker compose up -d --build --force-recreate api worker
docker compose ps
docker compose exec api sh -c 'test "$APP_ENV" = development && test "$ACCOUNTING_ANALYSIS_ENABLED" = true && test -n "$GEMINI_API_KEY" && echo "accounting analysis API configured"'
docker compose exec worker sh -c 'test "$ACCOUNTING_ANALYSIS_ENABLED" = true && test -n "$GEMINI_API_KEY" && echo "accounting analysis worker configured"'
```

În `.env.local` există în prezent două declarații `GEMINI_API_KEY`. Păstrează una singură pentru a evita configurația ambiguă; nu publica valoarea în loguri sau documentație.

4. Deschide `http://127.0.0.1:5173`, autentifică-te cu persona `CONTABIL`, alege o factură API V2 și intră în fila **Clasificare**. În cardul **Analiză contabilă asistată**, apasă **Generează propunerea**. Rezultatul așteptat este `RUNNING`, urmat de `PROPOSED` sau `VALIDATION_FAILED`.

5. Pentru un rezultat `PROPOSED`, verifică în ordine: totalul 401/4111, conturile, TVA pe fiecare linie, deductibilitatea și citările. Testează întâi **Respinge** pe o factură de probă sau **Aprobă propunerea** dacă rezultatul este contabil corect. `EDIT` acceptă momentan JSON și este revalidat integral de backend.

6. Dacă analiza rămâne `RUNNING` sau ajunge `PROVIDER_FAILED`, colectează doar logurile relevante:

```bash
docker compose logs --since=10m api worker
```

Nu copia cheia Gemini din mediu. Pentru verificarea persistării, folosește:

```bash
docker compose exec postgres psql -U diana -d diana -c "SELECT id, invoice_id, status, model, started_at, completed_at FROM accounting_analysis_runs ORDER BY started_at DESC LIMIT 10;"
```

### Criteriu de acceptare pentru etapa locală

Etapa este confirmată când o factură API V2 trece prin `POST → Asynq → PROPOSED/VALIDATION_FAILED`, cardul afișează propunerea și citările, iar un review de contabil este persistat. Outputul trebuie să rămână separat de exportul SAGA. După acest test urmează teste HTTP/worker dedicate și apoi promovarea controlată a review-urilor aprobate în knowledge; activarea în afara mediului local rămâne blocată până la corpusul juridic verificat.

## Implementarea vertical slice (2026-09-22)

Vertical slice implementat în cod: `POST/GET /api/v1/clients/{clientId}/invoices/{invoiceId}/accounting-analysis`, `POST .../review`, run persistat, job Asynq, propunere Gemini, validare deterministă, review `APPROVE/EDIT/REJECT` și card în fila Clasificare a facturilor API `ACCOUNTING_DOMAIN_V2`. `APPROVE` folosește exclusiv propunerea stocată; `EDIT` este revalidat pe server. Un review pe altă revizie a facturii este refuzat. Outputul **nu intră în SAGA** și nu promovează încă reguli reutilizabile. Feature flag-ul este permis numai local (`development`, `test`, `local-real`).

Snapshoturile convertite pentru Codul fiscal (638 articole) și OMFP 1802/2014 (831 fragmente) pot fi validate/importate cu `legislationdraftimport`, doar `TEST_ONLY`. `-effective-from` este un filtru de test, **nu o afirmație juridică** despre data intrării în vigoare. Importatorul păstrează hash-urile, folosește identificatori `-local-draft` separați și nu activează o versiune de producție. Pentru OMFP, sursa este identificată prin SHA-256 al PDF-ului local, nu prin URL oficial. Nu folosi analiza pentru decizii fiscale reale până la verificarea actelor modificatoare și a intervalelor pe prevederi.

Comenzile de mai jos au fost rulate de utilizator pe baza separată `diana_contract_commercial_test`; nu reprezintă configurația Docker activă:

```bash
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_contract_commercial_test?sslmode=disable' atlas migrate apply --env local
APP_ENV=development DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_contract_commercial_test?sslmode=disable' go run ./cmd/legislationdraftimport -file legislation/source-snapshots/legea-227-2015-cod-fiscal-oug-38-2026.draft.json -effective-from 2026-09-22 -accept-test-only
APP_ENV=development DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_contract_commercial_test?sslmode=disable' go run ./cmd/legislationdraftimport -file legislation/source-snapshots/omfp-1802-2014-original.draft.json -effective-from 2026-09-22 -accept-test-only
```

În `.env.local`, setează `APP_ENV=development`, `ACCOUNTING_ANALYSIS_ENABLED=true`, `GEMINI_API_KEY=...`, opțional `GEMINI_ACCOUNTING_MODEL=...`; repornește API-ul și workerul. Alege `-effective-from` anterior datei facturilor pe care vrei să le testezi, însă numai în baza locală de test. CLI-ul este idempotent doar la nivel de comandă de analiză; un al doilea import cu același ID de versiune este refuzat pentru a păstra istoricul imutabil.

Verificări suplimentare de rulat de utilizator (suitele lungi nu au fost executate de agent):

```bash
cd /Users/adriantudoran/Projects/diana_contabilitate/backend && go test ./internal/accountinganalysis ./internal/legislation ./internal/platform/httpserver ./internal/platform/postgres ./internal/workerruntime ./internal/platform/config
cd /Users/adriantudoran/Projects/diana_contabilitate && npm run typecheck
```

Ulterior, utilizatorul a confirmat testele complete țintite, typecheck-ul, migrarea și importurile pe baza separată. Fluxul API→Asynq→UI rămâne de confirmat pe baza Docker `diana`, conform pașilor actuali de mai sus.

Secțiunea de status de mai jos descrie etapa anterioară și este păstrată ca istoric de proiect; lista sa „NOT IMPLEMENTED” este depășită de continuarea de mai sus.

## Status (2026-09-22)

### IMPLEMENTED AND TESTED

- model intern, independent de SAGA, pentru monografie pe linii, tratament TVA, deductibilitate, citări și înregistrări viitoare de plată/încasare;
- output Gemini strict, cu versiune de prompt/schemă, temperatură zero și protecție contra instrucțiunilor din conținutul facturii;
- validator deterministic pentru tenant/factură/revizie, conturi postabile și permise de profil, line IDs, monedă, reconcilierea 401/4111, TVA sursă, acoperirea liniilor și citări existente în corpus;
- golden tests pentru Orange, BT Leasing și factura emisă de consultanță cu TVA la încasare;
- negative tests pentru tenant, cont, sumă, TVA, linie, citare și bypass de review;
- manifest legislativ verificabil prin SHA-256 și importator cu decodare strictă;
- retrieval lexical, datat și citabil; nu s-a introdus vector DB fără nevoie demonstrată;
- metrici fără identificatori de tenant pentru apeluri, latență, tokeni și erori de validare.

Testele țintite executate:

```bash
cd backend
GOCACHE=/private/tmp/diana-accounting-analysis-go-cache go test ./internal/accountinganalysis ./internal/legislation ./cmd/legislationimport
```

Rezultat: toate pachetele au trecut.

### ISTORIC — IMPLEMENTED BUT NOT EXECUTED LA ACEL MOMENT

- migrarea `000024_accounting_analysis_legislation.sql` și verificarea Atlas hash;
- tabele imutabile pentru surse/versiuni/fragmente legislative, analysis runs și review-uri;
- tabel tenant-scoped pentru knowledge aprobat, legat compozit de profilul și review-ul aceluiași client;
- CLI-ul de import legislativ pe o bază reală.

Comenzi de verificare pentru utilizator:

```bash
cd backend
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_contract_commercial_test?sslmode=disable' atlas migrate apply --env local
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_contract_commercial_test?sslmode=disable' go run ./cmd/legislationimport -file /cale/manifest-revizuit.json
```

Conținutul unei versiuni este hash-ul SHA-256 al concatenării, în ordinea `ordinal`, a fiecărui `fragment.contentHash` urmat de newline. Fiecare fragment are propriul SHA-256 calculat peste textul exact.

### LIMITĂRI CURENTE

- promovarea unui review aprobat în knowledge și invalidarea automată la schimbarea profilului/legislației;
- testele HTTP/Asynq dedicate pentru întregul workflow, inclusiv retry terminal și idempotency concurent;
- eval runner și dashboard pentru metricile de accuracy/correction;
- generarea SAGA din noul model de monografie. Exporterul existent rămâne autoritatea și nu primește output direct de la LLM;
- invoice ↔ payment/collection matching. Fazele `PAYMENT` și `COLLECTION` sunt reprezentate, dar nu sunt executabile fără evenimente bancare și alocări.

## Capitolul A — consistența contextului

Începând cu migrarea `000027_accounting_context_consistency.sql`, clasificarea nu mai este identificată numai prin snapshotul de pe factură. Fiecare execuție are un `classification_run` imuabil, iar `line_classifications` sunt legate de rularea care le-a produs. Factura indică explicit rularea curentă; rulările și deciziile precedente rămân auditabile.

Garanții introduse:

- un cont final trebuie să existe, să fie activ, postabil și permis de profil; erorile sunt `ACCOUNT_NOT_FOUND`, `ACCOUNT_INACTIVE`, `ACCOUNT_NOT_POSTABLE` și `ACCOUNT_NOT_ALLOWED_BY_PROFILE`;
- conturile sintetice pot apărea în căutare pentru context, dar nu pot fi selectate sau salvate în profil;
- copiii postabili sugerați sunt derivați exclusiv din `accounts.parent_code`;
- lipsa profilului produce blocker-ul `MISSING_FISCAL_PROFILE`, nu `ACCOUNTING RULE SOURCE REQUIRED`;
- profilurile aprobate sunt corectate prin succesor explicit, fără modificarea profilului istoric;
- schimbarea profilului sau a stării conturilor marchează contextul curent ca stale;
- reanalizarea este explicită, idempotentă și permisă numai înainte de exportul SAGA;
- un analysis run nou consumă profilul și catalogul capturate de rularea clasificării, nu configurația curentă citită ulterior.

`invoice.accounting_snapshot` rămâne pentru compatibilitate. Pentru cod nou, autoritatea este `classification_runs.snapshot` împreună cu `invoices.current_classification_run_id`.

Aplicare locală:

```bash
cd backend
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' atlas migrate apply --env local
```

Migrarea nu schimbă automat 628 în 6281 și nu rescrie profiluri ori decizii istorice. Profilurile vechi care conțin conturi nepostabile trebuie înlocuite printr-o versiune succesoare corectată.

### BLOCKED / REQUIRES PRODUCT DECISION

- corpusul oficial/versionat pentru OMFP 1802/2014 și Legea 227/2015 nu există în repository și nu a fost inventat;
- nu este stabilit cine poate activa o versiune legislativă și cine are rol contabil pentru aprobare;
- nu este stabilită politica prin care un semantic match devine suficient de puternic pentru reutilizare. Implementarea existentă de account learning este exactă și rămâne cu review obligatoriu;
- nu există cerințe pentru matching bancar și plăți parțiale;
- acceptarea reală SAGA rămâne cea documentată în `BACKEND_SAGA_REAL.md`; schema nu este regenerată de LLM.

## Current state reutilizat

- `internal/accounting`: facts XML, profil fiscal temporal, valori tipate și snapshot de release;
- `internal/classification`: reguli deterministe, review CAS și account mappings per client;
- `internal/audit` și activity events: modelul existent de audit al workflow-ului;
- `internal/saga`: adapterul SAGA separat și propriile readiness guards;
- `internal/contractingestion`: convenția Gemini structured output și tratarea conținutului extern ca date neîncrezătoare;
- PostgreSQL/Asynq/metrics existente.

Nu s-a creat un al doilea sistem de clasificare. `accountinganalysis` produce monografia agregată care lipsea și consumă aceleași `accounting.SourceFacts` și `accounting.Profile`.

## Arhitectură și provenance

Fluxul implementabil este:

`SourceFacts + Profile + approved tenant knowledge + dated fragments → Gemini proposal → deterministic Validate → immutable run → human review → approved knowledge → existing SAGA adapter`

Un run păstrează factura/revizia, input snapshot, fragmentele și knowledge-ul folosit, provider/model, prompt/schema, răspunsul structurat, validările, tokenii și timpii. Coloanele de tokeni ale run-ului sunt legacy (doar apelul final reușit, fără raționament); consumul complet pe apel, cu retry-uri, eșecuri și cost, este în `llm_usage_events` (D-121, `Docs/AI_USAGE_COST_TRACKING.md`). Review-ul păstrează separat propunerea AI și decizia finală. Knowledge-ul are `client_id`, profil, versiuni legislative și perioadă de valabilitate; cheile compozite împiedică asocierea cu profilul/review-ul altui client.

Retrieval-ul folosește PostgreSQL full-text `simple`, cu filtru obligatoriu pe data facturii. Citarea este acceptată doar dacă ID-ul fragmentului, versiunea, cheia umană și hash-ul coincid cu fragmentul efectiv trimis modelului.

## Schimbarea legislației

O versiune legislativă existentă este imutabilă. O modificare se importă drept versiune nouă, cu perioadă nouă. Knowledge-ul păstrează lista versiunilor de care depinde. Adapterul de reutilizare care urmează trebuie să selecteze numai knowledge cu același profil aplicabil și aceleași versiuni legislative; orice diferență produce `STALE/NEEDS_REVIEW`, nu auto-match.

## Fișiere principale

- `backend/internal/accountinganalysis/model.go`, `service.go`, `gemini.go`, `validate.go`
- `backend/internal/accountinganalysis/workflow.go`
- `backend/internal/legislation/model.go`, `ingest.go`
- `backend/internal/platform/postgres/legislation.go`, `legislation_ingest.go`, `accounting_analysis.go`
- `backend/internal/platform/httpserver/accounting_analysis.go`
- `backend/internal/workerruntime/accounting_analysis.go`
- `backend/cmd/legislationimport/main.go`, `backend/cmd/legislationdraftimport/main.go`
- `backend/migrations/000024_accounting_analysis_legislation.sql`, `000025_accounting_analysis_test_only.sql`
- `src/features/invoices/AccountingAnalysisCard.tsx`
- `backend/internal/accountinganalysis/validate_test.go`
- `backend/internal/legislation/ingest_test.go`

## Ordinea de lucru după testul local

1. confirmarea manuală API→Asynq→UI pe una dintre cele 37 facturi V2 locale;
2. teste automate pentru endpointuri, handlerul Asynq, retry terminal, CAS/idempotency și izolarea tenantului;
3. evaluare contabilă pe un set mic de facturi aprobate și măsurarea corecțiilor, fără SAGA;
4. promovare exactă a review-urilor aprobate în knowledge și invalidare la schimbarea profilului sau legislației;
5. furnizarea și aprobarea manifestelor oficiale/versionate și eliminarea dependenței de corpusul `TEST_ONLY`;
6. numai după aceste gate-uri, proiectarea integrării cu SAGA și a evenimentelor bancare pentru plată/încasare.

## Capitolul C — workflow automat Asynq și persistență (2026-09-25)

Fluxul normal nu mai pornește din cardul separat de analiză. Etapa existentă de clasificare rulează în ordinea:

`reguli deterministe / mapări exacte → detectare canonică unresolved → job Asynq → Gemini numai pentru dimensiunile lipsă → validare granulară → LineClassification → un singur task contabil → decizii finale`

`LineClassification` rămâne singura autoritate. Funcțiile din `internal/accounting` disting explicit `NEEDS_AI`, `NEEDS_REVIEW` și `FINAL`; o propunere AI validă tehnic dar neaprobată nu este finală, însă nici nu este retrimisă inutil furnizorului. Valorile finale, mapările deja propuse și propunerile AI existente sunt trimise modelului doar drept context read-only.

### Asynq, idempotency și retry

- payloadul conține doar `tenantId`, `invoiceId`, `classificationRunId` și `analysisRunId`;
- inputul este reconstruit la execuție din rularea curentă și snapshotul imuabil al `classificationRun`;
- `command_key`, `TaskID` Asynq stabil și cheia `accounting-review:<classificationRunId>` fac enqueue-ul, persistența și taskul idempotente;
- worker-ul recalculează dimensiunile lipsă înainte de apelul providerului; un job stale devine no-op auditat (`SUPERSEDED`/`NOT_NEEDED`);
- timeout/network/429/5xx sunt retryable și folosesc limita globală Asynq; configurația invalidă, identitatea inconsistentă și răspunsul provider nevalid sunt terminale;
- după epuizarea retry-urilor sau o eroare terminală, factura trece la `REVIEW_REQUIRED`, cu toate dimensiunile nefinale editabile manual. Gemini nu poate lăsa factura blocată în `RUNNING`.

### Concurență și task unic

Persistența AI actualizează numai rândurile `NO_MATCH`/`AMBIGUOUS` fără valoare efectivă din rularea încă activă. CAS-ul și filtrele SQL împiedică suprascrierea unei decizii deterministe sau manuale apărute între enqueue și commit. Validarea parțială nu face rollback logic al dimensiunilor valide; o eroare DB face rollback tranzacțional integral.

Taskul de tip `CLASSIFICATION` este legat de `classification_run_id` și este unic pentru acea rulare. El apare numai după terminarea AI, după failure definitiv sau când intervenția umană este direct necesară. Taskul se închide numai când `IsAccountingClassificationComplete` confirmă toate cele patru dimensiuni obligatorii pentru fiecare linie.

### Approve All și reanalizare

`POST /api/v1/invoices/{id}/classification-decisions/approve-all` aprobă într-o singură tranzacție numai propunerile tipate, valide, `PENDING`, din rularea curentă. Requestul include reviziile facturii, taskului și fiecărei clasificări eligibile; orice diferență produce `409 CONFLICT` fără aprobare parțială. Propunerile invalide, respinse, stale sau nerezolvate sunt excluse. Aprobarea nu creează knowledge reutilizabil.

Reanalizarea păstrează run-ul vechi, marchează taskul vechi drept superseded, creează snapshot/run nou, reaplică regulile/mapările și pornește automat AI numai pentru noul set unresolved.

### API și compatibilitate

Factura expune `accountingWorkflowStatus`: `APPLYING_RULES`, `AI_ANALYSIS_PENDING`, `AI_ANALYSIS_RUNNING`, `REVIEW_REQUIRED` sau `COMPLETED`. Acesta este derivat backend-side. Endpointurile manuale `GET/POST .../accounting-analysis` rămân temporar pentru diagnostic și compatibilitate, dar folosesc același contract V2 și aceeași materializare canonică; ele nu reprezintă fluxul normal de producție.

Learning-ul reutilizabil și exportul SAGA rămân explicit în afara Capitolului C. O aprobare finalizează numai factura/rularea curentă; nu creează automat reguli sau mapări noi și o propunere pending nu este export-ready.

## Capitolul D — review contabil unificat

Pentru `ACCOUNTING_DOMAIN_V2`, fila de clasificare a facturii este unica suprafață autoritativă de review. Ea grupează pe fiecare linie cele patru decizii canonice (`ACCOUNT`, `VAT_TREATMENT`, `VAT_DEDUCTIBILITY`, `EXPENSE_TAX_TREATMENT`) și afișează distinct valorile finale, propunerile valide, propunerile invalide, câmpurile nerezolvate și propunerile respinse. Sursele, validările și citările sunt traduse în limbaj contabil și rămân asociate dimensiunii pe care o susțin.

Stările principale sunt:

- **blocked** — profilul fiscal lipsă sau invalid are o singură acțiune principală către configurarea clientului;
- **analyzing** — aplicarea regulilor și analiza asistată sunt prezentate drept progres, fără acțiuni de aprobare;
- **review** — acțiunea principală este aprobarea tuturor propunerilor valide, iar editarea și respingerea rămân acțiuni locale;
- **stale** — rezultatul vechi este marcat explicit și blocat pentru editare, iar reanalizarea este acțiunea principală;
- **completed** — deciziile sunt read-only, nu mai există CTA de aprobare, iar monografia derivată poate fi consultată.

`AccountingAnalysisCard` nu mai este randat în fluxul V2 și editorul său JSON nu este accesibil clasificărilor V2. Cardul rămâne numai în istoricul explicit al analizelor `LEGACY_V1`. Monografia V2 este derivată, read-only și secundară; nu poate fi modificată independent de `LineClassification` și nu declară plăți sau încasări fără evenimente reale.

## Capitolul E — reguli, surse și learning controlat

O aprobare individuală, o corecție sau `Approve All` finalizează numai clasificarea curentă. Niciuna nu promovează implicit knowledge. După ce o decizie este finală și human-reviewed, contabilul poate alege acțiunea secundară „Folosește pentru situații similare”, inspectează scope-ul propus de backend și îl confirmă explicit. Comanda verifică prin CAS revizia facturii și a clasificării, run-ul curent și starea finală; o decizie dintr-un run superseded nu poate fi promovată.

Cele trei concepte rămân separate:

- **regulă verificată** — logică deterministă executabilă, cu predicate, rezultat, versiune și perioadă;
- **decizie reutilizabilă** — rezultat uman final, per client și per dimensiune, cu scope factual exact și provenance către factură/linie/run/clasificare;
- **sursă legislativă** — document și versiune pentru retrieval/evidence/citare; fragmentele nu sunt reguli executabile.

### Modele reutilizate

`account_mappings` rămâne intenționat modelul restrâns `supplier + service identity → ACCOUNT`; nu a fost extins artificial pentru TVA sau tratamentul fiscal. `approved_accounting_knowledge`, creat inițial lângă auditul analizei legislative, este extins pentru celelalte trei dimensiuni. Rândurile istorice fără câmpurile Chapter E rămân audit-only și nu sunt activate automat. `LineClassification.source=LEARNED_MAPPING` este reutilizat deoarece semantica sa existentă este exact o propunere dintr-o decizie aprobată anterior; nu s-a introdus un enum duplicat. Referința concretă de knowledge și factura sursă sunt păstrate în provenance-ul imuabil al propunerii.

### Matching, prioritate și cost AI

Scope-ul persistat folosește numai fapte verificabile existente: client, supplier identity normalizat, cea mai puternică identitate disponibilă (`SELLER_ITEM_ID`, apoi `STANDARD_ITEM_ID`, altfel descriere normalizată exact), monedă, tip document, cotă TVA și profil contabil fiscal cu versiune. Nu există fuzzy/vector/LLM matching și nu există fallback supplier-only. Astfel Orange „Abonament Smart 15” nu devine `Orange → 626`, iar liniile RCA/CAS/diferență de curs de la BT Leasing rămân identități independente.

Pipeline-ul este:

`regulă deterministă verificată → account mapping / approved knowledge exact → AI numai pentru dimensiunile unresolved → review manual`

Knowledge-ul produce o propunere `LEARNED_MAPPING` care cere review, la aceeași autoritate ca maparea existentă. Un match exact împiedică apelul AI pentru dimensiunea respectivă. Două valori umane diferite pe același scope produc `AMBIGUOUS` cu `KnowledgeConflict`; AI nu alege câștigătorul. Regulile deterministe își păstrează prioritatea.

### Validare, stale și revocare

`ACCOUNT` este revalidat prin catalogul canonic activ/postable și allowlist-ul profilului înainte de reuse. Celelalte dimensiuni cer același `profile_id + profile_version`; schimbarea profilului marchează knowledge-ul fiscal anterior drept `STALE`, iar înlocuirea versiunii legislative citate îl marchează stale cu audit. În ambele cazuri nu mai poate produce propuneri. O nepotrivire de aplicabilitate la data facturii este de asemenea exclusă din matching. Revocarea este CAS-protected, auditată și nu șterge istoricul. Reactivarea automată nu există; se creează o versiune succesoare după o promovare explicită.

Promovarea identică este deduplicată. O valoare diferită pentru același identity hash returnează conflict explicit. Fiecare promovare, utilizare/conflict și revocare produce activity audit cu clientul și agregatul de knowledge.

### Reguli și surse

Pagina `/rules` este „Reguli și surse” și are trei secțiuni independente: numai reguli `productionEligible`, decizii reutilizabile inspectabile/revocabile și versiuni ale corpusului legislativ. Empty state-ul regulilor este „Nu există încă reguli verificate”; textul despre reguli fictive a fost eliminat. Numărul fragmentelor legislative este etichetat drept evidence, nu drept număr de reguli.
