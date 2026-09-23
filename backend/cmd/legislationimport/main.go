package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"diana-contabilitate/backend/internal/legislation"
	"diana-contabilitate/backend/internal/platform/postgres"
)

func main() {
	file := flag.String("file", "", "path to a reviewed legislation JSON manifest")
	flag.Parse()
	if strings.TrimSpace(*file) == "" {
		fatal("-file is required")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		fatal("DATABASE_URL is required")
	}
	raw, err := os.ReadFile(*file)
	if err != nil {
		fatal(err.Error())
	}
	var manifest legislation.Manifest
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&manifest); err != nil {
		fatal(err.Error())
	}
	if decoder.Decode(new(any)) != io.EOF {
		fatal("trailing JSON value")
	}
	store, err := postgres.Open(databaseURL)
	if err != nil {
		fatal(err.Error())
	}
	defer store.Close()
	if err = store.IngestLegislation(context.Background(), manifest); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("ingested legislation version %s with %d fragments\n", manifest.Version.ID, len(manifest.Fragments))
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
