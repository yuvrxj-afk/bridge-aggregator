package ethutil

import (
	"fmt"
	"math/big"
	"strings"
)

const maxUint256Digits = 78

var maxUint256 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

// ParseStrictUint256 parses s as an unsigned base-10 integer string in [0, 2^256-1].
// It rejects signs, decimals, scientific notation, whitespace, and non-digits.
func ParseStrictUint256(s string) (*big.Int, error) {
	if s == "" {
		return nil, fmt.Errorf("value is required")
	}
	if s != strings.TrimSpace(s) {
		return nil, fmt.Errorf("value must not contain surrounding whitespace")
	}
	if len(s) > maxUint256Digits {
		return nil, fmt.Errorf("value exceeds uint256 decimal length")
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return nil, fmt.Errorf("value must be an unsigned decimal integer string")
		}
	}
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("failed to parse integer value")
	}
	if n.Cmp(maxUint256) > 0 {
		return nil, fmt.Errorf("value exceeds uint256 max")
	}
	return n, nil
}

// ValidatePositiveUint256String validates s as an unsigned uint256 integer > 0.
func ValidatePositiveUint256String(s string) error {
	n, err := ParseStrictUint256(s)
	if err != nil {
		return err
	}
	if n.Sign() <= 0 {
		return fmt.Errorf("value must be greater than zero")
	}
	return nil
}
