package main

import "testing"

func TestDemoSeedingEnvironmentBoundary(t *testing.T) {
	for _, environment := range []string{"production", "PRODUCTION", "staging", "local-real", "", "unknown"} {
		if demoSeedingAllowed(environment) {
			t.Fatal(environment)
		}
	}
	for _, environment := range []string{"development", "test"} {
		if !demoSeedingAllowed(environment) {
			t.Fatal(environment)
		}
	}
}
