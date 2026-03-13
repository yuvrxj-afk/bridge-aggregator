package bridges

import (
	"context"
	"fmt"
	"strings"

	"bridge-aggregator/internal/models"
)

// AcrossAdapter implements Adapter for Across bridge.
// If Client is set, GetQuote calls the real Across API; otherwise it returns an error or placeholder.
type AcrossAdapter struct {
	Client *AcrossClient
}

func (a AcrossAdapter) ID() string {
	return "across"
}

func (a AcrossAdapter) GetQuote(ctx context.Context, req models.QuoteRequest) (*models.Route, error) {
	if a.Client == nil {
		return nil, fmt.Errorf("across: client not configured")
	}

	originChain, ok := ChainNameToID[strings.ToLower(req.Source.Chain)]
	if !ok {
		return nil, fmt.Errorf("across: unsupported source chain: %s", req.Source.Chain)
	}
	destChain, ok := ChainNameToID[strings.ToLower(req.Destination.Chain)]
	if !ok {
		return nil, fmt.Errorf("across: unsupported destination chain: %s", req.Destination.Chain)
	}

	originTokens := TokenByChainAndSymbol[originChain]
	destTokens := TokenByChainAndSymbol[destChain]
	if originTokens == nil || destTokens == nil {
		return nil, fmt.Errorf("across: token registry missing for chain(s)")
	}

	fromSymbol := strings.ToUpper(req.Source.Asset)
	toSymbol := strings.ToUpper(req.Destination.Asset)

	inputToken, ok := originTokens[fromSymbol]
	if !ok {
		return nil, fmt.Errorf("across: unsupported input asset %s on %s", fromSymbol, req.Source.Chain)
	}
	outputToken, ok := destTokens[toSymbol]
	if !ok {
		return nil, fmt.Errorf("across: unsupported output asset %s on %s", toSymbol, req.Destination.Chain)
	}

	depositor := req.Source.Address
	if depositor == "" {
		depositor = "0x0000000000000000000000000000000000000001"
	}

	amountSmallest, err := HumanToSmallest(req.Amount, inputToken.Decimals)
	if err != nil {
		return nil, fmt.Errorf("across: invalid amount: %w", err)
	}

	q, err := a.Client.GetQuote(
		ctx,
		int64(originChain),
		int64(destChain),
		inputToken.Address,
		outputToken.Address,
		amountSmallest,
		depositor,
	)
	if err != nil {
		return nil, err
	}

	fee := q.TotalFeeAmount
	timeSec := q.ExpectedFillTimeSec
	outputAmount := q.ExpectedOutputAmount

	return &models.Route{
		RouteID:               "across",
		Score:                 0,
		EstimatedOutputAmount: outputAmount,
		EstimatedTimeSeconds:  timeSec,
		TotalFee:              fee,
		Hops: []models.Hop{
			{
				BridgeID:     "across",
				FromChain:    req.Source.Chain,
				ToChain:      req.Destination.Chain,
				FromAsset:    fromSymbol,
				ToAsset:      toSymbol,
				EstimatedFee: fee,
			},
		},
	}, nil
}
