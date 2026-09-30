// Operator backfills for contracts confirmed before a clause could be settled
// without a reviewer:
//   - autoconfirm-source-text confirms pending VAT and payment-term clauses
//     whose value is stated unambiguously in the cited clause;
//   - close-covered-clauses closes contract-reference, identity and pricing
//     clauses the dossier's reference, parties' CUIs (supplier and client)
//     or reviewed service tariffs already cover.
//
// New confirmations do both automatically. Idempotent; prints only
// identifiers and counts.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"diana-contabilitate/backend/internal/platform/postgres"
)

const usage = "usage: contractrules autoconfirm-source-text | close-covered-clauses"

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "autoconfirm-source-text" && os.Args[1] != "close-covered-clauses") {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(2)
	}
	store, err := postgres.Open(databaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database connection failed")
		os.Exit(1)
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	list, apply, label := store.ConfirmedDocumentsWithSourceTextCandidates, store.AutoConfirmSourceTextClauses, "auto_confirmed"
	if os.Args[1] == "close-covered-clauses" {
		list, apply, label = store.ConfirmedDocumentsWithCoverableClauses, store.CloseCoveredClauses, "closed"
	}
	targets, err := list(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list documents failed: %v\n", err)
		os.Exit(1)
	}
	total := 0
	for _, target := range targets {
		count, err := apply(ctx, target.ClientID, target.DocumentID, target.ActorID, target.ActorDisplay, time.Now().UTC())
		if err != nil {
			fmt.Fprintf(os.Stderr, "document=%s failed: %v\n", target.DocumentID, err)
			os.Exit(1)
		}
		fmt.Printf("document=%s %s=%d\n", target.DocumentID, label, count)
		total += count
	}
	fmt.Printf("documents=%d %s=%d\n", len(targets), label, total)
}
