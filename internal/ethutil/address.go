// Package ethutil provides lightweight Ethereum utility helpers.
// It is the Go equivalent of validation helpers like viem's isAddress().
package ethutil

import (
	"strings"

	"golang.org/x/crypto/sha3"
)

// IsAddress reports whether addr is a valid EVM address.
//
// A valid address is a 0x-prefixed string of exactly 40 hex characters (42 total).
// Both checksummed (EIP-55 mixed-case) and non-checksummed forms are accepted.
// Placeholder strings like "0xYourWallet" return false.
//
// This is the direct Go equivalent of viem's isAddress(addr).
func IsAddress(addr string) bool {
	if len(addr) != 42 {
		return false
	}
	if addr[0] != '0' || (addr[1] != 'x' && addr[1] != 'X') {
		return false
	}
	for _, c := range addr[2:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// ChecksumAddress returns addr in EIP-55 mixed-case checksum format.
// Returns the input unchanged if it is not a valid address.
// Equivalent to viem's getAddress(addr).
func ChecksumAddress(addr string) string {
	lower := strings.ToLower(addr)
	if !IsAddress(lower) {
		return addr
	}
	hex := lower[2:]

	h := sha3.NewLegacyKeccak256() // Keccak-256 (not SHA3-256) — same as go-ethereum
	h.Write([]byte(hex))
	hash := h.Sum(nil)

	result := make([]byte, 42)
	result[0] = '0'
	result[1] = 'x'
	for i := 0; i < 40; i++ {
		c := hex[i]
		// Each hex nibble of the address maps to one nibble of the hash.
		// Uppercase the address char if the corresponding hash nibble >= 8.
		hashByte := hash[i/2]
		var hashNibble byte
		if i%2 == 0 {
			hashNibble = hashByte >> 4
		} else {
			hashNibble = hashByte & 0x0f
		}
		if c >= 'a' && c <= 'f' && hashNibble >= 8 {
			result[i+2] = c - 32 // to uppercase
		} else {
			result[i+2] = c
		}
	}
	return string(result)
}
