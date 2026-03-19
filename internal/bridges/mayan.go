package bridges

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"bridge-aggregator/internal/ethutil"
	"bridge-aggregator/internal/models"
)

const mayanPriceAPIURL = "https://price-api.mayan.finance/v3/quote"

// mayanChainName maps our ChainID constants to Mayan API chain name strings.
var mayanChainName = map[ChainID]string{
	ChainIDEthereum: "ethereum",
	ChainIDBase:     "base",
	ChainIDArbitrum: "arbitrum",
	ChainIDOptimism: "optimism",
	ChainIDPolygon:  "polygon",
	ChainIDBSC:      "bsc",
	ChainIDAvax:     "avalanche",
}

// mayanNativeAddress is the zero address Mayan uses to represent native tokens (ETH, BNB, AVAX, MATIC).
const mayanNativeAddress = "0x0000000000000000000000000000000000000000"

// mayanRoute represents a single route option returned by the Mayan v3/quote API.
type mayanRoute struct {
	Type              string  `json:"type"` // "SWIFT", "WH", "MCTP"
	ExpectedAmountOut string  `json:"expectedAmountOut"`
	MinAmountOut      string  `json:"minAmountOut"`
	Eta               int64   `json:"eta"` // seconds
	BridgeFee         struct {
		Amount string `json:"amount"`
		Symbol string `json:"symbol"`
	} `json:"bridgeFee"`
	ToToken struct {
		Contract string `json:"contract"`
		Decimals int    `json:"decimals"`
	} `json:"toToken"`
}

// MayanAdapter calls the Mayan Finance price API (Wormhole-based) for cross-chain quotes.
// No API key required for quotes. Supports EVM ↔ EVM and EVM ↔ Solana.
type MayanAdapter struct {
	client *http.Client
}

func NewMayanAdapter() MayanAdapter {
	return MayanAdapter{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (m MayanAdapter) ID() string { return "mayan" }

// mayanNativeSymbols groups native/wrapped token pairs that Mayan considers equivalent
// (e.g. ETH and WETH are the same asset on different chains).
var mayanNativeSymbols = map[string]string{
	"ETH":   "ETH",
	"WETH":  "ETH",
	"MATIC": "MATIC",
	"WMATIC": "MATIC",
	"BNB":   "BNB",
	"WBNB":  "BNB",
	"AVAX":  "AVAX",
	"WAVAX": "AVAX",
}

// mayanTokensCompatible returns true when Mayan can bridge between the two token symbols.
// Mayan routes the same underlying asset across chains (USDC→USDC, ETH→ETH/WETH, etc.).
func mayanTokensCompatible(srcSymbol, dstSymbol string) bool {
	s := strings.ToUpper(srcSymbol)
	d := strings.ToUpper(dstSymbol)
	if s == d {
		return true
	}
	// Treat native and wrapped as the same asset.
	sg := mayanNativeSymbols[s]
	dg := mayanNativeSymbols[d]
	return sg != "" && sg == dg
}

// normalizeMayanTokenAddress converts our sentinel 0xEeee... native address to Mayan's 0x000... format.
func normalizeMayanTokenAddress(addr string) string {
	if strings.EqualFold(addr, "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE") {
		return mayanNativeAddress
	}
	return addr
}

func (m MayanAdapter) GetQuote(ctx context.Context, req models.QuoteRequest) (*models.Route, error) {
	src, err := resolveBridgeEndpoint(req.Source)
	if err != nil {
		return nil, fmt.Errorf("mayan: %w", err)
	}
	dst, err := resolveBridgeEndpoint(req.Destination)
	if err != nil {
		return nil, fmt.Errorf("mayan: %w", err)
	}

	if src.ChainID == dst.ChainID {
		return nil, fmt.Errorf("mayan: same-chain swaps not supported")
	}

	// Mayan is a same-token Wormhole bridge: it moves a token from one chain to the same
	// token on another chain (USDC→USDC, ETH→ETH). Cross-token pairs (USDC→ETH) are not
	// supported and return a 500 from the API.
	if !mayanTokensCompatible(src.Symbol, dst.Symbol) {
		return nil, fmt.Errorf("mayan: cross-token bridging not supported (%s→%s); use swap+bridge composition", src.Symbol, dst.Symbol)
	}

	fromChain, ok := mayanChainName[src.ChainID]
	if !ok {
		return nil, fmt.Errorf("mayan: unsupported source chain %d", src.ChainID)
	}
	toChain, ok := mayanChainName[dst.ChainID]
	if !ok {
		return nil, fmt.Errorf("mayan: unsupported destination chain %d", dst.ChainID)
	}

	amountSmallest, err := resolveAmountBaseUnits(req, src.Token.Decimals)
	if err != nil {
		return nil, fmt.Errorf("mayan: invalid amount: %w", err)
	}

	// Mayan takes human-readable decimal amounts.
	humanAmt, err := ethutil.FormatUnits(amountSmallest, src.Token.Decimals)
	if err != nil {
		return nil, fmt.Errorf("mayan: amount conversion: %w", err)
	}

	fromToken := normalizeMayanTokenAddress(src.Token.Address)
	toToken := normalizeMayanTokenAddress(dst.Token.Address)

	u, _ := url.Parse(mayanPriceAPIURL)
	q := u.Query()
	q.Set("amountIn", humanAmt) // Mayan v3 uses "amountIn", not "amount"
	q.Set("fromToken", fromToken)
	q.Set("fromChain", fromChain)
	q.Set("toToken", toToken)
	q.Set("toChain", toChain)
	q.Set("slippage", "0.005")
	u.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/json")

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("mayan: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mayan api %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var routes []mayanRoute
	if err := json.Unmarshal(body, &routes); err != nil {
		return nil, fmt.Errorf("mayan response decode: %w", err)
	}
	if len(routes) == 0 {
		return nil, fmt.Errorf("mayan: no routes available for this token pair")
	}

	// Pick the route with the highest expectedAmountOut.
	sort.Slice(routes, func(i, j int) bool {
		ai, _ := strconv.ParseFloat(routes[i].ExpectedAmountOut, 64)
		aj, _ := strconv.ParseFloat(routes[j].ExpectedAmountOut, 64)
		return ai > aj
	})
	best := routes[0]

	// Determine output decimals: prefer the API's toToken.decimals, fall back to registry.
	outDecimals := dst.Token.Decimals
	if best.ToToken.Decimals > 0 {
		outDecimals = best.ToToken.Decimals
	}

	outputBaseUnits, err := ethutil.ParseUnitsString(best.ExpectedAmountOut, outDecimals)
	if err != nil {
		return nil, fmt.Errorf("mayan: output amount conversion: %w", err)
	}

	eta := best.Eta
	if eta == 0 {
		eta = 60
	}

	fee := best.BridgeFee.Amount
	if fee == "" {
		fee = "0"
	}

	providerData, _ := json.Marshal(map[string]any{
		"source":     string(ProviderTierDirect),
		"protocol":   "mayan_" + strings.ToLower(best.Type),
		"route_type": best.Type,
	})

	toTokenAddr := dst.Token.Address
	if best.ToToken.Contract != "" && best.ToToken.Contract != mayanNativeAddress {
		toTokenAddr = best.ToToken.Contract
	}

	return &models.Route{
		RouteID:               "mayan",
		Score:                 0,
		EstimatedOutputAmount: outputBaseUnits,
		EstimatedTimeSeconds:  eta,
		TotalFee:              fee,
		Hops: []models.Hop{
			{
				BridgeID:          "mayan",
				HopType:           models.HopTypeBridge,
				FromChain:         firstNonEmptyString(req.Source.Chain, src.ChainKey),
				ToChain:           firstNonEmptyString(req.Destination.Chain, dst.ChainKey),
				FromAsset:         src.Symbol,
				ToAsset:           dst.Symbol,
				FromTokenAddress:  src.Token.Address,
				ToTokenAddress:    toTokenAddr,
				AmountInBaseUnits: amountSmallest,
				EstimatedFee:      fee,
				ProviderData:      providerData,
			},
		},
	}, nil
}
