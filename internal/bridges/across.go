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

func (a AcrossAdapter) ID() string { return "across" }
func (a AcrossAdapter) Tier() models.AdapterTier { return models.TierProduction }

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

	q, swapErr := a.Client.GetQuote(
		ctx,
		int64(src.ChainID),
		int64(dst.ChainID),
		src.Token.Address,
		dst.Token.Address,
		amountSmallest,
		depositor,
	)

	// /swap/approval can fail with 400 when the Across API rejects parameter formats on certain
	// token pairs (e.g. same-token USDC→USDC routes where the new API schema rejects the
	// outputToken format). Fall back to /suggested-fees which is purpose-built for direct
	// SpokePool.depositV3() deposits and is more stable for same-token same-symbol pairs.
	if swapErr != nil {
		dep, ferr := a.Client.FetchDeposit(ctx,
			int64(src.ChainID), int64(dst.ChainID),
			src.Token.Address, dst.Token.Address,
			amountSmallest, depositor,
		)
		if ferr != nil {
			// Return the original /swap/approval error for clarity.
			return nil, swapErr
		}
		providerData, _ := json.Marshal(map[string]any{
			"source":          string(ProviderTierDirect),
			"protocol":        "across_v3",
			"cross_swap_type": "bridgeable",
			"deposit":         dep,
		})
		return &models.Route{
			RouteID:               "across",
			EstimatedOutputAmount: dep.OutputAmount,
			EstimatedTimeSeconds:  0,
			TotalFee:              "0",
			Hops: []models.Hop{{
				BridgeID:          "across",
				HopType:           models.HopTypeBridge,
				FromChain:         firstNonEmptyString(req.Source.Chain, src.ChainKey),
				ToChain:           firstNonEmptyString(req.Destination.Chain, dst.ChainKey),
				FromAsset:         src.Symbol,
				ToAsset:           dst.Symbol,
				FromTokenAddress:  src.Token.Address,
				ToTokenAddress:    dst.Token.Address,
				AmountInBaseUnits: amountSmallest,
				EstimatedFee:      "0",
				ProviderData:      providerData,
			}},
		}, nil
	}

	fee := q.TotalFeeAmount
	timeSec := q.ExpectedFillTimeSec
	outputAmount := q.ExpectedOutputAmount

	// Build provider_data based on what the Across API returned.
	//
	// "bridgeable" (same-token bridge): deposit params for SpokePool.depositV3() are present.
	// "anyToBridgeable" (cross-token): Across does an origin swap first and returns a
	//   pre-built swapTx calldata + approvalTxns. These are used directly for execution.
	pdPayload := map[string]any{
		"source":          string(ProviderTierDirect),
		"protocol":        "across_v3",
		"cross_swap_type": q.CrossSwapType,
	}
	switch q.CrossSwapType {
	case "anyToBridgeable", "bridgeableToAny", "anyToAny", "bridgeableToBridgeable":
		// "bridgeableToBridgeable": Across routes via CCTP internally (both tokens are
		// bridgeable but on different protocols). Returns a pre-built swapTx like cross-token routes.
		if q.SwapTx != nil {
			pdPayload["swap_tx"] = q.SwapTx
		}
		if len(q.ApprovalTxns) > 0 {
			pdPayload["approval_txns"] = q.ApprovalTxns
		}
	default:
		// "bridgeable" or unset: store deposit params.
		// /swap/approval sometimes omits them for same-token routes (e.g. USDC Eth→Polygon).
		// Fall back to /suggested-fees which always returns them.
		dep := q.Deposit
		if dep == nil {
			var ferr error
			dep, ferr = a.Client.FetchDeposit(ctx,
				int64(src.ChainID), int64(dst.ChainID),
				src.Token.Address, dst.Token.Address,
				amountSmallest, depositor,
			)
			if ferr != nil || dep == nil {
				return nil, fmt.Errorf("across: could not obtain deposit params: %w", ferr)
			}
		}
		pdPayload["deposit"] = dep
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
