//go:build integration

package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountinganalysis"
	"diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/legislation"
	"diana-contabilitate/backend/internal/money"
)

type capturedAnalysisPublisher struct {
	job   accountinganalysis.AnalysisJob
	calls int
}

func (p *capturedAnalysisPublisher) PublishAccountingAnalysis(_ context.Context, job accountinganalysis.AnalysisJob) error {
	p.job = job
	p.calls++
	return nil
}

type workflowFakeAnalyzer struct{}

func (workflowFakeAnalyzer) Analyze(_ context.Context, request accountinganalysis.AnalysisRequest) (accountinganalysis.ProviderResult, error) {
	fragment := request.Fragments[0]
	citation := []accountinganalysis.Citation{{FragmentID: fragment.ID, VersionID: fragment.VersionID, CitationKey: fragment.CitationKey, ContentHash: fragment.ContentHash}}
	proposal := accountinganalysis.Proposal{SchemaVersion: accountinganalysis.SchemaVersion, Source: accountinganalysis.SourceAIProposal, Summary: "TEST_ONLY automatic workflow"}
	for _, line := range request.Input.Lines {
		item := accountinganalysis.LineProposal{InvoiceLineID: line.ID}
		for _, dimension := range line.UnresolvedDimensions {
			decision := accountinganalysis.DimensionProposal{Dimension: dimension, Explanation: "TEST_ONLY automatic proposal", Citations: citation, Confidence: accountinganalysis.ConfidenceHigh}
			switch dimension {
			case "ACCOUNT":
				decision.ProposedValue = accounting.Value{Kind: "ACCOUNT", Account: "628"}
			case "VAT_TREATMENT":
				decision.ProposedValue = accounting.Value{Kind: "ORDINARY", Timing: "IMMEDIATE", SourceCategory: line.Facts.Code, SourceRate: line.Facts.Rate}
			case "VAT_DEDUCTIBILITY":
				decision.ProposedValue = accounting.Value{Kind: "FULL"}
			case "EXPENSE_TAX_TREATMENT":
				decision.ProposedValue = accounting.Value{Kind: "FULLY_DEDUCTIBLE"}
			}
			item.Decisions = append(item.Decisions, decision)
		}
		proposal.Lines = append(proposal.Lines, item)
	}
	return accountinganalysis.ProviderResult{Proposal: proposal, Provider: "FAKE_GEMINI", Model: "TEST_ONLY"}, nil
}

func TestUnifiedAIProposalPartialValidationAndManualCorrectionAudit(t *testing.T) {
	tc := newModule5TestContext(t)
	facts, lineFacts, _, _ := domainReleaseFixture(t, tc, func(pack *accounting.Pack) {
		for index := range pack.Rules {
			pack.Rules[index].Predicate.SellerItemID = "NON_MATCHING_AI_FIXTURE"
		}
	})
	invoiceID := domainPersistedInvoice(t, tc, facts, lineFacts, "unified-ai")
	classifier := classification.NewService(tc.store, classification.DomainPolicy{AllowTestOnly: true}, func() time.Time { return tc.now })
	if _, _, err := classifier.ProcessInvoice(tc.ctx, classification.ProcessCommand{InvoiceID: invoiceID, ExpectedRevision: 1, CommandID: invoiceID + ":classify"}); err != nil {
		t.Fatal(err)
	}
	item, err := tc.store.GetInvoice(tc.ctx, invoiceID)
	if err != nil || item.CurrentClassificationRunID == nil || item.ActiveTask == nil {
		t.Fatalf("missing canonical review context: %#v %v", item, err)
	}
	input, err := tc.store.loadAnalysisInput(tc.ctx, tc.clientID, invoiceID)
	if err != nil {
		t.Fatal(err)
	}
	text := "TEST_ONLY immutable legal fragment for unified proposal integration."
	digest := sha256.Sum256([]byte(text))
	fragment := legislation.Fragment{ID: "fragment-" + tc.clientID, VersionID: "version-" + tc.clientID, CitationKey: "TEST_ONLY art. unified", Text: text, ContentHash: hex.EncodeToString(digest[:]), Ordinal: 1}
	citation := []accountinganalysis.Citation{{FragmentID: fragment.ID, VersionID: fragment.VersionID, CitationKey: fragment.CitationKey, ContentHash: fragment.ContentHash}}
	rate := money.MustParse("21")
	lineID := item.Lines[0].ID
	proposal := accountinganalysis.Proposal{SchemaVersion: accountinganalysis.SchemaVersion, Source: accountinganalysis.SourceAIProposal, Summary: "TEST_ONLY partial validation", Lines: []accountinganalysis.LineProposal{{InvoiceLineID: lineID, Decisions: []accountinganalysis.DimensionProposal{
		{Dimension: "ACCOUNT", ProposedValue: accounting.Value{Kind: "ACCOUNT", Account: "628"}, Explanation: "TEST_ONLY invalid synthetic account", Citations: citation, Confidence: accountinganalysis.ConfidenceMedium},
		{Dimension: "VAT_TREATMENT", ProposedValue: accounting.Value{Kind: "ORDINARY", Timing: "IMMEDIATE", SourceCategory: "S", SourceRate: &rate}, Explanation: "TEST_ONLY source-aligned VAT", Citations: citation, Confidence: accountinganalysis.ConfidenceHigh},
		{Dimension: "VAT_DEDUCTIBILITY", ProposedValue: accounting.Value{Kind: "FULL"}, Explanation: "TEST_ONLY reviewable VAT deduction", Citations: citation, Confidence: accountinganalysis.ConfidenceMedium},
		{Dimension: "EXPENSE_TAX_TREATMENT", ProposedValue: accounting.Value{Kind: "FULLY_DEDUCTIBLE"}, Explanation: "TEST_ONLY reviewable expense treatment", Citations: citation, Confidence: accountinganalysis.ConfidenceMedium},
	}}}}
	decisions, issues := accountinganalysis.ValidateUnified(input, proposal, []legislation.Fragment{fragment}, input.AccountCatalog)
	inputRaw, _ := json.Marshal(input)
	analysisID := "analysis-" + tc.clientID
	if _, err = tc.store.DB.ExecContext(tc.ctx, `INSERT INTO accounting_analysis_runs(id,client_id,invoice_id,invoice_revision,schema_version,prompt_version,provider,model,status,input_snapshot,retrieved_fragment_ids,approved_knowledge_ids,validation_results,started_at,command_key,classification_run_id) VALUES($1,$2,$3,$4,$5,$6,'GEMINI','TEST_ONLY','RUNNING',$7,'[]','[]','[]',$8,$9,$10)`, analysisID, tc.clientID, invoiceID, item.Revision, accountinganalysis.SchemaVersion, accountinganalysis.PromptVersion, inputRaw, tc.now, analysisID+":command", *item.CurrentClassificationRunID); err != nil {
		t.Fatal(err)
	}
	if err = tc.store.CompleteAnalysis(tc.ctx, analysisID, accountinganalysis.ProviderResult{Proposal: proposal, Provider: "GEMINI", Model: "TEST_ONLY"}, decisions, issues, tc.now); err != nil {
		t.Fatal(err)
	}
	run, err := tc.store.GetAnalysis(tc.ctx, tc.clientID, invoiceID, analysisID)
	if err != nil || run.Status != "PARTIAL_VALIDATION" {
		t.Fatalf("expected partial aggregate status: %#v %v", run, err)
	}
	item, err = tc.store.GetInvoice(tc.ctx, invoiceID)
	if err != nil {
		t.Fatal(err)
	}
	var accountDecision, vatDecision classification.Decision
	for _, decision := range item.Lines[0].Classifications {
		switch decision.Dimension {
		case classification.DimensionAccount:
			accountDecision = decision
		case classification.DimensionVATreatment:
			vatDecision = decision
		}
	}
	if accountDecision.Source != classification.SourceAIProposal || accountDecision.ProposedTypedValue == nil || accountDecision.ProposedTypedValue.Account != "628" || len(accountDecision.ValidationResults) == 0 || accountDecision.ValidationResults[0].Code != "ACCOUNT_NOT_POSTABLE" {
		t.Fatalf("invalid AI account proposal was not preserved: %#v", accountDecision)
	}
	if vatDecision.Source != classification.SourceAIProposal || len(vatDecision.ValidationResults) != 0 || vatDecision.ProposedTypedValue == nil {
		t.Fatalf("valid VAT sibling was discarded: %#v", vatDecision)
	}
	if _, err = classifier.Review(tc.ctx, classification.ReviewCommand{Action: "EDIT", InvoiceID: invoiceID, TaskID: item.ActiveTask.ID, ClassificationID: accountDecision.ID, ExpectedInvoiceRevision: item.Revision, ExpectedTaskRevision: item.ActiveTask.Revision, ExpectedClassificationRevision: accountDecision.Revision, TypedValue: &accounting.Value{Kind: "ACCOUNT", Account: "6281"}, Reason: "TEST_ONLY accountant correction", MappingAction: "OCCURRENCE_ONLY", CommandID: analysisID + ":review", ActorDisplay: "TEST_ONLY accountant"}); err != nil {
		t.Fatal(err)
	}
	item, err = tc.store.GetInvoice(tc.ctx, invoiceID)
	if err != nil {
		t.Fatal(err)
	}
	for _, decision := range item.Lines[0].Classifications {
		if decision.Dimension == classification.DimensionAccount {
			if decision.ProposedTypedValue == nil || decision.ProposedTypedValue.Account != "628" || decision.TypedValue == nil || decision.TypedValue.Account != "6281" || decision.EffectiveSource == nil || *decision.EffectiveSource != "MANUAL" || len(decision.ValidationResults) == 0 || !decision.HumanReviewed {
				t.Fatalf("proposal/correction audit was lost: %#v", decision)
			}
			return
		}
	}
	t.Fatal("ACCOUNT classification missing after review")
}

func TestAutomaticAccountingWorkflowCreatesOneTaskAndCompletesThroughEditAndApproveAll(t *testing.T) {
	tc := newModule5TestContext(t)
	facts, lineFacts, _, _ := domainReleaseFixture(t, tc, func(pack *accounting.Pack) {
		for index := range pack.Rules {
			pack.Rules[index].Predicate.SellerItemID = "NO_AUTOMATIC_MATCH"
		}
	})
	invoiceID := domainPersistedInvoice(t, tc, facts, lineFacts, "automatic-workflow")
	text := "contabilitate TVA deductibilitate servicii TEST_ONLY"
	digest := sha256.Sum256([]byte(text))
	hash := hex.EncodeToString(digest[:])
	sourceID := "source-" + invoiceID
	versionID := "version-" + invoiceID
	fragmentID := "fragment-" + invoiceID
	if _, err := tc.store.DB.ExecContext(tc.ctx, `INSERT INTO legislation_sources(id,kind,title,issuer,official_url,created_at) VALUES($1,'OTHER','TEST_ONLY workflow','TEST','https://example.invalid',$2)`, sourceID, tc.now); err != nil {
		t.Fatal(err)
	}
	if _, err := tc.store.DB.ExecContext(tc.ctx, `INSERT INTO legislation_versions(id,source_id,label,effective_from,content_hash,ingested_by,ingested_at,test_only) VALUES($1,$2,'TEST_ONLY','2020-01-01',$3,'test',$4,true)`, versionID, sourceID, hash, tc.now); err != nil {
		t.Fatal(err)
	}
	if _, err := tc.store.DB.ExecContext(tc.ctx, `INSERT INTO legislation_fragments(id,version_id,citation_key,heading,ordinal,content,content_hash) VALUES($1,$2,'TEST_ONLY art. C','',1,$3,$4)`, fragmentID, versionID, text, hash); err != nil {
		t.Fatal(err)
	}
	publisher := &capturedAnalysisPublisher{}
	workflow := accountinganalysis.NewWorkflowService(tc.store, publisher, workflowFakeAnalyzer{}, "FAKE_GEMINI", "TEST_ONLY", nil)
	classifier := classification.NewService(tc.store, classification.DomainPolicy{AllowTestOnly: true}, func() time.Time { return tc.now })
	classifier.SetAutomaticAccountingFallback(workflow)
	if _, _, err := classifier.ProcessInvoice(tc.ctx, classification.ProcessCommand{InvoiceID: invoiceID, ExpectedRevision: 1, CommandID: invoiceID + ":automatic"}); err != nil {
		t.Fatal(err)
	}
	item, err := tc.store.GetInvoice(tc.ctx, invoiceID)
	if err != nil {
		t.Fatal(err)
	}
	if publisher.calls != 1 || publisher.job.ClassificationRunID == "" || item.ActiveTask != nil {
		t.Fatalf("queued=%d job=%#v task=%#v", publisher.calls, publisher.job, item.ActiveTask)
	}
	foreignJob := publisher.job
	foreignJob.TenantID = "another-tenant"
	if err = workflow.Process(tc.ctx, foreignJob); err == nil {
		t.Fatal("tenant-mismatched job was accepted")
	}
	if err = workflow.Process(tc.ctx, publisher.job); err != nil {
		t.Fatal(err)
	}
	if err = workflow.Process(tc.ctx, publisher.job); err != nil {
		t.Fatal("worker replay must be an idempotent no-op", err)
	}
	item, err = tc.store.GetInvoice(tc.ctx, invoiceID)
	if err != nil || item.ActiveTask == nil {
		t.Fatalf("missing review task: %#v %v", item, err)
	}
	var invalid classification.Decision
	for _, decision := range item.Lines[0].Classifications {
		if decision.Dimension == classification.DimensionAccount {
			invalid = decision
		}
	}
	if len(invalid.ValidationResults) == 0 {
		t.Fatal("invalid 628 proposal not preserved")
	}
	var taskCount int
	if err = tc.store.DB.QueryRowContext(tc.ctx, `SELECT count(*) FROM validation_tasks WHERE classification_run_id=$1 AND task_type='CLASSIFICATION'`, *item.CurrentClassificationRunID).Scan(&taskCount); err != nil || taskCount != 1 {
		t.Fatalf("review task count=%d err=%v", taskCount, err)
	}
	staleExpected := []classification.ExpectedClassification{}
	for _, decision := range item.ActiveTask.ClassificationItems {
		if decision.Status == classification.ReviewPending && decision.ProposedTypedValue != nil && len(decision.ValidationResults) == 0 {
			staleExpected = append(staleExpected, classification.ExpectedClassification{ID: decision.ID, Revision: decision.Revision})
		}
	}
	staleExpected[0].Revision++
	if _, staleErr := classifier.ApproveAll(tc.ctx, classification.ApproveAllCommand{InvoiceID: invoiceID, TaskID: item.ActiveTask.ID, ExpectedInvoiceRevision: item.Revision, ExpectedTaskRevision: item.ActiveTask.Revision, Expected: staleExpected, CommandID: invoiceID + ":stale-approve-all", ActorDisplay: "TEST accountant"}); !errors.Is(staleErr, classification.ErrStaleReview) {
		t.Fatalf("expected atomic CAS conflict, got %v", staleErr)
	}
	if _, err = classifier.Review(tc.ctx, classification.ReviewCommand{Action: "EDIT", InvoiceID: invoiceID, TaskID: item.ActiveTask.ID, ClassificationID: invalid.ID, ExpectedInvoiceRevision: item.Revision, ExpectedTaskRevision: item.ActiveTask.Revision, ExpectedClassificationRevision: invalid.Revision, TypedValue: &accounting.Value{Kind: "ACCOUNT", Account: "6281"}, Reason: "TEST_ONLY correction", MappingAction: "OCCURRENCE_ONLY", CommandID: invoiceID + ":edit", ActorDisplay: "TEST accountant"}); err != nil {
		t.Fatal(err)
	}
	item, err = tc.store.GetInvoice(tc.ctx, invoiceID)
	if err != nil {
		t.Fatal(err)
	}
	expected := []classification.ExpectedClassification{}
	for _, decision := range item.ActiveTask.ClassificationItems {
		if decision.Status == classification.ReviewPending && decision.ProposedTypedValue != nil && len(decision.ValidationResults) == 0 {
			expected = append(expected, classification.ExpectedClassification{ID: decision.ID, Revision: decision.Revision})
		}
	}
	if _, err = classifier.ApproveAll(tc.ctx, classification.ApproveAllCommand{InvoiceID: invoiceID, TaskID: item.ActiveTask.ID, ExpectedInvoiceRevision: item.Revision, ExpectedTaskRevision: item.ActiveTask.Revision, Expected: expected, CommandID: invoiceID + ":approve-all", ActorDisplay: "TEST accountant"}); err != nil {
		t.Fatal(err)
	}
	item, err = tc.store.GetInvoice(tc.ctx, invoiceID)
	if err != nil || item.ActiveTask != nil || item.PipelineStatus != "READY_FOR_SAGA" {
		t.Fatalf("workflow not completed: %#v %v", item, err)
	}
}

func TestAutomaticAccountingWorkflowSkipsAIWhenRulesResolveEverything(t *testing.T) {
	tc := newModule5TestContext(t)
	facts, lineFacts, _, _ := domainReleaseFixture(t, tc, nil)
	invoiceID := domainPersistedInvoice(t, tc, facts, lineFacts, "automatic-zero-ai")
	publisher := &capturedAnalysisPublisher{}
	workflow := accountinganalysis.NewWorkflowService(tc.store, publisher, workflowFakeAnalyzer{}, "FAKE_GEMINI", "TEST_ONLY", nil)
	classifier := classification.NewService(tc.store, classification.DomainPolicy{AllowTestOnly: true}, func() time.Time { return tc.now })
	classifier.SetAutomaticAccountingFallback(workflow)
	if _, _, err := classifier.ProcessInvoice(tc.ctx, classification.ProcessCommand{InvoiceID: invoiceID, ExpectedRevision: 1, CommandID: invoiceID + ":classify"}); err != nil {
		t.Fatal(err)
	}
	if publisher.calls != 0 {
		t.Fatalf("fully resolved invoice queued %d AI jobs", publisher.calls)
	}
}
