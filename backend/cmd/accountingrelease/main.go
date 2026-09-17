// accountingrelease consumes reviewed JSON from stdin. It has no test-only mode
// and no public HTTP endpoint. DATABASE_URL selects the operator's target DB.
package main

import (
	"context"
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/platform/postgres"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 2 || (os.Args[1] != "profile" && os.Args[1] != "pack" && os.Args[1] != "rule") {
		return fmt.Errorf("usage: accountingrelease profile|rule|pack < reviewed.json")
	}
	store, err := postgres.Open(os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer store.Close()
	decoder := json.NewDecoder(os.Stdin)
	decoder.DisallowUnknownFields()
	decode := func(v any) error {
		if err := decoder.Decode(v); err != nil {
			return err
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return fmt.Errorf("trailing JSON")
		}
		return nil
	}
	if os.Args[1] == "rule" {
		var r postgres.ReviewedAccountingRule
		if err := decode(&r); err != nil {
			return err
		}
		return store.InstallReviewedAccountingRule(context.Background(), r)
	}
	if os.Args[1] == "profile" {
		var p accounting.Profile
		if err := decode(&p); err != nil {
			return err
		}
		return store.ApproveAccountingProfile(context.Background(), p)
	}
	var p accounting.Pack
	if err := decode(&p); err != nil {
		return err
	}
	return store.ReleaseAccountingPack(context.Background(), p)
}
