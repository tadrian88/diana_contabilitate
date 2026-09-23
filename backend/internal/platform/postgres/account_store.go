package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"diana-contabilitate/backend/internal/accounts"
	"diana-contabilitate/backend/internal/apperrors"
)

func (s *Store) SearchAccounts(ctx context.Context, query string, limit int) ([]accounts.Account, error) {
	pattern := "%" + strings.TrimSpace(query) + "%"
	rows, err := s.DB.QueryContext(ctx, `
		SELECT code,name,account_type,is_synthetic,postable,is_active
		FROM accounts
		WHERE is_active AND postable AND ($1='' OR code ILIKE $2 OR name ILIKE $2)
		ORDER BY CASE WHEN code=$1 THEN 0 WHEN code ILIKE $1||'%' THEN 1 ELSE 2 END,code
		LIMIT $3`, strings.TrimSpace(query), pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]accounts.Account, 0)
	for rows.Next() {
		var item accounts.Account
		if err := rows.Scan(&item.Code, &item.Name, &item.AccountType, &item.Synthetic, &item.Postable, &item.Active); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) GetSelectableAccount(ctx context.Context, code string) (accounts.Account, error) {
	var item accounts.Account
	err := s.DB.QueryRowContext(ctx, `SELECT code,name,account_type,is_synthetic,postable,is_active FROM accounts WHERE code=$1`, code).
		Scan(&item.Code, &item.Name, &item.AccountType, &item.Synthetic, &item.Postable, &item.Active)
	if errors.Is(err, sql.ErrNoRows) {
		return accounts.Account{}, apperrors.ErrNotFound
	}
	if err != nil {
		return accounts.Account{}, err
	}
	if !item.Active || !item.Postable {
		return accounts.Account{}, apperrors.ErrValidation
	}
	return item, nil
}
