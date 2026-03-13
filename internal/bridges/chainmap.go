package bridges

// ChainID is an EVM chain ID.
type ChainID int64

const (
	ChainIDEthereum ChainID = 1
	ChainIDArbitrum  ChainID = 42161
	ChainIDOptimism  ChainID = 10
	ChainIDPolygon   ChainID = 137
)

// TokenInfo is a token's contract address and decimals on a chain.
type TokenInfo struct {
	Address  string
	Decimals int
}

// ChainNameToID maps our API chain names (lowercase) to chain IDs.
var ChainNameToID = map[string]ChainID{
	"ethereum": ChainIDEthereum,
	"arbitrum": ChainIDArbitrum,
	"optimism": ChainIDOptimism,
	"polygon":  ChainIDPolygon,
}

// TokenByChainAndSymbol maps (chain ID, symbol) to token address and decimals.
// Symbol is uppercase (e.g. USDC, ETH).
var TokenByChainAndSymbol = map[ChainID]map[string]TokenInfo{
	ChainIDEthereum: {
		"USDC": {Address: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", Decimals: 6},
		"ETH":  {Address: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", Decimals: 18},
	},
	ChainIDArbitrum: {
		"USDC": {Address: "0xaf88d065e77c8cC2239327C5EDb3A432268e5831", Decimals: 6},
		"ETH":  {Address: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", Decimals: 18},
	},
	ChainIDOptimism: {
		"USDC": {Address: "0x0b2C639c533813f4Aa9D7837CAf62653d097Ff85", Decimals: 6},
		"ETH":  {Address: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", Decimals: 18},
	},
	ChainIDPolygon: {
		"USDC": {Address: "0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359", Decimals: 6},
		"ETH":  {Address: "0x7ceB23fD6bC0adD59E62ac25578270cFf1b9f619", Decimals: 18},
	},
}
