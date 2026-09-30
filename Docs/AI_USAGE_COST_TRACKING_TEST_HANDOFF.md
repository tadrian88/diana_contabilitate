# Consum AI — test handoff

Status: verificat doar static de Claude (cod scris și formatat cu `gofmt`). Nu au fost rulate compilarea, testele unit, testele de integrare, vitest, E2E, migrațiile sau Gemini live.

## 1. Înainte de migrare: confirmă modelele și prețurile

Rulează local (și pe cloud-test, dacă ai acces) query-ul de mai jos. El arată modelele din istoric și volumul de tokeni:

```bash
docker compose exec -T postgres psql -U diana -d diana -c "
SELECT 'ACCOUNTING_ANALYSIS' AS kind, upper(provider), model, count(*), min(started_at), max(started_at), sum(input_tokens), sum(output_tokens)
FROM accounting_analysis_runs WHERE input_tokens IS NOT NULL OR output_tokens IS NOT NULL GROUP BY 2,3
UNION ALL
SELECT 'CONTRACT_EXTRACTION', upper(provider), model, count(*), min(started_at), max(started_at), sum(input_tokens), sum(output_tokens)
FROM contract_extraction_attempts WHERE input_tokens IS NOT NULL OR output_tokens IS NOT NULL GROUP BY 2,3;"
```

Pentru fiecare model din rezultat care nu e `gemini-3.8-flash` trebuie adăugat un preț în 000038, înainte de hash. Altfel rândurile lui rămân `NO_PRICE`.

## 2. Backend

```bash
cd backend
atlas migrate hash
atlas migrate validate --dir file://migrations
GOCACHE=/private/tmp/diana-go-cache go vet ./...
GOCACHE=/private/tmp/diana-go-cache go test ./internal/llmusage/... ./internal/platform/gemini/... ./internal/contractingestion/... ./internal/accountinganalysis/... ./internal/platform/httpserver/... ./internal/platform/observability/...
GOCACHE=/private/tmp/diana-go-cache go test ./...
```

Integrare (bază de test izolată, deja migrată):

```bash
cd backend
TEST_DATABASE_URL=... atlas migrate apply --env test
TEST_DATABASE_URL=... GOCACHE=/private/tmp/diana-go-cache go test -count=1 -tags=integration ./internal/platform/postgres -run 'LLMUsage'
TEST_DATABASE_URL=... GOCACHE=/private/tmp/diana-go-cache go test -count=1 -tags=integration ./internal/platform/postgres
```

## 3. Frontend

```bash
npm run typecheck
npx vitest run src/features/ai-usage src/repositories/http src/features/clients
npm test
```

## 4. Local, end-to-end

```bash
make migrate
docker compose exec -T postgres psql -U diana -d diana -c "
SELECT run_kind, source, cost_status, count(*), sum(input_tokens), sum(output_tokens), sum(cost_usd) FROM llm_usage_events GROUP BY 1,2,3 ORDER BY 1,2,3;"
```

Sumele `BACKFILL` pe fiecare `run_kind` trebuie să fie egale cu sumele din query-ul de la pasul 1.

Apoi:
1. Pornește aplicația cu `make dev`.
2. Declanșează o analiză contabilă și o extragere de contract (sau o re-extragere).
3. Verifică rândurile noi `LIVE`: `cost_status='PRICED'`, `thought_tokens` și `usage_detail` completate, iar la contracte un rând `CONTRACT_CLAUSE_NORMALIZATION` când există clauze fără expresie.
4. În UI, verifică pagina „Consum AI” și cardul „Consum AI” de pe pagina clientului: filtrul de perioadă, lista rulărilor și detaliul pe apeluri.
5. Loghează-te cu un utilizator cu grant-uri limitate. Pagina trebuie să arate doar clienții lui, iar `/api/v1/clients/<alt-client>/ai-usage` trebuie să răspundă cu 404.
6. `curl` pe `/metrics` (cu sesiune) trebuie să conțină `llm_usage_events_recorded_total`.

Gate complet:

```bash
make release-local
```
