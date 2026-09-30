package postgres

import "testing"

func TestAccountCodesFingerprintIsOrderIndependentAndChangeSensitive(t *testing.T) {
	a := accountCodesFingerprint([]string{"6022", "628", "401"})
	if a != accountCodesFingerprint([]string{"401", "6022", "628"}) {
		t.Fatal("fingerprint depends on order")
	}
	if a == accountCodesFingerprint([]string{"401", "6022"}) {
		t.Fatal("removed account not detected")
	}
	input := []string{"b", "a"}
	_ = accountCodesFingerprint(input)
	if input[0] != "b" {
		t.Fatal("caller slice mutated")
	}
}
