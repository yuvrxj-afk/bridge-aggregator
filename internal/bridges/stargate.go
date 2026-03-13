package bridges

import (
	"context"
	"fmt"
	"strings"

	"bridge-aggregator/internal/models"
)

// StargateChainKeys are chain names we support for Stargate (API uses same slugs).
var StargateChainKeys = map[string]bool{
	"ethereum": true, "arbitrum": true, "optimism": true, "polygon": true,
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

	fromChain := strings.ToLower(req.Source.Chain)
	toChain := strings.ToLower(req.Destination.Chain)
	if !StargateChainKeys[fromChain] {
		return nil, fmt.Errorf("stargate: unsupported source chain: %s", req.Source.Chain)
	}
	if !StargateChainKeys[toChain] {
		return nil, fmt.Errorf("stargate: unsupported destination chain: %s", req.Destination.Chain)
	}

	originChain, ok := ChainNameToID[fromChain]
	if !ok {
		return nil, fmt.Errorf("stargate: unsupported source chain: %s", req.Source.Chain)
	}
	destChain, ok := ChainNameToID[toChain]
	if !ok {
		return nil, fmt.Errorf("stargate: unsupported destination chain: %s", req.Destination.Chain)
	}

	originTokens := TokenByChainAndSymbol[originChain]
	destTokens := TokenByChainAndSymbol[destChain]
	if originTokens == nil || destTokens == nil {
		return nil, fmt.Errorf("stargate: token registry missing for chain(s)")
	}

	fromSymbol := strings.ToUpper(req.Source.Asset)
	toSymbol := strings.ToUpper(req.Destination.Asset)
	inputToken, ok := originTokens[fromSymbol]
	if !ok {
		return nil, fmt.Errorf("stargate: unsupported input asset %s on %s", fromSymbol, req.Source.Chain)
	}
	outputToken, ok := destTokens[toSymbol]
	if !ok {
		return nil, fmt.Errorf("stargate: unsupported output asset %s on %s", toSymbol, req.Destination.Chain)
	}

	amountSmallest, err := HumanToSmallest(req.Amount, inputToken.Decimals)
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
		inputToken.Address,
		outputToken.Address,
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
				FromChain:    req.Source.Chain,
				ToChain:      req.Destination.Chain,
				FromAsset:    fromSymbol,
				ToAsset:      toSymbol,
				EstimatedFee: q.TotalFeeAmount,
			},
		},
	}, nil
}
