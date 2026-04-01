package bridges

import (
	"fmt"
	"strings"

	"bridge-aggregator/internal/ethutil"
	"bridge-aggregator/internal/models"
)

type resolvedBridgeInput struct {
	ChainID  ChainID
	ChainKey string // lowercased name when available (e.g. "ethereum")

	Symbol   string // uppercase when available (display)
	Token    TokenInfo
	HasToken bool
}

func firstNonEmptyString(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// IsValidEVMAddress is an alias for ethutil.IsAddress kept for internal bridge package use.
var IsValidEVMAddress = ethutil.IsAddress

// resolveBridgeEndpoint prefers address-first inputs (chain_id + token_address/decimals),
// and falls back to chain+asset using the token registry for backward compatibility.
func resolveBridgeEndpoint(ep models.Endpoint) (resolvedBridgeInput, error) {
	var out resolvedBridgeInput

	// Address-first.
	if ep.ChainID != 0 && ep.TokenAddress != "" {
		out.ChainID = ChainID(ep.ChainID)
		out.ChainKey = strings.ToLower(ep.Chain)
		out.Symbol = strings.ToUpper(ep.Asset)
		out.Token = TokenInfo{
			Address:  ep.TokenAddress,
			Decimals: ep.TokenDecimals,
		}
		out.HasToken = true
		if out.Token.Decimals == 0 {
			return out, fmt.Errorf("token_decimals required when using token_address (chain_id=%d token_address=%s)", ep.ChainID, ep.TokenAddress)
		}
		return out, nil
	}

	// Back-compat: derive from symbol registry.
	chainKey := strings.ToLower(ep.Chain)
	if chainKey == "" {
		return out, fmt.Errorf("chain is required (or chain_id + token_address)")
	}
	chainID, ok := ChainNameToID[chainKey]
	if !ok {
		return out, fmt.Errorf("unsupported chain: %s", ep.Chain)
	}
	asset := strings.ToUpper(ep.Asset)
	if asset == "" {
		return out, fmt.Errorf("asset is required (or chain_id + token_address)")
	}
	reg := TokenByChainAndSymbol[chainID]
	if reg == nil {
		return out, fmt.Errorf("token registry missing for chain: %s", ep.Chain)
	}
	tok, ok := reg[asset]
	if !ok {
		return out, fmt.Errorf("unsupported asset %s on %s", asset, ep.Chain)
	}

	out.ChainID = chainID
	out.ChainKey = chainKey
	out.Symbol = asset
	out.Token = tok
	out.HasToken = true
	return out, nil
}

// isNativeETH returns true if addr is the canonical zero-address or EeeE sentinel used
// to represent native ETH in bridge and DEX APIs.
func isNativeETH(addr string) bool {
	a := strings.ToLower(strings.TrimSpace(addr))
	return a == "0x0000000000000000000000000000000000000000" ||
		a == "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
}

func resolveAmountBaseUnits(req models.QuoteRequest, decimals int) (string, error) {
	if req.AmountBaseUnits != "" {
		return req.AmountBaseUnits, nil
	}
	if req.Amount == "" {
		return "", fmt.Errorf("amount or amount_base_units required")
	}
	return ethutil.ParseUnitsString(req.Amount, decimals)
}

