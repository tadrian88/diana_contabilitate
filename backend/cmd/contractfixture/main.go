package main

import (
	"diana-contabilitate/backend/internal/contractingestion/fixtures"
	"flag"
	"os"
)

func main() {
	name := flag.String("name", "romanian", "synthetic fixture name")
	flag.Parse()
	_, _ = os.Stdout.Write(fixtures.PDF(*name))
}
