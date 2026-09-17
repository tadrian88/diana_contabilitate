package money

import (
	"errors"
	"math/big"
	"regexp"
	"strings"
)

var ErrInvalidAmount = errors.New("invalid decimal amount")
var decimalPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`)

// Amount is an exact base-10 value. It crosses persistence boundaries as text
// and is converted to a JSON number only by the HTTP transport.
type Amount string

func Parse(raw string) (Amount, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", ErrInvalidAmount
	}
	if !decimalPattern.MatchString(value) {
		return "", ErrInvalidAmount
	}
	if _, ok := new(big.Rat).SetString(value); !ok {
		return "", ErrInvalidAmount
	}
	return Amount(value), nil
}

func MustParse(raw string) Amount {
	amount, err := Parse(raw)
	if err != nil {
		panic(err)
	}
	return amount
}

func (a Amount) String() string { return string(a) }

func (a Amount) Valid() bool {
	_, err := Parse(string(a))
	return err == nil
}

// Equal compares decimal values numerically, independently of their scale.
func (a Amount) Equal(other Amount) bool {
	left, leftOK := new(big.Rat).SetString(string(a))
	right, rightOK := new(big.Rat).SetString(string(other))
	return leftOK && rightOK && left.Cmp(right) == 0
}

type Money struct {
	Amount   Amount
	Currency string
}
