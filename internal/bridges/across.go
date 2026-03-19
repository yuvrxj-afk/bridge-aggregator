package bridges

import (
	"context"
	"encoding/json"
	"fmt"

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

	src, err := resolveBridgeEndpoint(req.Source)
	if err != nil {
		return nil, fmt.Errorf("across: %w", err)
	}
	dst, err := resolveBridgeEndpoint(req.Destination)
	if err != nil {
		return nil, fmt.Errorf("across: %w", err)
	}

	// Prefer request-level address (if valid), then fall back to the configured depositor.
	// Reject placeholder strings like "0xYourWallet" that are not valid EVM addresses.
	depositor := req.Source.Address
	if !IsValidEVMAddress(depositor) {
		depositor = a.Client.Depositor
	}
	if !IsValidEVMAddress(depositor) {
		return nil, fmt.Errorf("across: depositor address required — set ACROSS_DEPOSITOR env var or pass a valid source.address")
	}

	amountSmallest, err := resolveAmountBaseUnits(req, src.Token.Decimals)
	if err != nil {
		return nil, fmt.Errorf("across: invalid amount: %w", err)
	}

	q, err := a.Client.GetQuote(
		ctx,
		int64(src.ChainID),
		int64(dst.ChainID),
		src.Token.Address,
		dst.Token.Address,
		amountSmallest,
		depositor,
	)
	if err != nil {
		return nil, err
	}

	fee := q.TotalFeeAmount
	timeSec := q.ExpectedFillTimeSec
	outputAmount := q.ExpectedOutputAmount

	// Build provider_data: always include the tier, and embed the deposit params
	// when Across returned them so stepTransaction can build the on-chain call.
	pdPayload := map[string]any{
		"source":   string(ProviderTierDirect),
		"protocol": "across_v3",
	}
	if q.Deposit != nil {
		pdPayload["deposit"] = q.Deposit
	}
	providerData, _ := json.Marshal(pdPayload)

	return &models.Route{
		RouteID:               "across",
		Score:                 0,
		EstimatedOutputAmount: outputAmount,
		EstimatedTimeSeconds:  timeSec,
		TotalFee:              fee,
		Hops: []models.Hop{
			{
				BridgeID:          "across",
				HopType:           models.HopTypeBridge,
				FromChain:         firstNonEmptyString(req.Source.Chain, src.ChainKey),
				ToChain:           firstNonEmptyString(req.Destination.Chain, dst.ChainKey),
				FromAsset:         src.Symbol,
				ToAsset:           dst.Symbol,
				FromTokenAddress:  src.Token.Address,
				ToTokenAddress:    dst.Token.Address,
				AmountInBaseUnits: amountSmallest,
				EstimatedFee:      fee,
				ProviderData:      providerData,
			},
		},
	}, nil
}
