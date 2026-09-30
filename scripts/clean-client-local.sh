#!/bin/sh
# Șterge datele operaționale ale unui client din baza locală Docker (facturi, task-uri, clasificări,
# analize AI, contracte, mapări învățate), păstrând clientul, profilul contabil, configurația SAGA
# și conexiunea SPV, ca setup-ul de test să poată fi rulat din nou. Doar pentru mediul local.
#
# Utilizare: scripts/clean-client-local.sh <CUI> "<NUME CLIENT>"
#   CONFIRM_CLIENT_DELETE=1 sare peste confirmarea interactivă.
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT_DIR"

if [ "$#" -ne 2 ]; then
  echo 'Utilizare: scripts/clean-client-local.sh <CUI> "<NUME CLIENT>"' >&2
  exit 2
fi
CLIENT_CUI=$(printf '%s' "$1" | sed 's/^[Rr][Oo]//')
CLIENT_NAME=$2

for command_name in docker date; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "Lipsește comanda necesară: $command_name" >&2
    exit 1
  fi
done

APP_ENV_VALUE=$(sed -n 's/^APP_ENV=//p' .env.local 2>/dev/null | tail -n 1)
case "$APP_ENV_VALUE" in
  development|test) ;;
  *)
    echo "Curățarea este permisă doar local (APP_ENV=development sau test în .env.local); găsit: '${APP_ENV_VALUE}'." >&2
    exit 1
    ;;
esac

psql_local() {
  docker compose exec -T postgres psql -U diana -d diana -v ON_ERROR_STOP=1 "$@"
}

echo '==> Pornesc/verific PostgreSQL local'
docker compose up -d --wait postgres

# ID-ul clientului se schimbă la fiecare make reset; îl identificăm după nume și CUI.
CLIENT_ID=$(psql_local -At -v cui="$CLIENT_CUI" -v name="$CLIENT_NAME" <<'SQL'
SELECT id FROM clients WHERE name = :'name' AND normalized_identifier = :'cui';
SQL
)

if [ -z "$CLIENT_ID" ] || [ "$(printf '%s\n' "$CLIENT_ID" | wc -l)" -ne 1 ]; then
  echo "Clientul exact $CLIENT_NAME / CUI $CLIENT_CUI nu a fost găsit. Nu s-a șters nimic." >&2
  exit 1
fi

COUNTS=$(psql_local -At -F '|' -v client_id="$CLIENT_ID" <<'SQL'
SELECT
  (SELECT count(*) FROM invoices WHERE client_id = :'client_id'),
  (SELECT count(*) FROM contracts WHERE client_id = :'client_id'),
  (SELECT count(*) FROM contract_source_documents WHERE client_id = :'client_id'),
  (SELECT count(*) FROM account_mappings WHERE client_id = :'client_id'),
  (SELECT count(*) FROM approved_accounting_knowledge WHERE client_id = :'client_id');
SQL
)

OLD_IFS=$IFS
IFS='|'
set -- $COUNTS
IFS=$OLD_IFS

echo "Vor fi eliminate pentru $CLIENT_NAME ($CLIENT_ID):"
echo "  facturi: $1"
echo "  contracte: $2"
echo "  documente-sursă contract: $3"
echo "  mapări contabile învățate: $4"
echo "  cunoștințe contabile aprobate: $5"
echo 'Rămân: clientul, profilul contabil, configurația SAGA și conexiunea SPV.'

if [ "${CONFIRM_CLIENT_DELETE:-}" != '1' ]; then
  printf 'Operația este ireversibilă după COMMIT. Tastează exact "STERGE %s": ' "$CLIENT_NAME"
  read -r answer
  if [ "$answer" != "STERGE $CLIENT_NAME" ]; then
    echo 'Curățare anulată. Nu s-a șters nimic.'
    exit 0
  fi
fi

BACKUP_DIR="$ROOT_DIR/backups/local-db"
mkdir -p "$BACKUP_DIR"
BACKUP_FILE="$BACKUP_DIR/diana-before-client-$CLIENT_CUI-cleanup-$(date +%Y%m%d-%H%M%S).sql"

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

if [ "$API_WAS_RUNNING" = '1' ] || [ "$WORKER_WAS_RUNNING" = '1' ]; then
  echo '==> Opresc temporar api și worker'
  docker compose stop api worker
fi

echo '==> Curăț datele clientului într-o singură tranzacție'
psql_local -P pager=off -v client_id="$CLIENT_ID" <<'SQL'
BEGIN;
-- Istoricul contabil (rulări AI, versiuni de mapare) este protejat de triggere BEFORE DELETE.
-- Numai pentru această tranzacție locală: triggerele și verificările FK sunt suspendate;
-- verificarea de la final confirmă că nu a rămas nimic legat de client.
SET LOCAL session_replication_role = replica;

CREATE TEMP TABLE _client ON COMMIT DROP AS SELECT id FROM clients WHERE id = :'client_id';
DO $$
BEGIN
  IF (SELECT count(*) FROM _client) <> 1 THEN
    RAISE EXCEPTION 'Clientul nu a fost identificat unic';
  END IF;
END $$;

CREATE TEMP TABLE _invoices ON COMMIT DROP AS SELECT id FROM invoices WHERE client_id IN (SELECT id FROM _client);
CREATE TEMP TABLE _tasks ON COMMIT DROP AS SELECT id FROM validation_tasks WHERE client_id IN (SELECT id FROM _client);
CREATE TEMP TABLE _contracts ON COMMIT DROP AS SELECT id FROM contracts WHERE client_id IN (SELECT id FROM _client);
CREATE TEMP TABLE _documents ON COMMIT DROP AS SELECT id FROM contract_source_documents WHERE client_id IN (SELECT id FROM _client);
CREATE TEMP TABLE _dossiers ON COMMIT DROP AS SELECT id FROM contract_dossiers WHERE client_id IN (SELECT id FROM _client);
CREATE TEMP TABLE _snapshots ON COMMIT DROP AS SELECT id FROM contract_commercial_snapshots WHERE dossier_id IN (SELECT id FROM _dossiers);
CREATE TEMP TABLE _clauses ON COMMIT DROP AS SELECT id FROM contract_clause_candidates WHERE dossier_id IN (SELECT id FROM _dossiers);
CREATE TEMP TABLE _commercial_runs ON COMMIT DROP AS SELECT id FROM invoice_commercial_validation_runs WHERE invoice_id IN (SELECT id FROM _invoices);
CREATE TEMP TABLE _findings ON COMMIT DROP AS SELECT id FROM invoice_commercial_findings WHERE run_id IN (SELECT id FROM _commercial_runs);
CREATE TEMP TABLE _classification_runs ON COMMIT DROP AS SELECT id FROM classification_runs WHERE client_id IN (SELECT id FROM _client);
CREATE TEMP TABLE _analyses ON COMMIT DROP AS SELECT id FROM accounting_analysis_runs WHERE client_id IN (SELECT id FROM _client);
CREATE TEMP TABLE _mappings ON COMMIT DROP AS SELECT id FROM account_mappings WHERE client_id IN (SELECT id FROM _client);
CREATE TEMP TABLE _knowledge ON COMMIT DROP AS SELECT id FROM approved_accounting_knowledge WHERE client_id IN (SELECT id FROM _client);
CREATE TEMP TABLE _aggregates ON COMMIT DROP AS
  SELECT id FROM _invoices UNION SELECT id FROM _tasks UNION SELECT id FROM _contracts UNION SELECT id FROM _documents
  UNION SELECT id FROM _dossiers UNION SELECT id FROM _classification_runs UNION SELECT id FROM _analyses
  UNION SELECT id FROM _mappings UNION SELECT id FROM _knowledge;

-- Validare comercială și dosare contractuale
DELETE FROM invoice_commercial_overrides WHERE finding_id IN (SELECT id FROM _findings);
DELETE FROM invoice_commercial_findings WHERE id IN (SELECT id FROM _findings);
DELETE FROM commercial_review_commands WHERE client_id IN (SELECT id FROM _client) OR invoice_id IN (SELECT id FROM _invoices);
DELETE FROM contract_commercial_ledger WHERE invoice_id IN (SELECT id FROM _invoices) OR dossier_id IN (SELECT id FROM _dossiers);
DELETE FROM invoice_commercial_validation_runs WHERE id IN (SELECT id FROM _commercial_runs);
DELETE FROM invoice_commercial_date_facts WHERE invoice_id IN (SELECT id FROM _invoices);
DELETE FROM contract_snapshot_sources WHERE snapshot_id IN (SELECT id FROM _snapshots) OR document_id IN (SELECT id FROM _documents) OR clause_id IN (SELECT id FROM _clauses);
DELETE FROM contract_variable_values WHERE definition_id IN (SELECT id FROM contract_variable_definitions WHERE dossier_id IN (SELECT id FROM _dossiers));
DELETE FROM contract_variable_definitions WHERE dossier_id IN (SELECT id FROM _dossiers);
DELETE FROM contract_clause_candidates WHERE id IN (SELECT id FROM _clauses);
DELETE FROM contract_commercial_snapshots WHERE id IN (SELECT id FROM _snapshots);
DELETE FROM contract_service_aliases WHERE client_id IN (SELECT id FROM _client);
DELETE FROM invoice_contract_associations WHERE client_id IN (SELECT id FROM _client);
DELETE FROM contract_match_candidates WHERE client_id IN (SELECT id FROM _client);
DELETE FROM contract_match_runs WHERE client_id IN (SELECT id FROM _client);

-- Clasificare, analiză AI și învățare
DELETE FROM account_mapping_versions WHERE mapping_id IN (SELECT id FROM _mappings);
DELETE FROM account_mappings WHERE id IN (SELECT id FROM _mappings);
DELETE FROM approved_accounting_knowledge WHERE id IN (SELECT id FROM _knowledge);
DELETE FROM llm_usage_events WHERE client_id IN (SELECT id FROM _client);
DELETE FROM accounting_analysis_reviews WHERE client_id IN (SELECT id FROM _client) OR analysis_id IN (SELECT id FROM _analyses);
DELETE FROM accounting_analysis_runs WHERE id IN (SELECT id FROM _analyses);
DELETE FROM classification_reanalysis_commands WHERE client_id IN (SELECT id FROM _client);
DELETE FROM line_classifications WHERE client_id IN (SELECT id FROM _client) OR invoice_id IN (SELECT id FROM _invoices);
DELETE FROM classification_runs WHERE id IN (SELECT id FROM _classification_runs);

-- Istoric, task-uri, outbox, SPV, SAGA
DELETE FROM activity_events
WHERE invoice_id IN (SELECT id FROM _invoices)
   OR validation_task_id IN (SELECT id FROM _tasks)
   OR (client_id IN (SELECT id FROM _client) AND aggregate_id IN (SELECT id FROM _aggregates));
DELETE FROM validation_tasks WHERE id IN (SELECT id FROM _tasks);
DELETE FROM outbox_entries WHERE aggregate_id IN (SELECT id FROM _aggregates);
DELETE FROM saga_export_attempts WHERE client_id IN (SELECT id FROM _client) OR invoice_id IN (SELECT id FROM _invoices);
DELETE FROM spv_source_documents WHERE client_id IN (SELECT id FROM _client) OR invoice_id IN (SELECT id FROM _invoices);

-- Facturi și contracte
DELETE FROM invoice_lines WHERE invoice_id IN (SELECT id FROM _invoices);
DELETE FROM invoices WHERE id IN (SELECT id FROM _invoices);
DELETE FROM contract_extraction_attempts WHERE document_id IN (SELECT id FROM _documents);
DELETE FROM contract_source_documents WHERE id IN (SELECT id FROM _documents);
DELETE FROM contract_service_terms WHERE contract_id IN (SELECT id FROM _contracts);
DELETE FROM contract_dossiers WHERE id IN (SELECT id FROM _dossiers);
DELETE FROM contracts WHERE id IN (SELECT id FROM _contracts);

-- Verificare dinamică: nicio tabelă nu mai are rânduri legate de client sau de facturile lui,
-- în afară de configurația păstrată intenționat. Altfel tranzacția este anulată.
DO $$
DECLARE
  item record;
  remaining bigint;
BEGIN
  FOR item IN
    SELECT c.table_name, c.column_name
    FROM information_schema.columns c
    JOIN information_schema.tables t ON t.table_schema = c.table_schema AND t.table_name = c.table_name AND t.table_type = 'BASE TABLE'
    WHERE c.table_schema = 'public' AND c.column_name IN ('client_id', 'invoice_id')
  LOOP
    IF item.column_name = 'client_id' AND item.table_name IN (
      'client_accounting_profiles', 'client_saga_configurations', 'client_management_commands',
      'spv_connections', 'spv_oauth_states', 'auth_user_client_grants', 'classification_rules',
      'accounting_rule_packs', 'activity_events'
    ) THEN
      CONTINUE;
    END IF;
    IF item.column_name = 'client_id' THEN
      EXECUTE format('SELECT count(*) FROM %I WHERE %I::text IN (SELECT id FROM _client)', item.table_name, item.column_name) INTO remaining;
    ELSE
      EXECUTE format('SELECT count(*) FROM %I WHERE %I::text IN (SELECT id FROM _invoices)', item.table_name, item.column_name) INTO remaining;
    END IF;
    IF remaining > 0 THEN
      RAISE EXCEPTION 'Au rămas % rânduri în %.% pentru client; nu s-a șters nimic.', remaining, item.table_name, item.column_name;
    END IF;
  END LOOP;
END $$;

COMMIT;
SQL

RESULT=$(psql_local -At -F '|' -v client_id="$CLIENT_ID" <<'SQL'
SELECT
  (SELECT count(*) FROM invoices WHERE client_id = :'client_id'),
  (SELECT count(*) FROM contracts WHERE client_id = :'client_id'),
  (SELECT count(*) FROM contract_source_documents WHERE client_id = :'client_id'),
  (SELECT count(*) FROM clients WHERE id = :'client_id');
SQL
)

if [ "$RESULT" != '0|0|0|1' ]; then
  echo "Curățarea s-a încheiat, dar verificarea a returnat: $RESULT" >&2
  echo "Backup disponibil la: $BACKUP_FILE" >&2
  exit 1
fi

echo '==> Repornesc serviciile care rulau înainte'
restart_services
API_WAS_RUNNING=0
WORKER_WAS_RUNNING=0

echo "Curățarea clientului $CLIENT_NAME s-a încheiat cu succes."
echo "Backup: $BACKUP_FILE"
