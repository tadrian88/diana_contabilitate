package postgres

import (
	"context"
	"fmt"

	"diana-contabilitate/backend/internal/legislation"
)

// Retrieve uses dated, lexical full-text retrieval. The corpus is small and
// authoritative, so a vector database would add operational complexity without
// improving citation identity. Ranking never changes the effective-date guard.
func (s *Store) Retrieve(ctx context.Context, query legislation.Query) ([]legislation.Fragment, error) {
	if !query.ApplicableDate.Valid() || len(query.Terms) == 0 {
		return nil, fmt.Errorf("invalid legislation query")
	}
	limit := query.Limit
	if limit <= 0 || limit > 50 {
		limit = 12
	}
	terms := make([]string, 0, len(query.Terms))
	for _, term := range query.Terms {
		if term != "" {
			terms = append(terms, term)
		}
	}
	if len(terms) == 0 {
		return nil, fmt.Errorf("empty legislation query")
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT f.id,f.version_id,f.citation_key,f.heading,f.content,f.content_hash,f.ordinal
		FROM legislation_fragments f
		JOIN legislation_versions v ON v.id=f.version_id
		WHERE $1::date BETWEEN v.effective_from AND COALESCE(v.effective_to,'infinity'::date)
		  AND ($4::boolean OR NOT v.test_only)
		  AND EXISTS (SELECT 1 FROM unnest($2::text[]) q(term) WHERE f.search_vector @@ plainto_tsquery('simple',q.term))
		ORDER BY (SELECT max(ts_rank_cd(f.search_vector,plainto_tsquery('simple',q.term))) FROM unnest($2::text[]) q(term)) DESC,v.effective_from DESC,f.ordinal
		LIMIT $3`, string(query.ApplicableDate), terms, limit, query.AllowTestOnly)
	if err != nil {
		return nil, fmt.Errorf("retrieve legislation: %w", err)
	}
	defer rows.Close()
	result := []legislation.Fragment{}
	for rows.Next() {
		var item legislation.Fragment
		if err = rows.Scan(&item.ID, &item.VersionID, &item.CitationKey, &item.Heading, &item.Text, &item.ContentHash, &item.Ordinal); err != nil {
			return nil, err
		}
		if !item.Valid() {
			return nil, fmt.Errorf("corrupt legislation fragment %s", item.ID)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
