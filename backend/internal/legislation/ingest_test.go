package legislation

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"diana-contabilitate/backend/internal/accountingdate"
)

func TestManifestRequiresVerifiableFragmentAndVersionHashes(t *testing.T) {
	text := "TEST_ONLY reviewed source text"
	fragmentHash := sha256.Sum256([]byte(text))
	fragmentHashText := hex.EncodeToString(fragmentHash[:])
	versionHash := sha256.Sum256([]byte(fragmentHashText + "\n"))
	manifest := Manifest{
		Source:     Source{ID: "source", Kind: "LAW", Title: "Test", Issuer: "Test", Jurisdiction: "RO", OfficialURL: "https://example.invalid/source"},
		Version:    Version{ID: "source-v1", SourceID: "source", Label: "v1", EffectiveFrom: accountingdate.Date("2026-01-01"), ContentHash: hex.EncodeToString(versionHash[:])},
		Fragments:  []Fragment{{ID: "fragment", VersionID: "source-v1", CitationKey: "art. test", Text: text, ContentHash: fragmentHashText, Ordinal: 1}},
		IngestedBy: "test",
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	manifest.Fragments[0].Text = "tampered"
	if err := manifest.Validate(); err == nil {
		t.Fatal("tampered fragment accepted")
	}
}
