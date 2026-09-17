package validationtasks

import (
	"context"
	"errors"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/audit"
)

const (
	InvoiceMatching             = "MATCHING"
	InvoiceAwaitingContract     = "AWAITING_CONTRACT"
	InvoiceAwaitingMatchConfirm = "AWAITING_MATCH_CONFIRM"
	InvoiceClassified           = "CLASSIFIED"
	InvoiceAwaitingReview       = "AWAITING_REVIEW"
)

type CreationCommand struct {
	ID                      string
	InvoiceID               string
	ExpectedInvoiceRevision uint64
	Title                   string
	Reason                  string
	BlockerCode             *string
	CommandID               string
	Actor                   audit.ActorKind
	ActorID                 string
	ActorDisplay            string
	CorrelationID           string
	ContractMatchRunID      string
}

type CreationDefinition struct {
	TaskType    Type
	InvoiceFrom string
	InvoiceTo   string
}

type RequestMissingContractCommand struct {
	InvoiceID        string
	TaskID           string
	ExpectedRevision uint64
	CommandID        string
	ActorID          string
	ActorDisplay     string
	CorrelationID    string
}

type Store interface {
	ListValidationTasks(context.Context, Filter) ([]InboxItem, error)
	CreateBlockingTask(context.Context, CreationCommand, CreationDefinition, time.Time) (*Task, bool, error)
	RequestMissingContract(context.Context, RequestMissingContractCommand, time.Time) (*Task, bool, error)
}

type Clock func() time.Time

type Service struct {
	store Store
	clock Clock
}

func NewService(store Store, clock Clock) *Service {
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &Service{store: store, clock: clock}
}

func (s *Service) List(ctx context.Context, filter Filter) ([]InboxItem, error) {
	if (filter.Type != nil && !ValidType(*filter.Type)) || (filter.Status != nil && !ValidStatus(*filter.Status)) {
		return nil, apperrors.ErrValidation
	}
	return s.store.ListValidationTasks(ctx, filter)
}

func (s *Service) CreateMissingContractBlock(ctx context.Context, command CreationCommand) (*Task, bool, error) {
	return s.create(ctx, command, CreationDefinition{TaskType: TypeMissingContract, InvoiceFrom: InvoiceMatching, InvoiceTo: InvoiceAwaitingContract})
}

func (s *Service) CreateContractReviewBlock(ctx context.Context, command CreationCommand) (*Task, bool, error) {
	return s.create(ctx, command, CreationDefinition{TaskType: TypeContractMatch, InvoiceFrom: InvoiceMatching, InvoiceTo: InvoiceAwaitingMatchConfirm})
}

func (s *Service) CreateClassificationReviewBlock(ctx context.Context, command CreationCommand) (*Task, bool, error) {
	return s.create(ctx, command, CreationDefinition{TaskType: TypeClassification, InvoiceFrom: InvoiceClassified, InvoiceTo: InvoiceAwaitingReview})
}

func (s *Service) create(ctx context.Context, command CreationCommand, definition CreationDefinition) (*Task, bool, error) {
	if command.InvoiceID == "" || command.ExpectedInvoiceRevision == 0 || command.Title == "" || command.Reason == "" || command.CommandID == "" || command.Actor == "" {
		return nil, false, apperrors.ErrValidation
	}
	return s.store.CreateBlockingTask(ctx, command, definition, s.clock())
}

func (s *Service) RequestMissingContract(ctx context.Context, command RequestMissingContractCommand) (*Task, bool, error) {
	if command.InvoiceID == "" || command.TaskID == "" || command.ExpectedRevision == 0 || command.CommandID == "" {
		return nil, false, apperrors.ErrValidation
	}
	return s.store.RequestMissingContract(ctx, command, s.clock())
}

var ErrTaskAlreadyResolved = errors.New("task already resolved")
