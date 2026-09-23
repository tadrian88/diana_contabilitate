// Command accountlearninge2e is test support for the isolated Playwright database.
// It invokes the real classification service and inspects persistence; it never
// creates mappings or seeds learned classification results.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"time"

	"diana-contabilitate/backend/ent"
	"diana-contabilitate/backend/ent/accountmapping"
	"diana-contabilitate/backend/ent/accountmappingversion"
	"diana-contabilitate/backend/ent/lineclassification"
	"diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/platform/postgres"
)

const (
	e2eDatabase = "diana_backend_e2e"
	clientID    = "TEST_ONLY-accounting-client"
	i2ID        = "inv-account-learning-i2"
)

type state struct {
	MappingCount        int    `json:"mappingCount"`
	MappingVersionCount int    `json:"mappingVersionCount"`
	CurrentVersion      int    `json:"currentVersion"`
	I2Source            string `json:"i2Source"`
	I2ReviewStatus      string `json:"i2ReviewStatus"`
}

func main() {
	action := flag.String("action", "state", "state or process-i2")
	flag.Parse()
	databaseURL := os.Getenv("DATABASE_URL")
	parsed, err := url.Parse(databaseURL)
	if os.Getenv("APP_ENV") != "test" || err != nil || parsed.Path != "/"+e2eDatabase {
		fmt.Fprintf(os.Stderr, "APP_ENV=test and DATABASE_URL targeting only %s are required\n", e2eDatabase)
		os.Exit(2)
	}
	store, err := postgres.Open(databaseURL)
	if err != nil {
		fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if *action == "process-i2" {
		invoice, err := store.GetInvoice(ctx, i2ID)
		if err != nil {
			fatal(err)
		}
		service := classification.NewService(store, classification.DomainPolicy{AllowTestOnly: true}, func() time.Time {
			return time.Date(2026, time.September, 15, 0, 5, 0, 0, time.UTC)
		})
		if _, changed, err := service.ProcessInvoice(ctx, classification.ProcessCommand{InvoiceID: i2ID, ExpectedRevision: invoice.Revision, CommandID: "TEST_ONLY:account-learning-e2e:i2"}); err != nil {
			fatal(fmt.Errorf("process I2: %w", err))
		} else if !changed {
			fatal(fmt.Errorf("process I2 did not persist a classification"))
		}
	} else if *action != "state" {
		fatal(fmt.Errorf("unsupported action %q", *action))
	}
	result, err := readState(ctx, store)
	if err != nil {
		fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fatal(err)
	}
}

func readState(ctx context.Context, store *postgres.Store) (state, error) {
	result := state{}
	mappings, err := store.Client.AccountMapping.Query().Where(accountmapping.ClientIDEQ(clientID)).All(ctx)
	if err != nil {
		return result, err
	}
	result.MappingCount = len(mappings)
	for _, mapping := range mappings {
		result.CurrentVersion = mapping.CurrentVersion
		count, err := store.Client.AccountMappingVersion.Query().Where(accountmappingversion.MappingIDEQ(mapping.ID)).Count(ctx)
		if err != nil {
			return result, err
		}
		result.MappingVersionCount += count
	}
	row, err := store.Client.LineClassification.Query().Where(
		lineclassification.InvoiceIDEQ(i2ID),
		lineclassification.DimensionEQ(lineclassification.DimensionACCOUNT),
	).Only(ctx)
	if err == nil {
		result.I2Source = string(row.Source)
		result.I2ReviewStatus = string(row.ReviewStatus)
	} else if !ent.IsNotFound(err) {
		return result, err
	}
	return result, nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
