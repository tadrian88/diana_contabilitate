#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT_DIR"

CLIENT_ID='client-fcb7f867c09dad08a3fe65e1'
CLIENT_NAME='SOFTCO2 SRL'
CLIENT_CUI='49678244'

for command_name in docker date; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "Lipsește comanda necesară: $command_name" >&2
    exit 1
  fi
done

echo '==> Pornesc/verific PostgreSQL local'
docker compose up -d --wait postgres

CLIENT_ROW=$(docker compose exec -T postgres psql -U diana -d diana -At -F '|' -v ON_ERROR_STOP=1 \
  -c "SELECT id, name, cui FROM clients WHERE id='$CLIENT_ID' AND name='$CLIENT_NAME' AND cui='$CLIENT_CUI';")

if [ "$CLIENT_ROW" != "$CLIENT_ID|$CLIENT_NAME|$CLIENT_CUI" ]; then
  echo "Clientul exact $CLIENT_NAME / CUI $CLIENT_CUI nu a fost găsit. Nu s-a șters nimic." >&2
  exit 1
fi

COUNTS=$(docker compose exec -T postgres psql -U diana -d diana -At -F '|' -v ON_ERROR_STOP=1 \
  -c "SELECT
        (SELECT count(*) FROM invoices WHERE client_id='$CLIENT_ID'),
        (SELECT count(*) FROM contracts WHERE client_id='$CLIENT_ID'),
        (SELECT count(*) FROM contract_source_documents WHERE client_id='$CLIENT_ID'),
        (SELECT count(*) FROM account_mappings WHERE client_id='$CLIENT_ID');")

OLD_IFS=$IFS
IFS='|'
set -- $COUNTS
IFS=$OLD_IFS
INVOICE_COUNT=$1
CONTRACT_COUNT=$2
DOCUMENT_COUNT=$3
MAPPING_COUNT=$4

echo "Vor fi eliminate pentru $CLIENT_NAME:"
echo "  facturi: $INVOICE_COUNT"
echo "  contracte: $CONTRACT_COUNT"
echo "  documente-sursă contract: $DOCUMENT_COUNT"

if [ "$MAPPING_COUNT" != '0' ]; then
  echo "Există $MAPPING_COUNT mapări contabile învățate pentru client." >&2
  echo 'Scriptul se oprește pentru a nu șterge accidental politici contabile.' >&2
  exit 1
fi

if [ "${CONFIRM_SOFTCO2_DELETE:-}" != '1' ]; then
  printf 'Operația este ireversibilă după COMMIT. Tastează exact "STERGE SOFTCO2": '
  read -r answer
  if [ "$answer" != 'STERGE SOFTCO2' ]; then
    echo 'Curățare anulată. Nu s-a șters nimic.'
    exit 0
  fi
fi

BACKUP_DIR="$ROOT_DIR/backups/local-db"
mkdir -p "$BACKUP_DIR"
BACKUP_FILE="$BACKUP_DIR/diana-before-softco2-cleanup-$(date +%Y%m%d-%H%M%S).sql"

echo "==> Creez backup: $BACKUP_FILE"
docker compose exec -T postgres pg_dump -U diana -d diana > "$BACKUP_FILE"

RUNNING_SERVICES=" $(docker compose ps --status running --services | tr '\n' ' ') "
API_WAS_RUNNING=0
WORKER_WAS_RUNNING=0

case "$RUNNING_SERVICES" in
  *' api '*) API_WAS_RUNNING=1 ;;
esac
case "$RUNNING_SERVICES" in
  *' worker '*) WORKER_WAS_RUNNING=1 ;;
esac

restart_services() {
  if [ "$API_WAS_RUNNING" = '1' ] && [ "$WORKER_WAS_RUNNING" = '1' ]; then
    docker compose start api worker >/dev/null || true
  elif [ "$API_WAS_RUNNING" = '1' ]; then
    docker compose start api >/dev/null || true
  elif [ "$WORKER_WAS_RUNNING" = '1' ]; then
    docker compose start worker >/dev/null || true
  fi
}
trap restart_services EXIT

if [ "$API_WAS_RUNNING" = '1' ] && [ "$WORKER_WAS_RUNNING" = '1' ]; then
  echo '==> Opresc temporar api și worker'
  docker compose stop api worker
elif [ "$API_WAS_RUNNING" = '1' ]; then
  echo '==> Opresc temporar api'
  docker compose stop api
elif [ "$WORKER_WAS_RUNNING" = '1' ]; then
  echo '==> Opresc temporar worker'
  docker compose stop worker
fi

echo '==> Curăț datele SOFTCO2 într-o singură tranzacție'
docker compose exec -T postgres psql -U diana -d diana -v ON_ERROR_STOP=1 -P pager=off <<'SQL'
BEGIN;

CREATE TEMP TABLE _client ON COMMIT DROP AS
SELECT id FROM clients
WHERE id = 'client-fcb7f867c09dad08a3fe65e1'
  AND name = 'SOFTCO2 SRL'
  AND cui = '49678244';

DO $$
BEGIN
  IF (SELECT count(*) FROM _client) <> 1 THEN
    RAISE EXCEPTION 'Clientul SOFTCO2 SRL / CUI 49678244 nu a fost identificat unic';
  END IF;
END $$;

CREATE TEMP TABLE _invoices ON COMMIT DROP AS SELECT id FROM invoices WHERE client_id IN (SELECT id FROM _client);
CREATE TEMP TABLE _lines ON COMMIT DROP AS SELECT id FROM invoice_lines WHERE invoice_id IN (SELECT id FROM _invoices);
CREATE TEMP TABLE _tasks ON COMMIT DROP AS SELECT id FROM validation_tasks WHERE client_id IN (SELECT id FROM _client);
CREATE TEMP TABLE _match_runs ON COMMIT DROP AS SELECT id FROM contract_match_runs WHERE client_id IN (SELECT id FROM _client);
CREATE TEMP TABLE _contracts ON COMMIT DROP AS SELECT id FROM contracts WHERE client_id IN (SELECT id FROM _client);
CREATE TEMP TABLE _documents ON COMMIT DROP AS SELECT id FROM contract_source_documents WHERE client_id IN (SELECT id FROM _client);
CREATE TEMP TABLE _dossiers ON COMMIT DROP AS SELECT id FROM contract_dossiers WHERE client_id IN (SELECT id FROM _client);
CREATE TEMP TABLE _snapshots ON COMMIT DROP AS SELECT id FROM contract_commercial_snapshots WHERE dossier_id IN (SELECT id FROM _dossiers);
CREATE TEMP TABLE _clauses ON COMMIT DROP AS SELECT id FROM contract_clause_candidates WHERE dossier_id IN (SELECT id FROM _dossiers);
CREATE TEMP TABLE _commercial_runs ON COMMIT DROP AS SELECT id FROM invoice_commercial_validation_runs WHERE invoice_id IN (SELECT id FROM _invoices);
CREATE TEMP TABLE _findings ON COMMIT DROP AS SELECT id FROM invoice_commercial_findings WHERE run_id IN (SELECT id FROM _commercial_runs);

DELETE FROM invoice_commercial_overrides WHERE finding_id IN (SELECT id FROM _findings);
DELETE FROM invoice_commercial_findings WHERE id IN (SELECT id FROM _findings);
DELETE FROM commercial_review_commands WHERE invoice_id IN (SELECT id FROM _invoices);
DELETE FROM contract_commercial_ledger WHERE invoice_id IN (SELECT id FROM _invoices) OR dossier_id IN (SELECT id FROM _dossiers);
DELETE FROM invoice_commercial_validation_runs WHERE id IN (SELECT id FROM _commercial_runs);
DELETE FROM contract_snapshot_sources WHERE snapshot_id IN (SELECT id FROM _snapshots) OR document_id IN (SELECT id FROM _documents) OR clause_id IN (SELECT id FROM _clauses);
DELETE FROM contract_variable_values WHERE definition_id IN (SELECT id FROM contract_variable_definitions WHERE dossier_id IN (SELECT id FROM _dossiers));
DELETE FROM contract_variable_definitions WHERE dossier_id IN (SELECT id FROM _dossiers);
DELETE FROM contract_clause_candidates WHERE id IN (SELECT id FROM _clauses);
UPDATE contract_dossiers SET active_snapshot_id = NULL WHERE id IN (SELECT id FROM _dossiers);
DELETE FROM contract_commercial_snapshots WHERE id IN (SELECT id FROM _snapshots);
DELETE FROM contract_service_aliases WHERE client_id IN (SELECT id FROM _client);
DELETE FROM invoice_contract_associations WHERE client_id IN (SELECT id FROM _client);
DELETE FROM contract_match_candidates WHERE client_id IN (SELECT id FROM _client);
DELETE FROM activity_events
WHERE invoice_id IN (SELECT id FROM _invoices)
   OR validation_task_id IN (SELECT id FROM _tasks)
   OR (client_id IN (SELECT id FROM _client) AND aggregate_id IN (
       SELECT id FROM _invoices UNION SELECT id FROM _contracts UNION SELECT id FROM _documents UNION SELECT id FROM _dossiers
   ));
DELETE FROM validation_tasks WHERE id IN (SELECT id FROM _tasks);
DELETE FROM contract_match_runs WHERE id IN (SELECT id FROM _match_runs);
DELETE FROM saga_export_attempts WHERE invoice_id IN (SELECT id FROM _invoices);
DELETE FROM spv_source_documents WHERE invoice_id IN (SELECT id FROM _invoices);
DELETE FROM outbox_entries WHERE aggregate_id IN (
  SELECT id FROM _invoices UNION SELECT id FROM _contracts UNION SELECT id FROM _documents UNION SELECT id FROM _dossiers
);
DELETE FROM line_classifications WHERE invoice_id IN (SELECT id FROM _invoices);
DELETE FROM invoice_lines WHERE id IN (SELECT id FROM _lines);
DELETE FROM invoices WHERE id IN (SELECT id FROM _invoices);
DELETE FROM contract_extraction_attempts WHERE document_id IN (SELECT id FROM _documents);
DELETE FROM contract_source_documents WHERE id IN (SELECT id FROM _documents);
DELETE FROM contract_service_terms WHERE contract_id IN (SELECT id FROM _contracts);
DELETE FROM contract_dossiers WHERE id IN (SELECT id FROM _dossiers);
DELETE FROM contracts WHERE id IN (SELECT id FROM _contracts);

COMMIT;
SQL

RESULT=$(docker compose exec -T postgres psql -U diana -d diana -At -F '|' -v ON_ERROR_STOP=1 \
  -c "SELECT
        (SELECT count(*) FROM invoices WHERE client_id='$CLIENT_ID'),
        (SELECT count(*) FROM contracts WHERE client_id='$CLIENT_ID'),
        (SELECT count(*) FROM contract_source_documents WHERE client_id='$CLIENT_ID');")

if [ "$RESULT" != '0|0|0' ]; then
  echo "Curățarea s-a încheiat, dar verificarea a returnat: $RESULT" >&2
  echo "Backup disponibil la: $BACKUP_FILE" >&2
  exit 1
fi

echo '==> Repornesc serviciile care rulau înainte'
restart_services
API_WAS_RUNNING=0
WORKER_WAS_RUNNING=0

echo 'Curățarea SOFTCO2 s-a încheiat cu succes.'
echo "Backup: $BACKUP_FILE"
echo 'Au rămas 0 facturi, 0 contracte și 0 documente-sursă pentru SOFTCO2 SRL.'
