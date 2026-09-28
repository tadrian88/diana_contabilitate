package accountinganalysis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/legislation"
)

type Run struct {
	ID                  string            `json:"id"`
	ClientID            string            `json:"clientId"`
	InvoiceID           string            `json:"invoiceId"`
	InvoiceRevision     uint64            `json:"invoiceRevision"`
	ClassificationRunID string            `json:"classificationRunId,omitempty"`
	ContextStale        bool              `json:"contextStale"`
	Status              string            `json:"status"`
	Provider            string            `json:"provider"`
	Model               string            `json:"model"`
	Proposal            *Proposal         `json:"proposal,omitempty"`
	ValidationIssues    []ValidationIssue `json:"validationIssues"`
	Review              *Review           `json:"review,omitempty"`
	StartedAt           time.Time         `json:"startedAt"`
	CompletedAt         *time.Time        `json:"completedAt,omitempty"`
}

var ErrMissingFiscalProfile = errors.New("missing fiscal profile")
var ErrNoAnalysisNeeded = errors.New("no accounting analysis needed")

type Review struct {
	ID, Action, Reason, ActorDisplay string
	FinalDecision                    *Proposal
	CreatedAt                        time.Time
}

type RequestCommand struct {
	ClientID, InvoiceID, CommandID, ActorID, ActorDisplay string
}

type ReviewCommand struct {
	ClientID, InvoiceID, AnalysisID, Action, Reason, CommandID, ActorID, ActorDisplay string
	FinalDecision                                                                     *Proposal
}

type Execution struct {
	Run       Run
	Input     Input
	Fragments []legislation.Fragment
	Approved  []ApprovedKnowledge
	Accounts  Catalog
}

type WorkflowStore interface {
	PrepareAnalysis(context.Context, RequestCommand, string, string, time.Time) (Run, bool, error)
	GetAnalysis(context.Context, string, string, string) (Run, error)
	LatestAnalysis(context.Context, string, string) (Run, error)
	LoadAnalysisExecution(context.Context, string) (Execution, error)
	CompleteAnalysis(context.Context, string, ProviderResult, []ValidatedDecision, []ValidationIssue, time.Time) error
	SkipAnalysis(context.Context, string, string, time.Time) error
	FailAnalysis(context.Context, string, string, time.Time) error
	OpenManualReview(context.Context, string, string, string, time.Time) error
	ReviewAnalysis(context.Context, ReviewCommand, time.Time) (Run, error)
}

type AnalysisPublisher interface {
	PublishAccountingAnalysis(context.Context, AnalysisJob) error
}

type AnalysisJob struct {
	TenantID            string `json:"tenantId"`
	InvoiceID           string `json:"invoiceId"`
	ClassificationRunID string `json:"classificationRunId"`
	AnalysisRunID       string `json:"analysisRunId"`
}

type WorkflowService struct {
	store           WorkflowStore
	publisher       AnalysisPublisher
	analyzer        Analyzer
	provider, model string
	observer        Observer
	now             func() time.Time
}

func NewWorkflowService(store WorkflowStore, publisher AnalysisPublisher, analyzer Analyzer, provider, model string, observer Observer) *WorkflowService {
	return &WorkflowService{store: store, publisher: publisher, analyzer: analyzer, provider: provider, model: model, observer: observer, now: func() time.Time { return time.Now().UTC() }}
}

func (s *WorkflowService) Request(ctx context.Context, command RequestCommand) (Run, error) {
	if s == nil || s.store == nil || s.publisher == nil || command.ClientID == "" || command.InvoiceID == "" || command.CommandID == "" || command.ActorDisplay == "" {
		return Run{}, fmt.Errorf("%w: invalid accounting analysis request", apperrors.ErrValidation)
	}
	run, _, err := s.store.PrepareAnalysis(ctx, command, s.provider, s.model, s.now())
	if err != nil {
		return Run{}, err
	}
	if run.Status == "RUNNING" {
		if err = s.publisher.PublishAccountingAnalysis(ctx, AnalysisJob{TenantID: run.ClientID, InvoiceID: run.InvoiceID, ClassificationRunID: run.ClassificationRunID, AnalysisRunID: run.ID}); err != nil {
			if failErr := s.store.FailAnalysis(context.WithoutCancel(ctx), run.ID, "QUEUE_PUBLISH_FAILED", s.now()); failErr != nil {
				return Run{}, fmt.Errorf("publish analysis: %v; manual fallback: %w", err, failErr)
			}
			return s.store.GetAnalysis(ctx, run.ClientID, run.InvoiceID, run.ID)
		}
		if observer, ok := s.observer.(interface{ AccountingAIJobQueued() }); ok {
			observer.AccountingAIJobQueued()
		}
	}
	return run, nil
}

// EnsureAutomatic is the classification pipeline adapter. Its stable command
// key makes both analysis-run creation and Asynq enqueue idempotent.
func (s *WorkflowService) EnsureAutomatic(ctx context.Context, clientID, invoiceID, commandID string) error {
	_, err := s.Request(ctx, RequestCommand{ClientID: clientID, InvoiceID: invoiceID, CommandID: "automatic:" + commandID, ActorID: "system", ActorDisplay: "Sistem clasificare"})
	if errors.Is(err, ErrNoAnalysisNeeded) {
		return nil
	}
	if err != nil && !IsRetryable(err) {
		return s.store.OpenManualReview(ctx, clientID, invoiceID, FailureCode(err), s.now())
	}
	return err
}
func (s *WorkflowService) Get(ctx context.Context, clientID, invoiceID string) (Run, error) {
	return s.store.LatestAnalysis(ctx, clientID, invoiceID)
}
func (s *WorkflowService) Process(ctx context.Context, job AnalysisJob) error {
	execution, err := s.store.LoadAnalysisExecution(ctx, job.AnalysisRunID)
	if err != nil {
		return err
	}
	if execution.Run.ClientID != job.TenantID || execution.Run.InvoiceID != job.InvoiceID || execution.Run.ClassificationRunID != job.ClassificationRunID {
		return &ProviderError{Code: "JOB_IDENTITY_MISMATCH", Err: fmt.Errorf("analysis job does not match immutable run identity")}
	}
	if execution.Run.Status != "RUNNING" {
		return nil
	}
	if execution.Run.ContextStale {
		return s.store.SkipAnalysis(ctx, job.AnalysisRunID, "SUPERSEDED_CLASSIFICATION_RUN", s.now())
	}
	if !HasDimensionsNeedingAI(execution.Input) {
		return s.store.SkipAnalysis(ctx, job.AnalysisRunID, "NO_UNRESOLVED_DIMENSIONS", s.now())
	}
	engine := NewService(staticCorpus{items: execution.Fragments}, s.analyzer, execution.Accounts, s.observer)
	result, decisions, issues, err := engine.Analyze(ctx, execution.Input, execution.Approved)
	if err != nil {
		return err
	}
	if err = s.store.CompleteAnalysis(ctx, job.AnalysisRunID, result, decisions, issues, s.now()); err != nil {
		return err
	}
	if observer, ok := s.observer.(interface{ AccountingAIJobSucceeded(bool) }); ok {
		observer.AccountingAIJobSucceeded(len(issues) > 0)
	}
	return nil
}
func (s *WorkflowService) Review(ctx context.Context, command ReviewCommand) (Run, error) {
	if !oneOf(command.Action, "APPROVE", "EDIT", "REJECT") || command.CommandID == "" || command.ActorDisplay == "" || command.AnalysisID == "" {
		return Run{}, fmt.Errorf("%w: invalid accounting analysis review", apperrors.ErrValidation)
	}
	if command.Action == "REJECT" && command.FinalDecision != nil {
		return Run{}, fmt.Errorf("%w: rejection cannot contain final decision", apperrors.ErrValidation)
	}
	if command.Action == "APPROVE" && command.FinalDecision != nil {
		return Run{}, fmt.Errorf("%w: approval uses the stored proposal", apperrors.ErrValidation)
	}
	if command.Action == "EDIT" && command.FinalDecision == nil {
		return Run{}, fmt.Errorf("%w: edited review requires final decision", apperrors.ErrValidation)
	}
	if (command.Action == "EDIT" || command.Action == "REJECT") && command.Reason == "" {
		return Run{}, fmt.Errorf("%w: edit and rejection require a reason", apperrors.ErrValidation)
	}
	return s.store.ReviewAnalysis(ctx, command, s.now())
}

func (s *WorkflowService) Fail(ctx context.Context, runID, reason string) error {
	err := s.store.FailAnalysis(ctx, runID, reason, s.now())
	if err == nil {
		if observer, ok := s.observer.(interface{ AccountingAIJobExhausted() }); ok {
			observer.AccountingAIJobExhausted()
		}
	}
	return err
}

type staticCorpus struct{ items []legislation.Fragment }

func (c staticCorpus) Retrieve(context.Context, legislation.Query) ([]legislation.Fragment, error) {
	return c.items, nil
}

var ErrAnalysisNotFound = errors.New("accounting analysis not found")
