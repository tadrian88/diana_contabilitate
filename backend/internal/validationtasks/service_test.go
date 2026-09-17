package validationtasks

import (
	"context"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/audit"
)

type captureStore struct{ definition CreationDefinition }

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
