package bridges

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"bridge-aggregator/internal/models"
)

// Circle CCTP is USDC-only burn/mint. This adapter is quote-only for now.
// It supports address-first requests and returns deterministic output (amount in == amount out).
//
// Execution will later be modeled as multi-step tx population:
// - approve USDC
// - TokenMessengerV2.depositForBurn(...)
// - wait for attestation from Iris API
// - MessageTransmitterV2.receiveMessage(...)
type CCTPAdapter struct{}

func (c CCTPAdapter) ID() string   { return "cctp" }
func (c CCTPAdapter) Tier() models.AdapterTier { return models.TierProduction }

// cctpSupportedChainIDs restricts CCTP quotes to chains with known contract deployments.
var cctpSupportedChainIDs = map[ChainID]bool{
	ChainIDEthereum: true,
	ChainIDBase:     true,
	ChainIDOptimism: true,
	ChainIDArbitrum: true,
	ChainIDPolygon:  true,
	ChainIDAvax:     true,
}

// cctpDomainID maps each EVM chain to its Circle CCTP domain number.
// Source: https://developers.circle.com/stablecoins/supported-domains
var cctpDomainID = map[ChainID]uint32{
	ChainIDEthereum: 0,
	ChainIDAvax:     1,
	ChainIDOptimism: 2,
	ChainIDArbitrum: 3,
	ChainIDBase:     6,
	ChainIDPolygon:  7,
}

// cctpTokenMessenger maps each chain to the TokenMessenger contract address for that chain.
// Source: https://developers.circle.com/stablecoins/evm-smart-contracts
var cctpTokenMessenger = map[ChainID]string{
	ChainIDEthereum: "0xBd3fa81B58Ba92a82136038B25aDec7066af3155",
	ChainIDAvax:     "0x6B25532e1060CE10cc3B0A99e5683b91BFDe6982",
	ChainIDOptimism: "0x2B4069517957735bE00ceE0fadAE88a26365528f",
	ChainIDArbitrum: "0x19330d10D9Cc8751218eaf51E8885D058642E08A",
	ChainIDBase:     "0x1682Ae6375C4E4A97e4B583BC394c861A46D8962",
	ChainIDPolygon:  "0x9daF8c91AEFAE50b9c0E69629D3F6Ca40cA3B3FE",
}

// cctpMessageTransmitter maps each chain to the MessageTransmitter address (used to receive on dst).
// Source: https://developers.circle.com/stablecoins/evm-smart-contracts
var cctpMessageTransmitter = map[ChainID]string{
	ChainIDEthereum: "0x0a992d191DEeC32aFe36203Ad87D7d289a738F81",
	ChainIDAvax:     "0x8186359aF5F57FbB40c6b14A588d2A59C0C29880",
	ChainIDOptimism: "0x4D41f22c5a0e5c74090899E5a8Fb597a8842b3e8",
	ChainIDArbitrum: "0xC30362313FBBA5cf9163F0bb16a0e01f01A896ca",
	ChainIDBase:     "0xAD09780d193884d503182aD4588450C416D6F9D4",
	ChainIDPolygon:  "0xF3be9355363857F3e001be68856A2f96b4C39Ba9",
}

// registerCCTPTestnetChains adds Sepolia testnet chain IDs and contract addresses to the CCTP
// registries. Called by RegisterTestnetChains() when NETWORK=testnet. Safe to call multiple times.
// Testnet contract addresses sourced from https://developers.circle.com/stablecoins/evm-smart-contracts
func registerCCTPTestnetChains() {
	// Testnet chain support
	cctpSupportedChainIDs[ChainIDSepolia] = true
	cctpSupportedChainIDs[ChainIDBaseSepolia] = true
	cctpSupportedChainIDs[ChainIDArbitrumSepolia] = true
	cctpSupportedChainIDs[ChainIDOPSepolia] = true

	// CCTP domain IDs are the same for mainnet and testnet counterparts.
	cctpDomainID[ChainIDSepolia] = 0         // same domain as Ethereum mainnet
	cctpDomainID[ChainIDOPSepolia] = 2        // same domain as OP mainnet
	cctpDomainID[ChainIDArbitrumSepolia] = 3  // same domain as Arbitrum mainnet
	cctpDomainID[ChainIDBaseSepolia] = 6      // same domain as Base mainnet

	// Testnet TokenMessenger addresses (all four chains share the same address on testnet).
	const testnetTokenMessenger = "0x9f3B8679c73C2Fef8b59B4f3444d4e156fb70AA5"
	cctpTokenMessenger[ChainIDSepolia] = testnetTokenMessenger
	cctpTokenMessenger[ChainIDBaseSepolia] = testnetTokenMessenger
	cctpTokenMessenger[ChainIDOPSepolia] = testnetTokenMessenger
	cctpTokenMessenger[ChainIDArbitrumSepolia] = testnetTokenMessenger

	// Testnet MessageTransmitter addresses.
	// Arbitrum Sepolia has a distinct address; the others share one.
	const testnetMessageTransmitter = "0x7865fAfC2db2093669d92c0F33AeEF291086BEFD"
	cctpMessageTransmitter[ChainIDSepolia] = testnetMessageTransmitter
	cctpMessageTransmitter[ChainIDBaseSepolia] = testnetMessageTransmitter
	cctpMessageTransmitter[ChainIDOPSepolia] = testnetMessageTransmitter
	cctpMessageTransmitter[ChainIDArbitrumSepolia] = "0xaCF1ceeF35caAc005e15888dDb8A3515C41B4872"
}

func (c CCTPAdapter) GetQuote(ctx context.Context, req models.QuoteRequest) (*models.Route, error) {
	_ = ctx

	src, err := resolveBridgeEndpoint(req.Source)
	if err != nil {
		return nil, fmt.Errorf("cctp: %w", err)
	}
	dst, err := resolveBridgeEndpoint(req.Destination)
	if err != nil {
		return nil, fmt.Errorf("cctp: %w", err)
	}
	if !cctpSupportedChainIDs[src.ChainID] || !cctpSupportedChainIDs[dst.ChainID] {
		return nil, fmt.Errorf("cctp: unsupported chain(s) for cctp (src=%d dst=%d)", src.ChainID, dst.ChainID)
	}

	// Restrict to USDC symbol when provided; address-first callers can still pass any token address,
	// but CCTP only works for USDC, so we reject when symbol implies non-USDC.
	if src.Symbol != "" && !strings.EqualFold(src.Symbol, "USDC") {
		return nil, fmt.Errorf("cctp: only USDC supported (source.asset=%s)", src.Symbol)
	}
	if dst.Symbol != "" && !strings.EqualFold(dst.Symbol, "USDC") {
		return nil, fmt.Errorf("cctp: only USDC supported (destination.asset=%s)", dst.Symbol)
	}

	amountSmallest, err := resolveAmountBaseUnits(req, src.Token.Decimals)
	if err != nil {
		return nil, fmt.Errorf("cctp: invalid amount: %w", err)
	}

	providerData, _ := json.Marshal(map[string]any{
		"source":                  string(ProviderTierDirect),
		"protocol":                "circle_cctp",
		"src_domain":              cctpDomainID[src.ChainID],
		"dst_domain":              cctpDomainID[dst.ChainID],
		"token_messenger_src":     cctpTokenMessenger[src.ChainID],
		"token_messenger_dst":     cctpTokenMessenger[dst.ChainID],
		"message_transmitter_dst": cctpMessageTransmitter[dst.ChainID],
		"burn_token":              src.Token.Address,
		"amount":                  amountSmallest,
	})

	// Note: CCTP finality is typically minutes depending on chain finality threshold.
	return &models.Route{
		RouteID:               "cctp",
		Score:                 0,
		EstimatedOutputAmount: amountSmallest,
		EstimatedTimeSeconds:  420,
		TotalFee:              "0",
		Hops: []models.Hop{
			{
				BridgeID:          "cctp",
				HopType:           models.HopTypeBridge,
				FromChain:         firstNonEmptyString(req.Source.Chain, src.ChainKey),
				ToChain:           firstNonEmptyString(req.Destination.Chain, dst.ChainKey),
				FromAsset:         firstNonEmptyString(src.Symbol, "USDC"),
				ToAsset:           firstNonEmptyString(dst.Symbol, "USDC"),
				FromTokenAddress:  src.Token.Address,
				ToTokenAddress:    dst.Token.Address,
				AmountInBaseUnits: amountSmallest,
				EstimatedFee:      "0",
				ProviderData:      providerData,
			},
		},
	}, nil
}

