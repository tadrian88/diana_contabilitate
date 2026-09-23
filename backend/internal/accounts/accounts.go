package accounts

import (
	"context"
	"strings"

	"diana-contabilitate/backend/internal/apperrors"
)

type Account struct {
	Code                string `json:"code"`
	Name                string `json:"name"`
	AccountType         string `json:"accountType"`
	CatalogScope        string `json:"catalogScope"`
	RegulatoryReference string `json:"regulatoryReference,omitempty"`
	Synthetic           bool   `json:"synthetic"`
	Postable            bool   `json:"postable"`
	Active              bool   `json:"active"`
}

type Store interface {
	SearchAccounts(context.Context, string, int) ([]Account, error)
	GetSelectableAccount(context.Context, string) (Account, error)
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) Search(ctx context.Context, query string, limit int) ([]Account, error) {
	query = strings.TrimSpace(query)
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	return s.store.SearchAccounts(ctx, query, limit)
}

func (s *Service) RequireSelectable(ctx context.Context, code string) (Account, error) {
	if strings.TrimSpace(code) == "" {
		return Account{}, apperrors.ErrValidation
	}
	return s.store.GetSelectableAccount(ctx, strings.TrimSpace(code))
}
