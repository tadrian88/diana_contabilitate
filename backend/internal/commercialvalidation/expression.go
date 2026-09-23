package commercialvalidation

import (
	"fmt"
	"math/big"
	"strings"
)

var allowedOps = map[string]bool{
	"literal": true, "variable": true, "add": true, "multiply": true,
	"percent": true, "tier": true, "min": true, "max": true,
	"prorate": true, "fx": true, "round": true,
}

func ValidateExpression(expr Expression) error {
	count := 0
	return validateExpression(expr, 0, &count)
}

func validateExpression(expr Expression, depth int, count *int) error {
	*count = *count + 1
	if depth > 16 || *count > 128 {
		return fmt.Errorf("%w: expression complexity limit", ErrInvalidExpression)
	}
	if !allowedOps[expr.Op] {
		return fmt.Errorf("%w: unsupported op %q", ErrInvalidExpression, expr.Op)
	}
	if expr.Scale < 0 || expr.Scale > 8 {
		return fmt.Errorf("%w: invalid scale", ErrInvalidExpression)
	}
	switch expr.Op {
	case "literal":
		if _, ok := rat(expr.Value); !ok {
			return fmt.Errorf("%w: literal", ErrInvalidExpression)
		}
	case "variable":
		if strings.TrimSpace(expr.Variable) == "" {
			return fmt.Errorf("%w: variable", ErrInvalidExpression)
		}
	case "add":
		if len(expr.Args) < 1 {
			return fmt.Errorf("%w: add args", ErrInvalidExpression)
		}
	case "multiply", "percent", "prorate", "fx":
		if len(expr.Args) != 2 {
			return fmt.Errorf("%w: %s args", ErrInvalidExpression, expr.Op)
		}
	case "min", "max":
		if len(expr.Args) < 2 {
			return fmt.Errorf("%w: %s args", ErrInvalidExpression, expr.Op)
		}
	case "round":
		if len(expr.Args) != 1 {
			return fmt.Errorf("%w: round args", ErrInvalidExpression)
		}
	case "tier":
		if len(expr.Args) != 1 || len(expr.Tiers) == 0 || len(expr.Tiers) > 128 {
			return fmt.Errorf("%w: tier shape", ErrInvalidExpression)
		}
		var previous *big.Rat
		for index, tier := range expr.Tiers {
			if _, ok := rat(tier.Value); !ok {
				return fmt.Errorf("%w: tier value", ErrInvalidExpression)
			}
			if tier.UpTo == nil && index != len(expr.Tiers)-1 {
				return fmt.Errorf("%w: open tier must be last", ErrInvalidExpression)
			}
			if tier.UpTo != nil {
				limit, ok := rat(*tier.UpTo)
				if !ok || previous != nil && limit.Cmp(previous) <= 0 {
					return fmt.Errorf("%w: tier boundary", ErrInvalidExpression)
				}
				previous = limit
			}
		}
	}
	for _, arg := range expr.Args {
		if err := validateExpression(arg, depth+1, count); err != nil {
			return err
		}
	}
	return nil
}

func EvaluateExpression(expr Expression, variables map[string]string) (string, []string, error) {
	if err := ValidateExpression(expr); err != nil {
		return "", nil, err
	}
	value, missing, err := eval(expr, variables)
	if err != nil || len(missing) > 0 {
		return "", unique(missing), err
	}
	return decimal(value, expr.Scale), nil, nil
}

func eval(expr Expression, variables map[string]string) (*big.Rat, []string, error) {
	switch expr.Op {
	case "literal":
		value, _ := rat(expr.Value)
		return value, nil, nil
	case "variable":
		raw, ok := variables[expr.Variable]
		if !ok || strings.TrimSpace(raw) == "" {
			return nil, []string{expr.Variable}, nil
		}
		value, valid := rat(raw)
		if !valid {
			return nil, nil, fmt.Errorf("%w: variable %s", ErrInvalidExpression, expr.Variable)
		}
		return value, nil, nil
	}
	values := make([]*big.Rat, 0, len(expr.Args))
	var missing []string
	for _, arg := range expr.Args {
		value, absent, err := eval(arg, variables)
		if err != nil {
			return nil, nil, err
		}
		missing = append(missing, absent...)
		values = append(values, value)
	}
	if len(missing) > 0 {
		return nil, missing, nil
	}
	switch expr.Op {
	case "add":
		result := new(big.Rat)
		for _, value := range values {
			result.Add(result, value)
		}
		return result, nil, nil
	case "multiply", "prorate", "fx":
		return new(big.Rat).Mul(values[0], values[1]), nil, nil
	case "percent":
		return new(big.Rat).Quo(new(big.Rat).Mul(values[0], values[1]), big.NewRat(100, 1)), nil, nil
	case "min", "max":
		result := new(big.Rat).Set(values[0])
		for _, value := range values[1:] {
			if expr.Op == "min" && value.Cmp(result) < 0 || expr.Op == "max" && value.Cmp(result) > 0 {
				result.Set(value)
			}
		}
		return result, nil, nil
	case "round":
		return values[0], nil, nil
	case "tier":
		for _, tier := range expr.Tiers {
			if tier.UpTo == nil {
				value, _ := rat(tier.Value)
				return value, nil, nil
			}
			limit, _ := rat(*tier.UpTo)
			if values[0].Cmp(limit) <= 0 {
				value, _ := rat(tier.Value)
				return value, nil, nil
			}
		}
	}
	return nil, nil, ErrInvalidExpression
}

func rat(raw string) (*big.Rat, bool) {
	if strings.TrimSpace(raw) != raw || raw == "" || strings.HasPrefix(raw, "+") {
		return nil, false
	}
	value, ok := new(big.Rat).SetString(raw)
	return value, ok
}

func decimal(value *big.Rat, scale int) string {
	if scale == 0 {
		scale = 4
	}
	return value.FloatString(scale)
}

func unique(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
