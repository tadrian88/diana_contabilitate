package postgres

import (
	"context"
	"fmt"
	"time"

	"diana-contabilitate/backend/internal/legislation"
)

func (s *Store) IngestLegislation(ctx context.Context, manifest legislation.Manifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, `INSERT INTO legislation_sources(id,kind,title,issuer,jurisdiction,official_url,created_at) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(id) DO NOTHING`, manifest.Source.ID, manifest.Source.Kind, manifest.Source.Title, manifest.Source.Issuer, manifest.Source.Jurisdiction, manifest.Source.OfficialURL, now)
	if err != nil {
		return fmt.Errorf("insert legislation source: %w", err)
	}
	var storedKind, storedTitle, storedIssuer, storedJurisdiction, storedURL string
	if err = tx.QueryRowContext(ctx, `SELECT kind,title,issuer,jurisdiction,official_url FROM legislation_sources WHERE id=$1`, manifest.Source.ID).Scan(&storedKind, &storedTitle, &storedIssuer, &storedJurisdiction, &storedURL); err != nil || storedKind != manifest.Source.Kind || storedTitle != manifest.Source.Title || storedIssuer != manifest.Source.Issuer || storedJurisdiction != manifest.Source.Jurisdiction || storedURL != manifest.Source.OfficialURL {
		return fmt.Errorf("legislation source identity conflicts with immutable source")
	}
	var effectiveTo any
	if manifest.Version.EffectiveTo != nil {
		effectiveTo = string(*manifest.Version.EffectiveTo)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO legislation_versions(id,source_id,label,effective_from,effective_to,test_only,content_hash,ingested_by,ingested_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, manifest.Version.ID, manifest.Version.SourceID, manifest.Version.Label, string(manifest.Version.EffectiveFrom), effectiveTo, manifest.Version.TestOnly, manifest.Version.ContentHash, manifest.IngestedBy, now)
	if err != nil {
		return fmt.Errorf("insert legislation version: %w", err)
	}
	for _, fragment := range manifest.Fragments {
		if _, err = tx.ExecContext(ctx, `INSERT INTO legislation_fragments(id,version_id,citation_key,heading,ordinal,content,content_hash) VALUES($1,$2,$3,$4,$5,$6,$7)`, fragment.ID, fragment.VersionID, fragment.CitationKey, fragment.Heading, fragment.Ordinal, fragment.Text, fragment.ContentHash); err != nil {
			return fmt.Errorf("insert legislation fragment: %w", err)
		}
	}
	return tx.Commit()
}
