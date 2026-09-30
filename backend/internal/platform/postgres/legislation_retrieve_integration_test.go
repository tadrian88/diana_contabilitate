//go:build integration

package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/legislation"
)

func TestRetrieveSelectsFragmentsByExactCitationKey(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	store, err := Open(url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	suffix := fmt.Sprintf("%d", now.UnixNano())
	sourceID, versionID := "source-keys-"+suffix, "version-keys-"+suffix
	// Legislation rows are immutable; unique citation keys keep reruns independent.
	keyOf := func(account string) string { return "TEST_ONLY " + suffix + " contul " + account }
	if _, err = store.DB.ExecContext(ctx, `INSERT INTO legislation_sources(id,kind,title,issuer,official_url,created_at) VALUES($1,'ORDER','TEST_ONLY OMFP','TEST','https://example.invalid',$2)`, sourceID, now); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.ExecContext(ctx, `INSERT INTO legislation_versions(id,source_id,label,effective_from,content_hash,ingested_by,ingested_at,test_only) VALUES($1,$2,'TEST_ONLY','2020-01-01',$3,'test',$4,true)`, versionID, sourceID, fmt.Sprintf("%064x", now.UnixNano()), now); err != nil {
		t.Fatal(err)
	}
	for ordinal, account := range []string{"622", "623", "628"} {
		text := fmt.Sprintf("Contul %s TEST_ONLY funcțiunea contului servicii %s", account, suffix)
		digest := sha256.Sum256([]byte(text))
		if _, err = store.DB.ExecContext(ctx, `INSERT INTO legislation_fragments(id,version_id,citation_key,heading,ordinal,content,content_hash) VALUES($1,$2,$3,'',$4,$5,$6)`,
			"fragment-keys-"+suffix+"-"+account, versionID, keyOf(account), ordinal+1, text, hex.EncodeToString(digest[:])); err != nil {
			t.Fatal(err)
		}
	}

	byKeys, err := store.Retrieve(ctx, legislation.Query{ApplicableDate: "2026-06-01", AllowTestOnly: true, Kinds: []string{"ORDER"}, Limit: 10,
		CitationKeys: []string{keyOf("622"), keyOf("628"), keyOf("999")}})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, fragment := range byKeys {
		got = append(got, fragment.CitationKey)
	}
	sort.Strings(got)
	if len(got) != 2 || got[0] != keyOf("622") || got[1] != keyOf("628") {
		t.Fatalf("citation keys without terms returned %v", got)
	}

	withTerm, err := store.Retrieve(ctx, legislation.Query{ApplicableDate: "2026-06-01", AllowTestOnly: true, Limit: 10,
		CitationKeys: []string{keyOf("622"), keyOf("623")}, Terms: []string{"Contul 623"}})
	if err != nil || len(withTerm) != 1 || withTerm[0].CitationKey != keyOf("623") {
		t.Fatalf("keys and terms must both apply: %+v err=%v", withTerm, err)
	}

	if _, err = store.Retrieve(ctx, legislation.Query{ApplicableDate: "2026-06-01", AllowTestOnly: true}); err == nil {
		t.Fatal("a query without terms and without citation keys must be rejected")
	}
	if production, err := store.Retrieve(ctx, legislation.Query{ApplicableDate: "2026-06-01", CitationKeys: []string{keyOf("622")}}); err != nil || len(production) != 0 {
		t.Fatalf("TEST_ONLY fragments must stay hidden without AllowTestOnly: %+v err=%v", production, err)
	}
}
