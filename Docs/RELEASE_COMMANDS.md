# Comenzi standard de validare și deployment

Acestea sunt cele două comenzi canonice care se rulează după o modificare sau
o implementare. Ambele sunt `fail-fast`: la primul test, migration gate, build
sau health check eșuat, deployment-ul nu continuă.

## 1. Validare completă și deployment local

Din rădăcina repository-ului:

```bash
make release-local
```

Comanda execută, în ordine:

1. `npm ci`, verificarea diff-ului și validarea checksum-urilor Atlas;
2. `go vet`, TypeScript typecheck și build-ul frontend de producție;
3. toate testele Go și Vitest;
4. PostgreSQL + Redis în Docker;
5. toate migrațiile pe baza izolată `diana_release_gate`;
6. toate testele Go de integrare PostgreSQL și Redis; suitele Asynq care golesc
   baza Redis rulează serial, pe baze Redis dedicate, pentru a nu se invalida
   reciproc;
7. toate suitele Playwright, inclusiv backend modules, accounting V2, SPV,
   SAGA, onboarding, authentication și contract ingestion;
8. migrațiile pe baza locală normală `diana`;
9. rebuild și restart pentru PostgreSQL, Redis, API, worker și frontend;
10. health checks externe pentru API, worker și frontend, cu retry limitat cât
    timp porturile Docker devin accesibile pe host; la eșec sunt afișate
    automat starea containerelor și ultimele loguri ale serviciului.

Bazele E2E sunt dedicate și resetabile; testele nu mai folosesc baza locală
principală `diana`. Volumele și datele aplicației locale nu sunt șterse.
Suita Playwright UI de bază rulează intenționat cu repository și utilizator
mock, fără proxy către API. Suitele care validează backend-ul își creează în
fiecare bază izolată un utilizator exclusiv de test și obțin o sesiune reală;
testul complet de login/logout rulează separat în suita `authentication`.

Dacă un gate eșuează, noul cod nu este deployat. Serviciile locale care rulau
înainte de Playwright sunt repornite. Pentru diagnostic:

```bash
make diagnose
```

La succes, aplicația este disponibilă la `http://127.0.0.1:5173`.

## 2. Validare completă și deployment Google Cloud TEST

Cloud deployment-ul este permis numai dintr-un commit curat și folosește
proiectul/contul fixate în `deploy/gcp-test.env`:

```bash
make deploy-gcp-test
```

Comanda cere tastarea explicită a textului `DEPLOY`, apoi:

1. rulează exact gate-ul complet folosit de release-ul local;
2. creează un tag imutabil `GIT_SHA-UTC_TIMESTAMP`;
3. construiește și publică imaginile backend, migration și frontend;
4. actualizează și execută jobul Atlas de migrare, cu `--wait`;
5. actualizează și execută jobul de migration status, cu `--wait`;
6. face rollout pentru API, callback ANAF și worker cu aceeași imagine backend;
7. face rollout pentru frontend;
8. rulează verificările DNS, TLS, health și acces anonim din
   `scripts/gcp/verify-test.sh`;
9. afișează tag-ul release-ului și reviziile precedente pentru rollback manual.

Pentru CI controlat, promptul poate fi eliminat explicit:

```bash
CONFIRM_GCP_TEST_DEPLOY=1 make deploy-gcp-test
```

Nu folosi această variabilă într-un shell interactiv obișnuit. Comanda vizează
doar mediul Google Cloud **TEST**, nu PROD.

## Cerințe inițiale

- Docker și Docker Compose funcționale;
- Node/npm și dependențele din lockfile;
- Go și Atlas CLI;
- Chromium Playwright instalat (`npx playwright install chromium` o singură dată);
- pentru Cloud: `gcloud` autentificat ca utilizatorul din `deploy/gcp-test.env`;
- working tree curat și commit publicabil pentru Cloud;
- acces la Artifact Registry, Cloud Run services/jobs și resursele TEST.

## Ce nu intră în automatizare

Gate-ul include toate testele automate deterministe din repository. Nu poate
automatiza verificările care necesită intervenție umană sau sisteme externe
reale: autentificarea ANAF cu certificat, acceptanța unui document real în
Gemini, importul în aplicația desktop SAGA și validarea vizuală/business în
Cloud. Acestea rămân checklist-uri de acceptanță separate.

Nu se face rollback automat al bazei de date. Migrațiile sunt forward-only;
dacă rollout-ul aplicației eșuează după migrare, serviciile pot fi mutate manual
pe reviziile precedente afișate de comandă, iar baza necesită un forward-fix sau
o restaurare revizuită separat.

## Implementare

- gate comun: `scripts/release-check.sh`;
- release local: `scripts/release-local.sh`;
- release Cloud TEST: `scripts/gcp/deploy-test.sh`;
- entry points stabile: `Makefile`.
