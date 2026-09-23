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
	ID               string            `json:"id"`
	ClientID         string            `json:"clientId"`
	InvoiceID        string            `json:"invoiceId"`
	InvoiceRevision  uint64            `json:"invoiceRevision"`
	Status           string            `json:"status"`
	Provider         string            `json:"provider"`
	Model            string            `json:"model"`
	Proposal         *Proposal         `json:"proposal,omitempty"`
	ValidationIssues []ValidationIssue `json:"validationIssues"`
	Review           *Review           `json:"review,omitempty"`
	StartedAt        time.Time         `json:"startedAt"`
	CompletedAt      *time.Time        `json:"completedAt,omitempty"`
}

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
	CompleteAnalysis(context.Context, string, ProviderResult, []ValidationIssue, time.Time) error
	FailAnalysis(context.Context, string, time.Time) error
	ReviewAnalysis(context.Context, ReviewCommand, time.Time) (Run, error)
}

type AnalysisPublisher interface {
	PublishAccountingAnalysis(context.Context, string) error
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
		if err = s.publisher.PublishAccountingAnalysis(ctx, run.ID); err != nil {
			return Run{}, err
		}
	}
	return run, nil
}
func (s *WorkflowService) Get(ctx context.Context, clientID, invoiceID string) (Run, error) {
	return s.store.LatestAnalysis(ctx, clientID, invoiceID)
}
func (s *WorkflowService) Process(ctx context.Context, runID string) error {
	execution, err := s.store.LoadAnalysisExecution(ctx, runID)
	if err != nil {
		return err
	}
	if execution.Run.Status != "RUNNING" {
		return nil
	}
	engine := NewService(staticCorpus{items: execution.Fragments}, s.analyzer, execution.Accounts, s.observer)
	result, issues, err := engine.Analyze(ctx, execution.Input, execution.Approved)
	if err != nil {
		return err
	}
	return s.store.CompleteAnalysis(ctx, runID, result, issues, s.now())
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

func (s *WorkflowService) Fail(ctx context.Context, runID string) error {
	return s.store.FailAnalysis(ctx, runID, s.now())
}

type staticCorpus struct{ items []legislation.Fragment }

func (c staticCorpus) Retrieve(context.Context, legislation.Query) ([]legislation.Fragment, error) {
	return c.items, nil
}

var ErrAnalysisNotFound = errors.New("accounting analysis not found")
