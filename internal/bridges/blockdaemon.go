package bridges

import (
	"context"
	"fmt"
	"strings"

	"bridge-aggregator/internal/models"
)

// BlockdaemonAdapter implements Adapter using Blockdaemon DeFi API (aggregated bridge quotes).
// See https://docs.blockdaemon.com/docs/defi-api-overview
type BlockdaemonAdapter struct {
	Client *BlockdaemonClient
}

// ID returns the bridge identifier for execute validation.
func (b BlockdaemonAdapter) ID() string {
	return "blockdaemon"
}

// GetQuote returns the best bridge quote from Blockdaemon's aggregator (Squid, Stargate, AllBridge, etc.).
func (b BlockdaemonAdapter) GetQuote(ctx context.Context, req models.QuoteRequest) (*models.Route, error) {
	if b.Client == nil {
		return nil, fmt.Errorf("blockdaemon: client not configured")
	}
	if b.Client.APIKey == "" {
		return nil, fmt.Errorf("blockdaemon: API key required (set blockdaemon_api_key)")
	}

	fromChain := strings.ToLower(req.Source.Chain)
	toChain := strings.ToLower(req.Destination.Chain)
	originChain, ok := ChainNameToID[fromChain]
	if !ok {
		return nil, fmt.Errorf("blockdaemon: unsupported source chain: %s", req.Source.Chain)
	}
	destChain, ok := ChainNameToID[toChain]
	if !ok {
		return nil, fmt.Errorf("blockdaemon: unsupported destination chain: %s", req.Destination.Chain)
	}

	originTokens := TokenByChainAndSymbol[originChain]
	destTokens := TokenByChainAndSymbol[destChain]
	if originTokens == nil || destTokens == nil {
		return nil, fmt.Errorf("blockdaemon: token registry missing for chain(s)")
	}

	fromSymbol := strings.ToUpper(req.Source.Asset)
	toSymbol := strings.ToUpper(req.Destination.Asset)
	inputToken, ok := originTokens[fromSymbol]
	if !ok {
		return nil, fmt.Errorf("blockdaemon: unsupported input asset %s on %s", fromSymbol, req.Source.Chain)
	}
	if _, ok := destTokens[toSymbol]; !ok {
		return nil, fmt.Errorf("blockdaemon: unsupported output asset %s on %s", toSymbol, req.Destination.Chain)
	}

	amountSmallest, err := HumanToSmallest(req.Amount, inputToken.Decimals)
	if err != nil {
		return nil, fmt.Errorf("blockdaemon: invalid amount: %w", err)
	}

	q, err := b.Client.GetBridgeQuotes(ctx, int64(originChain), int64(destChain), fromSymbol, toSymbol, amountSmallest)
	if err != nil {
		return nil, err
	}

	return &models.Route{
		RouteID:               "blockdaemon",
		Score:                 0,
		EstimatedOutputAmount: q.AmountOut,
		EstimatedTimeSeconds:  q.EstimatedTimeSec,
		TotalFee:              "0",
		Hops: []models.Hop{
			{
				BridgeID:     "blockdaemon",
				FromChain:    req.Source.Chain,
				ToChain:      req.Destination.Chain,
				FromAsset:    fromSymbol,
				ToAsset:      toSymbol,
				EstimatedFee: "0",
			},
		},
	}, nil
}
