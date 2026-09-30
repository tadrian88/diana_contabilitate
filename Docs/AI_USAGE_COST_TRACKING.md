# Consum AI: tokeni și cost pe rulare, client și cont

Status: **IMPLEMENTAT ÎN WORKSPACE — AȘTEAPTĂ TESTELE UTILIZATORULUI.** Deciziile sunt D-121 și D-122.

## De ce

Diana apelează Gemini în trei locuri:
- extragerea contractelor;
- normalizarea clauzelor comerciale;
- analiza contabilă a facturilor.

Până acum tokenii se salvau doar parțial, pe `accounting_analysis_runs` și `contract_extraction_attempts`:
- doar la succes;
- fără retry-uri și fără apelurile eșuate;
- fără tokenii de raționament, pe care Google îi facturează ca output.

Costul nu exista deloc. Modulul acesta măsoară fiecare apel și arată tokenii și costul estimat în USD la trei niveluri:
- **pe rulare**;
- **pe client**;
- **pe cont**, adică suma clienților la care are acces contabilul autentificat.

## Definiții

| Termen | Sens |
|---|---|
| Apel | Un request `POST /interactions` către Gemini. Se salvează un rând în `llm_usage_events`, inclusiv pentru 429/5xx, răspunsuri invalide și erori de transport. |
| Rulare | O analiză contabilă (`accounting_analysis_runs`; retry-urile Asynq reutilizează rândul, deci toate încercările sunt în aceeași rulare) sau o încercare de extragere a unui contract (`contract_extraction_attempts`, care conține extragerea și normalizarea clauzelor; fiecare retry Asynq e o rulare nouă, „încercarea k”). |
| Client | Clientul rulării. Se ia din rularea însăși, niciodată din contextul apelantului. |
| Cont | Un rând `auth_users`. Totalul contului = suma clienților din `auth_user_client_grants`, sau a tuturor clienților când `all_clients=true`. Totalul urmează grant-urile curente. Numele „account” e evitat în cod, pentru că `Account` înseamnă planul de conturi. |

## Captură

- **Transport comun.** `internal/platform/gemini.Client.Interact` înlocuiește codul HTTP copiat în trei locuri. El citește mereu body-ul, apoi decodează `usage` separat de restul răspunsului, astfel încât usage-ul supraviețuiește unui `steps` invalid sau unui status `failed`. Înregistrează exact un eveniment per request trimis, înainte ca apelantul să interpreteze răspunsul. Orice cale de eroare de după (status, envelope, schemă, validare semantică) e deci acoperită din construcție.
- **Taxonomia de erori rămâne neschimbată.** Asta e valabil pentru `FailureProvider*`, `PROVIDER_HTTP_*`, `INVALID_PROVIDER_*` și pentru retururile `nil, nil` ale normalizării.
- **Atribuirea se face prin context.** Serviciul care deține rularea setează `llmusage.WithScope`:
  - `contractingestion.Service.Extract`, după `BeginExtraction`;
  - `accountinganalysis.WorkflowService.Process`, înainte de `engine.Analyze`.

  Interfețele `ContractExtractor` și `Analyzer`, fake-urile și `cmd/*` nu se schimbă.
- **Câmpurile Interactions API salvate:**
  - `total_input_tokens`, `total_output_tokens`, `total_thought_tokens`, `total_cached_tokens`, `total_tool_use_tokens` și `total_tokens`;
  - obiectul `usage` brut, în `usage_detail` (maximum 16 KiB, doar contori și defalcări pe modalitate, fără conținut).
- **Înregistrarea e best-effort** (`llmusage.BestEffort`).
  - O eroare de scriere nu eșuează operația de business, pentru că un retry ar însemna încă un apel facturat.
  - Inserarea rulează pe `context.WithoutCancel` cu timeout de 5 s.
  - La eșec se scrie un log `llm usage record failed` cu toți tokenii și se incrementează `llm_usage_record_failures_total`.
  - Un apel fără scope se loghează ca `llm call without usage scope` și incrementează `llm_usage_unattributed_total`.
- **Coloanele legacy** `input_tokens`/`output_tokens` de pe tabelele de rulări își păstrează sensul: doar succes, fără tokenii de raționament.

## Cost

Costul se calculează într-un singur loc, funcția SQL `llm_usage_cost`, la momentul inserării:

```
cost = (input − cached) × preț_input + cached × preț_cache + (output + raționament) × preț_output     (per 1M tokeni)
```

- `total_cached_tokens` face parte din prompt (API: „tokens in the cached part of the prompt”).
- Tokenii de tool-use se salvează, dar nu se tarifează separat; Diana nu folosește tools.
- Rezultatul se stochează ca `numeric(20,10)`, deci un apel de o fracțiune de cent nu devine 0. Costul rămâne înghețat.
- `cost_status` poate fi:
  - `PRICED`;
  - `NO_PRICE`: nu există preț pentru model, iar costul e NULL și exclus din sumă, dar semnalat;
  - `NO_USAGE`: răspunsul nu a raportat tokeni.

Prețurile stau în `llm_model_prices`, care e append-only (un trigger respinge UPDATE și DELETE) și versionată prin `effective_from`. Un preț nou înseamnă un rând nou, adăugat într-o migrație nouă.

Seed-ul din 000038 folosește prețurile de pe https://ai.google.dev/gemini-api/docs/pricing, pagina actualizată la 2026-09-24, tier paid, standard:

| Model | De la | Input | Cache | Output (include raționament) |
|---|---|---|---|---|
| `gemini-3.8-flash` | 2020-01-01 (aplicat retroactiv istoricului) | 0,75 | 0,075 | 3,75 |
| `gemini-3.8-flash` | 2027-01-01 | 1,50 | 0,15 | 7,50 |

Valorile sunt în USD per 1M tokeni.

Worker-ul loghează un warning la pornire dacă un model configurat nu are preț. Dacă cheia API e pe free tier, costul afișat e o estimare la prețul de listă, nu o sumă facturată.

## Date

- **Migrația `000038_llm_usage.sql`** e scrisă în raw SQL, fără Ent, după precedentul `accounting_analysis_runs`, `auth_*` și `legislation_*`. Tabelele se citesc doar prin agregări.
- **Operația și outcome-ul** au un CHECK de format (`^[A-Z][A-Z0-9_]{1,63}$`), nu o listă închisă, ca o operație nouă să nu poată pierde un apel facturat.
- **Backfill.** Rulările existente cu tokeni sunt importate ca `source='BACKFILL'`:
  - un rând per rulare;
  - thought și cached = 0;
  - costul se calculează cu prețul valabil la `COALESCE(completed_at, started_at)`;
  - modelele fără preț primesc `NO_PRICE`.

  UI-ul le marchează „istoric parțial”. Importul e idempotent: `NOT EXISTS` plus indecși unici parțiali pe `source='BACKFILL'`.
- **Catch-up după deploy.** Între aplicarea migrației și pornirea noului worker, rulările terminate de worker-ul vechi nu au evenimente. Se recuperează rulând din nou cele două `INSERT … SELECT` de backfill din 000038 (idempotente) cu `psql` pe baza respectivă.
- **Scripturile de curățare.** `scripts/clean-client-local.sh` și `scripts/clean-softco2-local.sh` șterg și `llm_usage_events` ale clientului.

## API

Toate rutele cer sesiune. Clienții neautorizați primesc 404.

| Rută | Parametri | Rol |
|---|---|---|
| `GET /api/v1/ai-usage` | `from`, `to` | totalul contului curent + defalcarea pe clienți (inclusiv clienții fără consum); nu acceptă un utilizator ca parametru |
| `GET /api/v1/clients/{clientId}/ai-usage` | `from`, `to` | totalul clientului, pe operație și pe model |
| `GET /api/v1/clients/{clientId}/ai-usage/runs` | `from`, `to`, `limit` (implicit 20, max 100), `offset` | rulările clientului, cele mai recente primele, cu starea și eticheta proprie (factura sau fișierul) |
| `GET /api/v1/clients/{clientId}/ai-usage/runs/{runKind}/{runId}` | — | toate apelurile rulării; `runKind` ∈ `accounting-analysis`, `contract-extraction` |

- **Perioada:**
  - zile calendaristice inclusive în `Europe/Bucharest`;
  - implicit luna curentă până azi;
  - maximum 366 de zile;
  - 400 `VALIDATION_ERROR` altfel.
- **Filtrarea e per apel.** O rulare care traversează granița perioadei apare parțial în ambele perioade.
- **Costul** e un număr JSON exact, `{"amount": 0.0071625, "currency": "USD"}`.

## UI

- **Pagina „Consum AI”** (`/ai-usage`, item nou în sidebar) arată:
  - totalul contului;
  - tabelul pe clienți, cu link către rulările clientului;
  - filtrul de perioadă: luna curentă, luna trecută sau un interval personalizat.
- **Cardul „Consum AI”** de pe pagina clientului arată:
  - totalul și defalcarea pe operație;
  - lista rulărilor, cu link la factură sau la documentul de contract, și butonul „Încarcă mai multe”;
  - pentru fiecare rulare, apelurile ei: rezultatul, tokenii (intrare, cache, ieșire, raționament) și costul.
- **Avertismente afișate:**
  - istoric parțial;
  - apeluri fără preț.
- **Formatare:** costurile sub un cent păstrează 3 cifre semnificative (de exemplu `0,00716 USD`).

## Limitări cunoscute

- Un timeout înainte de răspuns lasă un rând `TRANSPORT_ERROR` cu cost necunoscut, deși Google poate factura apelul.
- Semantica lui `total_cached_tokens` (inclus în input) și a lui `total_tokens` trebuie confirmată pe un răspuns live. `usage_detail` permite recalcularea ulterioară.
- Tier-ul de context lung (>200k), batch și flex nu sunt modelate. Coloana `pricing_tier` e pregătită, iar lookup-ul folosește `STANDARD`.
- Uneltele de dev `cmd/contractextract` și `cmd/contractdebug` nu înregistrează consum (`Noop`).
