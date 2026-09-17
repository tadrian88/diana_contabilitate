package clients

import (
	"context"
	"diana-contabilitate/backend/internal/platform/requestactor"
)

type Reader interface {
	ListClients(context.Context) ([]Client, error)
}

type Service struct{ reader Reader }

func NewService(reader Reader) *Service { return &Service{reader: reader} }

func (s *Service) List(ctx context.Context) ([]Client, error) {
	items, err := s.reader.ListClients(ctx)
	if err != nil {
		return nil, err
	}
	actor, ok := requestactor.FromContext(ctx)
	// Direct internal readers retain legacy compatibility; HTTP always supplies an actor.
	if !ok {
		return items, nil
	}
	result := make([]Client, 0, len(items))
	for _, c := range items {
		if Allows(actor, c.ID) {
			result = append(result, c)
		}
	}
	return result, nil
}
