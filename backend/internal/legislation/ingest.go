package legislation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
)

var ErrInvalidManifest = errors.New("invalid legislation manifest")

type Manifest struct {
	Source     Source     `json:"source"`
	Version    Version    `json:"version"`
	Fragments  []Fragment `json:"fragments"`
	IngestedBy string     `json:"ingestedBy"`
}

type Ingester interface {
	IngestLegislation(context.Context, Manifest) error
}

func (m Manifest) Validate() error {
	if strings.TrimSpace(m.Source.ID) == "" || !manifestOneOf(m.Source.Kind, "LAW", "ORDER", "METHODOLOGICAL_NORMS", "OTHER") || strings.TrimSpace(m.Source.Title) == "" || strings.TrimSpace(m.Source.Issuer) == "" || strings.TrimSpace(m.Source.Jurisdiction) == "" || strings.TrimSpace(m.Source.OfficialURL) == "" || strings.TrimSpace(m.IngestedBy) == "" || (!m.Version.TestOnly && !strings.HasPrefix(m.Source.OfficialURL, "https://")) || (strings.HasPrefix(m.Source.OfficialURL, "local-sha256:") && !m.Version.TestOnly) {
		return ErrInvalidManifest
	}
	if m.Version.ID == "" || m.Version.SourceID != m.Source.ID || m.Version.Label == "" || !m.Version.EffectiveFrom.Valid() || m.Version.EffectiveTo != nil && (!m.Version.EffectiveTo.Valid() || *m.Version.EffectiveTo < m.Version.EffectiveFrom) || len(m.Fragments) == 0 {
		return ErrInvalidManifest
	}
	items := append([]Fragment(nil), m.Fragments...)
	sort.Slice(items, func(i, j int) bool { return items[i].Ordinal < items[j].Ordinal })
	seenID, seenCitation, seenOrdinal := map[string]bool{}, map[string]bool{}, map[int]bool{}
	h := sha256.New()
	for _, fragment := range items {
		if fragment.VersionID != m.Version.ID || fragment.Ordinal < 1 || !fragment.Valid() || seenID[fragment.ID] || seenCitation[fragment.CitationKey] || seenOrdinal[fragment.Ordinal] {
			return ErrInvalidManifest
		}
		seenID[fragment.ID], seenCitation[fragment.CitationKey], seenOrdinal[fragment.Ordinal] = true, true, true
		_, _ = h.Write([]byte(fragment.ContentHash))
		_, _ = h.Write([]byte("\n"))
	}
	if !strings.EqualFold(m.Version.ContentHash, hex.EncodeToString(h.Sum(nil))) {
		return ErrInvalidManifest
	}
	return nil
}

func manifestOneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}
