package bridges

// BridgeSupportedSymbols maps bridgeID → chainID → list of token symbols that bridge natively
// supports for that chain. Used by the router to select candidate intermediate tokens for
// swap→bridge→swap composition.
//
// Only include bridges that do NOT handle arbitrary token pairs themselves (i.e., not Across
// or Mayan, which already accept any inputToken/outputToken and route internally).
// These bridges require specific bridgeable tokens as inputs and outputs.
var BridgeSupportedSymbols = map[string]map[ChainID][]string{
	// CCTP: Circle USDC only, on all supported chains.
	"cctp": {
		ChainIDEthereum: {"USDC"},
		ChainIDBase:     {"USDC"},
		ChainIDArbitrum: {"USDC"},
		ChainIDOptimism: {"USDC"},
		ChainIDPolygon:  {"USDC"},
		ChainIDBSC:      {"USDC"},
		ChainIDAvax:     {"USDC"},
	},

	// Stargate (LayerZero): USDC and USDT on most chains; ETH on EVM chains.
	"stargate": {
		ChainIDEthereum: {"USDC", "USDT", "ETH"},
		ChainIDBase:     {"USDC", "ETH"},
		ChainIDArbitrum: {"USDC", "USDT", "ETH"},
		ChainIDOptimism: {"USDC", "USDT", "ETH"},
		ChainIDPolygon:  {"USDC", "USDT"},
		ChainIDBSC:      {"USDT"},
		ChainIDAvax:     {"USDC", "USDT"},
	},

	// Canonical Base bridge (Ethereum ↔ Base only, 7-min deposit / ~1-hour withdrawal).
	"canonical_base": {
		ChainIDEthereum: {"ETH", "USDC", "USDT", "WBTC"},
		ChainIDBase:     {"ETH", "USDC", "USDT", "WBTC"},
	},

	// Canonical Optimism bridge (Ethereum ↔ Optimism only, 7-day withdrawal).
	"canonical_optimism": {
		ChainIDEthereum: {"ETH", "USDC", "USDT", "WBTC"},
		ChainIDOptimism: {"ETH", "USDC", "USDT", "WBTC"},
	},

	// Canonical Arbitrum bridge (Ethereum ↔ Arbitrum only, 7-day withdrawal).
	"canonical_arbitrum": {
		ChainIDEthereum: {"ETH", "USDC", "USDT", "WBTC"},
		ChainIDArbitrum: {"ETH", "USDC", "USDT", "WBTC"},
	},
}

// BridgeIntermediateCandidates returns the intermediate token symbols that bridgeID supports
// on both srcChain and dstChain simultaneously. These are the valid intermediate tokens for
// a swap→bridge→swap route via that bridge.
func BridgeIntermediateCandidates(bridgeID string, srcChain, dstChain ChainID) []string {
	supported, ok := BridgeSupportedSymbols[bridgeID]
	if !ok {
		return nil
	}
	srcSymbols := supported[srcChain]
	dstSymbols := supported[dstChain]
	if len(srcSymbols) == 0 || len(dstSymbols) == 0 {
		return nil
	}
	// Return only symbols available on BOTH chains.
	dstSet := make(map[string]bool, len(dstSymbols))
	for _, s := range dstSymbols {
		dstSet[s] = true
	}
	var candidates []string
	for _, s := range srcSymbols {
		if dstSet[s] {
			candidates = append(candidates, s)
		}
	}
	return candidates
}
