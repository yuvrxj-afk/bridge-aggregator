package bridges

import (
	"encoding/json"
	"strings"

	"bridge-aggregator/internal/models"
)

// ChainID is an EVM chain ID.
type ChainID int64

const (
	ChainIDEthereum ChainID = 1
	ChainIDBase     ChainID = 8453
	ChainIDArbitrum ChainID = 42161
	ChainIDOptimism ChainID = 10
	ChainIDPolygon  ChainID = 137
	ChainIDBSC      ChainID = 56
	ChainIDAvax     ChainID = 43114
	// ChainIDSolana is a sentinel for Solana mainnet. Solana has no EVM chain ID;
	// 900 is used internally to distinguish it from all real EVM chains.
	ChainIDSolana ChainID = 900

	// Testnet chain IDs — only activated when NETWORK=testnet via RegisterTestnetChains().
	ChainIDSepolia         ChainID = 11155111
	ChainIDBaseSepolia     ChainID = 84532
	ChainIDArbitrumSepolia ChainID = 421614
	ChainIDOPSepolia       ChainID = 11155420
	// ChainIDSolanaDevnet is a sentinel for Solana devnet (distinct from mainnet sentinel 900).
	ChainIDSolanaDevnet ChainID = 901
)

// TokenInfo is a token's contract address and decimals on a chain.
type TokenInfo struct {
	Address  string
	Decimals int
}

// ChainNameToID maps our API chain names (lowercase) to chain IDs.
var ChainNameToID = map[string]ChainID{
	"ethereum":  ChainIDEthereum,
	"base":      ChainIDBase,
	"arbitrum":  ChainIDArbitrum,
	"optimism":  ChainIDOptimism,
	"polygon":   ChainIDPolygon,
	"bsc":       ChainIDBSC,
	"avalanche": ChainIDAvax,
	"solana":    ChainIDSolana,
}

// ChainIDFromName maps a lowercase chain name to its integer chain ID, returning 0 if unknown.
func ChainIDFromName(name string) int64 {
	id, ok := ChainNameToID[strings.ToLower(name)]
	if !ok {
		return 0
	}
	return int64(id)
}

// TokenByChainAndSymbol maps (chain ID, symbol) to token address and decimals.
// Symbol is uppercase (e.g. USDC, ETH, WETH).
// ETH entry uses the 0xEeee... sentinel address (Across/CCTP convention for native token).
// WETH entry uses the actual wrapped token contract address (used by DEXes).
// MATIC/BNB/AVAX entries use 0xEeee... sentinel for native; WMATIC/WBNB/WAVAX for wrapped.
var TokenByChainAndSymbol = map[ChainID]map[string]TokenInfo{
	ChainIDEthereum: {
		"ETH":  {Address: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", Decimals: 18},
		"WETH": {Address: "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", Decimals: 18},
		"USDC": {Address: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", Decimals: 6},
		"USDT": {Address: "0xdAC17F958D2ee523a2206206994597C13D831ec7", Decimals: 6},
		"DAI":  {Address: "0x6B175474E89094C44Da98b954EedeAC495271d0F", Decimals: 18},
		"WBTC": {Address: "0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599", Decimals: 8},
	},
	ChainIDBase: {
		"ETH":  {Address: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", Decimals: 18},
		"WETH": {Address: "0x4200000000000000000000000000000000000006", Decimals: 18},
		"USDC": {Address: "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913", Decimals: 6},
		"USDT": {Address: "0xfde4C96c8593536E31F229EA8f37b2ADa2699bb2", Decimals: 6},
		"DAI":  {Address: "0x50c5725949A6F0c72E6C4a641F24049A917DB0Cb", Decimals: 18},
		// cbBTC is the canonical BTC token on Base (Coinbase-wrapped)
		"WBTC": {Address: "0xcbB7C0000aB88B473b1f5aFd9ef808440eed33Bf", Decimals: 8},
	},
	ChainIDArbitrum: {
		"ETH":  {Address: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", Decimals: 18},
		"WETH": {Address: "0x82aF49447D8a07e3bd95BD0d56f35241523fBab1", Decimals: 18},
		"USDC": {Address: "0xaf88d065e77c8cC2239327C5EDb3A432268e5831", Decimals: 6},
		"USDT": {Address: "0xFd086bC7CD5C481DCC9C85ebE478A1C0b69FCbb9", Decimals: 6},
		"DAI":  {Address: "0xDA10009cBd5D07dd0CeCc66161FC93D7c9000da1", Decimals: 18},
		"WBTC": {Address: "0x2f2a2543B76A4166549F7aaB2e75Bef0aefC5B0f", Decimals: 8},
	},
	ChainIDOptimism: {
		"ETH":  {Address: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", Decimals: 18},
		"WETH": {Address: "0x4200000000000000000000000000000000000006", Decimals: 18},
		"USDC": {Address: "0x0b2C639c533813f4Aa9D7837CAf62653d097Ff85", Decimals: 6},
		"USDT": {Address: "0x94b008aA00579c1307B0EF2c499aD98a8ce58e58", Decimals: 6},
		"DAI":  {Address: "0xDA10009cBd5D07dd0CeCc66161FC93D7c9000da1", Decimals: 18},
		"WBTC": {Address: "0x68f180fcCe6836688e9084f035309E29Bf0A2095", Decimals: 8},
	},
	ChainIDPolygon: {
		// MATIC/POL is the native token on Polygon; WMATIC is its wrapped ERC-20.
		"MATIC":  {Address: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", Decimals: 18},
		"WMATIC": {Address: "0x0d500B1d8E8eF31E21C99d1Db9A6444d3ADf1270", Decimals: 18},
		// ETH on Polygon is bridged Ethereum WETH (PoS bridge). Both ETH and WETH point here.
		"ETH":  {Address: "0x7ceB23fD6bC0adD59E62ac25578270cFf1b9f619", Decimals: 18},
		"WETH": {Address: "0x7ceB23fD6bC0adD59E62ac25578270cFf1b9f619", Decimals: 18},
		"USDC": {Address: "0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359", Decimals: 6},
		"USDT": {Address: "0xc2132D05D31c914a87C6611C10748AEb04B58e8F", Decimals: 6},
		"DAI":  {Address: "0x8f3Cf7ad23Cd3CaDbD9735AFf958023239c6A063", Decimals: 18},
		"WBTC": {Address: "0x1BFD67037B42Cf73acF2047067bd4F2C47D9BfD6", Decimals: 8},
	},
	ChainIDBSC: {
		// BNB is the native token on BSC.
		"BNB":  {Address: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", Decimals: 18},
		"WBNB": {Address: "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c", Decimals: 18},
		// Bridged ETH on BSC (from Binance Bridge).
		"ETH":  {Address: "0x2170Ed0880ac9A755fd29B2688956BD959F933F8", Decimals: 18},
		"USDC": {Address: "0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d", Decimals: 18},
		"USDT": {Address: "0x55d398326f99059fF775485246999027B3197955", Decimals: 18},
		"DAI":  {Address: "0x1AF3F329e8BE154074D8769D1FFa4eE058B1DBc3", Decimals: 18},
		// BTCB is wrapped BTC on BSC.
		"WBTC": {Address: "0x7130d2A12B9BCbFAe4f2634d864A1Ee1Ce3Ead9c", Decimals: 18},
	},
	ChainIDAvax: {
		// AVAX is the native token on Avalanche C-Chain.
		"AVAX":  {Address: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", Decimals: 18},
		"WAVAX": {Address: "0xB31f66AA3C1e785363F0875A1B74E27b85FD66c7", Decimals: 18},
		// Bridged ETH on Avalanche.
		"ETH":  {Address: "0x49D5c2BdFfac6CE2BFdB6640F4F80f226bc10bAB", Decimals: 18},
		"USDC": {Address: "0xB97EF9Ef8734C71904D8002F8b6Bc66Dd9c48a6E", Decimals: 6},
		"USDT": {Address: "0x9702230A8Ea53601f5cD2dc00fDBc13d4dF4A8c7", Decimals: 6},
		"DAI":  {Address: "0xd586E7F844cEa2F87f50152665BCbc2C279D8d70", Decimals: 18},
		"WBTC": {Address: "0x50b7545627a5162F82A992c33b87aDc75187B218", Decimals: 8},
	},
	// Solana mainnet — token addresses are SPL mint pubkeys (base58).
	// SOL uses the Wrapped SOL mint as its canonical address (Mayan convention).
	ChainIDSolana: {
		"SOL":  {Address: "So11111111111111111111111111111111111111112", Decimals: 9},
		"USDC": {Address: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", Decimals: 6},
		"USDT": {Address: "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB", Decimals: 6},
	},
}

// RegisterTestnetChains activates testnet chain IDs, token addresses, and adapter contract
// registries. Called once at startup when NETWORK=testnet. Safe to call multiple times.
// Does NOT modify any mainnet chain IDs or token addresses.
func RegisterTestnetChains() {
	// Chain name → ID mappings for testnet chains.
	ChainNameToID["sepolia"] = ChainIDSepolia
	ChainNameToID["base-sepolia"] = ChainIDBaseSepolia
	ChainNameToID["arbitrum-sepolia"] = ChainIDArbitrumSepolia
	ChainNameToID["op-sepolia"] = ChainIDOPSepolia
	ChainNameToID["solana-devnet"] = ChainIDSolanaDevnet

	// Testnet USDC addresses from Circle's sandbox deployment.
	// WETH addresses from each chain's canonical wrapped ETH deployment.
	TokenByChainAndSymbol[ChainIDSepolia] = map[string]TokenInfo{
		"ETH":  {Address: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", Decimals: 18},
		"WETH": {Address: "0xfFf9976782d46CC05630D1f6eBAb18b2324d6B14", Decimals: 18},
		"USDC": {Address: "0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238", Decimals: 6},
	}
	TokenByChainAndSymbol[ChainIDBaseSepolia] = map[string]TokenInfo{
		"ETH":  {Address: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", Decimals: 18},
		"WETH": {Address: "0x4200000000000000000000000000000000000006", Decimals: 18},
		"USDC": {Address: "0x036CbD53842c5426634e7929541eC2318f3dCF7e", Decimals: 6},
	}
	TokenByChainAndSymbol[ChainIDArbitrumSepolia] = map[string]TokenInfo{
		"ETH":  {Address: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", Decimals: 18},
		"WETH": {Address: "0x980B62Da83eFf3D4576C647993b0c1D7faf17c73", Decimals: 18},
		"USDC": {Address: "0x75faf114eafb1BDbe2F0316DF893fd58CE46AA4d", Decimals: 6},
	}
	TokenByChainAndSymbol[ChainIDOPSepolia] = map[string]TokenInfo{
		"ETH":  {Address: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", Decimals: 18},
		"WETH": {Address: "0x4200000000000000000000000000000000000006", Decimals: 18},
		"USDC": {Address: "0x5fd84259d66Cd46123540766Be93DFE6D43130D7", Decimals: 6},
	}
	// Solana devnet USDC mint (Circle's official devnet deployment).
	TokenByChainAndSymbol[ChainIDSolanaDevnet] = map[string]TokenInfo{
		"SOL":  {Address: "So11111111111111111111111111111111111111112", Decimals: 9},
		"USDC": {Address: "4zMMC9srt5Ri5X14GAgXhaHii3GnPAEERYPJgZJDncDU", Decimals: 6},
	}

	// Activate testnet support for individual adapters.
	registerCCTPTestnetChains()
	registerCanonicalTestnetChains()
}

// HopHasAcrossDeposit returns true if the hop's provider_data already contains deposit params.
func HopHasAcrossDeposit(hop models.Hop) bool {
	if len(hop.ProviderData) == 0 {
		return false
	}
	var pd map[string]json.RawMessage
	if err := json.Unmarshal(hop.ProviderData, &pd); err != nil {
		return false
	}
	dep, ok := pd["deposit"]
	return ok && len(dep) > 0 && string(dep) != "null"
}

// InjectAcrossDeposit returns a copy of the hop with deposit params merged into provider_data.
func InjectAcrossDeposit(hop models.Hop, deposit *AcrossDepositParams) (models.Hop, error) {
	var pd map[string]json.RawMessage
	if len(hop.ProviderData) > 0 {
		if err := json.Unmarshal(hop.ProviderData, &pd); err != nil {
			return hop, err
		}
	} else {
		pd = map[string]json.RawMessage{
			"protocol": json.RawMessage(`"across_v3"`),
		}
	}
	depBytes, err := json.Marshal(deposit)
	if err != nil {
		return hop, err
	}
	pd["deposit"] = depBytes
	updated, err := json.Marshal(pd)
	if err != nil {
		return hop, err
	}
	hop.ProviderData = updated
	return hop, nil
}
