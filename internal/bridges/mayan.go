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
const mayanTxBuilderAPIURL = "https://tx-builder.mayan.finance"

// mayanChainName maps our ChainID constants to Mayan API chain name strings.
var mayanChainName = map[ChainID]string{
	ChainIDEthereum: "ethereum",
	ChainIDBase:     "base",
	ChainIDArbitrum: "arbitrum",
	ChainIDOptimism: "optimism",
	ChainIDPolygon:  "polygon",
	ChainIDBSC:      "bsc",
	ChainIDAvax:     "avalanche",
	ChainIDSolana:   "solana",
}

// mayanWormholeChainID maps our internal ChainID sentinels to Wormhole chain IDs,
// which the Mayan tx-builder uses in the signerChainId field.
var mayanWormholeChainID = map[ChainID]int{
	ChainIDEthereum: 2,
	ChainIDBase:     30,
	ChainIDArbitrum: 23,
	ChainIDOptimism: 24,
	ChainIDPolygon:  5,
	ChainIDBSC:      4,
	ChainIDAvax:     6,
	ChainIDSolana:   1, // Solana is Wormhole chain 1
}

// mayanNativeAddress is the zero address Mayan uses to represent native tokens (ETH, BNB, AVAX, MATIC).
const mayanNativeAddress = "0x0000000000000000000000000000000000000000"

// mayanFlexFloat handles fields that the old Mayan API returned as a string (e.g. "9.85")
// but the new API returns as a JSON number (e.g. 9.85).
type mayanFlexFloat struct {
	Value float64
}

func (f *mayanFlexFloat) UnmarshalJSON(b []byte) error {
	// Try plain number first (new API format).
	var v float64
	if json.Unmarshal(b, &v) == nil {
		f.Value = v
		return nil
	}
	// Fall back to quoted string (old API / test fixtures).
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	f.Value, _ = strconv.ParseFloat(s, 64)
	return nil
}

// mayanBridgeFee handles both the old { amount, symbol } object shape and the new plain float.
type mayanBridgeFee struct {
	Value float64
}

func (f *mayanBridgeFee) UnmarshalJSON(b []byte) error {
	// Try plain number first (new API format).
	var v float64
	if json.Unmarshal(b, &v) == nil {
		f.Value = v
		return nil
	}
	// Fall back to old { amount, symbol } object (test fixtures / legacy API).
	var obj struct {
		Amount string `json:"amount"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	parsed, _ := strconv.ParseFloat(obj.Amount, 64)
	f.Value = parsed
	return nil
}

// mayanRoute represents a single quote returned by the Mayan v3/quote API.
// The response wrapper changed from a bare array to { quotes: [...], minimumSdkVersion: [...] }.
// Field names also changed: eta is now etaSeconds (int), bridgeFee is now a plain float.
type mayanRoute struct {
	Type              string         `json:"type"` // "SWIFT", "WH", "MCTP"
	ExpectedAmountOut mayanFlexFloat `json:"expectedAmountOut"`
	MinAmountOut      mayanFlexFloat `json:"minAmountOut"`
	EtaSeconds        int64          `json:"etaSeconds"` // seconds (new field name)
	Eta               int64          `json:"eta"`         // kept for backward compat with mocked tests
	BridgeFee         mayanBridgeFee `json:"bridgeFee"`
	ToToken           struct {
		Contract string `json:"contract"`
		Decimals int    `json:"decimals"`
	} `json:"toToken"`
}

// mayanQuoteResponse is the new top-level Mayan API response wrapper.
type mayanQuoteResponse struct {
	Quotes             []mayanRoute `json:"quotes"`
	MinimumSdkVersion  []int        `json:"minimumSdkVersion"`
}

// MayanAdapter calls the Mayan Finance price API (Wormhole-based) for cross-chain quotes.
// No API key required for quotes. Supports EVM ↔ EVM and EVM ↔ Solana.
type MayanAdapter struct {
	client      *http.Client
	PriceAPIURL string // overrides mayanPriceAPIURL when set (used in tests)
}

func NewMayanAdapter() MayanAdapter {
	return MayanAdapter{
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

func (m MayanAdapter) ID() string   { return "mayan" }
func (m MayanAdapter) Tier() models.AdapterTier { return models.TierDegraded }

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

	priceAPI := m.PriceAPIURL
	if priceAPI == "" {
		priceAPI = mayanPriceAPIURL
	}
	u, _ := url.Parse(priceAPI)
	q := u.Query()
	q.Set("amountIn", humanAmt) // Mayan v3 uses "amountIn", not "amount"
	q.Set("fromToken", fromToken)
	q.Set("fromChain", fromChain)
	q.Set("toToken", toToken)
	q.Set("toChain", toChain)
	slippage := 0.005
	if req.Preferences != nil && req.Preferences.MaxSlippageBps > 0 {
		slippage = float64(req.Preferences.MaxSlippageBps) / 10000.0
	}
	q.Set("slippage", strconv.FormatFloat(slippage, 'f', 4, 64))
	// The Mayan v3 API requires at least one route type flag; without them it returns
	// 406 "Update to the latest sdk version". Enable all supported route types.
	q.Set("swift", "true")
	q.Set("mctp", "true")
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

	// The Mayan API switched from a bare array to a { quotes: [...] } wrapper.
	// Support both shapes to keep test fixtures working.
	var routes []mayanRoute
	if len(body) > 0 && body[0] == '[' {
		if err := json.Unmarshal(body, &routes); err != nil {
			return nil, fmt.Errorf("mayan response decode: %w", err)
		}
	} else {
		var wrapper mayanQuoteResponse
		if err := json.Unmarshal(body, &wrapper); err != nil {
			return nil, fmt.Errorf("mayan response decode: %w", err)
		}
		routes = wrapper.Quotes
	}
	if len(routes) == 0 {
		return nil, fmt.Errorf("mayan: no routes available for this token pair")
	}

	// Pick the route with the highest expectedAmountOut.
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].ExpectedAmountOut.Value > routes[j].ExpectedAmountOut.Value
	})
	best := routes[0]

	// Determine output decimals: prefer the API's toToken.decimals, fall back to registry.
	outDecimals := dst.Token.Decimals
	if best.ToToken.Decimals > 0 {
		outDecimals = best.ToToken.Decimals
	}

	outputBaseUnits, err := ethutil.ParseUnitsString(
		strconv.FormatFloat(best.ExpectedAmountOut.Value, 'f', -1, 64),
		outDecimals,
	)
	if err != nil {
		return nil, fmt.Errorf("mayan: output amount conversion: %w", err)
	}

	eta := best.EtaSeconds
	if eta == 0 {
		eta = best.Eta // old field name (used in test fixtures)
	}
	if eta == 0 {
		eta = 60
	}

	fee := strconv.FormatFloat(best.BridgeFee.Value, 'f', -1, 64)
	if fee == "0" || fee == "" {
		fee = "0"
	}

	providerData, _ := json.Marshal(map[string]any{
		"source":        string(ProviderTierDirect),
		"protocol":      "mayan_" + strings.ToLower(best.Type),
		"route_type":    best.Type,
		"src_chain_key": fromChain,
		"dst_chain_key": toChain,
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

// MayanTxResult holds the unsigned transaction data from the Mayan tx-builder.
// For EVM source chains, To/Data/Value are populated.
// For Solana source, SerializedTx and SignerPublicKey are populated instead.
type MayanTxResult struct {
	// EVM transaction fields
	To    string
	Data  string
	Value string
	// Solana transaction fields
	IsSolana        bool
	SerializedTx    string // base64-encoded Solana transaction
	SignerPublicKey string
}

// GetTransactionData fetches a fresh signed quote from the tx-builder, then calls
// POST /build to get unsigned transaction data ready for wallet signing.
// For EVM source chains it returns EVM calldata (To/Data/Value).
// For Solana source it returns a base64-encoded Solana transaction (SerializedTx).
func (m MayanAdapter) GetTransactionData(ctx context.Context, h models.Hop, senderAddr, recipientAddr string, srcChainID int) (*MayanTxResult, error) {
	fromToken := normalizeMayanTokenAddress(h.FromTokenAddress)
	toToken := normalizeMayanTokenAddress(h.ToTokenAddress)

	fromChain := h.FromChain
	toChain := h.ToChain

	// Step 1: Get a signed quote from the tx-builder's /quote endpoint.
	qURL, _ := url.Parse(mayanTxBuilderAPIURL + "/quote")
	q := qURL.Query()
	q.Set("fromToken", fromToken)
	q.Set("fromChain", fromChain)
	q.Set("toToken", toToken)
	q.Set("toChain", toChain)
	q.Set("amountIn64", h.AmountInBaseUnits)
	q.Set("slippageBps", "auto")
	q.Set("swift", "true")
	q.Set("mctp", "true")
	qURL.RawQuery = q.Encode()

	quoteReq, err := http.NewRequestWithContext(ctx, http.MethodGet, qURL.String(), nil)
	if err != nil {
		return nil, err
	}
	quoteReq.Header.Set("Accept", "application/json")

	quoteResp, err := m.client.Do(quoteReq)
	if err != nil {
		return nil, fmt.Errorf("mayan tx-builder quote: %w", err)
	}
	defer quoteResp.Body.Close()

	quoteBody, _ := io.ReadAll(quoteResp.Body)
	if quoteResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mayan tx-builder quote %s: %s", quoteResp.Status, strings.TrimSpace(string(quoteBody)))
	}

	var quoteResult struct {
		Success bool              `json:"success"`
		Quotes  []json.RawMessage `json:"quotes"`
	}
	if err := json.Unmarshal(quoteBody, &quoteResult); err != nil {
		return nil, fmt.Errorf("mayan tx-builder quote decode: %w", err)
	}
	if !quoteResult.Success || len(quoteResult.Quotes) == 0 {
		return nil, fmt.Errorf("mayan tx-builder: no quotes returned")
	}

	// Step 2: POST /build with the signed quote and wallet params.
	// signerChainId uses the Wormhole chain ID, not the EVM/internal chain ID.
	signerChainID := srcChainID
	for internalID, wormholeID := range mayanWormholeChainID {
		if int(internalID) == srcChainID {
			signerChainID = wormholeID
			break
		}
	}
	buildPayload, _ := json.Marshal(map[string]any{
		"quote": json.RawMessage(quoteResult.Quotes[0]),
		"params": map[string]any{
			"swapperAddress":     senderAddr,
			"destinationAddress": recipientAddr,
			"signerChainId":      signerChainID,
		},
	})

	buildReq, err := http.NewRequestWithContext(ctx, http.MethodPost, mayanTxBuilderAPIURL+"/build", strings.NewReader(string(buildPayload)))
	if err != nil {
		return nil, err
	}
	buildReq.Header.Set("Content-Type", "application/json")
	buildReq.Header.Set("Accept", "application/json")

	buildResp, err := m.client.Do(buildReq)
	if err != nil {
		return nil, fmt.Errorf("mayan tx-builder build: %w", err)
	}
	defer buildResp.Body.Close()

	buildBody, _ := io.ReadAll(buildResp.Body)
	if buildResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mayan tx-builder build %s: %s", buildResp.Status, strings.TrimSpace(string(buildBody)))
	}

	// The build response differs by chain category:
	//   EVM:    { chainCategory: "evm",    transaction: { to, data, value } }
	//   Solana: { chainCategory: "solana", transaction: { serializedTx, signerPublicKey } }
	var buildResult struct {
		Success     bool `json:"success"`
		Transaction struct {
			ChainCategory string          `json:"chainCategory"`
			Transaction   json.RawMessage `json:"transaction"`
		} `json:"transaction"`
	}
	if err := json.Unmarshal(buildBody, &buildResult); err != nil {
		return nil, fmt.Errorf("mayan tx-builder build decode: %w", err)
	}
	if !buildResult.Success {
		return nil, fmt.Errorf("mayan tx-builder: build failed")
	}

	cat := strings.ToLower(buildResult.Transaction.ChainCategory)

	if cat == "solana" {
		var sol struct {
			SerializedTx    string `json:"serializedTx"`
			SignerPublicKey string `json:"signerPublicKey"`
		}
		if err := json.Unmarshal(buildResult.Transaction.Transaction, &sol); err != nil {
			return nil, fmt.Errorf("mayan tx-builder: solana transaction decode: %w", err)
		}
		if sol.SerializedTx == "" {
			return nil, fmt.Errorf("mayan tx-builder: solana build returned empty serializedTx")
		}
		return &MayanTxResult{
			IsSolana:        true,
			SerializedTx:    sol.SerializedTx,
			SignerPublicKey: sol.SignerPublicKey,
		}, nil
	}

	// EVM transaction
	var evm struct {
		To    string `json:"to"`
		Data  string `json:"data"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(buildResult.Transaction.Transaction, &evm); err != nil {
		return nil, fmt.Errorf("mayan tx-builder: evm transaction decode: %w", err)
	}
	if evm.To == "" || evm.Data == "" {
		return nil, fmt.Errorf("mayan tx-builder: evm build returned empty transaction")
	}
	return &MayanTxResult{
		To:    evm.To,
		Data:  evm.Data,
		Value: evm.Value,
	}, nil
}
