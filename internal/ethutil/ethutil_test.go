package ethutil_test

import (
	"math/big"
	"testing"

	"bridge-aggregator/internal/ethutil"
)

// ── ParseUnits ────────────────────────────────────────────────────────────────

func TestParseUnits(t *testing.T) {
	cases := []struct {
		name     string
		human    string
		decimals int
		want     string // base-units decimal string
		wantErr  bool
	}{
		// Basic USDC (6 decimals)
		{"usdc whole", "5", 6, "5000000", false},
		{"usdc decimal", "1.5", 6, "1500000", false},
		{"usdc fractional", "0.000001", 6, "1", false},
		{"usdc max fraction", "0.999999", 6, "999999", false},

		// ETH (18 decimals) — the int64 overflow case our old code got wrong
		{"eth 1", "1", 18, "1000000000000000000", false},
		{"eth 0.01", "0.01", 18, "10000000000000000", false},
		{"eth 10", "10", 18, "10000000000000000000", false}, // > int64 max in wei
		{"eth 100", "100", 18, "100000000000000000000", false},
		// 1000 ETH — was silently overflowing int64 in old toHumanAmount
		{"eth 1000", "1000", 18, "1000000000000000000000", false},

		// WBTC (8 decimals)
		{"wbtc 0.5", "0.5", 8, "50000000", false},
		{"wbtc 1", "1", 8, "100000000", false},

		// Edge cases
		{"zero", "0", 18, "0", false},
		{"empty", "", 6, "0", false},
		{"no decimals", "42", 0, "42", false},
		{"invalid input", "not-a-number", 6, "", true},
		{"negative", "-1", 6, "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ethutil.ParseUnitsString(tc.human, tc.decimals)
			if tc.wantErr {
				if err == nil {
					t.Errorf("ParseUnitsString(%q, %d): expected error, got %q", tc.human, tc.decimals, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseUnitsString(%q, %d): unexpected error: %v", tc.human, tc.decimals, err)
			}
			if got != tc.want {
				t.Errorf("ParseUnitsString(%q, %d) = %q, want %q", tc.human, tc.decimals, got, tc.want)
			}
		})
	}
}

func TestParseUnits_ReturnsBigInt(t *testing.T) {
	// Verify the *big.Int form is also correct and can handle uint256-sized values.
	// Max uint256 = 2^256 - 1.
	maxUint256Str := "115792089237316195423570985008687907853269984665640564039457584007913129639935"
	n, err := ethutil.ParseUnits(maxUint256Str, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n.String() != maxUint256Str {
		t.Errorf("max uint256: got %s", n.String())
	}

	// Sanity: 10 ETH in wei
	ten, _ := ethutil.ParseUnits("10", 18)
	want, _ := new(big.Int).SetString("10000000000000000000", 10)
	if ten.Cmp(want) != 0 {
		t.Errorf("10 ETH in wei: got %s, want %s", ten.String(), want.String())
	}
}

// ── FormatUnits ───────────────────────────────────────────────────────────────

func TestFormatUnits(t *testing.T) {
	cases := []struct {
		name      string
		baseUnits string
		decimals  int
		want      string
		wantErr   bool
	}{
		// USDC (6 decimals)
		{"usdc whole", "5000000", 6, "5", false},
		{"usdc decimal", "1500000", 6, "1.5", false},
		{"usdc min unit", "1", 6, "0.000001", false},
		{"usdc no trailing zeros", "1100000", 6, "1.1", false},

		// ETH (18 decimals) — these were silently wrong with int64 overflow
		{"eth 1", "1000000000000000000", 18, "1", false},
		{"eth 0.01", "10000000000000000", 18, "0.01", false},
		{"eth 10", "10000000000000000000", 18, "10", false},
		// This is the exact value from the Across bug we fixed: 2144958781785984 wei
		{"across real output", "2144958781785984", 18, "0.002144958781785984", false},
		// Large amount — was crashing with int64 before
		{"eth 1000", "1000000000000000000000", 18, "1000", false},

		// WBTC (8 decimals)
		{"wbtc 1", "100000000", 8, "1", false},
		{"wbtc 0.5", "50000000", 8, "0.5", false},

		// Edge cases
		{"zero", "0", 18, "0", false},
		{"empty", "", 6, "0", false},
		{"no decimals", "42", 0, "42", false},
		{"invalid input", "not-a-number", 6, "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ethutil.FormatUnits(tc.baseUnits, tc.decimals)
			if tc.wantErr {
				if err == nil {
					t.Errorf("FormatUnits(%q, %d): expected error, got %q", tc.baseUnits, tc.decimals, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("FormatUnits(%q, %d): unexpected error: %v", tc.baseUnits, tc.decimals, err)
			}
			if got != tc.want {
				t.Errorf("FormatUnits(%q, %d) = %q, want %q", tc.baseUnits, tc.decimals, got, tc.want)
			}
		})
	}
}

func TestParseFormatRoundTrip(t *testing.T) {
	// FormatUnits(ParseUnitsString(x, d), d) == x for exact decimal amounts.
	cases := []struct {
		human    string
		decimals int
	}{
		{"1.5", 6},
		{"0.000001", 6},
		{"1", 18},
		{"0.01", 18},
		{"0.002144958781785984", 18},
		{"100", 8},
	}
	for _, tc := range cases {
		base, err := ethutil.ParseUnitsString(tc.human, tc.decimals)
		if err != nil {
			t.Fatalf("ParseUnitsString(%q, %d): %v", tc.human, tc.decimals, err)
		}
		got, err := ethutil.FormatUnits(base, tc.decimals)
		if err != nil {
			t.Fatalf("FormatUnits(%q, %d): %v", base, tc.decimals, err)
		}
		if got != tc.human {
			t.Errorf("round-trip(%q, %d) = %q", tc.human, tc.decimals, got)
		}
	}
}

// ── IsAddress ─────────────────────────────────────────────────────────────────

func TestIsAddress(t *testing.T) {
	valid := []string{
		"0x0000000000000000000000000000000000000000",
		"0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045", // vitalik.eth (EIP-55 checksum)
		"0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48", // USDC (lowercase)
		"0xA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", // USDC (uppercase)
		"0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913", // USDC on Base
		"0xaf88d065e77c8cC2239327C5EDb3A432268e5831", // USDC on Arbitrum
	}

	// Note: 0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE is the "native ETH" sentinel
	// used in DeFi protocols but it is 21 bytes (42 hex chars), NOT a standard 20-byte
	// EVM address. IsAddress correctly rejects it.
	for _, addr := range valid {
		if !ethutil.IsAddress(addr) {
			t.Errorf("IsAddress(%q) = false, want true", addr)
		}
	}

	invalid := []string{
		"",
		"0xYourWallet", // placeholder
		"0x123",        // too short
		"0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb4",       // 41 chars
		"a0b86991c6218b36c1d19d4a2e9eb0ce3606eb48",        // no 0x prefix
		"0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb4g",      // invalid hex char 'g'
		"0x0000000000000000000000000000000000000000extra", // too long
		"not an address at all",
	}
	for _, addr := range invalid {
		if ethutil.IsAddress(addr) {
			t.Errorf("IsAddress(%q) = true, want false", addr)
		}
	}
}

// ── ChecksumAddress ───────────────────────────────────────────────────────────

func TestChecksumAddress(t *testing.T) {
	// EIP-55 reference values from the spec (https://eips.ethereum.org/EIPS/eip-55).
	cases := []struct {
		input string
		want  string
	}{
		{
			"0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed",
			"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
		},
		{
			"0xfb6916095ca1df60bb79ce92ce3ea74c37c5d359",
			"0xfB6916095ca1df60bB79Ce92cE3Ea74c37c5d359",
		},
		{
			"0xdbf03b407c01e7cd3cbea99509d93f8dddc8c6fb",
			"0xdbF03B407c01E7cD3CBea99509d93f8DDDC8C6FB",
		},
		{
			"0xd1220a0cf47c7b9be7a2e6ba89f429762e7b9adb",
			"0xD1220A0cf47c7B9Be7A2E6BA89F429762e7b9aDb",
		},
		// Already checksummed — should be a no-op
		{
			"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
			"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
		},
		// All-zero address is its own checksum
		{
			"0x0000000000000000000000000000000000000000",
			"0x0000000000000000000000000000000000000000",
		},
	}

	for _, tc := range cases {
		got := ethutil.ChecksumAddress(tc.input)
		if got != tc.want {
			t.Errorf("ChecksumAddress(%q)\n  got  %q\n  want %q", tc.input, got, tc.want)
		}
	}

	// Invalid input is returned unchanged.
	bad := "not-an-address"
	if got := ethutil.ChecksumAddress(bad); got != bad {
		t.Errorf("ChecksumAddress(%q) = %q, want unchanged", bad, got)
	}
}

func TestParseStrictUint256(t *testing.T) {
	valid := []string{
		"1",
		"5000000",
		"115792089237316195423570985008687907853269984665640564039457584007913129639935",
	}
	for _, v := range valid {
		if _, err := ethutil.ParseStrictUint256(v); err != nil {
			t.Fatalf("expected valid uint256 for %q: %v", v, err)
		}
	}

	invalid := []string{
		"",
		"0",
		"-1",
		"+1",
		"1e18",
		"1.5",
		"1/2",
		" 1",
		"1 ",
		"abc",
		"115792089237316195423570985008687907853269984665640564039457584007913129639936",
	}
	for _, v := range invalid {
		if err := ethutil.ValidatePositiveUint256String(v); err == nil {
			t.Fatalf("expected invalid uint256 for %q", v)
		}
	}
}
