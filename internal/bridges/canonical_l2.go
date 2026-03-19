package bridges

import (
	"context"
	"encoding/json"
	"fmt"

	"bridge-aggregator/internal/models"
)

// Canonical bridges are chain-specific, but they don't require vendor API keys.
// Execution is multi-step: deposit on the source chain, wait for finality, then
// optionally claim/withdraw on the destination chain.

// L1 bridge contract addresses (Ethereum side).
// Source: official bridge documentation for each network.
const (
	baseL1Bridge      = "0x3154Cf16ccdb4C6d922629664174b904d80F2C35" // Base L1StandardBridge
	optimismL1Bridge  = "0x99C9fc46f92E8a1c0deC1b1747d010903E884bE1" // Optimism L1StandardBridge
	arbitrumL1Inbox   = "0x4Dbd4fc535Ac27206064B68FfCf827b0A60BAB3f" // Arbitrum One Delayed Inbox

	// L2 bridge addresses are identical across OP-stack chains.
	opStackL2Bridge = "0x4200000000000000000000000000000000000010" // L2StandardBridge (OP-stack)
)

type BaseCanonicalAdapter struct{}

func (b BaseCanonicalAdapter) ID() string { return "canonical_base" }

func (b BaseCanonicalAdapter) GetQuote(ctx context.Context, req models.QuoteRequest) (*models.Route, error) {
	_ = ctx
	src, err := resolveBridgeEndpoint(req.Source)
	if err != nil {
		return nil, fmt.Errorf("canonical_base: %w", err)
	}
	dst, err := resolveBridgeEndpoint(req.Destination)
	if err != nil {
		return nil, fmt.Errorf("canonical_base: %w", err)
	}
	// Base canonical is primarily Ethereum <-> Base.
	if !((src.ChainID == ChainIDEthereum && dst.ChainID == ChainIDBase) || (src.ChainID == ChainIDBase && dst.ChainID == ChainIDEthereum)) {
		return nil, fmt.Errorf("canonical_base: unsupported chain pair (src=%d dst=%d)", src.ChainID, dst.ChainID)
	}
	amountSmallest, err := resolveAmountBaseUnits(req, src.Token.Decimals)
	if err != nil {
		return nil, fmt.Errorf("canonical_base: invalid amount: %w", err)
	}
	depositOnL1 := src.ChainID == ChainIDEthereum
	pd, _ := json.Marshal(map[string]any{
		"source":        string(ProviderTierDirect),
		"protocol":      "canonical",
		"bridge":        "base",
		"l1_bridge":     baseL1Bridge,
		"l2_bridge":     opStackL2Bridge,
		"deposit_on_l1": depositOnL1,
		"amount":        amountSmallest,
		"input_token":   src.Token.Address,
		"output_token":  dst.Token.Address,
	})
	return &models.Route{
		RouteID:               "canonical_base",
		EstimatedOutputAmount: amountSmallest,
		EstimatedTimeSeconds:  900,
		TotalFee:              "0",
		Hops: []models.Hop{
			{
				HopType:           models.HopTypeBridge,
				BridgeID:          "canonical_base",
				FromChain:         firstNonEmptyString(req.Source.Chain, src.ChainKey),
				ToChain:           firstNonEmptyString(req.Destination.Chain, dst.ChainKey),
				FromAsset:         src.Symbol,
				ToAsset:           dst.Symbol,
				FromTokenAddress:  src.Token.Address,
				ToTokenAddress:    dst.Token.Address,
				AmountInBaseUnits: amountSmallest,
				EstimatedFee:      "0",
				ProviderData:      pd,
			},
		},
	}, nil
}

type OptimismCanonicalAdapter struct{}

func (o OptimismCanonicalAdapter) ID() string { return "canonical_optimism" }

func (o OptimismCanonicalAdapter) GetQuote(ctx context.Context, req models.QuoteRequest) (*models.Route, error) {
	_ = ctx
	src, err := resolveBridgeEndpoint(req.Source)
	if err != nil {
		return nil, fmt.Errorf("canonical_optimism: %w", err)
	}
	dst, err := resolveBridgeEndpoint(req.Destination)
	if err != nil {
		return nil, fmt.Errorf("canonical_optimism: %w", err)
	}
	if !((src.ChainID == ChainIDEthereum && dst.ChainID == ChainIDOptimism) || (src.ChainID == ChainIDOptimism && dst.ChainID == ChainIDEthereum)) {
		return nil, fmt.Errorf("canonical_optimism: unsupported chain pair (src=%d dst=%d)", src.ChainID, dst.ChainID)
	}
	amountSmallest, err := resolveAmountBaseUnits(req, src.Token.Decimals)
	if err != nil {
		return nil, fmt.Errorf("canonical_optimism: invalid amount: %w", err)
	}
	depositOnL1 := src.ChainID == ChainIDEthereum
	pd, _ := json.Marshal(map[string]any{
		"source":        string(ProviderTierDirect),
		"protocol":      "canonical",
		"bridge":        "optimism",
		"l1_bridge":     optimismL1Bridge,
		"l2_bridge":     opStackL2Bridge,
		"deposit_on_l1": depositOnL1,
		"amount":        amountSmallest,
		"input_token":   src.Token.Address,
		"output_token":  dst.Token.Address,
	})
	return &models.Route{
		RouteID:               "canonical_optimism",
		EstimatedOutputAmount: amountSmallest,
		EstimatedTimeSeconds:  900,
		TotalFee:              "0",
		Hops: []models.Hop{
			{
				HopType:           models.HopTypeBridge,
				BridgeID:          "canonical_optimism",
				FromChain:         firstNonEmptyString(req.Source.Chain, src.ChainKey),
				ToChain:           firstNonEmptyString(req.Destination.Chain, dst.ChainKey),
				FromAsset:         src.Symbol,
				ToAsset:           dst.Symbol,
				FromTokenAddress:  src.Token.Address,
				ToTokenAddress:    dst.Token.Address,
				AmountInBaseUnits: amountSmallest,
				EstimatedFee:      "0",
				ProviderData:      pd,
			},
		},
	}, nil
}

type ArbitrumCanonicalAdapter struct{}

func (a ArbitrumCanonicalAdapter) ID() string { return "canonical_arbitrum" }

func (a ArbitrumCanonicalAdapter) GetQuote(ctx context.Context, req models.QuoteRequest) (*models.Route, error) {
	_ = ctx
	src, err := resolveBridgeEndpoint(req.Source)
	if err != nil {
		return nil, fmt.Errorf("canonical_arbitrum: %w", err)
	}
	dst, err := resolveBridgeEndpoint(req.Destination)
	if err != nil {
		return nil, fmt.Errorf("canonical_arbitrum: %w", err)
	}
	if !((src.ChainID == ChainIDEthereum && dst.ChainID == ChainIDArbitrum) || (src.ChainID == ChainIDArbitrum && dst.ChainID == ChainIDEthereum)) {
		return nil, fmt.Errorf("canonical_arbitrum: unsupported chain pair (src=%d dst=%d)", src.ChainID, dst.ChainID)
	}
	amountSmallest, err := resolveAmountBaseUnits(req, src.Token.Decimals)
	if err != nil {
		return nil, fmt.Errorf("canonical_arbitrum: invalid amount: %w", err)
	}
	depositOnL1 := src.ChainID == ChainIDEthereum
	pd, _ := json.Marshal(map[string]any{
		"source":        string(ProviderTierDirect),
		"protocol":      "canonical",
		"bridge":        "arbitrum",
		"l1_inbox":      arbitrumL1Inbox,
		"deposit_on_l1": depositOnL1,
		"amount":        amountSmallest,
		"input_token":   src.Token.Address,
		"output_token":  dst.Token.Address,
	})
	return &models.Route{
		RouteID:               "canonical_arbitrum",
		EstimatedOutputAmount: amountSmallest,
		EstimatedTimeSeconds:  900,
		TotalFee:              "0",
		Hops: []models.Hop{
			{
				HopType:           models.HopTypeBridge,
				BridgeID:          "canonical_arbitrum",
				FromChain:         firstNonEmptyString(req.Source.Chain, src.ChainKey),
				ToChain:           firstNonEmptyString(req.Destination.Chain, dst.ChainKey),
				FromAsset:         src.Symbol,
				ToAsset:           dst.Symbol,
				FromTokenAddress:  src.Token.Address,
				ToTokenAddress:    dst.Token.Address,
				AmountInBaseUnits: amountSmallest,
				EstimatedFee:      "0",
				ProviderData:      pd,
			},
		},
	}, nil
}

