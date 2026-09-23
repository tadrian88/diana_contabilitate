// Command legislationdraftimport imports the converted, unreviewed snapshots
// exclusively into a local TEST_ONLY corpus. It is not a legal activation path.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/legislation"
	"diana-contabilitate/backend/internal/platform/postgres"
)

type snapshot struct {
	FormatVersion string `json:"formatVersion"`
	Importable    bool   `json:"importable"`
	Source        struct {
		ID               string  `json:"id"`
		Kind             string  `json:"kind"`
		Title            string  `json:"title"`
		Issuer           string  `json:"issuer"`
		Jurisdiction     string  `json:"jurisdiction"`
		OfficialURL      *string `json:"officialUrl"`
		SourceFileSHA256 string  `json:"sourceFileSha256"`
	} `json:"source"`
	Articles []struct {
		CitationKey string `json:"citationKey"`
		Heading     string `json:"heading"`
		Text        string `json:"text"`
		ContentHash string `json:"contentHash"`
		Ordinal     int    `json:"ordinal"`
	} `json:"articles"`
	Fragments []struct {
		CitationKey string `json:"citationKey"`
		Heading     string `json:"heading"`
		Text        string `json:"text"`
		ContentHash string `json:"contentHash"`
		Ordinal     int    `json:"ordinal"`
	} `json:"fragments"`
}

func main() {
	file := flag.String("file", "", "converted draft snapshot JSON")
	date := flag.String("effective-from", "", "TEST_ONLY retrieval start date; not a statement of legal effectiveness")
	accept := flag.Bool("accept-test-only", false, "acknowledge unverified legal dates and content")
	validateOnly := flag.Bool("validate-only", false, "validate and preview without database writes")
	flag.Parse()
	if !*accept || *file == "" || *date == "" {
		fatal("-file, -effective-from and -accept-test-only are required")
	}
	env := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if env != "development" && env != "test" && env != "local-real" {
		fatal("APP_ENV must be development, test or local-real")
	}
	effective, err := accountingdate.Parse(*date)
	if err != nil {
		fatal(err.Error())
	}
	raw, err := os.ReadFile(*file)
	if err != nil {
		fatal(err.Error())
	}
	var item snapshot
	if err := json.Unmarshal(raw, &item); err != nil {
		fatal(err.Error())
	}
	if item.FormatVersion != "DIANA_LEGISLATION_SOURCE_SNAPSHOT_V1" || item.Importable {
		fatal("expected non-importable draft snapshot")
	}
	url := ""
	if item.Source.OfficialURL != nil {
		url = *item.Source.OfficialURL
	}
	if url == "" && len(item.Source.SourceFileSHA256) == 64 {
		url = "local-sha256:" + item.Source.SourceFileSHA256
	}
	sourceID := item.Source.ID + "-local-draft"
	versionID := sourceID + "-TEST_ONLY-" + *date
	manifest := legislation.Manifest{
		Source:     legislation.Source{ID: sourceID, Kind: item.Source.Kind, Title: "TEST_ONLY " + item.Source.Title, Issuer: item.Source.Issuer, Jurisdiction: item.Source.Jurisdiction, OfficialURL: url},
		Version:    legislation.Version{ID: versionID, SourceID: sourceID, Label: "TEST_ONLY draft " + *date, EffectiveFrom: effective, TestOnly: true},
		IngestedBy: "local-draft-import",
	}
	fragments := item.Fragments
	if len(fragments) == 0 {
		fragments = item.Articles
	}
	h := sha256.New()
	for _, fragment := range fragments {
		manifest.Fragments = append(manifest.Fragments, legislation.Fragment{ID: fmt.Sprintf("%s-%d", versionID, fragment.Ordinal), VersionID: versionID, CitationKey: fragment.CitationKey, Heading: fragment.Heading, Text: fragment.Text, ContentHash: fragment.ContentHash, Ordinal: fragment.Ordinal})
		_, _ = h.Write([]byte(fragment.ContentHash + "\n"))
	}
	manifest.Version.ContentHash = hex.EncodeToString(h.Sum(nil))
	if err := manifest.Validate(); err != nil {
		fatal(fmt.Sprintf("draft snapshot failed integrity validation: %v", err))
	}
	if *validateOnly {
		fmt.Printf("Validated TEST_ONLY %s (%d fragments); no database writes.\n", versionID, len(manifest.Fragments))
		return
	}
	if os.Getenv("DATABASE_URL") == "" {
		fatal("DATABASE_URL is required")
	}
	store, err := postgres.Open(os.Getenv("DATABASE_URL"))
	if err != nil {
		fatal(err.Error())
	}
	defer store.Close()
	if err := store.IngestLegislation(context.Background(), manifest); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("Imported TEST_ONLY %s (%d fragments); effective-from is a local test filter only.\n", versionID, len(manifest.Fragments))
}

func fatal(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
