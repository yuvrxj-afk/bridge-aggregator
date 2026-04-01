package bridges

import (
	"context"
	"encoding/json"
	"fmt"

	"bridge-aggregator/internal/models"
)

// BlockdaemonAdapter implements Adapter using Blockdaemon DeFi API (aggregated bridge quotes).
// See https://docs.blockdaemon.com/docs/defi-api-overview
type BlockdaemonAdapter struct {
	Client *BlockdaemonClient
}

// ID returns the bridge identifier for execute validation.
func (b BlockdaemonAdapter) ID() string { return "blockdaemon" }

// Tier returns TierDegraded — Blockdaemon is functional but may return incomplete tx data.
func (b BlockdaemonAdapter) Tier() models.AdapterTier { return models.TierDegraded }

// GetQuote returns the best bridge quote from Blockdaemon's aggregator (Squid, Stargate, AllBridge, etc.).
func (b BlockdaemonAdapter) GetQuote(ctx context.Context, req models.QuoteRequest) (*models.Route, error) {
	if b.Client == nil {
		return nil, fmt.Errorf("blockdaemon: client not configured")
	}
	if b.Client.APIKey == "" {
		return nil, fmt.Errorf("blockdaemon: not configured (missing api key)")
	}

	src, err := resolveBridgeEndpoint(req.Source)
	if err != nil {
		return nil, fmt.Errorf("blockdaemon: %w", err)
	}
	dst, err := resolveBridgeEndpoint(req.Destination)
	if err != nil {
		return nil, fmt.Errorf("blockdaemon: %w", err)
	}
	if src.Symbol == "" || dst.Symbol == "" {
		return nil, fmt.Errorf("blockdaemon: requires source.asset and destination.asset symbols (blockdaemon API is symbol-based)")
	}
	amountSmallest, err := resolveAmountBaseUnits(req, src.Token.Decimals)
	if err != nil {
		return nil, fmt.Errorf("blockdaemon: invalid amount: %w", err)
	}

	q, err := b.Client.GetBridgeQuotes(ctx, int64(src.ChainID), int64(dst.ChainID), src.Symbol, dst.Symbol, amountSmallest)
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
				HopType:      models.HopTypeBridge,
				FromChain:    firstNonEmptyString(req.Source.Chain, src.ChainKey),
				ToChain:      firstNonEmptyString(req.Destination.Chain, dst.ChainKey),
				FromAsset:    src.Symbol,
				ToAsset:      dst.Symbol,
				FromTokenAddress: src.Token.Address,
				ToTokenAddress: dst.Token.Address,
				AmountInBaseUnits: amountSmallest,
				EstimatedFee: "0",
				ProviderData: func() json.RawMessage {
					b, _ := json.Marshal(map[string]any{"source": ProviderTierAggregator, "protocol": "blockdaemon_defi_api"})
					return b
				}(),
			},
		},
	}, nil
}
