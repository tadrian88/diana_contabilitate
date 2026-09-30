package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountinganalysis"
	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/accounts"
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
	if !accountinganalysis.HasDimensionsNeedingAI(input) {
		return accountinganalysis.Run{}, false, accountinganalysis.ErrNoAnalysisNeeded
	}
	// One focused lookup per unresolved dimension, so each proposal can cite
	// evidence for its own decision; the merged set is size-bounded.
	groups := [][]legislation.Fragment{}
	for _, planned := range accountinganalysis.RetrievalPlan(input) {
		group, retrieveErr := s.Retrieve(ctx, planned.Query(input, true))
		if retrieveErr != nil {
			return accountinganalysis.Run{}, false, retrieveErr
		}
		groups = append(groups, group)
	}
	fragments := accountinganalysis.MergeFragments(groups)
	if len(fragments) == 0 {
		return accountinganalysis.Run{}, false, fmt.Errorf("%w: corpusul legislativ local nu conține fragmente aplicabile", apperrors.ErrValidation)
	}
	inputRaw, _ := json.Marshal(input)
	ids := make([]string, len(fragments))
	for i, f := range fragments {
		ids[i] = f.ID
	}
	fragmentRaw, _ := json.Marshal(ids)
	approvedRaw := []byte(`[]`)
	if err = s.DB.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(DISTINCT COALESCE(proposal_provenance->>'knowledgeId',account_mapping_id)) FILTER (WHERE COALESCE(proposal_provenance->>'knowledgeId',account_mapping_id) IS NOT NULL),'[]'::jsonb) FROM line_classifications WHERE client_id=$1 AND invoice_id=$2 AND classification_run_id=$3`, input.ClientID, input.InvoiceID, input.ClassificationRunID).Scan(&approvedRaw); err != nil {
		return accountinganalysis.Run{}, false, err
	}
	runID := stableID("analysis", command.CommandID)
	empty := []byte(`[]`)
	_, err = s.DB.ExecContext(ctx, `INSERT INTO accounting_analysis_runs(id,client_id,invoice_id,invoice_revision,schema_version,prompt_version,provider,model,status,input_snapshot,retrieved_fragment_ids,approved_knowledge_ids,validation_results,started_at,command_key,classification_run_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'RUNNING',$9,$10,$11,$12,$13,$14,$15)`, runID, input.ClientID, input.InvoiceID, input.InvoiceRevision, accountinganalysis.SchemaVersion, accountinganalysis.PromptVersion, provider, model, inputRaw, fragmentRaw, approvedRaw, empty, now, command.CommandID, input.ClassificationRunID)
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

const analysisSelect = `SELECT r.id,r.client_id,r.invoice_id,r.invoice_revision,r.status,r.provider,r.model,r.raw_structured_response,r.validation_results,r.started_at,r.completed_at,r.classification_run_id,(r.classification_run_id IS DISTINCT FROM i.current_classification_run_id),rv.id,rv.action,rv.reason,rv.actor_display,rv.final_decision,rv.created_at FROM accounting_analysis_runs r JOIN invoices i ON i.id=r.invoice_id AND i.client_id=r.client_id LEFT JOIN LATERAL(SELECT * FROM accounting_analysis_reviews x WHERE x.analysis_id=r.id ORDER BY x.created_at DESC LIMIT 1)rv ON true`

type rowScanner interface{ Scan(...any) error }

func (s *Store) scanAnalysis(row rowScanner) (accountinganalysis.Run, error) {
	var run accountinganalysis.Run
	var proposalRaw, issuesRaw []byte
	var completed sql.NullTime
	var reviewID, action, reason, actor, classificationRunID sql.NullString
	var finalRaw []byte
	var reviewAt sql.NullTime
	if err := row.Scan(&run.ID, &run.ClientID, &run.InvoiceID, &run.InvoiceRevision, &run.Status, &run.Provider, &run.Model, &proposalRaw, &issuesRaw, &run.StartedAt, &completed, &classificationRunID, &run.ContextStale, &reviewID, &action, &reason, &actor, &finalRaw, &reviewAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return run, accountinganalysis.ErrAnalysisNotFound
		}
		return run, err
	}
	run.ClassificationRunID = classificationRunID.String
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
	if !run.ContextStale {
		currentInput, currentErr := s.loadAnalysisInput(ctx, clientID, invoiceID)
		if currentErr != nil {
			return accountinganalysis.Execution{}, currentErr
		}
		if currentInput.ClassificationRunID == run.ClassificationRunID {
			// Recompute unresolved dimensions at execution time. The fiscal/profile
			// context still comes from the immutable classification-run snapshot.
			input = currentInput
		}
	}
	fragments := []legislation.Fragment{}
	for _, id := range ids {
		var f legislation.Fragment
		if err = s.DB.QueryRowContext(ctx, `SELECT id,version_id,citation_key,heading,content,content_hash,ordinal FROM legislation_fragments WHERE id=$1`, id).Scan(&f.ID, &f.VersionID, &f.CitationKey, &f.Heading, &f.Text, &f.ContentHash, &f.Ordinal); err != nil {
			return accountinganalysis.Execution{}, err
		}
		fragments = append(fragments, f)
	}
	return accountinganalysis.Execution{Run: run, Input: input, Fragments: fragments, Accounts: input.AccountCatalog}, nil
}

func (s *Store) CompleteAnalysis(ctx context.Context, runID string, result accountinganalysis.ProviderResult, decisions []accountinganalysis.ValidatedDecision, issues []accountinganalysis.ValidationIssue, now time.Time) error {
	proposalRaw, _ := json.Marshal(result.Proposal)
	issuesRaw, _ := json.Marshal(issues)
	valid := 0
	globalInvalid := false
	for _, issue := range issues {
		if issue.Code == "SCHEMA_VERSION" || issue.Code == "SOURCE" {
			globalInvalid = true
		}
	}
	for _, decision := range decisions {
		if decision.Valid() && !globalInvalid {
			valid++
		}
	}
	status := "PROPOSED"
	if valid == 0 {
		status = "VALIDATION_FAILED"
	} else if len(issues) > 0 || valid != len(decisions) {
		status = "PARTIAL_VALIDATION"
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	changed, err := tx.ExecContext(ctx, `UPDATE accounting_analysis_runs SET status=$2,raw_structured_response=$3,validation_results=$4,input_tokens=$5,output_tokens=$6,completed_at=$7 WHERE id=$1 AND status='RUNNING'`, runID, status, proposalRaw, issuesRaw, result.InputTokens, result.OutputTokens, now)
	if err != nil {
		return err
	}
	count, _ := changed.RowsAffected()
	if count == 0 {
		runErr := tx.QueryRowContext(ctx, `SELECT status FROM accounting_analysis_runs WHERE id=$1`, runID).Scan(&status)
		return runErr
	}
	provenanceRaw, _ := json.Marshal(accounting.ProposalProvenance{AnalysisRunID: runID, Provider: result.Provider, Model: result.Model, SchemaVersion: accountinganalysis.SchemaVersion, PromptVersion: accountinganalysis.PromptVersion})
	applied := map[string]bool{}
	for _, decision := range decisions {
		if globalInvalid {
			break
		}
		decisionKey := decision.InvoiceLineID + ":" + decision.Dimension
		if applied[decisionKey] {
			continue
		}
		applied[decisionKey] = true
		proposedRaw, _ := json.Marshal(decision.Proposal.ProposedValue)
		citations := decision.Citations
		if citations == nil {
			citations = []accounting.LegalCitation{}
		}
		citationsRaw, _ := json.Marshal(citations)
		validationRaw, _ := json.Marshal(accountinganalysis.ValidationResults(decision.Issues))
		legalBasis := "Citările propunerii AI necesită validare deterministă."
		verifiedKeys := []string{}
		for _, citation := range decision.Citations {
			if citation.Verified {
				verifiedKeys = append(verifiedKeys, citation.CitationKey)
			}
		}
		if len(verifiedKeys) > 0 {
			legalBasis = strings.Join(verifiedKeys, "; ")
		}
		proposedDisplay := decision.Proposal.ProposedValue.Text()
		if strings.TrimSpace(proposedDisplay) == "" {
			proposedDisplay = "Necesită decizie"
		}
		_, err = tx.ExecContext(ctx, `UPDATE line_classifications lc
			SET proposed_typed_value=$2,proposed_value=$3,confidence_display=$4,explanation=$5,legal_basis=$6,
			    legal_citations=$7,validation_results=$8,proposal_provenance=$9,source='AI_PROPOSAL',
			    required_review=true,review_status='PENDING',effective_value=NULL,effective_typed_value=NULL,effective_source=NULL,
			    rule_version_id=NULL,account_mapping_id=NULL,account_mapping_version=NULL,revision=revision+1,updated_at=$10
			WHERE lc.classification_run_id=(SELECT classification_run_id FROM accounting_analysis_runs WHERE id=$1)
			  AND lc.invoice_line_id=$11 AND lc.dimension=$12 AND lc.model_version=$13
			  AND lc.source IN ('NO_MATCH','AMBIGUOUS') AND lc.effective_typed_value IS NULL
			  AND EXISTS(SELECT 1 FROM accounting_analysis_runs ar JOIN invoices i ON i.id=ar.invoice_id AND i.client_id=ar.client_id
			             WHERE ar.id=$1 AND i.current_classification_run_id=ar.classification_run_id)`,
			runID, proposedRaw, proposedDisplay, string(decision.Proposal.Confidence), decision.Proposal.Explanation, legalBasis,
			citationsRaw, validationRaw, provenanceRaw, now, decision.InvoiceLineID, decision.Dimension, accounting.ModelVersion)
		if err != nil {
			return err
		}
	}
	if err = s.ensureAccountingReviewTx(ctx, tx, runID, "AI_ANALYSIS_COMPLETED", now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SkipAnalysis(ctx context.Context, runID, reason string, now time.Time) error {
	status := "NOT_NEEDED"
	if reason == "SUPERSEDED_CLASSIFICATION_RUN" {
		status = "SUPERSEDED"
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE accounting_analysis_runs SET status=$2,failure_reason=$3,completed_at=$4 WHERE id=$1 AND status='RUNNING'`, runID, status, reason, now)
	return err
}

func (s *Store) FailAnalysis(ctx context.Context, runID, reason string, now time.Time) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE accounting_analysis_runs SET status='PROVIDER_FAILED',failure_reason=$2,validation_results='[{"code":"PROVIDER_FAILURE","path":"provider","message":"Analiza asistată nu este disponibilă; completați manual clasificările."}]'::jsonb,completed_at=$3 WHERE id=$1 AND status='RUNNING'`, runID, reason, now)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return tx.Commit()
	}
	if err = s.ensureAccountingReviewTx(ctx, tx, runID, "AI_ANALYSIS_PERMANENT_FAILURE", now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) OpenManualReview(ctx context.Context, clientID, invoiceID, reason string, now time.Time) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var runID, currentClient, pipeline string
	var revision uint64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(current_classification_run_id,''),client_id,pipeline_status,revision FROM invoices WHERE id=$1 FOR UPDATE`, invoiceID).Scan(&runID, &currentClient, &pipeline, &revision); err != nil {
		return err
	}
	if currentClient != clientID {
		return apperrors.ErrNotFound
	}
	if runID == "" || pipeline != "CLASSIFIED" {
		return tx.Commit()
	}
	if _, err = tx.ExecContext(ctx, `UPDATE invoices SET pipeline_status='AWAITING_REVIEW',revision=revision+1,updated_at=$2 WHERE id=$1 AND revision=$3`, invoiceID, now, revision); err != nil {
		return err
	}
	creationKey := "accounting-review:" + runID
	taskID := stableID("task", creationKey)
	_, err = tx.ExecContext(ctx, `INSERT INTO validation_tasks(id,client_id,invoice_id,classification_run_id,task_type,status,title,reason,blocker_code,created_by_kind,created_by_display,creation_key,revision,created_at,updated_at)
		VALUES($1,$2,$3,$4,'CLASSIFICATION','OPEN','Completează clasificarea contabilă manual','Analiza asistată nu este disponibilă; toate câmpurile rămân editabile.',$5,'SYSTEM','Sistem clasificare',$6,1,$7,$7) ON CONFLICT DO NOTHING`, taskID, clientID, invoiceID, runID, reason, creationKey, now)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO activity_events(id,client_id,invoice_id,validation_task_id,aggregate_type,aggregate_id,event_type,occurred_at,actor_kind,actor_display,automatic,detail,pipeline_from,pipeline_to,trigger,idempotency_key)
		VALUES($1,$2,$3,$4,'VALIDATION_TASK',$4,'AI_ANALYSIS_PREPARATION_FAILED',$5,'SYSTEM','Sistem clasificare',true,$6,'CLASSIFIED','AWAITING_REVIEW',$7,$8) ON CONFLICT(idempotency_key) DO NOTHING`, stableID("evt", creationKey+":preparation-failed"), clientID, invoiceID, taskID, now, "Automatic analysis preparation failed; manual accounting review remains available.", reason, creationKey+":preparation-failed")
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ensureAccountingReviewTx(ctx context.Context, tx *sql.Tx, analysisRunID, trigger string, now time.Time) error {
	var clientID, invoiceID, classificationRunID, currentRunID, pipeline string
	var invoiceRevision uint64
	err := tx.QueryRowContext(ctx, `SELECT ar.client_id,ar.invoice_id,ar.classification_run_id,COALESCE(i.current_classification_run_id,''),i.pipeline_status,i.revision
		FROM accounting_analysis_runs ar JOIN invoices i ON i.id=ar.invoice_id AND i.client_id=ar.client_id
		WHERE ar.id=$1 FOR UPDATE OF i`, analysisRunID).Scan(&clientID, &invoiceID, &classificationRunID, &currentRunID, &pipeline, &invoiceRevision)
	if err != nil {
		return err
	}
	if classificationRunID == "" || currentRunID != classificationRunID {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT dimension,effective_typed_value,proposed_typed_value,source,review_status
		FROM line_classifications WHERE client_id=$1 AND invoice_id=$2 AND classification_run_id=$3 AND model_version=$4`, clientID, invoiceID, classificationRunID, accounting.ModelVersion)
	if err != nil {
		return err
	}
	items := []accounting.ClassificationResolutionItem{}
	pending := 0
	for rows.Next() {
		var dimension, source, reviewStatus string
		var effectiveRaw, proposedRaw []byte
		if err = rows.Scan(&dimension, &effectiveRaw, &proposedRaw, &source, &reviewStatus); err != nil {
			rows.Close()
			return err
		}
		var effective, proposed *accounting.Value
		if len(effectiveRaw) > 0 {
			var value accounting.Value
			if json.Unmarshal(effectiveRaw, &value) == nil {
				effective = &value
			}
		}
		if len(proposedRaw) > 0 {
			var value accounting.Value
			if json.Unmarshal(proposedRaw, &value) == nil {
				proposed = &value
			}
		}
		item := accounting.ClassificationResolutionItem{Dimension: dimension, Effective: effective, Proposed: proposed, Source: source, ReviewStatus: reviewStatus}
		items = append(items, item)
		if accounting.ResolveClassification(dimension, effective, proposed, source, reviewStatus) != accounting.ResolutionFinal {
			pending++
		}
	}
	if err = rows.Close(); err != nil {
		return err
	}
	var lineCount int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM invoice_lines WHERE invoice_id=$1`, invoiceID).Scan(&lineCount); err != nil {
		return err
	}
	if accounting.IsAccountingClassificationComplete(items, lineCount*len(accounting.Dimensions)) {
		_, err = tx.ExecContext(ctx, `UPDATE invoices SET pipeline_status='READY_FOR_SAGA',saga_status='READY',revision=revision+1,updated_at=$2 WHERE id=$1 AND current_classification_run_id=$3 AND pipeline_status='CLASSIFIED'`, invoiceID, now, classificationRunID)
		return err
	}
	if pipeline == "CLASSIFIED" {
		if _, err = tx.ExecContext(ctx, `UPDATE invoices SET pipeline_status='AWAITING_REVIEW',revision=revision+1,updated_at=$2 WHERE id=$1 AND revision=$3 AND current_classification_run_id=$4`, invoiceID, now, invoiceRevision, classificationRunID); err != nil {
			return err
		}
	}
	creationKey := "accounting-review:" + classificationRunID
	taskID := stableID("task", creationKey)
	result, err := tx.ExecContext(ctx, `INSERT INTO validation_tasks(id,client_id,invoice_id,classification_run_id,task_type,status,title,reason,blocker_code,created_by_kind,created_by_display,creation_key,revision,created_at,updated_at)
		VALUES($1,$2,$3,$4,'CLASSIFICATION','OPEN','Revizuiește clasificarea contabilă',$5,$6,'SYSTEM','Sistem clasificare',$7,1,$8,$8)
		ON CONFLICT DO NOTHING`, taskID, clientID, invoiceID, classificationRunID, fmt.Sprintf("%d dimensiuni necesită decizia contabilului.", pending), nullString(map[bool]string{true: "AI_UNAVAILABLE", false: ""}[trigger == "AI_ANALYSIS_PERMANENT_FAILURE"]), creationKey, now)
	if err != nil {
		return err
	}
	created, _ := result.RowsAffected()
	if created == 0 {
		if err = tx.QueryRowContext(ctx, `SELECT id FROM validation_tasks WHERE classification_run_id=$1 AND task_type='CLASSIFICATION'`, classificationRunID).Scan(&taskID); err != nil {
			return err
		}
		return nil
	}
	if s.AccountingWorkflowObserver != nil {
		s.AccountingWorkflowObserver.AccountingReviewTaskCreated()
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO activity_events(id,client_id,invoice_id,validation_task_id,aggregate_type,aggregate_id,event_type,occurred_at,actor_kind,actor_display,automatic,detail,pipeline_from,pipeline_to,trigger,idempotency_key)
		VALUES($1,$2,$3,$4,'VALIDATION_TASK',$4,'VALIDATION_TASK_CREATED',$5,'SYSTEM','Sistem clasificare',true,$6,'CLASSIFIED','AWAITING_REVIEW',$7,$8)
		ON CONFLICT(idempotency_key) DO NOTHING`, stableID("evt", creationKey), clientID, invoiceID, taskID, now, fmt.Sprintf("One accounting review task groups %d non-final dimensions.", pending), trigger, creationKey)
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
	if execution.Run.Proposal != nil && execution.Run.Proposal.SchemaVersion == accountinganalysis.SchemaVersion {
		return accountinganalysis.Run{}, fmt.Errorf("%w: propunerile unificate se revizuiesc pe clasificarea canonică", apperrors.ErrValidation)
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
		issues := accountinganalysis.ValidateLegacy(execution.Input, decision, execution.Fragments, execution.Accounts)
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
	var sourceRaw, classificationSnapshotRaw []byte
	var issue time.Time
	var supplierID sql.NullString
	var clientCUI, total, direction, customerKind string
	err := s.DB.QueryRowContext(ctx, `SELECT i.client_id,i.id,i.revision,i.issue_day,i.currency,i.total_amount,i.supplier_name,i.normalized_supplier_cui,i.source_facts,c.normalized_identifier,COALESCE(i.current_classification_run_id,''),COALESCE(r.snapshot,'{}'::jsonb),i.direction,COALESCE(i.customer_name,''),COALESCE(i.customer_identifier_kind,'') FROM invoices i JOIN clients c ON c.id=i.client_id LEFT JOIN classification_runs r ON r.id=i.current_classification_run_id AND r.client_id=i.client_id AND r.invoice_id=i.id WHERE i.id=$1 AND i.client_id=$2 AND i.model_version=$3`, invoiceID, clientID, accounting.ModelVersion).Scan(&input.ClientID, &input.InvoiceID, &input.InvoiceRevision, &issue, &input.Currency, &total, &input.SupplierName, &supplierID, &sourceRaw, &clientCUI, &input.ClassificationRunID, &classificationSnapshotRaw, &direction, &input.CustomerName, &customerKind)
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
	// The stored direction (D-124) is authoritative; the parties must still agree
	// with it.
	clientIDNorm := normalizeFiscal(clientCUI)
	buyerIsClient := normalizeFiscal(source.BuyerVATID) == clientIDNorm || normalizeFiscal(source.BuyerLegalID) == clientIDNorm
	supplierIsClient := normalizeFiscal(source.SupplierVATID) == clientIDNorm || normalizeFiscal(source.SupplierLegalID) == clientIDNorm
	switch {
	case direction == "OUTGOING" && supplierIsClient:
		input.Direction = accountinganalysis.Outgoing
		input.CustomerIdentifierKind = customerKind
	case direction != "OUTGOING" && buyerIsClient:
		input.Direction = accountinganalysis.Incoming
		input.CustomerName = ""
	default:
		return input, fmt.Errorf("%w: direcția facturii nu poate fi stabilită", apperrors.ErrValidation)
	}
	var classificationSnapshot accounting.Snapshot
	if input.ClassificationRunID == "" || json.Unmarshal(classificationSnapshotRaw, &classificationSnapshot) != nil || classificationSnapshot.Profile == nil {
		return input, accountinganalysis.ErrMissingFiscalProfile
	}
	input.Profile = classificationSnapshot.Profile
	input.AccountCatalog, err = s.LoadAccountingAnalysisCatalog(ctx, input.Profile.AccountCodes)
	if err != nil {
		return input, err
	}
	for _, code := range input.Profile.AccountCodes {
		item, exists := input.AccountCatalog.Account(code)
		if !exists {
			item = nil
		}
		if validationErr := accounts.ValidatePostingAccount(item, code, nil); validationErr != nil {
			return input, validationErr
		}
		input.AccountCandidates = append(input.AccountCandidates, accountinganalysis.AccountCandidate{Code: item.Code, Name: item.Name, AccountType: item.AccountType})
	}
	if len(input.Profile.AccountCodes) == 0 {
		// An empty profile vocabulary allows the whole global OMFP catalog
		// (D-110); the provider receives the active, postable accounts relevant
		// to the invoice direction to keep the envelope bounded.
		codes := make([]string, 0, len(input.AccountCatalog.Entries))
		for code, item := range input.AccountCatalog.Entries {
			if item.Active && item.Postable && !item.Synthetic && accountinganalysis.AccountRelevantForDirection(code, input.Direction) {
				codes = append(codes, code)
			}
		}
		sort.Strings(codes)
		for _, code := range codes {
			item := input.AccountCatalog.Entries[code]
			input.AccountCandidates = append(input.AccountCandidates, accountinganalysis.AccountCandidate{Code: item.Code, Name: item.Name, AccountType: item.AccountType})
		}
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
	if err = lineRows.Err(); err != nil {
		return input, err
	}
	resolved := map[string]bool{}
	classificationRows, err := s.DB.QueryContext(ctx, `SELECT invoice_line_id,dimension,effective_typed_value,proposed_typed_value,source,review_status FROM line_classifications WHERE client_id=$1 AND invoice_id=$2 AND classification_run_id=$3 AND model_version=$4`, clientID, invoiceID, input.ClassificationRunID, accounting.ModelVersion)
	if err != nil {
		return input, err
	}
	for classificationRows.Next() {
		var lineID, dimension, source, reviewStatus string
		var effectiveRaw, proposedRaw []byte
		if err = classificationRows.Scan(&lineID, &dimension, &effectiveRaw, &proposedRaw, &source, &reviewStatus); err != nil {
			classificationRows.Close()
			return input, err
		}
		var effective, proposed *accounting.Value
		if len(effectiveRaw) > 0 {
			var value accounting.Value
			if json.Unmarshal(effectiveRaw, &value) == nil {
				effective = &value
			}
		}
		if len(proposedRaw) > 0 {
			var value accounting.Value
			if json.Unmarshal(proposedRaw, &value) == nil {
				proposed = &value
			}
		}
		state := accounting.ResolveClassification(dimension, effective, proposed, source, reviewStatus)
		if state != accounting.ResolutionNeedsAI {
			value := effective
			if value == nil {
				value = proposed
			}
			if value != nil && value.Validate(dimension) == nil {
				input.ResolvedDimensions = append(input.ResolvedDimensions, accountinganalysis.ResolvedDimension{InvoiceLineID: lineID, Dimension: dimension, Value: *value})
			}
			resolved[lineID+":"+dimension] = true
		}
	}
	if err = classificationRows.Err(); err != nil {
		classificationRows.Close()
		return input, err
	}
	classificationRows.Close()
	for index := range input.Lines {
		for _, dimension := range accounting.Dimensions {
			if !resolved[input.Lines[index].ID+":"+dimension] {
				input.Lines[index].UnresolvedDimensions = append(input.Lines[index].UnresolvedDimensions, dimension)
			}
		}
	}
	return input, nil
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
