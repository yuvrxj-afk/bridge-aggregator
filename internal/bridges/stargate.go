package bridges

import (
	"context"
	"fmt"

	"bridge-aggregator/internal/models"
)

// StargateChainKeys are chain names we support for Stargate (API uses same slugs).
var StargateChainKeys = map[string]bool{
	"ethereum": true, "arbitrum": true, "optimism": true, "polygon": true, "base": true,
}

// StargateAdapter implements Adapter for Stargate/LayerZero bridge.
// Uses the LayerZero Value Transfer API (https://transfer.layerzero-api.com/v1/docs).
type StargateAdapter struct {
	Client *StargateClient
}

// ID returns the bridge identifier.
func (s StargateAdapter) ID() string {
	return "stargate"
}

// GetQuote returns a quote from Stargate API when Client is set; otherwise returns an error.
func (s StargateAdapter) GetQuote(ctx context.Context, req models.QuoteRequest) (*models.Route, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("stargate: client not configured")
	}
	if s.Client.APIKey == "" {
		return nil, fmt.Errorf("stargate: not configured (missing api key)")
	}

	// Prefer address-first, but we still require chain keys for LayerZero VT API.
	src, err := resolveBridgeEndpoint(req.Source)
	if err != nil {
		return nil, fmt.Errorf("stargate: %w", err)
	}
	dst, err := resolveBridgeEndpoint(req.Destination)
	if err != nil {
		return nil, fmt.Errorf("stargate: %w", err)
	}

	fromChain := src.ChainKey
	toChain := dst.ChainKey
	if !StargateChainKeys[fromChain] {
		return nil, fmt.Errorf("stargate: unsupported source chain: %s", req.Source.Chain)
	}
	if !StargateChainKeys[toChain] {
		return nil, fmt.Errorf("stargate: unsupported destination chain: %s", req.Destination.Chain)
	}
	amountSmallest, err := resolveAmountBaseUnits(req, src.Token.Decimals)
	if err != nil {
		return nil, fmt.Errorf("stargate: invalid amount: %w", err)
	}

	depositor := req.Source.Address
	if depositor == "" {
		depositor = "0x0000000000000000000000000000000000000001"
	}
	recipient := req.Destination.Address
	if recipient == "" {
		recipient = depositor
	}

	q, err := s.Client.GetQuote(
		ctx,
		amountSmallest,
		src.Token.Address,
		dst.Token.Address,
		fromChain,
		toChain,
		depositor,
		recipient,
	)
	if err != nil {
		return nil, err
	}

	return &models.Route{
		RouteID:               "stargate",
		Score:                 0,
		EstimatedOutputAmount: q.DstAmount,
		EstimatedTimeSeconds:  q.EstimatedTimeSec,
		TotalFee:              q.TotalFeeAmount,
		Hops: []models.Hop{
			{
				BridgeID:     "stargate",
				HopType:      models.HopTypeBridge,
				FromChain:    firstNonEmptyString(req.Source.Chain, src.ChainKey),
				ToChain:      firstNonEmptyString(req.Destination.Chain, dst.ChainKey),
				FromAsset:    src.Symbol,
				ToAsset:      dst.Symbol,
				FromTokenAddress: src.Token.Address,
				ToTokenAddress: dst.Token.Address,
				AmountInBaseUnits: amountSmallest,
				EstimatedFee: q.TotalFeeAmount,
			},
		},
	}, nil
}
