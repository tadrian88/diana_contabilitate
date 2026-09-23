package invoicing

import (
	"context"
	"errors"
)

type Reader interface {
	GetInvoice(context.Context, string) (*Invoice, error)
}

type Lister interface {
	ListInvoices(context.Context, Filter) ([]Invoice, error)
}

type Service struct {
	reader     Reader
	lister     Lister
	pipeline   PipelineStore
	exporter   SagaExporter
	matcher    ContractMatchingProcessor
	commercial CommercialValidationProcessor
	classifier ClassificationProcessor
	clock      Clock
}

func (s *Service) SetContractMatchingProcessor(processor ContractMatchingProcessor) {
	s.matcher = processor
}

func (s *Service) SetCommercialValidationProcessor(processor CommercialValidationProcessor) {
	s.commercial = processor
}

func (s *Service) SetClassificationProcessor(processor ClassificationProcessor) {
	s.classifier = processor
}

func NewService(reader Reader) *Service {
	service := &Service{reader: reader}
	if lister, ok := reader.(Lister); ok {
		service.lister = lister
	}
	return service
}

func (s *Service) Get(ctx context.Context, id string) (*Invoice, error) {
	return s.reader.GetInvoice(ctx, id)
}

func (s *Service) List(ctx context.Context, filter Filter) ([]Invoice, error) {
	if s.lister == nil {
		return nil, errors.New("invoice list reader is not configured")
	}
	return s.lister.ListInvoices(ctx, filter)
}
