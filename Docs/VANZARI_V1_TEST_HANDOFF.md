# Vânzări V1 — handoff de test

Ramura `feature/vanzari`, worktree `/Users/adriantudoran/Projects/diana_worktrees/vanzari`. Design: `Docs/VANZARI_V1.md`,
decizii D-124…D-131, migrări `000039_vanzari_direction.sql` și `000040_vanzari_classification_direction.sql`.

## Ce a verificat Claude (doar static)

- `go build ./...`, `go vet ./...` (inclusiv `-tags=integration` pe `internal/platform/postgres`; singurul avertisment,
  `copylocks` în `classification_integration_test.go`, exista dinainte);
- `atlas migrate hash` și `atlas migrate validate`;
- `tsc -p tsconfig.app.json --noEmit` (include testele vitest);
- `python3 -m py_compile` pe scripturile harness-ului.

Nu a fost rulat niciun test: unit Go, integrare, vitest, E2E, harness, Gemini live.

## Comenzi

Parola locală (`DIANA_PASSWORD`) este singura valoare pe care o completezi tu.

### A. Pregătire worktree (o singură dată)

`node_modules` este un symlink spre checkout-ul principal (lock identic). Dacă preferi o instalare proprie:

```sh
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari && rm node_modules && npm ci
```

### B. Teste unit Go și vitest

```sh
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari/backend && GOCACHE=/private/tmp/diana-go-cache go vet ./... && GOCACHE=/private/tmp/diana-go-cache go test -count=1 ./...
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari && npm run typecheck && npm test
```

### C. Teste de integrare pe o bază izolată (`diana_vanzari_it`, Redis DB 11)

```sh
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari && docker compose up -d postgres redis
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari && docker compose exec -T postgres psql -U diana -d postgres -c "DROP DATABASE IF EXISTS diana_vanzari_it WITH (FORCE)" && docker compose exec -T postgres psql -U diana -d postgres -c "CREATE DATABASE diana_vanzari_it OWNER diana"
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari/backend && TEST_DATABASE_URL="postgresql://diana:diana@127.0.0.1:5442/diana_vanzari_it?sslmode=disable" atlas migrate apply --env test && TEST_DATABASE_URL="postgresql://diana:diana@127.0.0.1:5442/diana_vanzari_it?sslmode=disable" atlas migrate status --env test
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari && docker compose exec -T redis redis-cli -n 11 FLUSHDB
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari/backend && TEST_DATABASE_URL="postgresql://diana:diana@127.0.0.1:5442/diana_vanzari_it?sslmode=disable" TEST_REDIS_URL="redis://127.0.0.1:6382/11" GOCACHE=/private/tmp/diana-go-cache go test -count=1 -tags=integration ./...
```

Doar testele noi ale milestone-ului:

```sh
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari/backend && TEST_DATABASE_URL="postgresql://diana:diana@127.0.0.1:5442/diana_vanzari_it?sslmode=disable" GOCACHE=/private/tmp/diana-go-cache go test -count=1 -tags=integration ./internal/platform/postgres -run 'TestIssuedInvoice|TestSaleContract|TestSentInvoiceFromSPV|TestLeaseWhereClientIsLocator'
```

### D. Suite E2E afectate

Fiecare suită își recreează propria bază. Rulează-le pe rând, din worktree:

```sh
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari && npm run test:e2e
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari && npm run test:e2e:spv-connection
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari && npm run test:e2e:client-onboarding
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari && npm run test:e2e:backend4
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari && npm run test:e2e:contract-ingestion
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari && npm run test:e2e:accounting-v2
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari && npm run test:e2e:accounting-workflow
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari && npm run test:e2e:account-learning
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari && npm run test:e2e:saga-export-ux
```

### E. Stack local cu codul din worktree (baza partajată `diana`)

Migrările 000039 și 000040 sunt finale și au `atlas.sum` recalculat. Aplicarea lor pe `diana` este ireversibilă (forward-only).

```sh
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari && make migrate
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari && docker compose up -d --build api worker
```

### F. Harness VICTORIA (date reale)

```sh
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari && python3 test-data/victoria1881/pregateste.py
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari && DIANA_PASSWORD=... bash test-data/victoria1881/setup-victoria.sh curata
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari && DIANA_PASSWORD=... bash test-data/victoria1881/setup-victoria.sh tot
cd /Users/adriantudoran/Projects/diana_worktrees/vanzari && python3 test-data/victoria1881/compara.py
```

`tot` se oprește după pasul `contracte` și așteaptă Enter: confirmă cele 5 contracte în UI (link-urile sunt afișate).
VICTORIA trebuie să apară ca furnizor, iar chiriașul ca cumpărător, cu denumire. `curata` șterge și contractele, deci
fiecare rulare completă plătește din nou extragerea Gemini a celor 5 PDF-uri.

## Ce să verifici

1. `/invoices` are tab-urile „Primite” (43) și „Emise” (19). În „Emise”: coloanele „Client” și „Emitent”, CNP-urile
   afișate ca `194***` / `293***`.
2. Facturile emise nu creează task-uri „Contract lipsă” și ajung în `AWAITING_REVIEW`. VE18810066, 74, 75 și 79 sunt
   legate de contractele Horia, Strategic Events, EPTISA și Endeavor, dacă perioada extrasă acoperă data facturii.
3. Pe o factură emisă: VAT_DEDUCTIBILITY și EXPENSE_TAX_TREATMENT sunt finale, cu sursa „Direcția facturii (emisă)”.
   Avertismentul „TVA la încasare” este afișat. AI-ul propune 706/7041/167 și TVA IMMEDIATE (4427).
4. După aprobare, factura rămâne în `AWAITING_REVIEW` cu motivul „Formatul SAGA Ieșiri nu este încă verificat.”
   (VE18810067, garanția cu categoria E, arată motivul de reconciliere).
5. `compara.py`: secțiunea „vânzări” din tabelul de metrici. Reconcilierea pe iunie și iulie: 4111 D 69.838,37 /
   188.839,32 și 4427 C.

## Rezultate rulate de utilizator — 2026-09-30

- Go unit (`go test ./...`): trec toate pachetele.
- vitest: 204/204 după corecția testului de tab-uri (așteaptă încărcarea listei).
- Integrare Go (`-tags=integration ./...`, bază `diana_vanzari_it`): trec toate. Singurul eșec, la confirmarea
  contractului cu clientul Locator, venea din fixture: clientul de test avea un CUI sintetic nenumeric. Testul a fost
  corectat; rerularea lui este încă de confirmat.
- E2E care trec: `spv-connection`, `backend4`, `contract-ingestion`, `accounting-v2`, `accounting-workflow`,
  `saga-export-ux`.
- E2E care pică identic pe `main` (eb39eda, fără Vânzări V1), deci existau dinainte:
  - `test:e2e` (9): `account-learning` și `accounting-automatic-workflow` rulează în suita de bază deși cer backend
    (lipsesc din `testIgnore`); `module6` (reguli) și `module7` (link-ul „Reguli”, azi „Reguli și surse”);
  - `test:e2e:client-onboarding` A–G, la salvarea versiunii 2 a profilului;
  - `test:e2e:account-learning`, butonul „Selectează cont” lipsește pe I1.
