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

// 1800101420010 is a synthetic CNP with a valid control digit.
func TestCNPControlDigitAndPlaceholder(t *testing.T) {
	for _, raw := range []string{"1800101420010", " 1800101420010 ", "0000000000000"} {
		if _, ok := CNP(raw); !ok {
			t.Fatalf("expected valid CNP: %q", raw)
		}
	}
	for _, raw := range []string{"1800101420011", "0800101420010", "180010142001", "RO1800101420010", "26999270"} {
		if _, ok := CNP(raw); ok {
			t.Fatalf("unexpected valid CNP: %q", raw)
		}
	}
}

func TestClassifyCustomerIdentifier(t *testing.T) {
	for _, tc := range []struct {
		raw, country string
		kind         Kind
		normalized   string
	}{
		{"RO26999270", "RO", KindCUI, "26999270"},
		{"31482767", "RO", KindCUI, "31482767"},
		{"1800101420010", "RO", KindCNP, "1800101420010"},
		{" de 123 456 ", "DE", KindOther, "DE 123 456"},
	} {
		kind, normalized := Classify(tc.raw, tc.country)
		if kind != tc.kind || normalized != tc.normalized {
			t.Fatalf("Classify(%q) = %s %q, want %s %q", tc.raw, kind, normalized, tc.kind, tc.normalized)
		}
	}
}

func TestMaskHidesOnlyCNP(t *testing.T) {
	if got := Mask(KindCNP, "1800101420010"); got != "180***" {
		t.Fatalf("got %q", got)
	}
	if got := MaskIfCNP("1800101420010"); got != "180***" {
		t.Fatalf("got %q", got)
	}
	if got := Mask(KindCUI, "26999270"); got != "26999270" {
		t.Fatalf("got %q", got)
	}
	if got := MaskIfCNP("RO26999270"); got != "RO26999270" {
		t.Fatalf("got %q", got)
	}
}
