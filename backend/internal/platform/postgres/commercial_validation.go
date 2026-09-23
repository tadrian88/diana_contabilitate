package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/commercialvalidation"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/spv"
)

var contractReferencePattern = regexp.MustCompile(`(?i)(?:(?:ctr\.?|contract(?:ul)?)\s*(?:nr\.?\s*)?|nr\.?\s*)[:#-]?\s*([A-Z0-9_-]+(?:\s*/\s*[0-9.]+)?)`)

func (s *Store) LoadInput(ctx context.Context, invoiceID string) (commercialvalidation.Input, error) {
	item, err := s.GetInvoice(ctx, invoiceID)
	if err != nil {
		return commercialvalidation.Input{}, err
	}
	// Older imports stored only Item/Name and discarded Item/Description. Read
	// the immutable archived SPV payload so those invoices benefit from the
	// corrected parser without destructive reimport or mutation.
	s.hydrateArchivedUBLText(ctx, item)
	input := commercialvalidation.Input{Invoice: *item, Variables: map[string]commercialvalidation.VariableValue{}, UnavailableVariables: map[string]commercialvalidation.VariableValue{}}
	input.Reference, input.ReferenceSource = invoiceContractReference(*item)
	var snapshot commercialvalidation.Snapshot
	var rules []byte
	var effectiveTo sql.NullTime
	err = s.DB.QueryRowContext(ctx, `
		SELECT s.id,s.dossier_id,s.contract_id,s.version,s.schema_version,s.coverage,
		       s.effective_from,s.effective_to,s.rules,s.confirmed_by_id,s.confirmed_at
		FROM invoice_contract_associations a
		JOIN contract_dossiers d ON d.contract_id=a.contract_id
		JOIN contract_commercial_snapshots s ON s.id=d.active_snapshot_id
		WHERE a.invoice_id=$1
		LIMIT 1`, invoiceID).
		Scan(&snapshot.ID, &snapshot.DossierID, &snapshot.ContractID, &snapshot.Version, &snapshot.SchemaVersion, &snapshot.Coverage,
			&snapshot.EffectiveFrom, &effectiveTo, &rules, &snapshot.ConfirmedByID, &snapshot.ConfirmedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return input, commercialvalidation.ErrNoSnapshot
	}
	if err != nil {
		return commercialvalidation.Input{}, fmt.Errorf("load commercial snapshot: %w", err)
	}
	if effectiveTo.Valid {
		snapshot.EffectiveTo = &effectiveTo.Time
	}
	if err = json.Unmarshal(rules, &snapshot.Rules); err != nil {
		return commercialvalidation.Input{}, fmt.Errorf("decode commercial rules: %w", err)
	}
	input.Snapshot = snapshot
	rows, err := s.DB.QueryContext(ctx, `
		SELECT d.name,v.value,v.source,COALESCE(v.source_reference,''),v.period_start,v.period_end,
		       v.recorded_by_id,v.recorded_at,v.applies
		FROM contract_variable_definitions d
		JOIN LATERAL (
		  SELECT candidate.*,
		         ((candidate.period_start IS NULL OR candidate.period_start <= $2)
		          AND (candidate.period_end IS NULL OR candidate.period_end >= $2)) AS applies
		  FROM contract_variable_values candidate
		  WHERE candidate.definition_id=d.id
		  ORDER BY ((candidate.period_start IS NULL OR candidate.period_start <= $2)
		            AND (candidate.period_end IS NULL OR candidate.period_end >= $2)) DESC,
		           candidate.recorded_at DESC LIMIT 1
		) v ON true WHERE d.dossier_id=$1`, snapshot.DossierID, item.IssueDay)
	if err != nil {
		return commercialvalidation.Input{}, err
	}
	for rows.Next() {
		var value commercialvalidation.VariableValue
		var start, end sql.NullTime
		var applies bool
		if err = rows.Scan(&value.Name, &value.Value, &value.Source, &value.SourceReference, &start, &end, &value.RecordedByID, &value.RecordedAt, &applies); err != nil {
			rows.Close()
			return commercialvalidation.Input{}, err
		}
		if start.Valid {
			value.PeriodStart = &start.Time
		}
		if end.Valid {
			value.PeriodEnd = &end.Time
		}
		if applies {
			input.Variables[value.Name] = value
		} else {
			input.UnavailableVariables[value.Name] = value
		}
	}
	if err = rows.Close(); err != nil {
		return commercialvalidation.Input{}, err
	}
	aliasRows, err := s.DB.QueryContext(ctx, `
		SELECT id,client_id,supplier_cui,service_id,normalized_label,effective_from,effective_to,confirmed_by_id,confirmed_at
		FROM contract_service_aliases
		WHERE client_id=$1 AND supplier_cui=$2 AND dossier_id=$4
		  AND (invoice_id=$5 OR (invoice_id IS NULL AND NOT EXISTS (
		    SELECT 1 FROM contract_service_aliases specific
		    WHERE specific.dossier_id=contract_service_aliases.dossier_id
		      AND specific.invoice_id=$5 AND specific.normalized_label=contract_service_aliases.normalized_label)))
		  AND (effective_from IS NULL OR effective_from <= $3)
		  AND (effective_to IS NULL OR effective_to >= $3)`, item.ClientID, valueOrBlank(item.NormalizedSupplierCUI), item.IssueDay, snapshot.DossierID, invoiceID)
	if err != nil {
		return commercialvalidation.Input{}, err
	}
	for aliasRows.Next() {
		var alias commercialvalidation.Alias
		var from, to sql.NullTime
		if err = aliasRows.Scan(&alias.ID, &alias.ClientID, &alias.SupplierCUI, &alias.ServiceID, &alias.NormalizedLabel, &from, &to, &alias.ConfirmedByID, &alias.ConfirmedAt); err != nil {
			aliasRows.Close()
			return commercialvalidation.Input{}, err
		}
		if from.Valid {
			alias.EffectiveFrom = &from.Time
		}
		if to.Valid {
			alias.EffectiveTo = &to.Time
		}
		input.Aliases = append(input.Aliases, alias)
	}
	if err = aliasRows.Close(); err != nil {
		return commercialvalidation.Input{}, err
	}
	dateRows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT ON (kind) kind,occurred_on,source_reference
		FROM invoice_commercial_date_facts WHERE invoice_id=$1 ORDER BY kind,recorded_at DESC,id DESC`, invoiceID)
	if err != nil {
		return commercialvalidation.Input{}, err
	}
	for dateRows.Next() {
		var kind, sourceReference string
		var occurred time.Time
		if err = dateRows.Scan(&kind, &occurred, &sourceReference); err != nil {
			dateRows.Close()
			return commercialvalidation.Input{}, err
		}
		switch kind {
		case "REMITTANCE":
			input.RemittanceDate = &occurred
			input.RemittanceSource = sourceReference
		case "RECEIPT":
			input.ReceiptDate = &occurred
		case "ACCEPTANCE":
			input.AcceptanceDate = &occurred
		}
	}
	if err = dateRows.Close(); err != nil {
		return commercialvalidation.Input{}, err
	}
	{
		var createdAt sql.NullTime
		var createdRaw, messageID sql.NullString
		var spvEnvironment string
		sourceErr := s.DB.QueryRowContext(ctx, `SELECT doc.source_created_at,doc.source_created_raw,doc.external_message_id,connection.environment
			FROM spv_source_documents doc JOIN spv_connections connection ON connection.id=doc.connection_id
			WHERE doc.invoice_id=$1 LIMIT 1`, invoiceID).Scan(&createdAt, &createdRaw, &messageID, &spvEnvironment)
		if sourceErr != nil && !errors.Is(sourceErr, sql.ErrNoRows) {
			return commercialvalidation.Input{}, sourceErr
		}
		var sourceTime *time.Time
		if createdAt.Valid {
			sourceTime = &createdAt.Time
		} else if createdRaw.Valid {
			sourceTime, _ = spv.ParseMessageCreatedAt(createdRaw.String)
		}
		if sourceTime != nil {
			day := spv.RomanianCalendarDay(*sourceTime)
			input.RemittanceDate = &day
			input.RemittanceSource = "ANAF SPV · data_creare"
			if messageID.Valid {
				input.RemittanceSource += " · mesaj " + messageID.String
			}
		} else if spvEnvironment == "TEST" {
			// Old local fixtures predate capture of ANAF data_creare. The fallback
			// is intentionally confined to TEST connections and visibly labelled;
			// production must never equate issue and remittance dates.
			day := time.Date(item.IssueDay.Year(), item.IssueDay.Month(), item.IssueDay.Day(), 0, 0, 0, 0, time.UTC)
			input.RemittanceDate = &day
			input.RemittanceSource = "Fixture SPV local · data emiterii folosită numai pentru test"
		}
	}
	if item.DocumentType == invoicing.DocumentTypeCreditNote && item.SourceFacts != nil && strings.TrimSpace(item.SourceFacts.PrecedingInvoice) != "" {
		var originalID string
		err = s.DB.QueryRowContext(ctx, `SELECT id FROM invoices WHERE client_id=$1 AND normalized_supplier_cui=$2 AND normalized_document_number=$3 AND document_type='INVOICE' ORDER BY issue_day DESC LIMIT 1`, item.ClientID, valueOrBlank(item.NormalizedSupplierCUI), invoicing.NormalizeBusinessIdentifier(item.SourceFacts.PrecedingInvoice)).Scan(&originalID)
		if err == nil {
			input.Original, err = s.GetInvoice(ctx, originalID)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return commercialvalidation.Input{}, err
		}
	}
	return input, nil
}

func (s *Store) hydrateArchivedUBLText(ctx context.Context, invoice *invoicing.Invoice) {
	if invoice == nil {
		return
	}
	var archived []byte
	if err := s.DB.QueryRowContext(ctx, `SELECT raw_document FROM spv_source_documents WHERE invoice_id=$1 AND processing_status='PROCESSED' ORDER BY discovered_at DESC LIMIT 1`, invoice.ID).Scan(&archived); err != nil {
		return
	}
	parsed, err := (spv.UBLParser{}).Parse(archived)
	if err != nil {
		return
	}
	if invoice.SourceFacts != nil && parsed.Invoice.SourceFacts != nil {
		parsed.Invoice.SourceFacts.SourceDocumentID = invoice.SourceFacts.SourceDocumentID
		parsed.Invoice.SourceFacts.SourceHash = invoice.SourceFacts.SourceHash
	}
	invoice.SourceFacts = parsed.Invoice.SourceFacts
	byPosition := make(map[int]invoicing.Line, len(parsed.Invoice.Lines))
	for _, line := range parsed.Invoice.Lines {
		byPosition[line.Position] = line
	}
	for index := range invoice.Lines {
		if parsedLine, ok := byPosition[invoice.Lines[index].Position]; ok {
			invoice.Lines[index].SourceFacts = parsedLine.SourceFacts
			invoice.Lines[index].AdditionalInfo = parsedLine.AdditionalInfo
		}
	}
}

func (s *Store) SaveRun(ctx context.Context, run commercialvalidation.Run, correlationID string) (bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	commandKey := "commercial-validation:" + run.ID
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM invoice_commercial_validation_runs WHERE command_key=$1)`, commandKey).Scan(&exists); err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	var clientID, status string
	var revision uint64
	if err = tx.QueryRowContext(ctx, `SELECT client_id,pipeline_status,revision FROM invoices WHERE id=$1 FOR UPDATE`, run.InvoiceID).Scan(&clientID, &status, &revision); errors.Is(err, sql.ErrNoRows) {
		return false, apperrors.ErrNotFound
	}
	if err != nil {
		return false, err
	}
	// A concurrent worker can commit the same run while this transaction waits
	// for the invoice lock. Recheck under the lock before judging its revision.
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM invoice_commercial_validation_runs WHERE command_key=$1)`, commandKey).Scan(&exists); err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	if status != string(invoicing.StatusCommercialValidating) || revision != run.InvoiceRevision {
		return false, apperrors.ErrConflict
	}
	var snapshot any
	if run.SnapshotID != "" {
		snapshot = run.SnapshotID
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO invoice_commercial_validation_runs(id,invoice_id,snapshot_id,invoice_revision,snapshot_version,engine_version,outcome,command_key,created_at,completed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, run.ID, run.InvoiceID, snapshot, run.InvoiceRevision, run.SnapshotVersion, run.EngineVersion, run.Outcome, commandKey, run.CreatedAt, run.CompletedAt)
	if err != nil {
		return false, err
	}
	for _, finding := range run.Findings {
		missing, _ := json.Marshal(finding.MissingInputs)
		evidence, _ := json.Marshal(finding.Evidence)
		candidates, _ := json.Marshal(finding.ServiceCandidates)
		var lineID any
		if finding.LineID != "" {
			lineID = finding.LineID
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO invoice_commercial_findings(id,run_id,rule_id,invoice_line_id,code,outcome,actual_value,actual_source,expected_value,calculation,reason,missing_inputs,evidence,service_candidates) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, finding.ID, run.ID, finding.RuleID, lineID, finding.Code, finding.Outcome, nullText(finding.Actual), nullText(finding.ActualSource), nullText(finding.Expected), nullText(finding.Calculation), finding.Reason, missing, evidence, candidates)
		if err != nil {
			return false, err
		}
	}
	now := run.CompletedAt
	if run.Outcome == commercialvalidation.Conform {
		result, execErr := tx.ExecContext(ctx, `UPDATE invoices SET pipeline_status='COMMERCIALLY_VALIDATED',revision=revision+1,updated_at=$3 WHERE id=$1 AND revision=$2 AND pipeline_status='COMMERCIAL_VALIDATING'`, run.InvoiceID, run.InvoiceRevision, now)
		if execErr != nil {
			return false, execErr
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return false, apperrors.ErrConflict
		}
		if err = insertCommercialAudit(ctx, tx, clientID, run.InvoiceID, "COMMERCIAL_VALIDATION_CONFORM", status, string(invoicing.StatusCommerciallyValidated), "COMMERCIAL_VALIDATION_DECISION", "Validarea comercială deterministă este conformă.", commandKey+":audit", correlationID, now, nil); err != nil {
			return false, err
		}
		if err = insertCommercialOutbox(ctx, tx, run.InvoiceID, commandKey+":continue", correlationID, now); err != nil {
			return false, err
		}
	} else {
		taskID := stableID("task", commandKey)
		result, execErr := tx.ExecContext(ctx, `UPDATE invoices SET pipeline_status='AWAITING_COMMERCIAL_REVIEW',revision=revision+1,updated_at=$3 WHERE id=$1 AND revision=$2 AND pipeline_status='COMMERCIAL_VALIDATING'`, run.InvoiceID, run.InvoiceRevision, now)
		if execErr != nil {
			return false, execErr
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return false, apperrors.ErrConflict
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO validation_tasks(id,client_id,invoice_id,task_type,status,title,reason,blocker_code,created_by_kind,created_by_display,creation_key,revision,created_at,updated_at) VALUES($1,$2,$3,'COMMERCIAL_REVIEW','OPEN',$4,$5,$6,'SYSTEM','Motor comercial',$7,1,$8,$8)`, taskID, clientID, run.InvoiceID, "Verifică factura față de contract", "Există abateri sau date contractuale care nu pot fi verificate.", string(run.Outcome), commandKey+":task", now)
		if err != nil {
			return false, err
		}
		if err = insertCommercialAudit(ctx, tx, clientID, run.InvoiceID, "COMMERCIAL_REVIEW_REQUIRED", status, string(invoicing.StatusAwaitingCommercialReview), "COMMERCIAL_VALIDATION_DECISION", "Validarea comercială necesită intervenție umană.", commandKey+":audit", correlationID, now, &taskID); err != nil {
			return false, err
		}
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) GetRun(ctx context.Context, clientID, invoiceID string) (*commercialvalidation.Run, error) {
	var run commercialvalidation.Run
	err := s.DB.QueryRowContext(ctx, `SELECT r.id,r.invoice_id,COALESCE(r.snapshot_id,''),COALESCE(s.dossier_id,''),r.invoice_revision,r.snapshot_version,r.engine_version,r.outcome,r.created_at,r.completed_at FROM invoice_commercial_validation_runs r JOIN invoices i ON i.id=r.invoice_id LEFT JOIN contract_commercial_snapshots s ON s.id=r.snapshot_id WHERE i.client_id=$1 AND i.id=$2 ORDER BY r.created_at DESC,r.id DESC LIMIT 1`, clientID, invoiceID).Scan(&run.ID, &run.InvoiceID, &run.SnapshotID, &run.DossierID, &run.InvoiceRevision, &run.SnapshotVersion, &run.EngineVersion, &run.Outcome, &run.CreatedAt, &run.CompletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT f.id,f.rule_id,f.code,f.outcome,COALESCE(f.invoice_line_id,''),COALESCE(f.actual_value,''),COALESCE(f.actual_source,''),COALESCE(f.expected_value,''),COALESCE(f.calculation,''),f.reason,f.missing_inputs,f.evidence,f.service_candidates,o.reason,o.actor_id,COALESCE(o.actor_display,''),o.created_at FROM invoice_commercial_findings f LEFT JOIN invoice_commercial_overrides o ON o.finding_id=f.id WHERE f.run_id=$1 ORDER BY f.id`, run.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var finding commercialvalidation.Finding
		var missing, evidence, candidates []byte
		var overrideReason, actorID, actorDisplay sql.NullString
		var overrideAt sql.NullTime
		if err = rows.Scan(&finding.ID, &finding.RuleID, &finding.Code, &finding.Outcome, &finding.LineID, &finding.Actual, &finding.ActualSource, &finding.Expected, &finding.Calculation, &finding.Reason, &missing, &evidence, &candidates, &overrideReason, &actorID, &actorDisplay, &overrideAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(missing, &finding.MissingInputs)
		_ = json.Unmarshal(evidence, &finding.Evidence)
		_ = json.Unmarshal(candidates, &finding.ServiceCandidates)
		if overrideReason.Valid {
			finding.Override = &commercialvalidation.Override{Reason: overrideReason.String, ActorID: actorID.String, ActorDisplay: actorDisplay.String, At: overrideAt.Time}
		}
		run.Findings = append(run.Findings, finding)
	}
	return &run, rows.Err()
}

func (s *Store) Resolve(ctx context.Context, res commercialvalidation.ReviewResolution, now time.Time) (bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	fingerprint := strings.Join([]string{res.ClientID, res.InvoiceID, res.RunID, res.FindingID, fmt.Sprintf("%d", res.ExpectedInvoiceRevision), res.Action, res.Reason}, "|")
	var priorFingerprint string
	err = tx.QueryRowContext(ctx, `SELECT fingerprint FROM commercial_review_commands WHERE command_key=$1`, "commercial-resolution:"+res.CommandID).Scan(&priorFingerprint)
	if err == nil {
		if priorFingerprint != fingerprint {
			return false, apperrors.ErrConflict
		}
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	var clientID, status string
	var revision uint64
	if err = tx.QueryRowContext(ctx, `SELECT client_id,pipeline_status,revision FROM invoices WHERE id=$1 AND client_id=$2 FOR UPDATE`, res.InvoiceID, res.ClientID).Scan(&clientID, &status, &revision); errors.Is(err, sql.ErrNoRows) {
		return false, apperrors.ErrNotFound
	}
	if err != nil {
		return false, err
	}
	if status != string(invoicing.StatusAwaitingCommercialReview) || revision != res.ExpectedInvoiceRevision {
		return false, apperrors.ErrConflict
	}
	var validRun bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM invoice_commercial_validation_runs WHERE id=$1 AND invoice_id=$2 AND id=(SELECT id FROM invoice_commercial_validation_runs WHERE invoice_id=$2 ORDER BY created_at DESC,id DESC LIMIT 1))`, res.RunID, res.InvoiceID).Scan(&validRun); err != nil {
		return false, err
	}
	if !validRun {
		return false, apperrors.ErrNotFound
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO commercial_review_commands(command_key,client_id,invoice_id,fingerprint,created_at) VALUES($1,$2,$3,$4,$5)`, "commercial-resolution:"+res.CommandID, res.ClientID, res.InvoiceID, fingerprint, now); err != nil {
		return false, err
	}
	if res.Action == "WAIT_FOR_CORRECTION" {
		result, execErr := tx.ExecContext(ctx, `UPDATE validation_tasks SET status='WAITING',waiting_since=$2,revision=revision+1,updated_at=$2 WHERE invoice_id=$1 AND task_type='COMMERCIAL_REVIEW' AND status='OPEN'`, res.InvoiceID, now)
		if execErr != nil {
			return false, execErr
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return false, apperrors.ErrConflict
		}
		if err = tx.Commit(); err != nil {
			return false, err
		}
		return true, nil
	}
	if res.Action == "RERUN" {
		result, execErr := tx.ExecContext(ctx, `UPDATE invoices SET pipeline_status='COMMERCIAL_VALIDATING',revision=revision+1,updated_at=$3 WHERE id=$1 AND revision=$2 AND pipeline_status='AWAITING_COMMERCIAL_REVIEW'`, res.InvoiceID, res.ExpectedInvoiceRevision, now)
		if execErr != nil {
			return false, execErr
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return false, apperrors.ErrConflict
		}
		if _, err = tx.ExecContext(ctx, `UPDATE validation_tasks SET status='RESOLVED',resolved_at=$2,resolution_metadata=jsonb_build_object('runId',$3::text,'action','RERUN'),revision=revision+1,updated_at=$2 WHERE invoice_id=$1 AND task_type='COMMERCIAL_REVIEW' AND status IN ('OPEN','WAITING')`, res.InvoiceID, now, res.RunID); err != nil {
			return false, err
		}
		if err = insertCommercialOutbox(ctx, tx, res.InvoiceID, "commercial-resolution:"+res.CommandID+":rerun", res.CorrelationID, now); err != nil {
			return false, err
		}
		if err = tx.Commit(); err != nil {
			return false, err
		}
		return true, nil
	}
	var validFinding bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM invoice_commercial_findings WHERE id=$1 AND run_id=$2 AND outcome<>'CONFORM')`, res.FindingID, res.RunID).Scan(&validFinding); err != nil {
		return false, err
	}
	if !validFinding {
		return false, apperrors.ErrNotFound
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO invoice_commercial_overrides(id,finding_id,reason,actor_id,actor_display,command_key,created_at) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(command_key) DO NOTHING`, stableID("commercial-override", res.CommandID), res.FindingID, res.Reason, res.ActorID, nullText(res.ActorDisplay), "commercial-resolution:"+res.CommandID, now); err != nil {
		return false, err
	}
	eventKey := "commercial-exception:" + res.CommandID
	if _, err = tx.ExecContext(ctx, `INSERT INTO activity_events(id,client_id,invoice_id,validation_task_id,aggregate_type,aggregate_id,event_type,occurred_at,actor_kind,actor_id,actor_display,automatic,detail,correlation_id,idempotency_key) SELECT $1,$2,$3,t.id,'INVOICE',$3,'COMMERCIAL_EXCEPTION_APPROVED',$4,'USER',$5,$6,false,$7,$8,$9 FROM validation_tasks t WHERE t.invoice_id=$3 AND t.task_type='COMMERCIAL_REVIEW' AND t.status IN ('OPEN','WAITING') ORDER BY t.created_at DESC LIMIT 1 ON CONFLICT(idempotency_key) DO NOTHING`, stableID("evt", eventKey), res.ClientID, res.InvoiceID, now, res.ActorID, nullText(res.ActorDisplay), "Excepție comercială aprobată motivat: "+res.Reason, nullText(res.CorrelationID), eventKey); err != nil {
		return false, err
	}
	var remaining int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM invoice_commercial_findings f LEFT JOIN invoice_commercial_overrides o ON o.finding_id=f.id WHERE f.run_id=$1 AND f.outcome<>'CONFORM' AND o.id IS NULL`, res.RunID).Scan(&remaining); err != nil {
		return false, err
	}
	if remaining == 0 {
		if _, err = tx.ExecContext(ctx, `UPDATE validation_tasks SET status='RESOLVED',resolved_at=$2,resolution_metadata=jsonb_build_object('runId',$3::text,'action','ACCEPT_EXCEPTION'),revision=revision+1,updated_at=$2 WHERE invoice_id=$1 AND task_type='COMMERCIAL_REVIEW' AND status IN ('OPEN','WAITING')`, res.InvoiceID, now, res.RunID); err != nil {
			return false, err
		}
		result, execErr := tx.ExecContext(ctx, `UPDATE invoices SET pipeline_status='COMMERCIALLY_VALIDATED',revision=revision+1,updated_at=$3 WHERE id=$1 AND revision=$2 AND pipeline_status='AWAITING_COMMERCIAL_REVIEW'`, res.InvoiceID, res.ExpectedInvoiceRevision, now)
		if execErr != nil {
			return false, execErr
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return false, apperrors.ErrConflict
		}
		if err = insertCommercialOutbox(ctx, tx, res.InvoiceID, "commercial-resolution:"+res.CommandID+":continue", res.CorrelationID, now); err != nil {
			return false, err
		}
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) PutVariable(ctx context.Context, clientID, dossierID string, value commercialvalidation.VariableValue, commandID string) (bool, error) {
	result, err := s.DB.ExecContext(ctx, `INSERT INTO contract_variable_values(id,definition_id,period_start,period_end,value,source,source_reference,recorded_by_id,recorded_at,command_key) SELECT $1,d.id,$2,$3,$4,$5,$6,$7,$8,$9 FROM contract_variable_definitions d JOIN contract_dossiers dossier ON dossier.id=d.dossier_id WHERE d.dossier_id=$10 AND d.name=$11 AND dossier.client_id=$12 ON CONFLICT(command_key) DO NOTHING`, stableID("contract-variable", commandID), value.PeriodStart, value.PeriodEnd, value.Value, value.Source, nullText(value.SourceReference), value.RecordedByID, value.RecordedAt, "commercial-variable:"+commandID, dossierID, value.Name, clientID)
	if err != nil {
		return false, err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		var storedClient, storedDossier, storedName, storedValue, storedSource, storedReference string
		var storedStart, storedEnd sql.NullTime
		err = s.DB.QueryRowContext(ctx, `SELECT dossier.client_id,d.dossier_id,d.name,v.value,v.source,COALESCE(v.source_reference,''),v.period_start,v.period_end FROM contract_variable_values v JOIN contract_variable_definitions d ON d.id=v.definition_id JOIN contract_dossiers dossier ON dossier.id=d.dossier_id WHERE v.command_key=$1`, "commercial-variable:"+commandID).Scan(&storedClient, &storedDossier, &storedName, &storedValue, &storedSource, &storedReference, &storedStart, &storedEnd)
		if err != nil {
			return false, err
		}
		if storedClient != clientID || storedDossier != dossierID || storedName != value.Name || storedValue != value.Value || storedSource != value.Source || storedReference != value.SourceReference || !sameNullableTime(storedStart, value.PeriodStart) || !sameNullableTime(storedEnd, value.PeriodEnd) {
			return false, apperrors.ErrConflict
		}
	}
	return count == 1, nil
}

func (s *Store) ConfirmAlias(ctx context.Context, alias commercialvalidation.Alias, commandID string) (bool, error) {
	input, err := s.LoadInput(ctx, alias.InvoiceID)
	if err != nil {
		return false, err
	}
	if input.Invoice.ClientID != alias.ClientID || input.Snapshot.DossierID == "" {
		return false, apperrors.ErrNotFound
	}
	var label string
	for _, line := range input.Invoice.Lines {
		if line.ID == alias.LineID {
			label = strings.ToUpper(strings.Join(strings.Fields(line.Description), " "))
			break
		}
	}
	if label == "" {
		return false, apperrors.ErrValidation
	}
	validService := false
	for _, rule := range input.Snapshot.Rules {
		serviceID := rule.Applicability.ServiceID
		if serviceID == "" {
			serviceID = rule.ID
		}
		if rule.ID == alias.ServiceID && rule.Kind != commercialvalidation.RuleContractReference && rule.Kind != commercialvalidation.RuleIdentity && rule.Kind != commercialvalidation.RulePaymentDue {
			validService = true
			alias.ServiceID = serviceID
			break
		}
	}
	if !validService {
		return false, apperrors.ErrValidation
	}
	alias.DossierID = input.Snapshot.DossierID
	alias.SupplierCUI = valueOrBlank(input.Invoice.NormalizedSupplierCUI)
	alias.NormalizedLabel = label
	if alias.ID == "" {
		alias.ID = stableID("service-alias", commandID)
	}
	var scopedInvoiceID any
	if !alias.ReuseForDossier {
		scopedInvoiceID = alias.InvoiceID
	}
	result, err := s.DB.ExecContext(ctx, `INSERT INTO contract_service_aliases(id,client_id,supplier_cui,service_id,normalized_label,effective_from,effective_to,confirmed_by_id,confirmed_at,command_key,dossier_id,invoice_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT DO NOTHING`, alias.ID, alias.ClientID, alias.SupplierCUI, alias.ServiceID, alias.NormalizedLabel, alias.EffectiveFrom, alias.EffectiveTo, alias.ConfirmedByID, alias.ConfirmedAt, "commercial-alias:"+commandID, alias.DossierID, scopedInvoiceID)
	if err != nil {
		return false, err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		var stored commercialvalidation.Alias
		var from, to sql.NullTime
		var storedInvoiceID sql.NullString
		err = s.DB.QueryRowContext(ctx, `SELECT client_id,supplier_cui,service_id,normalized_label,effective_from,effective_to,confirmed_by_id,dossier_id,invoice_id FROM contract_service_aliases WHERE command_key=$1`, "commercial-alias:"+commandID).Scan(&stored.ClientID, &stored.SupplierCUI, &stored.ServiceID, &stored.NormalizedLabel, &from, &to, &stored.ConfirmedByID, &stored.DossierID, &storedInvoiceID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return false, apperrors.ErrConflict
			}
			return false, err
		}
		if stored.ClientID != alias.ClientID || stored.DossierID != alias.DossierID || stored.SupplierCUI != alias.SupplierCUI || stored.ServiceID != alias.ServiceID || stored.NormalizedLabel != alias.NormalizedLabel || stored.ConfirmedByID != alias.ConfirmedByID || storedInvoiceID.Valid == alias.ReuseForDossier || (storedInvoiceID.Valid && storedInvoiceID.String != alias.InvoiceID) || !sameNullableTime(from, alias.EffectiveFrom) || !sameNullableTime(to, alias.EffectiveTo) {
			return false, apperrors.ErrConflict
		}
	}
	return count == 1, nil
}

func (s *Store) PutInvoiceDateFact(ctx context.Context, fact commercialvalidation.InvoiceDateFact, now time.Time) (bool, error) {
	commandKey := "commercial-date:" + fact.CommandID
	result, err := s.DB.ExecContext(ctx, `INSERT INTO invoice_commercial_date_facts(id,invoice_id,kind,occurred_on,source_reference,recorded_by_id,recorded_at,command_key)
		SELECT $1,id,$3,$4,$5,$6,$7,$8 FROM invoices WHERE id=$2 AND client_id=$9 ON CONFLICT(command_key) DO NOTHING`,
		stableID("commercial-date", commandKey), fact.InvoiceID, fact.Kind, fact.Date, strings.TrimSpace(fact.SourceReference), fact.ActorID, now, commandKey, fact.ClientID)
	if err != nil {
		return false, err
	}
	count, _ := result.RowsAffected()
	if count == 1 {
		return true, nil
	}
	var invoiceID, kind, source, actorID string
	var occurred time.Time
	err = s.DB.QueryRowContext(ctx, `SELECT invoice_id,kind,occurred_on,source_reference,recorded_by_id FROM invoice_commercial_date_facts WHERE command_key=$1`, commandKey).
		Scan(&invoiceID, &kind, &occurred, &source, &actorID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, apperrors.ErrNotFound
	}
	if err != nil {
		return false, err
	}
	if invoiceID != fact.InvoiceID || kind != fact.Kind || occurred.Format("2006-01-02") != fact.Date.Format("2006-01-02") || source != strings.TrimSpace(fact.SourceReference) || actorID != fact.ActorID {
		return false, apperrors.ErrConflict
	}
	return false, nil
}

func sameNullableTime(stored sql.NullTime, value *time.Time) bool {
	if !stored.Valid {
		return value == nil
	}
	return value != nil && stored.Time.Equal(*value)
}

func (s *Store) PreviewRevalidation(ctx context.Context, clientID, snapshotID string) ([]commercialvalidation.RevalidationCandidate, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT i.id,i.issue_day,COALESCE((SELECT r.id FROM invoice_commercial_validation_runs r WHERE r.invoice_id=i.id ORDER BY r.created_at DESC LIMIT 1),'') FROM contract_commercial_snapshots s JOIN contract_dossiers d ON d.id=s.dossier_id JOIN invoice_contract_associations a ON a.contract_id=s.contract_id JOIN invoices i ON i.id=a.invoice_id WHERE s.id=$1 AND d.client_id=$2 AND i.client_id=$2 AND i.issue_day>=s.effective_from AND (s.effective_to IS NULL OR i.issue_day<=s.effective_to) ORDER BY i.issue_day,i.id`, snapshotID, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []commercialvalidation.RevalidationCandidate
	for rows.Next() {
		var item commercialvalidation.RevalidationCandidate
		if err = rows.Scan(&item.InvoiceID, &item.IssueDate, &item.PreviousRunID); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) CreateDossier(ctx context.Context, dossier commercialvalidation.Dossier, commandID string) (commercialvalidation.Dossier, bool, error) {
	result, err := s.DB.ExecContext(ctx, `INSERT INTO contract_dossiers(id,client_id,supplier_cui,buyer_cui,primary_reference,status,revision,creation_key,created_at,updated_at) VALUES($1,$2,$3,$4,$5,'DRAFT',1,$6,$7,$7) ON CONFLICT(creation_key) DO NOTHING`, dossier.ID, dossier.ClientID, dossier.SupplierCUI, dossier.BuyerCUI, dossier.PrimaryReference, "contract-dossier:"+commandID, dossier.CreatedAt)
	if err != nil {
		return commercialvalidation.Dossier{}, false, err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		err = s.DB.QueryRowContext(ctx, `SELECT id FROM contract_dossiers WHERE creation_key=$1 AND client_id=$2`, "contract-dossier:"+commandID, dossier.ClientID).Scan(&dossier.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return commercialvalidation.Dossier{}, false, apperrors.ErrConflict
		}
		if err != nil {
			return commercialvalidation.Dossier{}, false, err
		}
	}
	loaded, err := s.GetDossier(ctx, dossier.ClientID, dossier.ID)
	if err == nil && count == 0 && (loaded.SupplierCUI != dossier.SupplierCUI || loaded.BuyerCUI != dossier.BuyerCUI || loaded.PrimaryReference != dossier.PrimaryReference) {
		return commercialvalidation.Dossier{}, false, apperrors.ErrConflict
	}
	return loaded, count == 1, err
}

func (s *Store) ListDossiers(ctx context.Context, clientID string) ([]commercialvalidation.Dossier, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT d.id,d.client_id,COALESCE(d.contract_id,''),d.supplier_cui,d.buyer_cui,d.primary_reference,d.status,COALESCE(d.active_snapshot_id,''),COALESCE(s.coverage,''),COALESCE(s.version,0),d.revision,d.created_at,d.updated_at FROM contract_dossiers d LEFT JOIN contract_commercial_snapshots s ON s.id=d.active_snapshot_id WHERE d.client_id=$1 ORDER BY d.updated_at DESC,d.id`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []commercialvalidation.Dossier{}
	for rows.Next() {
		var item commercialvalidation.Dossier
		if err = rows.Scan(&item.ID, &item.ClientID, &item.ContractID, &item.SupplierCUI, &item.BuyerCUI, &item.PrimaryReference, &item.Status, &item.ActiveSnapshotID, &item.Coverage, &item.SnapshotVersion, &item.Revision, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) GetDossier(ctx context.Context, clientID, dossierID string) (commercialvalidation.Dossier, error) {
	var item commercialvalidation.Dossier
	err := s.DB.QueryRowContext(ctx, `SELECT d.id,d.client_id,COALESCE(d.contract_id,''),d.supplier_cui,d.buyer_cui,d.primary_reference,d.status,COALESCE(d.active_snapshot_id,''),COALESCE(s.coverage,''),COALESCE(s.version,0),d.revision,d.created_at,d.updated_at FROM contract_dossiers d LEFT JOIN contract_commercial_snapshots s ON s.id=d.active_snapshot_id WHERE d.client_id=$1 AND d.id=$2`, clientID, dossierID).Scan(&item.ID, &item.ClientID, &item.ContractID, &item.SupplierCUI, &item.BuyerCUI, &item.PrimaryReference, &item.Status, &item.ActiveSnapshotID, &item.Coverage, &item.SnapshotVersion, &item.Revision, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return commercialvalidation.Dossier{}, apperrors.ErrNotFound
	}
	return item, err
}

func invoiceContractReference(invoice invoicing.Invoice) (string, string) {
	if invoice.SourceFacts != nil {
		for _, candidate := range invoice.SourceFacts.ContractReferences {
			if value := referenceFromText(candidate); value != "" {
				return value, "Referință contractuală structurată din e-Factura"
			}
		}
		if value := referenceFromText(invoice.SourceFacts.BuyerReference); value != "" {
			return value, "Referința cumpărătorului din e-Factura"
		}
		for _, note := range invoice.SourceFacts.Notes {
			if value := referenceFromText(note); value != "" {
				return value, "Nota facturii"
			}
		}
	}
	for _, line := range invoice.Lines {
		values := []struct{ value, source string }{{line.Description, fmt.Sprintf("Denumirea liniei %d", line.Position)}}
		if line.SourceFacts != nil {
			values = append(values,
				struct{ value, source string }{line.SourceFacts.ItemDescription, fmt.Sprintf("Descrierea liniei %d", line.Position)},
				struct{ value, source string }{line.SourceFacts.Note, fmt.Sprintf("Nota liniei %d", line.Position)},
			)
		}
		if line.AdditionalInfo != nil {
			values = append(values, struct{ value, source string }{*line.AdditionalInfo, fmt.Sprintf("Informațiile suplimentare ale liniei %d", line.Position)})
		}
		for _, candidate := range values {
			if value := referenceFromText(candidate.value); value != "" {
				return value, candidate.source
			}
		}
	}
	return "", ""
}

func referenceFromText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	match := contractReferencePattern.FindStringSubmatch(value)
	if len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	// Structured UBL reference fields commonly contain the bare identifier.
	if regexp.MustCompile(`^[A-Z0-9_-]+\s*/\s*[0-9.]+$`).MatchString(value) {
		return value
	}
	return ""
}
func valueOrBlank(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func nullText(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func insertCommercialOutbox(ctx context.Context, tx *sql.Tx, invoiceID, key, correlationID string, now time.Time) error {
	payload, _ := json.Marshal(map[string]string{"invoice_id": invoiceID})
	_, err := tx.ExecContext(ctx, `INSERT INTO outbox_entries(id,event_type,aggregate_type,aggregate_id,payload,idempotency_key,correlation_id,status,attempts,created_at,available_at) VALUES($1,'INVOICE_CONTINUE','INVOICE',$2,$3,$4,$5,'PENDING',0,$6,$6)`, stableID("out", key), invoiceID, payload, key, nullText(correlationID), now)
	return err
}
func insertCommercialAudit(ctx context.Context, tx *sql.Tx, clientID, invoiceID, eventType, from, to, trigger, detail, key, correlationID string, now time.Time, taskID *string) error {
	before, _ := json.Marshal(from)
	after, _ := json.Marshal(to)
	_, err := tx.ExecContext(ctx, `INSERT INTO activity_events(id,client_id,invoice_id,validation_task_id,aggregate_type,aggregate_id,event_type,occurred_at,actor_kind,actor_display,automatic,detail,before_snapshot,after_snapshot,correlation_id,pipeline_from,pipeline_to,trigger,idempotency_key) VALUES($1,$2,$3,$4,'INVOICE',$3,$5,$6,'SYSTEM','Motor comercial',true,$7,$8,$9,$10,$11,$12,$13,$14)`, stableID("evt", key), clientID, invoiceID, taskID, eventType, now, detail, before, after, nullText(correlationID), from, to, trigger, key)
	return err
}
