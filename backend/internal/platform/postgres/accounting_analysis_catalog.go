package postgres

import (
	"context"
	"fmt"

	"diana-contabilitate/backend/internal/accountinganalysis"
)

func (s *Store) LoadAccountingAnalysisCatalog(ctx context.Context, codes []string) (accountinganalysis.Catalog, error) {
	result := accountinganalysis.Catalog{}
	if len(codes) == 0 {
		return result, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT code,postable AND is_active FROM accounts WHERE code=ANY($1::text[])`, codes)
	if err != nil {
		return nil, fmt.Errorf("load accounting analysis account catalog: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		var allowed bool
		if err = rows.Scan(&code, &allowed); err != nil {
			return nil, err
		}
		result[code] = allowed
	}
	return result, rows.Err()
}
