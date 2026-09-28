package accounts

import (
	"context"
	"errors"
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
	ParentCode          string `json:"parentCode,omitempty"`
}

type ValidationCode string

const (
	AccountNotFound          ValidationCode = "ACCOUNT_NOT_FOUND"
	AccountInactive          ValidationCode = "ACCOUNT_INACTIVE"
	AccountNotPostable       ValidationCode = "ACCOUNT_NOT_POSTABLE"
	AccountNotAllowedProfile ValidationCode = "ACCOUNT_NOT_ALLOWED_BY_PROFILE"
)

type ValidationIssue struct {
	Code              ValidationCode `json:"code"`
	AccountCode       string         `json:"accountCode"`
	Message           string         `json:"message"`
	SuggestedAccounts []Account      `json:"suggestedAccounts,omitempty"`
}

func (i ValidationIssue) Error() string { return i.Message }
func (i ValidationIssue) Unwrap() error {
	if i.Code == AccountNotFound {
		return apperrors.ErrNotFound
	}
	return apperrors.ErrValidation
}

// ValidatePostingAccount is the canonical domain definition for an account
// that can be used as the final debit/credit account of an entry.
func ValidatePostingAccount(item *Account, code string, allowed func(string) bool) error {
	code = strings.TrimSpace(code)
	if item == nil {
		return ValidationIssue{Code: AccountNotFound, AccountCode: code, Message: "Contul " + code + " nu există în planul de conturi."}
	}
	if !item.Active {
		return ValidationIssue{Code: AccountInactive, AccountCode: code, Message: "Contul " + code + " este inactiv."}
	}
	if !item.Postable {
		return ValidationIssue{Code: AccountNotPostable, AccountCode: code, Message: "Contul " + code + " este sintetic sau nepostabil; selectează un cont analitic postabil."}
	}
	if allowed != nil && !allowed(code) {
		return ValidationIssue{Code: AccountNotAllowedProfile, AccountCode: code, Message: "Contul " + code + " nu este permis de profilul clientului."}
	}
	return nil
}

func AsValidationIssue(err error) (ValidationIssue, bool) {
	var issue ValidationIssue
	return issue, errors.As(err, &issue)
}

type Store interface {
	SearchAccounts(context.Context, string, int) ([]Account, error)
	GetSelectableAccount(context.Context, string) (Account, error)
}

type ValidationStore interface {
	LookupAccount(context.Context, string) (*Account, error)
	PostableChildren(context.Context, string) ([]Account, error)
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
	code = strings.TrimSpace(code)
	if store, ok := s.store.(ValidationStore); ok {
		item, err := store.LookupAccount(ctx, code)
		if err != nil {
			return Account{}, err
		}
		if err = ValidatePostingAccount(item, code, nil); err != nil {
			if issue, valid := AsValidationIssue(err); valid && issue.Code == AccountNotPostable {
				issue.SuggestedAccounts, _ = store.PostableChildren(ctx, code)
				return Account{}, issue
			}
			return Account{}, err
		}
		return *item, nil
	}
	return s.store.GetSelectableAccount(ctx, code)
}
