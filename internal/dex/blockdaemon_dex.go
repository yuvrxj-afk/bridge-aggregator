package dex

import (
	"context"
	"encoding/json"
	"fmt"

	"bridge-aggregator/internal/models"
)

// BlockdaemonDEXAdapter implements Adapter using Blockdaemon DeFi API for same-chain swaps.
// It aggregates quotes from multiple DEXs (Uniswap, PancakeSwap, Curve, etc.).
// See https://docs.blockdaemon.com/docs/defi-api-execute-a-local-swap
type BlockdaemonDEXAdapter struct {
	Client *BlockdaemonDEXClient
}

// NewBlockdaemonDEXAdapter creates a new Blockdaemon DEX adapter.
func NewBlockdaemonDEXAdapter(client *BlockdaemonDEXClient) *BlockdaemonDEXAdapter {
	return &BlockdaemonDEXAdapter{Client: client}
}

// ID returns the adapter identifier.
func (b *BlockdaemonDEXAdapter) ID() string { return "blockdaemon_dex" }

// Tier returns TierDegraded — Blockdaemon DEX may return quotes with missing tx data.
func (b *BlockdaemonDEXAdapter) Tier() models.AdapterTier { return models.TierDegraded }

// GetQuote returns the best DEX swap quote from Blockdaemon's aggregator.
func (b *BlockdaemonDEXAdapter) GetQuote(ctx context.Context, req QuoteRequest) (*Quote, error) {
	if b.Client == nil {
		return nil, fmt.Errorf("blockdaemon dex: client not configured")
	}
	if b.Client.APIKey == "" {
		return nil, fmt.Errorf("blockdaemon dex: not configured (missing api key)")
	}

	// Validate same-chain swap
	if req.TokenInChainID == 0 || req.TokenOutChainID == 0 {
		return nil, fmt.Errorf("blockdaemon dex: requires TokenInChainID and TokenOutChainID")
	}
	if req.TokenInChainID != req.TokenOutChainID {
		return nil, fmt.Errorf("blockdaemon dex: only supports same-chain swaps (got chainIn=%d chainOut=%d)", req.TokenInChainID, req.TokenOutChainID)
	}

	// Validate required fields
	if req.TokenIn == "" || req.TokenOut == "" {
		return nil, fmt.Errorf("blockdaemon dex: requires TokenIn and TokenOut addresses")
	}
	if req.Amount == "" {
		return nil, fmt.Errorf("blockdaemon dex: requires Amount")
	}

	// Call Blockdaemon API
	result, err := b.Client.GetSwapQuotes(ctx, int64(req.TokenInChainID), req.TokenIn, req.TokenOut, req.Amount)
	if err != nil {
		return nil, err
	}

	// Calculate fee from gas estimate (approximate)
	fee := "0"
	if result.EstimatedGas != "" {
		fee = result.EstimatedGas
	}

	return &Quote{
		DEXID:                 "blockdaemon_dex",
		EstimatedOutputAmount: result.AmountOut,
		EstimatedFeeAmount:    fee,
		ProviderQuote:         result.ProviderData,
		Routing:               fmt.Sprintf("%s (via %s)", result.DEXName, result.DEXID),
	}, nil
}

// CreateSwapTx builds a transaction for executing the swap.
// This is called during step transaction population.
func (b *BlockdaemonDEXAdapter) CreateSwapTx(ctx context.Context, req QuoteRequest, providerQuote json.RawMessage) (map[string]interface{}, error) {
	if b.Client == nil {
		return nil, fmt.Errorf("blockdaemon dex: client not configured")
	}

	// Parse the stored provider quote
	var quoteData struct {
		TransactionData map[string]interface{} `json:"transactionData"`
	}
	if err := json.Unmarshal(providerQuote, &quoteData); err != nil {
		return nil, fmt.Errorf("blockdaemon dex: failed to parse provider quote: %w", err)
	}

	if quoteData.TransactionData == nil {
		return nil, fmt.Errorf("blockdaemon dex: no transaction data in provider quote")
	}

	return quoteData.TransactionData, nil
}
