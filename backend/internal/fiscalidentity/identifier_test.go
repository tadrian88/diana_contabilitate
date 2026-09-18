package fiscalidentity

import "testing"

func TestRomanianIdentityComparison(t *testing.T) {
	for _, pair := range [][2]string{{"21592770", "RO21592770"}, {"RO21592770", "ro 21592770"}, {"RO 21592770", "21592770"}} {
		if !SameRomanian(pair[0], pair[1]) {
			t.Fatalf("expected same identity: %q %q", pair[0], pair[1])
		}
	}
	for _, pair := range [][2]string{{"21592770", "21592771"}, {"DE21592770", "RO21592770"}, {"RO00000000", "00000000"}} {
		if SameRomanian(pair[0], pair[1]) {
			t.Fatalf("unexpected same identity: %q %q", pair[0], pair[1])
		}
	}
}

func TestForeignIdentifierIsConservative(t *testing.T) {
	if got := ForComparison(" de-12 34 ", "DE"); got != "DE-12 34" {
		t.Fatalf("got %q", got)
	}
}
