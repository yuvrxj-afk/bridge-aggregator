package dex

import (
	"context"
	"encoding/json"

	"bridge-aggregator/internal/ethutil"
)

// QuoteRequest represents a simple on-chain swap quote request on a single chain.
type QuoteRequest struct {
	// New (preferred) shape: explicit chain IDs + token addresses, no internal token registry required.
	TokenInChainID  int    `json:"tokenInChainId"`
	TokenOutChainID int    `json:"tokenOutChainId"`
	TokenIn         string `json:"tokenIn"`
	TokenOut        string `json:"tokenOut"`

	// Amount is in base units (e.g. wei for ETH, 6-decimals for USDC).
	Amount  string `json:"amount"`
	Swapper string `json:"swapper,omitempty"`

	// Backward-compat fields (deprecated; will be removed). Kept so older callers don't break,
	// but without a token registry we can't reliably translate them.
	Chain     string `json:"chain,omitempty"`
	FromAsset string `json:"from_asset,omitempty"`
	ToAsset   string `json:"to_asset,omitempty"`
}

// Quote is the response for a DEX swap quote.
type Quote struct {
	DEXID                 string `json:"dex_id"`
	EstimatedOutputAmount string `json:"estimated_output_amount"`
	EstimatedFeeAmount    string `json:"estimated_fee_amount"`

	// Optional metadata for step transaction population.
	ProviderQuote json.RawMessage `json:"provider_quote,omitempty"`
	PermitData    json.RawMessage `json:"permit_data,omitempty"`
	Routing       string          `json:"routing,omitempty"`
}

// Adapter is the DEX adapter interface. Each implementation returns a swap Quote.
type Adapter interface {
	ID() string
	GetQuote(ctx context.Context, req QuoteRequest) (*Quote, error)
}

// IsValidEVMAddress is an alias for ethutil.IsAddress kept for internal dex package use.
var IsValidEVMAddress = ethutil.IsAddress

