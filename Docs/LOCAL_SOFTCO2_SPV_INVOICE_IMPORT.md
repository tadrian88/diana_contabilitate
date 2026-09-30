# Import local factură ANAF pentru SOFTCO2

Procedura reia importul arhivei ANAF reale `8253499374.zip` pentru clientul local
SOFTCO2 SRL, fără conectare la serviciile ANAF reale. Factura intră prin fluxul
aplicației: document SPV -> parser UBL -> factură -> outbox -> worker -> matching
contract -> clasificare.

## Date folosite

- client Diana: ID-ul se schimbă la fiecare `make reset`; setează `CLIENT_ID` (vezi mai jos)
- CUI client: `49678244`
- arhivă: `/Users/adriantudoran/Downloads/8253499374.zip`
- ID mesaj SPV simulat: `8253499374`
- factură așteptată: `FCO nr. 0878`, total `605 RON`

Comenzile presupun că stack-ul local rulează și că `.env.local` are
`APP_ENV=development`.

> Pașii 2–4 sunt automatizați de `bash test-data/softco2/setup-softco2.sh anaf` și
> `... facturi` (vezi `test-data/softco2/README.md`). Procedura manuală de mai jos rămâne
> pentru depanare.

În fiecare terminal folosit mai jos, setează mai întâi ID-ul clientului:

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate
CLIENT_ID=$(docker compose exec -T postgres psql -U diana -d diana -tAc \
  "SELECT id FROM clients WHERE normalized_identifier = '49678244'")
echo "$CLIENT_ID"
```

## 1. Verifică serviciile locale

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate
docker compose ps
```

Serviciile `postgres`, `redis`, `api` și `worker` trebuie să fie pornite; API și
worker trebuie să fie healthy.

## 2. Pornește simulatorul ANAF

Rulează într-un terminal separat și lasă procesul pornit până după pasul 3:

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
GOCACHE=/private/tmp/diana-go-cache \
go run ./cmd/fakeanaf -buyer-cui RO49678244
```

Simulatorul ascultă pe `127.0.0.1:8090`. Dacă portul este deja ocupat, verifică
mai întâi dacă simulatorul rulează deja; nu porni două instanțe.

## 3. Creează sau reactivează conexiunea SPV simulată

Rulează într-un al doilea terminal:

> Copiază comenzile direct din blocul de cod. Nu copia variante randate de chat
> care transformă URL-urile în `[http://...](http://...)` sau adaugă `\` înaintea
> caracterelor `_` și `--`.

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate

set -a
source .env.local
set +a

export DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable'
export SPV_ENVIRONMENT=TEST
export SPV_TOKEN_URL='http://127.0.0.1:8090/token'
export SPV_OAUTH_CLIENT_ID='fake-app'
export SPV_OAUTH_CLIENT_SECRET='fake-secret'
export SPV_TOKEN_ENCRYPTION_KEY='0101010101010101010101010101010101010101010101010101010101010101'

cd backend
GOCACHE=/private/tmp/diana-go-cache \
go run ./cmd/spvconnect exchange \
  --accounting-client-id "$CLIENT_ID" \
  --code fake-authorization-code \
  --redirect-uri http://127.0.0.1:8080/api/v1/integrations/anaf/callback
```

Comanda afișează ID-ul conexiunii. Oprește apoi simulatorul din primul terminal cu
`Ctrl+C`. Nu apăsa `Sincronizează acum` în UI: providerul simulat nu mai rulează,
iar factura va fi introdusă explicit la pasul următor.

Verifică faptul că legătura este activă și tokenul încă este valid:

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate
docker compose exec -T postgres psql -U diana -d diana -P pager=off -c \
"SELECT id, cif, environment, status, access_token_expires_at
 FROM spv_connections
 WHERE client_id = '$CLIENT_ID';"
```

Rezultatul necesar este `status = ACTIVE`. Tokenul simulat expiră după aproximativ
o oră; dacă este `EXPIRED`, repetă pașii 2 și 3.

## 4. Importă ZIP-ul original

În același terminal în care variabilele de la pasul 3 sunt încă exportate:

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
GOCACHE=/private/tmp/diana-go-cache \
go run ./cmd/spvconnect import-fixture \
  --accounting-client-id "$CLIENT_ID" \
  --zip /Users/adriantudoran/Downloads/8253499374.zip \
  --external-message-id 8253499374
```

Un import nou trebuie să afișeze `created=true` și identificatorii documentului
sursă și facturii. Workerul continuă automat procesarea prin outbox.

## 5. Confirmă idempotency

Repetă exact comanda de import:

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate/backend
GOCACHE=/private/tmp/diana-go-cache \
go run ./cmd/spvconnect import-fixture \
  --accounting-client-id "$CLIENT_ID" \
  --zip /Users/adriantudoran/Downloads/8253499374.zip \
  --external-message-id 8253499374
```

A doua execuție trebuie să returneze aceleași ID-uri și `created=false`, fără o a
doua factură.

## 6. Inspectează rezultatul

Așteaptă câteva secunde pentru worker, apoi rulează diagnosticul scurt:

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate
docker compose exec -T postgres psql -U diana -d diana -P pager=off \
  -c "SELECT id, external_message_id, processing_status, failure_kind, attempts,
             octet_length(raw_document) AS zip_bytes, content_sha256,
             parser_type, parser_version, invoice_id, last_error
      FROM spv_source_documents
      WHERE client_id = '$CLIENT_ID'
      ORDER BY discovered_at DESC LIMIT 5;" \
  -c "SELECT id, document_number, supplier_name, supplier_cui, total_amount,
             currency, model_version, pipeline_status, saga_status,
             readiness_reason, revision
      FROM invoices
      WHERE client_id = '$CLIENT_ID'
      ORDER BY created_at DESC;" \
  -c "SELECT i.document_number, l.position, l.description, l.quantity,
             l.unit_price, l.net_value, l.vat_rate, l.vat_value, l.total_value
      FROM invoice_lines l
      JOIN invoices i ON i.id = l.invoice_id
      WHERE i.client_id = '$CLIENT_ID'
      ORDER BY i.created_at DESC, l.position;" \
  -c "SELECT a.invoice_id, a.contract_id, a.association_kind,
             a.policy_version, a.contract_reference
      FROM invoice_contract_associations a
      WHERE a.client_id = '$CLIENT_ID';" \
  -c "SELECT invoice_id, task_type, status, blocker_code, reason, revision
      FROM validation_tasks
      WHERE client_id = '$CLIENT_ID'
      ORDER BY created_at;" \
  -c "SELECT invoice_id, dimension, proposed_value, review_status, source,
             policy_version, required_review
      FROM line_classifications
      WHERE client_id = '$CLIENT_ID'
      ORDER BY invoice_id, dimension;" \
  -c "SELECT aggregate_id, status, event_type, attempts, last_error
      FROM outbox_entries
      WHERE aggregate_id IN (
        SELECT id FROM invoices
        WHERE client_id = '$CLIENT_ID'
      )
      ORDER BY created_at;"
```

Pentru eventuale erori tehnice recente ale workerului:

```sh
cd /Users/adriantudoran/Projects/diana_contabilitate
docker compose logs --tail=150 worker
```

## Rezultatul funcțional așteptat

- documentul sursă ajunge în `PROCESSED`, fără `failure_kind` sau `last_error`;
- factura `FCO nr. 0878` este creată o singură dată;
- contractul `102/25.06.2025` este asociat automat;
- factura ajunge în mod normal la `AWAITING_REVIEW` dacă încă lipsesc deciziile
  contabile aplicabile;
- task-ul de clasificare conține deciziile care necesită validarea contabilului;
- nu continua manual clasificarea și nu genera export SAGA dacă scopul testului este
  observarea primului punct natural de oprire.

## Reluare după o execuție parțială

Comanda `import-fixture` este idempotentă pentru perechea conexiune SPV +
`external-message-id`. Dacă documentul este deja `PROCESSED`, returnează factura
existentă. Nu schimba ID-ul mesajului doar pentru a forța un nou import: aceasta ar
simula o livrare SPV distinctă și poate declanșa logica de duplicat comercial.

ZIP-ul local nu include răspunsul endpointului ANAF de listare. Pentru testarea
termenelor calculate de la remiterea facturii, `import-fixture` salvează explicit
o dată SPV simulată egală cu data emiterii din XML și o marchează
`LOCAL_FIXTURE_FROM_INVOICE_ISSUE_DATE`. Substituția este limitată la
development/test. În producție se folosește exclusiv `data_creare` din mesajul
ANAF; o valoare lipsă sau invalidă nu este înlocuită cu data emiterii sau cu data
importului. Repetarea comenzii pentru o factură locală mai veche completează
metadada simulată lipsă fără să creeze o factură nouă. Până la această
completare, validarea unei livrări vechi asociate unei conexiuni `TEST` folosește
aceeași dată din factură și afișează explicit că sursa este un fixture local;
această rezervă nu este activă pentru conexiunile `PRODUCTION`.
