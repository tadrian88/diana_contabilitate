package money

import "testing"

func TestAmountKeepsExactDecimalRepresentation(t *testing.T) {
	amount, err := Parse("123456789012345.6789")
	if err != nil {
		t.Fatal(err)
	}
	if amount.String() != "123456789012345.6789" {
		t.Fatalf("amount changed: %s", amount)
	}
}

func TestAmountRejectsInvalidInput(t *testing.T) {
	for _, value := range []string{"not-money", "1/3", "+1", "01.20"} {
		if _, err := Parse(value); err == nil {
			t.Fatalf("expected %q to fail", value)
		}
	}
}

func TestAmountEqualIgnoresDecimalScale(t *testing.T) {
	if !MustParse("100").Equal(MustParse("100.0000")) {
		t.Fatal("equivalent exact decimals should compare equal")
	}
	if MustParse("100.0001").Equal(MustParse("100.0000")) {
		t.Fatal("different exact decimals should not compare equal")
	}
}
