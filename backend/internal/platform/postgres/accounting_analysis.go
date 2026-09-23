package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountinganalysis"
	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/legislation"
	"diana-contabilitate/backend/internal/money"
)

func (s *Store) PrepareAnalysis(ctx context.Context, command accountinganalysis.RequestCommand, provider, model string, now time.Time) (accountinganalysis.Run, bool, error) {
	if existing, err := s.analysisByCommand(ctx, command); err == nil {
		return existing, false, nil
	} else if !errors.Is(err, accountinganalysis.ErrAnalysisNotFound) {
		return accountinganalysis.Run{}, false, err
	}
	input, err := s.loadAnalysisInput(ctx, command.ClientID, command.InvoiceID)
	if err != nil {
		return accountinganalysis.Run{}, false, err
	}
	terms := []string{"contabilitate", "TVA", "deductibilitate", input.SupplierName}
	for _, line := range input.Lines {
		terms = append(terms, line.Description)
	}
	fragments, err := s.Retrieve(ctx, legislation.Query{Terms: terms, ApplicableDate: input.IssueDate, Limit: 12, AllowTestOnly: true})
	if err != nil {
		return accountinganalysis.Run{}, false, err
	}
	if len(fragments) == 0 {
		return accountinganalysis.Run{}, false, fmt.Errorf("%w: corpusul legislativ local nu conține fragmente aplicabile", apperrors.ErrValidation)
	}
	inputRaw, _ := json.Marshal(input)
	ids := make([]string, len(fragments))
	for i, f := range fragments {
		ids[i] = f.ID
	}
	fragmentRaw, _ := json.Marshal(ids)
	runID := stableID("analysis", command.CommandID)
	empty := []byte(`[]`)
	_, err = s.DB.ExecContext(ctx, `INSERT INTO accounting_analysis_runs(id,client_id,invoice_id,invoice_revision,schema_version,prompt_version,provider,model,status,input_snapshot,retrieved_fragment_ids,approved_knowledge_ids,validation_results,started_at,command_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'RUNNING',$9,$10,'[]'::jsonb,$11,$12,$13)`, runID, input.ClientID, input.InvoiceID, input.InvoiceRevision, accountinganalysis.SchemaVersion, accountinganalysis.PromptVersion, provider, model, inputRaw, fragmentRaw, empty, now, command.CommandID)
	if err != nil {
		if raced, raceErr := s.analysisByCommand(ctx, command); raceErr == nil {
			return raced, false, nil
		}
		return accountinganalysis.Run{}, false, err
	}
	created, err := s.GetAnalysis(ctx, command.ClientID, command.InvoiceID, runID)
	return created, true, err
}

func (s *Store) analysisByCommand(ctx context.Context, command accountinganalysis.RequestCommand) (accountinganalysis.Run, error) {
	return s.scanAnalysis(s.DB.QueryRowContext(ctx, analysisSelect+` WHERE r.command_key=$1 AND r.client_id=$2 AND r.invoice_id=$3`, command.CommandID, command.ClientID, command.InvoiceID))
}
func (s *Store) GetAnalysis(ctx context.Context, clientID, invoiceID, runID string) (accountinganalysis.Run, error) {
	return s.scanAnalysis(s.DB.QueryRowContext(ctx, analysisSelect+` WHERE r.id=$1 AND r.client_id=$2 AND r.invoice_id=$3`, runID, clientID, invoiceID))
}
func (s *Store) LatestAnalysis(ctx context.Context, clientID, invoiceID string) (accountinganalysis.Run, error) {
	return s.scanAnalysis(s.DB.QueryRowContext(ctx, analysisSelect+` WHERE r.client_id=$1 AND r.invoice_id=$2 ORDER BY r.started_at DESC LIMIT 1`, clientID, invoiceID))
}

const analysisSelect = `SELECT r.id,r.client_id,r.invoice_id,r.invoice_revision,r.status,r.provider,r.model,r.raw_structured_response,r.validation_results,r.started_at,r.completed_at,rv.id,rv.action,rv.reason,rv.actor_display,rv.final_decision,rv.created_at FROM accounting_analysis_runs r LEFT JOIN LATERAL(SELECT * FROM accounting_analysis_reviews x WHERE x.analysis_id=r.id ORDER BY x.created_at DESC LIMIT 1)rv ON true`

type rowScanner interface{ Scan(...any) error }

func (s *Store) scanAnalysis(row rowScanner) (accountinganalysis.Run, error) {
	var run accountinganalysis.Run
	var proposalRaw, issuesRaw []byte
	var completed sql.NullTime
	var reviewID, action, reason, actor sql.NullString
	var finalRaw []byte
	var reviewAt sql.NullTime
	if err := row.Scan(&run.ID, &run.ClientID, &run.InvoiceID, &run.InvoiceRevision, &run.Status, &run.Provider, &run.Model, &proposalRaw, &issuesRaw, &run.StartedAt, &completed, &reviewID, &action, &reason, &actor, &finalRaw, &reviewAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return run, accountinganalysis.ErrAnalysisNotFound
		}
		return run, err
	}
	if completed.Valid {
		run.CompletedAt = &completed.Time
	}
	if len(proposalRaw) > 0 {
		var p accountinganalysis.Proposal
		if err := json.Unmarshal(proposalRaw, &p); err != nil {
			return run, err
		}
		run.Proposal = &p
	}
	if len(issuesRaw) > 0 {
		if err := json.Unmarshal(issuesRaw, &run.ValidationIssues); err != nil {
			return run, err
		}
	}
	if reviewID.Valid {
		review := &accountinganalysis.Review{ID: reviewID.String, Action: action.String, Reason: reason.String, ActorDisplay: actor.String, CreatedAt: reviewAt.Time}
		if len(finalRaw) > 0 {
			var p accountinganalysis.Proposal
			if err := json.Unmarshal(finalRaw, &p); err != nil {
				return run, err
			}
			review.FinalDecision = &p
		}
		run.Review = review
	}
	return run, nil
}

func (s *Store) LoadAnalysisExecution(ctx context.Context, runID string) (accountinganalysis.Execution, error) {
	var inputRaw, fragmentIDsRaw []byte
	var clientID, invoiceID string
	err := s.DB.QueryRowContext(ctx, `SELECT client_id,invoice_id,input_snapshot,retrieved_fragment_ids FROM accounting_analysis_runs WHERE id=$1`, runID).Scan(&clientID, &invoiceID, &inputRaw, &fragmentIDsRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return accountinganalysis.Execution{}, accountinganalysis.ErrAnalysisNotFound
	}
	if err != nil {
		return accountinganalysis.Execution{}, err
	}
	run, err := s.GetAnalysis(ctx, clientID, invoiceID, runID)
	if err != nil {
		return accountinganalysis.Execution{}, err
	}
	var input accountinganalysis.Input
	var ids []string
	if json.Unmarshal(inputRaw, &input) != nil || json.Unmarshal(fragmentIDsRaw, &ids) != nil {
		return accountinganalysis.Execution{}, fmt.Errorf("corrupt analysis snapshot")
	}
	fragments := []legislation.Fragment{}
	for _, id := range ids {
		var f legislation.Fragment
		if err = s.DB.QueryRowContext(ctx, `SELECT id,version_id,citation_key,heading,content,content_hash,ordinal FROM legislation_fragments WHERE id=$1`, id).Scan(&f.ID, &f.VersionID, &f.CitationKey, &f.Heading, &f.Text, &f.ContentHash, &f.Ordinal); err != nil {
			return accountinganalysis.Execution{}, err
		}
		fragments = append(fragments, f)
	}
	catalog, err := s.LoadAccountingAnalysisCatalog(ctx, input.Profile.AccountCodes)
	if err != nil {
		return accountinganalysis.Execution{}, err
	}
	return accountinganalysis.Execution{Run: run, Input: input, Fragments: fragments, Accounts: catalog}, nil
}

func (s *Store) CompleteAnalysis(ctx context.Context, runID string, result accountinganalysis.ProviderResult, issues []accountinganalysis.ValidationIssue, now time.Time) error {
	proposalRaw, _ := json.Marshal(result.Proposal)
	issuesRaw, _ := json.Marshal(issues)
	status := "PROPOSED"
	if len(issues) > 0 {
		status = "VALIDATION_FAILED"
	}
	changed, err := s.DB.ExecContext(ctx, `UPDATE accounting_analysis_runs SET status=$2,raw_structured_response=$3,validation_results=$4,input_tokens=$5,output_tokens=$6,completed_at=$7 WHERE id=$1 AND status='RUNNING'`, runID, status, proposalRaw, issuesRaw, result.InputTokens, result.OutputTokens, now)
	if err != nil {
		return err
	}
	count, _ := changed.RowsAffected()
	if count == 0 {
		runErr := s.DB.QueryRowContext(ctx, `SELECT status FROM accounting_analysis_runs WHERE id=$1`, runID).Scan(&status)
		return runErr
	}
	return nil
}

func (s *Store) FailAnalysis(ctx context.Context, runID string, now time.Time) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE accounting_analysis_runs SET status='PROVIDER_FAILED',validation_results='[{"Code":"PROVIDER_FAILURE","Path":"provider","Message":"Furnizorul nu a produs o propunere validă."}]'::jsonb,completed_at=$2 WHERE id=$1 AND status='RUNNING'`, runID, now)
	return err
}

func (s *Store) ReviewAnalysis(ctx context.Context, command accountinganalysis.ReviewCommand, now time.Time) (accountinganalysis.Run, error) {
	var priorAnalysisID string
	priorErr := s.DB.QueryRowContext(ctx, `SELECT analysis_id FROM accounting_analysis_reviews WHERE command_key=$1`, command.CommandID).Scan(&priorAnalysisID)
	if priorErr == nil {
		if priorAnalysisID != command.AnalysisID {
			return accountinganalysis.Run{}, apperrors.ErrConflict
		}
		return s.GetAnalysis(ctx, command.ClientID, command.InvoiceID, command.AnalysisID)
	}
	if !errors.Is(priorErr, sql.ErrNoRows) {
		return accountinganalysis.Run{}, priorErr
	}
	execution, err := s.LoadAnalysisExecution(ctx, command.AnalysisID)
	if err != nil {
		return accountinganalysis.Run{}, err
	}
	if execution.Run.ClientID != command.ClientID || execution.Run.InvoiceID != command.InvoiceID {
		return accountinganalysis.Run{}, apperrors.ErrNotFound
	}
	if execution.Run.Status != "PROPOSED" || execution.Run.Review != nil {
		return accountinganalysis.Run{}, apperrors.ErrConflict
	}
	var currentRevision uint64
	if err := s.DB.QueryRowContext(ctx, `SELECT revision FROM invoices WHERE id=$1 AND client_id=$2`, command.InvoiceID, command.ClientID).Scan(&currentRevision); err != nil {
		return accountinganalysis.Run{}, err
	}
	if currentRevision != execution.Run.InvoiceRevision {
		return accountinganalysis.Run{}, apperrors.ErrConflict
	}
	var finalRaw any
	if command.Action != "REJECT" {
		var decision accountinganalysis.Proposal
		if command.Action == "APPROVE" {
			if execution.Run.Proposal == nil {
				return accountinganalysis.Run{}, apperrors.ErrConflict
			}
			decision = *execution.Run.Proposal
		} else {
			decision = *command.FinalDecision
		}
		decision.ClientID = command.ClientID
		decision.InvoiceID = command.InvoiceID
		decision.InvoiceRevision = execution.Input.InvoiceRevision
		decision.RequiresReview = false
		decision.Source = accountinganalysis.SourceManual
		issues := accountinganalysis.Validate(execution.Input, decision, execution.Fragments, execution.Accounts)
		if len(issues) > 0 {
			return accountinganalysis.Run{}, fmt.Errorf("%w: decizia finală nu trece validările deterministe", apperrors.ErrValidation)
		}
		encoded, _ := json.Marshal(decision)
		finalRaw = encoded
	}
	aiRaw, _ := json.Marshal(execution.Run.Proposal)
	reviewID := stableID("analysis-review", command.CommandID)
	_, err = s.DB.ExecContext(ctx, `INSERT INTO accounting_analysis_reviews(id,analysis_id,client_id,invoice_id,action,ai_proposal,final_decision,reason,actor_id,actor_display,created_at,command_key) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12 WHERE EXISTS(SELECT 1 FROM invoices WHERE id=$4 AND client_id=$3 AND revision=$13) ON CONFLICT DO NOTHING`, reviewID, command.AnalysisID, command.ClientID, command.InvoiceID, command.Action, aiRaw, finalRaw, strings.TrimSpace(command.Reason), nullString(command.ActorID), command.ActorDisplay, now, command.CommandID, execution.Run.InvoiceRevision)
	if err != nil {
		return accountinganalysis.Run{}, err
	}
	result, err := s.GetAnalysis(ctx, command.ClientID, command.InvoiceID, command.AnalysisID)
	if err == nil && (result.Review == nil || result.Review.ID != reviewID) {
		return accountinganalysis.Run{}, apperrors.ErrConflict
	}
	return result, err
}

func (s *Store) loadAnalysisInput(ctx context.Context, clientID, invoiceID string) (accountinganalysis.Input, error) {
	var input accountinganalysis.Input
	var sourceRaw []byte
	var issue time.Time
	var supplierID sql.NullString
	var clientCUI, total string
	err := s.DB.QueryRowContext(ctx, `SELECT i.client_id,i.id,i.revision,i.issue_day,i.currency,i.total_amount,i.supplier_name,i.normalized_supplier_cui,i.source_facts,c.normalized_identifier FROM invoices i JOIN clients c ON c.id=i.client_id WHERE i.id=$1 AND i.client_id=$2 AND i.model_version=$3`, invoiceID, clientID, accounting.ModelVersion).Scan(&input.ClientID, &input.InvoiceID, &input.InvoiceRevision, &issue, &input.Currency, &total, &input.SupplierName, &supplierID, &sourceRaw, &clientCUI)
	if errors.Is(err, sql.ErrNoRows) {
		return input, apperrors.ErrNotFound
	}
	if err != nil {
		return input, err
	}
	input.Total, err = money.Parse(total)
	if err != nil {
		return input, err
	}
	input.SupplierID = supplierID.String
	input.IssueDate = accountingdate.FromTime(issue)
	var source accounting.SourceFacts
	if json.Unmarshal(sourceRaw, &source) != nil {
		return input, fmt.Errorf("invalid source facts")
	}
	input.SourceFacts = &source
	clientIDNorm := normalizeFiscal(clientCUI)
	if normalizeFiscal(source.BuyerVATID) == clientIDNorm || normalizeFiscal(source.BuyerLegalID) == clientIDNorm {
		input.Direction = accountinganalysis.Incoming
	} else if normalizeFiscal(source.SupplierVATID) == clientIDNorm || normalizeFiscal(source.SupplierLegalID) == clientIDNorm {
		input.Direction = accountinganalysis.Outgoing
	} else {
		return input, fmt.Errorf("%w: direcția facturii nu poate fi stabilită", apperrors.ErrValidation)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT payload FROM client_accounting_profiles WHERE client_id=$1 ORDER BY version DESC`, clientID)
	if err != nil {
		return input, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var profile accounting.Profile
		if err := rows.Scan(&raw); err != nil {
			return input, err
		}
		if err := json.Unmarshal(raw, &profile); err != nil {
			return input, err
		}
		if profile.Valid(clientID, input.IssueDate) {
			if input.Profile != nil {
				return input, fmt.Errorf("%w: profiluri fiscale suprapuse", apperrors.ErrValidation)
			}
			input.Profile = &profile
		}
	}
	if err := rows.Err(); err != nil {
		return input, err
	}
	if input.Profile == nil {
		return input, fmt.Errorf("%w: lipsește profilul fiscal aplicabil", apperrors.ErrValidation)
	}
	lineRows, err := s.DB.QueryContext(ctx, `SELECT id,description,source_facts,net_value,vat_value,total_value FROM invoice_lines WHERE invoice_id=$1 ORDER BY position`, invoiceID)
	if err != nil {
		return input, err
	}
	defer lineRows.Close()
	for lineRows.Next() {
		var line accountinganalysis.Line
		var factRaw []byte
		var net, vat, gross string
		if err = lineRows.Scan(&line.ID, &line.Description, &factRaw, &net, &vat, &gross); err != nil {
			return input, err
		}
		if line.Net, err = money.Parse(net); err != nil {
			return input, err
		}
		if line.VAT, err = money.Parse(vat); err != nil {
			return input, err
		}
		if line.Gross, err = money.Parse(gross); err != nil {
			return input, err
		}
		if len(factRaw) > 0 {
			var facts accounting.LineFacts
			if err := json.Unmarshal(factRaw, &facts); err != nil {
				return input, err
			}
			line.Facts = &facts
		}
		input.Lines = append(input.Lines, line)
	}
	if len(input.Lines) == 0 {
		return input, fmt.Errorf("%w: factura nu are linii", apperrors.ErrValidation)
	}
	return input, lineRows.Err()
}
func normalizeFiscal(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "RO")
	return value
}
func nullString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
