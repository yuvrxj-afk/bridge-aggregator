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

// L1 bridge contract addresses (Ethereum mainnet side).
// Source: official bridge documentation for each network.
const (
	baseL1Bridge     = "0x3154Cf16ccdb4C6d922629664174b904d80F2C35" // Base L1StandardBridge
	optimismL1Bridge = "0x99C9fc46f92E8a1c0deC1b1747d010903E884bE1" // Optimism L1StandardBridge
	arbitrumL1Inbox  = "0x4Dbd4fc535Ac27206064B68FfCf827b0A60BAB3f" // Arbitrum One Delayed Inbox

	arbitrumL1GatewayRouter = "0x72Ce9c846789fdB6fC1f34aC4AD25Dd9ef7031ef" // Arbitrum L1 GatewayRouter
	arbitrumL1ERC20Gateway  = "0xa3A7B6F88361F48403514059F1F16C8E78d60EeC" // Standard L1 ERC20 Gateway

	// L2 bridge addresses are identical across OP-stack chains.
	opStackL2Bridge = "0x4200000000000000000000000000000000000010" // L2StandardBridge (OP-stack)
)

// canonicalBridgeEntry holds the L1-side contract addresses for a canonical L2 bridge pair.
type canonicalBridgeEntry struct {
	L1BridgeOrInbox  string  // L1StandardBridge (OP-stack) or Delayed Inbox (Arbitrum)
	L1GatewayRouter  string  // Arbitrum GatewayRouter; empty for OP-stack
	L1ERC20Gateway   string  // Arbitrum ERC20Gateway; empty for OP-stack
	L2Bridge         string  // L2StandardBridge predeploy (OP-stack); empty for Arbitrum
	L1ChainID        ChainID // the L1 chain this pair bridges to (Ethereum or Sepolia)
	IsArbitrum       bool
}

// canonicalBridges maps each L2 chain ID to its bridge configuration.
// Testnet entries are added by registerCanonicalTestnetChains() when NETWORK=testnet.
var canonicalBridges = map[ChainID]canonicalBridgeEntry{
	ChainIDBase: {
		L1BridgeOrInbox: baseL1Bridge,
		L2Bridge:        opStackL2Bridge,
		L1ChainID:       ChainIDEthereum,
	},
	ChainIDOptimism: {
		L1BridgeOrInbox: optimismL1Bridge,
		L2Bridge:        opStackL2Bridge,
		L1ChainID:       ChainIDEthereum,
	},
	ChainIDArbitrum: {
		L1BridgeOrInbox: arbitrumL1Inbox,
		L1GatewayRouter: arbitrumL1GatewayRouter,
		L1ERC20Gateway:  arbitrumL1ERC20Gateway,
		L1ChainID:       ChainIDEthereum,
		IsArbitrum:      true,
	},
}

// canonicalTestnetETHOnly is set to true when NETWORK=testnet.
// On testnet, Circle's USDC is not registered as an OptimismMintableERC20 in the
// L1StandardBridge — ERC20 deposits succeed on-chain but funds are stuck in the bridge.
// ETH bridging via canonical is unaffected. Mainnet has proper token registrations and
// is never restricted by this flag.
var canonicalTestnetETHOnly bool

// registerCanonicalTestnetChains adds Sepolia testnet bridge configs to canonicalBridges.
// Called by RegisterTestnetChains() when NETWORK=testnet.
// Testnet contract addresses sourced from official rollup documentation.
func registerCanonicalTestnetChains() {
	canonicalTestnetETHOnly = true
	// Base Sepolia — L1StandardBridgeProxy on Sepolia
	canonicalBridges[ChainIDBaseSepolia] = canonicalBridgeEntry{
		L1BridgeOrInbox: "0xfd0Bf71F60660E2f608ed56e1659C450eB113120",
		L2Bridge:        opStackL2Bridge, // predeploy address is the same on testnet
		L1ChainID:       ChainIDSepolia,
	}
	// OP Sepolia — L1StandardBridgeProxy on Sepolia
	canonicalBridges[ChainIDOPSepolia] = canonicalBridgeEntry{
		L1BridgeOrInbox: "0xFBb0621E0B23b5478B630BD55a5f21f67730B0F1",
		L2Bridge:        opStackL2Bridge,
		L1ChainID:       ChainIDSepolia,
	}
	// Arbitrum Sepolia — Inbox + Gateway contracts on Sepolia
	canonicalBridges[ChainIDArbitrumSepolia] = canonicalBridgeEntry{
		L1BridgeOrInbox: "0xaAe29B0366299461418F5324a79Afc425BE5ae21",
		L1GatewayRouter: "0xcE18836b233C83325Cc8848CA4487e94C6288264",
		L1ERC20Gateway:  "0x902b3E5f8F19571859F4AB1003B960a5dF693aFF",
		L1ChainID:       ChainIDSepolia,
		IsArbitrum:      true,
	}
}

// lookupCanonicalBridge finds the bridge config for a (src, dst) chain pair.
// One of the two must be an L2 in canonicalBridges, and the other must be its L1.
// Returns the entry, whether src is L1 (deposit direction), and any error.
func lookupCanonicalBridge(srcID, dstID ChainID) (canonicalBridgeEntry, bool, error) {
	// dst is L2 → deposit direction (L1→L2)
	if entry, ok := canonicalBridges[dstID]; ok && entry.L1ChainID == srcID {
		return entry, true, nil
	}
	// src is L2 → withdrawal direction (L2→L1)
	if entry, ok := canonicalBridges[srcID]; ok && entry.L1ChainID == dstID {
		return entry, false, nil
	}
	return canonicalBridgeEntry{}, false, fmt.Errorf("unsupported canonical chain pair (src=%d dst=%d)", srcID, dstID)
}

// buildCanonicalProviderData returns the provider_data JSON for a canonical hop.
func buildCanonicalProviderData(entry canonicalBridgeEntry, bridge string, depositOnL1 bool, amount, inputToken, outputToken string) json.RawMessage {
	m := map[string]any{
		"source":        string(ProviderTierDirect),
		"protocol":      "canonical",
		"bridge":        bridge,
		"deposit_on_l1": depositOnL1,
		"amount":        amount,
		"input_token":   inputToken,
		"output_token":  outputToken,
	}
	if entry.IsArbitrum {
		m["l1_inbox"] = entry.L1BridgeOrInbox
		m["l1_gateway_router"] = entry.L1GatewayRouter
		m["l1_erc20_gateway"] = entry.L1ERC20Gateway
	} else {
		m["l1_bridge"] = entry.L1BridgeOrInbox
		m["l2_bridge"] = entry.L2Bridge
	}
	pd, _ := json.Marshal(m)
	return pd
}

// ── Base ──────────────────────────────────────────────────────────────────────

type BaseCanonicalAdapter struct{}

func (b BaseCanonicalAdapter) ID() string             { return "canonical_base" }
func (b BaseCanonicalAdapter) Tier() models.AdapterTier { return models.TierProduction }

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

	// Canonical bridge moves a token between L1 and L2 without swapping.
	// Source and destination must be the same underlying token.
	if src.Symbol != dst.Symbol {
		return nil, fmt.Errorf("canonical_base: cross-token bridging not supported (%s→%s); canonical bridge only moves the same token", src.Symbol, dst.Symbol)
	}

	// On testnet, Circle's USDC is not registered as an OptimismMintableERC20 — only ETH bridges.
	// On mainnet, token registrations exist and ERC20s are supported.
	if canonicalTestnetETHOnly && !isNativeETH(src.Token.Address) {
		return nil, fmt.Errorf("canonical_base: only native ETH is supported on testnet via canonical bridge; use CCTP or Across for ERC20 tokens")
	}

	entry, depositOnL1, err := lookupCanonicalBridge(src.ChainID, dst.ChainID)
	if err != nil {
		return nil, fmt.Errorf("canonical_base: %w", err)
	}
	// This adapter only handles Base ↔ its L1.
	l2ID := dst.ChainID
	if !depositOnL1 {
		l2ID = src.ChainID
	}
	if l2ID != ChainIDBase && l2ID != ChainIDBaseSepolia {
		return nil, fmt.Errorf("canonical_base: unsupported chain pair (src=%d dst=%d)", src.ChainID, dst.ChainID)
	}

	amountSmallest, err := resolveAmountBaseUnits(req, src.Token.Decimals)
	if err != nil {
		return nil, fmt.Errorf("canonical_base: invalid amount: %w", err)
	}

	pd := buildCanonicalProviderData(entry, "base", depositOnL1, amountSmallest, src.Token.Address, dst.Token.Address)
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

// ── Optimism ──────────────────────────────────────────────────────────────────

type OptimismCanonicalAdapter struct{}

func (o OptimismCanonicalAdapter) ID() string             { return "canonical_optimism" }
func (o OptimismCanonicalAdapter) Tier() models.AdapterTier { return models.TierProduction }

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

	if src.Symbol != dst.Symbol {
		return nil, fmt.Errorf("canonical_optimism: cross-token bridging not supported (%s→%s); canonical bridge only moves the same token", src.Symbol, dst.Symbol)
	}

	if canonicalTestnetETHOnly && !isNativeETH(src.Token.Address) {
		return nil, fmt.Errorf("canonical_optimism: only native ETH is supported on testnet via canonical bridge; use CCTP or Across for ERC20 tokens")
	}

	entry, depositOnL1, err := lookupCanonicalBridge(src.ChainID, dst.ChainID)
	if err != nil {
		return nil, fmt.Errorf("canonical_optimism: %w", err)
	}
	l2ID := dst.ChainID
	if !depositOnL1 {
		l2ID = src.ChainID
	}
	if l2ID != ChainIDOptimism && l2ID != ChainIDOPSepolia {
		return nil, fmt.Errorf("canonical_optimism: unsupported chain pair (src=%d dst=%d)", src.ChainID, dst.ChainID)
	}

	amountSmallest, err := resolveAmountBaseUnits(req, src.Token.Decimals)
	if err != nil {
		return nil, fmt.Errorf("canonical_optimism: invalid amount: %w", err)
	}

	pd := buildCanonicalProviderData(entry, "optimism", depositOnL1, amountSmallest, src.Token.Address, dst.Token.Address)
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

// ── Arbitrum ──────────────────────────────────────────────────────────────────

type ArbitrumCanonicalAdapter struct{}

func (a ArbitrumCanonicalAdapter) ID() string             { return "canonical_arbitrum" }
func (a ArbitrumCanonicalAdapter) Tier() models.AdapterTier { return models.TierProduction }

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

	if src.Symbol != dst.Symbol {
		return nil, fmt.Errorf("canonical_arbitrum: cross-token bridging not supported (%s→%s); canonical bridge only moves the same token", src.Symbol, dst.Symbol)
	}

	if canonicalTestnetETHOnly && !isNativeETH(src.Token.Address) {
		return nil, fmt.Errorf("canonical_arbitrum: only native ETH is supported on testnet via canonical bridge; use CCTP or Across for ERC20 tokens")
	}

	entry, depositOnL1, err := lookupCanonicalBridge(src.ChainID, dst.ChainID)
	if err != nil {
		return nil, fmt.Errorf("canonical_arbitrum: %w", err)
	}
	l2ID := dst.ChainID
	if !depositOnL1 {
		l2ID = src.ChainID
	}
	if l2ID != ChainIDArbitrum && l2ID != ChainIDArbitrumSepolia {
		return nil, fmt.Errorf("canonical_arbitrum: unsupported chain pair (src=%d dst=%d)", src.ChainID, dst.ChainID)
	}

	amountSmallest, err := resolveAmountBaseUnits(req, src.Token.Decimals)
	if err != nil {
		return nil, fmt.Errorf("canonical_arbitrum: invalid amount: %w", err)
	}

	pd := buildCanonicalProviderData(entry, "arbitrum", depositOnL1, amountSmallest, src.Token.Address, dst.Token.Address)
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
