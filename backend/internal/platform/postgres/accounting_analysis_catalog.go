package postgres

import (
	"context"
	"fmt"

	"diana-contabilitate/backend/internal/accountinganalysis"
	"diana-contabilitate/backend/internal/accounts"
)

func (s *Store) LoadAccountingAnalysisCatalog(ctx context.Context, codes []string) (accountinganalysis.Catalog, error) {
	result := accountinganalysis.Catalog{Entries: map[string]accounts.Account{}, Children: map[string][]accounts.Account{}}
	_ = codes // The full catalog is snapshotted so an out-of-profile proposal can still be classified precisely.
	rows, err := s.DB.QueryContext(ctx, `SELECT code,name,account_type,is_synthetic,postable,is_active,COALESCE(parent_code,'') FROM accounts ORDER BY code`)
	if err != nil {
		return accountinganalysis.Catalog{}, fmt.Errorf("load accounting analysis account catalog: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item accounts.Account
		if err = rows.Scan(&item.Code, &item.Name, &item.AccountType, &item.Synthetic, &item.Postable, &item.Active, &item.ParentCode); err != nil {
			return accountinganalysis.Catalog{}, err
		}
		result.Entries[item.Code] = item
		if item.ParentCode != "" && item.Active && item.Postable {
			result.Children[item.ParentCode] = append(result.Children[item.ParentCode], item)
		}
	}
	return result, rows.Err()
}
