package ethutil

import (
	"fmt"
	"math/big"
	"strings"
)

// ParseUnits converts a human-readable decimal amount string to the token's base units
// (smallest denomination) as a *big.Int.
//
//	ParseUnits("1.5",  6)  → 1_500_000          (USDC)
//	ParseUnits("1.0", 18)  → 1_000_000_000_000_000_000 (ETH)
//
// Equivalent to viem's parseUnits(value, decimals).
func ParseUnits(human string, decimals int) (*big.Int, error) {
	if human == "" || decimals < 0 {
		return big.NewInt(0), nil
	}
	// Use big.Rat for exact decimal arithmetic — no float rounding.
	r, ok := new(big.Rat).SetString(human)
	if !ok {
		return nil, fmt.Errorf("ethutil.ParseUnits: invalid decimal amount %q", human)
	}

	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	r.Mul(r, new(big.Rat).SetInt(scale))

	// Round half-up to the nearest integer.
	num := r.Num()
	den := r.Denom()
	quo, rem := new(big.Int).QuoRem(num, den, new(big.Int))
	if rem.Sign() != 0 {
		two := new(big.Int).Mul(rem, big.NewInt(2))
		if two.CmpAbs(den) >= 0 {
			quo.Add(quo, big.NewInt(1))
		}
	}
	if quo.Sign() < 0 {
		return nil, fmt.Errorf("ethutil.ParseUnits: amount must be non-negative, got %q", human)
	}
	return quo, nil
}

// ParseUnitsString is like ParseUnits but returns the result as a decimal string.
// Use this when downstream code expects a string (e.g. JSON fields, API params).
func ParseUnitsString(human string, decimals int) (string, error) {
	n, err := ParseUnits(human, decimals)
	if err != nil {
		return "", err
	}
	return n.String(), nil
}

// FormatUnits converts a token amount in base units (smallest denomination) to a
// human-readable decimal string, trimming unnecessary trailing zeros.
//
//	FormatUnits("1500000",                  6)  → "1.5"
//	FormatUnits("1000000000000000000",     18)  → "1"
//	FormatUnits("2144958781785984",        18)  → "0.002144958781785984"
//
// Equivalent to viem's formatUnits(value, decimals).
// Uses math/big throughout — safe for any uint256 value.
func FormatUnits(baseUnits string, decimals int) (string, error) {
	if baseUnits == "" {
		return "0", nil
	}
	n, ok := new(big.Int).SetString(baseUnits, 10)
	if !ok {
		return "", fmt.Errorf("ethutil.FormatUnits: invalid base-units amount %q", baseUnits)
	}
	if decimals <= 0 {
		return n.String(), nil
	}

	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	whole := new(big.Int).Quo(n, scale)
	frac := new(big.Int).Rem(n, scale)
	if frac.Sign() < 0 {
		frac.Neg(frac)
	}
	if frac.Sign() == 0 {
		return whole.String(), nil
	}

	// Left-pad fraction to exactly `decimals` digits, then trim trailing zeros.
	fracStr := frac.String()
	for len(fracStr) < decimals {
		fracStr = "0" + fracStr
	}
	fracStr = strings.TrimRight(fracStr, "0")
	return whole.String() + "." + fracStr, nil
}

// MustFormatUnits is like FormatUnits but panics on invalid input.
// Only use in tests or when the input is guaranteed to be valid.
func MustFormatUnits(baseUnits string, decimals int) string {
	s, err := FormatUnits(baseUnits, decimals)
	if err != nil {
		panic(err)
	}
	return s
}
