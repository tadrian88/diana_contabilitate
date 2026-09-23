# Diana — Automatic Accounting Analysis & Learning

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

2. Importă snapshoturile în aceeași bază `diana`. Pentru facturile locale existente, filtrul tehnic trebuie să înceapă cel târziu la `2026-09-04`; folosim `2026-09-01`. Aceasta este exclusiv o alegere de test, nu o dată juridică de aplicabilitate:

```bash
APP_ENV=development DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' go run ./cmd/legislationdraftimport -file legislation/source-snapshots/legea-227-2015-cod-fiscal-oug-38-2026.draft.json -effective-from 2026-09-01 -accept-test-only
APP_ENV=development DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' go run ./cmd/legislationdraftimport -file legislation/source-snapshots/omfp-1802-2014-original.draft.json -effective-from 2026-09-01 -accept-test-only
```

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

Un run păstrează factura/revizia, input snapshot, fragmentele și knowledge-ul folosit, provider/model, prompt/schema, răspunsul structurat, validările, tokenii și timpii. Review-ul păstrează separat propunerea AI și decizia finală. Knowledge-ul are `client_id`, profil, versiuni legislative și perioadă de valabilitate; cheile compozite împiedică asocierea cu profilul/review-ul altui client.

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
