package validationtasks

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/audit"
)

type captureStore struct {
	definition CreationDefinition
	waiver     *ContinueWithoutContractCommand
}

func (s *captureStore) ListValidationTasks(context.Context, Filter) ([]InboxItem, error) {
	return nil, nil
}

func (s *captureStore) CreateBlockingTask(_ context.Context, command CreationCommand, definition CreationDefinition, _ time.Time) (*Task, bool, error) {
	s.definition = definition
	return &Task{ID: command.ID, Type: definition.TaskType, Status: StatusOpen}, true, nil
}

func (s *captureStore) RequestMissingContract(context.Context, RequestMissingContractCommand, time.Time) (*Task, bool, error) {
	return nil, false, nil
}

func (s *captureStore) ContinueWithoutContract(_ context.Context, command ContinueWithoutContractCommand, _ time.Time) (*Task, bool, error) {
	s.waiver = &command
	return &Task{ID: command.TaskID, Type: TypeMissingContract, Status: StatusResolved}, true, nil
}

func TestContinueWithoutContractRequiresReasonedCommand(t *testing.T) {
	valid := ContinueWithoutContractCommand{InvoiceID: "invoice", TaskID: "task", ExpectedRevision: 2, Reason: "  Achiziție punctuală fără contract  ", CommandID: "command", ActorID: "user", ActorDisplay: "Contabil"}
	invalid := map[string]func(*ContinueWithoutContractCommand){
		"missing invoice":  func(c *ContinueWithoutContractCommand) { c.InvoiceID = "" },
		"missing task":     func(c *ContinueWithoutContractCommand) { c.TaskID = "" },
		"missing revision": func(c *ContinueWithoutContractCommand) { c.ExpectedRevision = 0 },
		"missing command":  func(c *ContinueWithoutContractCommand) { c.CommandID = "" },
		"missing actor":    func(c *ContinueWithoutContractCommand) { c.ActorDisplay = "" },
		"blank reason":     func(c *ContinueWithoutContractCommand) { c.Reason = "          " },
		"short reason":     func(c *ContinueWithoutContractCommand) { c.Reason = " fără ctr " },
		"long reason":      func(c *ContinueWithoutContractCommand) { c.Reason = strings.Repeat("ă", 501) },
	}
	for name, mutate := range invalid {
		t.Run(name, func(t *testing.T) {
			store := &captureStore{}
			command := valid
			mutate(&command)
			if _, _, err := NewService(store, nil).ContinueWithoutContract(context.Background(), command); !errors.Is(err, apperrors.ErrValidation) || store.waiver != nil {
				t.Fatalf("err=%v store called=%v", err, store.waiver != nil)
			}
		})
	}
	store := &captureStore{}
	if _, changed, err := NewService(store, nil).ContinueWithoutContract(context.Background(), valid); err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if store.waiver == nil || store.waiver.Reason != "Achiziție punctuală fără contract" {
		t.Fatalf("stored waiver=%+v", store.waiver)
	}
	boundary := valid
	boundary.Reason = strings.Repeat("ș", ContractWaiverReasonMaxRunes)
	if _, _, err := NewService(&captureStore{}, nil).ContinueWithoutContract(context.Background(), boundary); err != nil {
		t.Fatalf("500 runes must be accepted: %v", err)
	}
}

func TestDomainCreationCapabilitiesOwnInvoiceBlockingSemantics(t *testing.T) {
	tests := []struct {
		name string
		call func(*Service, context.Context, CreationCommand) (*Task, bool, error)
		want CreationDefinition
	}{
		{"missing contract", (*Service).CreateMissingContractBlock, CreationDefinition{TaskType: TypeMissingContract, InvoiceFrom: InvoiceMatching, InvoiceTo: InvoiceAwaitingContract}},
		{"contract review", (*Service).CreateContractReviewBlock, CreationDefinition{TaskType: TypeContractMatch, InvoiceFrom: InvoiceMatching, InvoiceTo: InvoiceAwaitingMatchConfirm}},
		{"classification review", (*Service).CreateClassificationReviewBlock, CreationDefinition{TaskType: TypeClassification, InvoiceFrom: InvoiceClassified, InvoiceTo: InvoiceAwaitingReview}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &captureStore{}
			service := NewService(store, nil)
			_, _, err := test.call(service, context.Background(), CreationCommand{ID: "task", InvoiceID: "invoice", ExpectedInvoiceRevision: 1, Title: "Title", Reason: "Reason", CommandID: "command", Actor: audit.ActorSystem})
			if err != nil || store.definition != test.want {
				t.Fatalf("definition=%+v want=%+v err=%v", store.definition, test.want, err)
			}
		})
	}
}
