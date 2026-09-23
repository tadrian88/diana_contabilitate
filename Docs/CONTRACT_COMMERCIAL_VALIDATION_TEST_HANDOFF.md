# Contract Commercial Validation — Test Handoff

## Flux reutilizabil pentru tarife, servicii, TVA și scadență (2026-09-22)

Contractele confirmate transformă acum serviciile cu preț și dovadă concordantă în reguli reutilizabile. Pentru contractele deja confirmate există acțiunea „Activează tarifele confirmate”; aceasta creează o versiune contractuală nouă și nu copiază valori din factură. O descriere generică se asociază prin alegerea explicită a serviciului: implicit numai pentru factura curentă, iar reutilizarea pe facturi viitoare necesită bifă și este limitată la același dosar contractual. Serviciile nefacturate nu mai produc constatări false.

Tariful unitar și cantitatea sunt verificate separat; de exemplu, 50 RON/salariat necesită numărul salariaților și sursa lui. Clauza „TVA aferentă/aplicabilă”, fără procent în contract, devine o regulă bazată pe `applicable_vat_rate`; cota, sursa fiscală și perioada se confirmă separat. Termenul de la remiterea facturii folosește o dată și o dovadă specifice facturii și nu substituie data primirii, emiterii sau importului SPV.

Testele unitare backend, typecheck-ul și cele 24 de teste UI țintite au trecut. Pentru migrarea și testarea runtime pe baza dedicată:

```sh
cd backend
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_contract_commercial_test?sslmode=disable' atlas migrate apply --env test
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_contract_commercial_test?sslmode=disable' GOCACHE=/private/tmp/diana-go-cache go test -count=1 -tags=integration ./internal/platform/postgres -run '^(TestReviewedServicePricesCanBeActivatedForAnExistingConfirmedContract|TestCommercialDateFactIsInvoiceScopedProvenancedAndIdempotent|TestPendingCommercialClausePersistsWithoutExecutablePriceFallback)$'
```

Nu aplica migrarea pe baza locală de lucru înainte ca testele de mai sus să treacă. După PASS: `cd .. && make migrate && docker compose up -d --build --wait api worker frontend`.

## Verificări pentru descrierea e-Factura și confirmarea regulilor (2026-09-22)

XML-ul 6763517426 are referința `NR.102/25.06.2025` în descrierea articolului, nu în denumirea articolului. Parserul și reverificarea din arhiva SPV păstrează acum acest text și indică sursa. Clauzele extrase fără formulă rămân propuneri până când utilizatorul confirmă tariful/documentul; formulările generice se confirmă pentru dosarul contractual, nu global pentru furnizor. Aliasurile vechi, fără dosar, rămân în istoric, dar nu se mai aplică automat. Lipsa datei reale de primire păstrează termenul de plată ca neverificabil; data importului din SPV nu o înlocuiește.

Nu rula pe baza locală obișnuită testele de integrare; migrează numai baza dedicată, deja folosită pentru aceste teste:

```sh
cd backend
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_contract_commercial_test?sslmode=disable' atlas migrate apply --env test
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_contract_commercial_test?sslmode=disable' GOCACHE=/private/tmp/diana-go-cache go test -count=1 -tags=integration ./internal/platform/postgres -run 'TestPendingCommercialClausePersistsWithoutExecutablePriceFallback|TestExecutableProposalCanBeConfirmedAfterContractConfirmation|TestCommercialValidationLegacyPartialIsIdempotentConcurrentAndClientScoped'
cd ..
npm run typecheck
npm test -- --run
```

Pentru gate-ul complet, rulează separat `./scripts/release-check.sh` din rădăcina proiectului; nu instala local prin `make release-local` până când toate testele trec și verifici manual tarifele și sursele din interfață. Verificările rapide Go țintite și `git diff --check` au trecut; migrarea, integrarea, suita frontend completă și gate-ul nu au fost rulate de Codex după aceste modificări.

## Actualizare: normalizare Gemini a clauzelor și confirmare umană per regulă

Utilizatorul a raportat PASS pentru `TestPendingCommercialClausePersistsWithoutExecutablePriceFallback` și `make release-local` pe versiunea anterioară. Aceste rezultate **nu** acoperă schimbarea de mai jos. Acum ingestia face o a doua trecere Gemini numai peste clauzele fără expresie, fără a retrimite PDF-ul; fiecare AST valid rămâne propunere până la confirmarea individuală în UI. Dacă a doua trecere eșuează, clauzele narative rămân `PARTIAL`. O regulă propusă, dar neconfirmată, nu devine preț activ și este stocată `PROPOSED`.

Codex a rulat rapid testele unitare țintite pentru cele două răspunsuri Gemini, compilarea testelor PostgreSQL integration fără execuție, frontend typecheck și cele 18 teste UI de review. Rulează tu verificările lungi, în ordine, din rădăcina proiectului:

```sh
cd backend
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_contract_commercial_test?sslmode=disable' atlas migrate apply --env test
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_contract_commercial_test?sslmode=disable' GOCACHE=/private/tmp/diana-go-cache go test -count=1 -tags=integration ./internal/platform/postgres -run '^(TestPendingCommercialClausePersistsWithoutExecutablePriceFallback|TestNormalizedCommercialClauseRequiresHumanSelection)$'
cd ..
docker compose stop api
make release-local
```

Nu îndrepta integrarea spre baza locală normală. După PASS, încarcă în UI un document nou sau reîncearcă o extragere eșuată; un document deja extras și deduplicat nu se reprocesează automat. Verifică în PDF fiecare formulă, moneda, intervalul/tier-ul, serviciul și pagina/citatul. Înainte de „Confirmă regula”, acoperirea este parțială; după confirmarea tuturor regulilor susținute de contract, poți alege acoperirea completă. Clauzele care încă nu au AST țin acoperirea parțială. Apoi validează o factură cu preț corect și una cu preț greșit pentru același contract, inclusiv referința contractuală. Nu marca Gemini live drept PASS doar pentru că ingestia a terminat: calitatea formulelor cere această verificare manuală.

## Schimbare ulterioară: arhivare și reingestia încărcării greșite

Sunt două acțiuni: „Scoate din utilizare” pentru un contract valid cu istoric și „Șterge încărcarea greșită” pentru un contract nefolosit. Ultima permite reîncărcarea identică a PDF-ului/referinței prin migrarea `000019`. Nu se șterg fizic documentele sau snapshot-urile. Codex a verificat rapid `atlas migrate validate`, compilarea testelor PostgreSQL integration fără execuție, testele Go unit țintite, frontend typecheck și 10 teste ContractsWorkspace; integrarea reală și gate-ul complet rămân pentru utilizator.

Rulează în ordine, din rădăcina proiectului:

```sh
docker compose up -d postgres redis
cd backend
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_contract_commercial_test?sslmode=disable' atlas migrate apply --env test
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_contract_commercial_test?sslmode=disable' GOCACHE=/private/tmp/diana-go-cache go test -count=1 -tags=integration ./internal/platform/postgres -run 'TestArchiveContract|TestArchivedValidContract|TestMistakenContract|TestMistakenDelete'
cd ..
npm run test:e2e:backend4
make release-local
```

Nu rula migrarea sau testele de integrare pe baza locală normală. Rezultate așteptate: arhivarea păstrează factura/snapshot-ul istoric, dar exclude matching-ul viitor; ștergerea unei încărcări folosite produce `CONTRACT_IN_USE`; un PDF greșit nefolosit poate fi încărcat și confirmat din nou cu aceeași referință, fără rescrierea vechiului dosar.

`make release-local` rulează singur gate-ul complet, apoi aplică migrările restante pe baza locală și reconstruiește aplicația pentru validarea manuală în browser. Nu rula separat `./scripts/release-check.sh` înainte decât dacă dorești doar verificarea fără instalare locală; ai repeta întregul gate. Nu folosi `make reset` pentru eliminarea unui contract; comanda șterge întregul volum local.

Rularea utilizatorului din 2026-09-21: migrarea pe baza dedicată nu avea fișiere restante, iar testele PostgreSQL țintite pentru arhivare/ștergere au trecut. `backend4` nu a pornit deoarece API-ul local ocupa deja `8080`; configurația suitei folosește acum `8184` pentru backend-ul său izolat. `make release-local` a trecut frontend typecheck/build, dar s-a oprit la testul auxiliar `TestSchemaDiagnostic`, care apela Gemini fără endpoint; testul live cere acum opt-in `GEMINI_SCHEMA_DIAGNOSTIC_LIVE=1` și configurația providerului. Codex a verificat rapid testul țintit (skip fără opt-in) și enumerarea celor 7 teste backend4. E2E-ul efectiv și gate-ul complet trebuie rerulate de utilizator, în această ordine:

```sh
npm run test:e2e:backend4
docker compose stop api
make release-local
```

`docker compose stop api` eliberează `8080` pentru celelalte suite E2E din gate; `make release-local` repornește API-ul la final dacă gate-ul trece. Dacă gate-ul eșuează, repornește vechiul API cu `docker compose up -d api` înainte de a relua lucrul local. Dacă portul `8080` aparține altui proces, identifică-l cu `lsof -nP -iTCP:8080 -sTCP:LISTEN` și nu opri un proces necunoscut. Nu considera instalarea locală sau gate-ul PASS până la terminarea lui `make release-local`. Diagnosticarea live a schemei Gemini este separată și poate genera cost; nu seta `GEMINI_SCHEMA_DIAGNOSTIC_LIVE=1` în gate-ul obișnuit.

Actualizare: utilizatorul a confirmat ulterior PASS pentru `backend4` și `make release-local`. Instalarea locală a trecut; smoke test-ul Gemini pentru contractul contabil 102 a primit un răspuns structurat, dar validatorul l-a respins la `commercialClauses[0].rule` cu `COMMERCIAL_RULE_INVALID`. Aceasta **nu** dovedește extracția corectă sau un snapshot comercial complet. Promptul `V4_1` explicitează enumurile și forma AST, iar eroarea sigură indică acum subcâmpul invalid când regula se decodează. Codex a rulat doar două teste unitare rapide pentru diagnostic/schema providerului și `git diff --check`; Gemini live și gate-ul complet nu au fost rerulate. Din `backend`, cu variabilele Gemini setate privat, utilizatorul poate rerula o singură dată:

```sh
GOCACHE=/private/tmp/diana-go-cache go run ./cmd/contractextract -file '/Users/adriantudoran/Downloads/CONTRACT SERVICII CONTABILE SOFTCO 102.pdf'
```

Comanda transmite din nou PDF-ul la provider și poate genera cost. Dacă eșuează, păstrează doar `code` și `path` din stderr; nu publica cheia, PDF-ul sau răspunsul brut. Un rezultat `validated=true` confirmă doar trecerea validatorului, nu corectitudinea tuturor tarifelor; verificarea în UI și confirmarea umană rămân obligatorii.

Următoarea rulare live a utilizatorului a indicat `COMMERCIAL_RULE_INVALID` la `commercialClauses[0].rule.id`: providerul a omis identificatorul tehnic. Adaptorul completează acum numai ID-urile goale cu un identificator determinist, distinct între PDF-uri; niciun câmp comercial nu este reparat automat. Două teste unitare rapide pentru stabilitate/izolare și păstrarea câmpurilor necunoscute au trecut; `git diff --check` a trecut. Live Gemini și gate-ul complet nu au fost rerulate după această corecție. Rerulează aceeași comandă live de mai sus o singură dată, dacă accepți transmiterea/costul; dacă trece, verifică manual conținutul înainte de confirmare. Dacă eșuează, trimite doar `code` și `path`.

Rularea live următoare a indicat `COMMERCIAL_RULE_INVALID` la `commercialClauses[0].rule.narrative`: providerul a omis copia narațiunii din regula internă, deși clauza are deja o narațiune separată. Adaptorul preia acum narațiunea și dovada din aceeași clauză doar când lipsesc din regulă; nu generează prețuri, formule sau tipuri de reguli. Testele unitare țintite pentru completare și pentru păstrarea erorilor semantice au trecut. Gemini live și gate-ul complet nu au fost rerulate după această corecție. Aceeași comandă live de mai sus este următorul pas opțional, cu transmitere și cost; raportează doar `code`/`path` dacă apare altă eroare.

Rularea live ulterioară a indicat `COMMERCIAL_RULE_INVALID` la `commercialClauses[0].rule.kind`. Adaptorul copiază acum `clause.kind` în `rule.kind` numai dacă regula nu are tip și valoarea clauzei este exact unul dintre tipurile acceptate de motor; valorile necunoscute sau contradictorii rămân erori. Smoke test-ul afișează acum, la un eșec comercial, forma tuturor regulilor (maximum 12): numai `present/missing/unsupported/malformed/incomplete`, fără valori, citate sau răspunsul brut. Aceasta permite diagnosticarea mai multor câmpuri dintr-un singur apel plătit. Testele unitare rapide țintite au trecut; Gemini live și gate-ul complet nu au fost rerulate după această schimbare. Înainte de alte confirmări în UI, rulează aceeași comandă live doar dacă accepți noua transmitere/costul și verifică manual toate clauzele extrase. Dacă eșuează, poți trimite `code`, `path` și liniile `Commercial rule shape`.

Următorul rezultat live a arătat că toate cele patru reguli au `expression=missing`, deși `id`, `kind`, `narrative` și `evidence` sunt prezente. Nu există o formulă sigură de dedus din aceste date. Codul acceptă acum aceste clauze ca propuneri narative `PROPOSED`, le păstrează cu pagina/citatul, forțează `PARTIAL` și nu activează fallback-ul de tarif plat. Dacă următoarea extragere are aceeași formă și trece celelalte validări, smoke test-ul va afișa `pending_commercial_rules=4`; acesta nu este un PASS de validare a prețurilor. Testele unitare Go țintite, compilarea testului PostgreSQL integration, frontend typecheck, 17 teste de review UI și `git diff --check` au trecut. Integration runtime, Gemini live și gate-ul complet **nu** au fost rerulate după această schimbare. Nu mai rula smoke test-ul Gemini doar ca să identifici alt câmp lipsă; după instalarea locală, un upload/retry în UI va face apelul necesar pentru verificarea produsului.

Pentru verificarea noului flux, rulează din rădăcina proiectului, în această ordine:

```sh
cd backend
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_contract_commercial_test?sslmode=disable' atlas migrate apply --env test
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_contract_commercial_test?sslmode=disable' GOCACHE=/private/tmp/diana-go-cache go test -count=1 -tags=integration ./internal/platform/postgres -run '^TestPendingCommercialClausePersistsWithoutExecutablePriceFallback$'
cd ..
docker compose stop api
make release-local
```

`make release-local` repornește API-ul dacă trece. Dacă eșuează după `docker compose stop api`, poți restaura versiunea locală anterioară cu `docker compose up -d api`. În UI, verifică cele patru clauze și dovezile, `PARTIAL` și faptul că prețurile facturilor sunt `NEVERIFICABIL`, nu `CONFORM`. Pentru validare automată de preț este încă necesară transpunerea confirmată a grilei în expresii AST.

Status istoric pentru corecția anterioară a cache-ului comercial: verificările lungi au fost rulate de utilizator; Codex a rulat `npm run typecheck` și `git diff --check`. E2E și gate-ul complet nu au fost rerulate de Codex după schimbarea de arhivare/reingestie.

## Istoric de teste anterior schimbării `000019` — 2026-09-21

Utilizatorul a comunicat ulterior că toate testele versiunii anterioare au trecut; detaliile de mai jos păstrează traseul erorilor intermediare, nu descriu gate-ul noii schimbări.

- Utilizatorul a raportat PASS pentru migrarea `000018`, `go vet`, toate testele unit Go, frontend typecheck/build și 115/115 teste frontend.
- PostgreSQL integration: PASS (8.135s); race țintit `Commercial|ContractDossier|CommercialSnapshot`: PASS (2.988s); Redis integration: PASS (20.126s).
- E2E contract ingestion: 6/6 PASS; Accounting V2: 2/2 PASS; E2E mock general: 23/23 PASS. Warning-ul `Retry exhausted` din suita de ingestie a apărut în scenariul de extragere eșuată, fără eșec de test.
- `release-check.sh` a trecut instalarea, static/build, unit, migrarea pe baza izolată `diana_release_gate` și PostgreSQL integration. S-a oprit în etapa combinată PostgreSQL outbox/Asynq la `TestOutboxAsynqWorkerCompletesFullAutomaticPipeline`: condiția de export nu s-a îndeplinit în limita de 10 secunde. Nu există încă dovada că este doar un timeout, nu un blocaj funcțional.
- Testul parcurge acum etape comerciale suplimentare. Limita de așteptare a fost extinsă doar pentru acest test la 25 secunde, iar la eșec va raporta starea facturii și stările outbox. Compilarea țintită a trecut; testul runtime și `release-check.sh` trebuie rerulate de utilizator. Gate-ul de release rămâne NO.
- `npm audit` a semnalat două vulnerabilități moderate, iar build-ul a emis avertizări despre dimensiunea bundle-ului și adnotări din dependențe. Acestea nu au oprit gate-ul; necesită triere separată, fără `npm audit fix --force` automat.

Actualizare după următorul `release-check.sh`: etapa combinată PostgreSQL outbox/Asynq a trecut (20.910s), la fel worker/Asynq (17.749s), E2E mock general 23/23 și backend1, backend3, backend4, backend5. Gate-ul s-a oprit la backend6, testul N (13/14), deoarece factura ajunsese la `AWAITING_COMMERCIAL_REVIEW`, dar UI-ul afișa „Rezultatul nu este disponibil încă”. `useCommercialValidation` memorase răspunsul 404 primit în `COMMERCIAL_VALIDATING` sub o cheie care nu se schimba când factura avansa. Cheia include acum revizia și starea facturii, deci tranziția cere din nou rezultatul. Typecheck și `git diff --check` au trecut; backend6 și gate-ul complet necesită rerulare. Backend7 și suitele ulterioare nu au fost executate de gate.

Rerulează întâi `npm run test:e2e:backend6`, apoi `npm run test:e2e:backend7`; dacă trec, rulează `./scripts/release-check.sh`. Scriptul recreează exclusiv baza `diana_release_gate`. Nu continua cu deployment dacă gate-ul eșuează.

## Istoric al corecțiilor și rerulărilor — 2026-09-21

- Primul gate a expus un test unit cu vocabular vechi; fixture-ul a fost actualizat. Ulterior, testele PostgreSQL au expus un parametru SQL netipizat la rerulare și coliziuni de ID în baza reutilizată; cast-ul și fixture-urile au fost corectate. Rerularea utilizatorului a confirmat PASS pentru PostgreSQL integration și race.
- Harness-urile E2E au fost mutate de pe porturile ocupate, iar fixture-urile de clasificare au fost aliniate la `COMMERCIALLY_VALIDATED`. Rerularea utilizatorului a confirmat PASS pentru toate cele trei suite E2E.
- Testul outbox/Asynq a trecut la următoarea rulare a gate-ului după ajustarea limitei; eșecul curent este E2E backend6 N, descris mai sus.

## Fixture-uri și rezultate așteptate

1. Contract contabil 102: snapshot-ul confirmat păstrează narațiunea/tabelul complet — pragurile după număr de documente și statut TVA, 50 RON/salariat, serviciile de 300 RON, situațiile anuale/semestriale, TVA exclusă, frecvența lunară, scadența și cheltuielile la cost. `500 RON` nu devine tarif universal dacă anexa aplicabilă îl schimbă.
2. Contract cadru fără preț: dosar `ACTIVE_PARTIAL`; factura poate fi asociată, iar prețul este `NEVERIFICABIL` până la anexă.
3. Anexă/act adițional: documentul este atașat prin `relatedReference`; numai rule ID-urile identificate sunt înlocuite; snapshot-ul anterior și run-urile anterioare rămân neschimbate.
4. Contract GRUP 19 / CUI RO27770826: discount 75%, tranșele anului 2 de 750 EUR și 625 EUR.
5. Contract SA 20 / CUI RO1446711: discount 25%, tranșele anului 2 de 2.250 EUR și 1.875 EUR.
6. COL30: se asociază GRUP/19; prețurile sunt conforme, referința textuală la 20 produce `CONTRACT_REFERENCE_MISMATCH`.
7. COL31: se asociază SA/20; prețurile sunt conforme, referința textuală la 19 produce `CONTRACT_REFERENCE_MISMATCH`.
8. Storno: fără original produce `ORIGINAL_INVOICE_UNAVAILABLE`; cu original verifică referința UBL, moneda, semnul și proporția.
9. Semnătura divergentă din contractul GRUP: `AMBIGUOUS`/clauză `IDENTITY`, fără suprascrierea preambulului sau CUI-ului.
10. Tabele/tier-uri, indexare, tranșe, prorata, min/max, cost-plus, curs, TVA, servicii mixte și termene: calcul exact dacă toate inputurile cu proveniență există; altfel `NEVERIFICABIL`.
11. Duplicate comerciale, idempotency, concurență, tenant isolation, prompt injection și snapshot history trebuie verificate explicit de suitele targetate.

## 1. Static, build și unit tests

```sh
git diff --check
cd backend
atlas migrate validate --dir file://migrations
GOCACHE=/private/tmp/diana-go-cache go vet ./...
GOCACHE=/private/tmp/diana-go-cache go test -count=1 ./...
cd ..
npm run typecheck
npm run build
npm test
```

## 2. Dependențe și bază izolată

```sh
docker compose up -d postgres redis
docker compose exec -T postgres createdb -U diana diana_contract_commercial_test
export TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_contract_commercial_test?sslmode=disable'
export TEST_REDIS_URL='redis://127.0.0.1:6382/14'
cd backend
atlas migrate apply --env test
atlas migrate status --env test
```

Dacă baza există deja, comanda `createdb` va eșua; nu o înlocui cu baza locală normală. Ștergerea/recrearea bazei dedicate este o decizie explicită a operatorului.

## 3. Integration și concurență

```sh
GOCACHE=/private/tmp/diana-go-cache go test -count=1 -tags=integration ./internal/platform/postgres
GOCACHE=/private/tmp/diana-go-cache go test -race -count=10 -tags=integration ./internal/platform/postgres -run 'Commercial|ContractDossier|CommercialSnapshot'
TEST_REDIS_URL='redis://127.0.0.1:6382/14' GOCACHE=/private/tmp/diana-go-cache go test -count=1 -tags=redis_integration ./internal/workerruntime
cd ..
```

## 4. E2E dedicat și regresii

```sh
npm run test:e2e:contract-ingestion
npm run test:e2e:accounting-v2
npm run test:e2e
```

## 5. Gate complet

Mai întâi, din rădăcina proiectului, verifică testul care a expirat în gate (folosește baza și Redis dedicate, existente din secțiunea 2):

```sh
cd backend
TEST_DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana_contract_commercial_test?sslmode=disable' TEST_REDIS_URL='redis://127.0.0.1:6382/14' GOCACHE=/private/tmp/diana-go-cache go test -count=1 -tags='integration redis_integration' ./internal/platform/postgres -run '^TestOutboxAsynqWorkerCompletesFullAutomaticPipeline$'
cd ..
```

Dacă trece, rulează gate-ul complet:

```sh
./scripts/release-check.sh
```

Scriptul recreează exclusiv `diana_release_gate`. Nu seta comenzile de integrare către baza locală normală sau către un mediu real.

## 6. Gemini live — separat, cu transmitere externă și cost

```sh
export GEMINI_CONTRACT_MODEL='gemini-3.8-flash'
# GEMINI_API_KEY se setează privat.
cd backend
GOCACHE=/private/tmp/diana-go-cache go run ./cmd/contractextract -file '/cale/absoluta/contract-aprobat.pdf'
cd ..
```

Rulați separat cele trei contracte reale și verificați manual JSON-ul V4 înainte de confirmare. Nu includeți cheia sau payload-ul providerului în loguri/tichete.

## 7. Deployment GCP TEST

Numai după commit curat și toate gate-urile:

```sh
make deploy-gcp-test
```

## Raportarea rezultatului

Pentru fiecare etapă, păstrați comanda, exit code-ul și primul mesaj de eroare relevant. Nu declarați `PASS` pentru Gemini live, migration, runtime, E2E, release gate sau deployment fără output-ul acelei execuții.
