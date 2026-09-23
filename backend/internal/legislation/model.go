// Package legislation models immutable, dated and independently verifiable
// legal source fragments. It deliberately contains no LLM or accounting logic.
package legislation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"diana-contabilitate/backend/internal/accountingdate"
)

type Source struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Title        string `json:"title"`
	Issuer       string `json:"issuer"`
	Jurisdiction string `json:"jurisdiction"`
	OfficialURL  string `json:"officialUrl"`
}

type Version struct {
	ID            string               `json:"id"`
	SourceID      string               `json:"sourceId"`
	Label         string               `json:"label"`
	ContentHash   string               `json:"contentHash"`
	EffectiveFrom accountingdate.Date  `json:"effectiveFrom"`
	EffectiveTo   *accountingdate.Date `json:"effectiveTo,omitempty"`
	TestOnly      bool                 `json:"testOnly,omitempty"`
}

// Fragment is the smallest retrieval and citation unit. CitationKey is a
// stable human reference such as "art. 297 alin. (4) lit. a)"; ID and hash are
// the machine-verifiable identity used in persisted analyses.
type Fragment struct {
	ID          string `json:"id"`
	VersionID   string `json:"versionId"`
	CitationKey string `json:"citationKey"`
	Heading     string `json:"heading"`
	Text        string `json:"text"`
	ContentHash string `json:"contentHash"`
	Ordinal     int    `json:"ordinal"`
}

func (f Fragment) Valid() bool {
	if strings.TrimSpace(f.ID) == "" || strings.TrimSpace(f.VersionID) == "" || strings.TrimSpace(f.CitationKey) == "" || strings.TrimSpace(f.Text) == "" {
		return false
	}
	sum := sha256.Sum256([]byte(f.Text))
	return strings.EqualFold(f.ContentHash, hex.EncodeToString(sum[:]))
}

type Query struct {
	Terms          []string
	ApplicableDate accountingdate.Date
	Limit          int
	AllowTestOnly  bool
}

type Store interface {
	Retrieve(context.Context, Query) ([]Fragment, error)
}
