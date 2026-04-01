package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"bridge-aggregator/internal/bridges"
	"bridge-aggregator/internal/dex"
	"bridge-aggregator/internal/models"
	"bridge-aggregator/internal/router"
)

// EnrichQuoteRequest fills in ChainID and TokenAddress on Source and Destination
// from the known token registry when the caller omits them (e.g. the frontend
// sends only chain name + asset symbol). This is required for the composition
// engine (quoteBridgeThenSwap, quoteSwapBridgeSwap) which needs both fields.
func EnrichQuoteRequest(req models.QuoteRequest) models.QuoteRequest {
	if req.Source.ChainID == 0 && req.Source.Chain != "" {
		req.Source.ChainID = int(bridges.ChainIDFromName(req.Source.Chain))
	}
	if req.Destination.ChainID == 0 && req.Destination.Chain != "" {
		req.Destination.ChainID = int(bridges.ChainIDFromName(req.Destination.Chain))
	}
	if req.Source.TokenAddress == "" && req.Source.ChainID != 0 && req.Source.Asset != "" {
		if m, ok := bridges.TokenByChainAndSymbol[bridges.ChainID(req.Source.ChainID)]; ok {
			if t, ok := m[strings.ToUpper(req.Source.Asset)]; ok {
				req.Source.TokenAddress = t.Address
				if req.Source.TokenDecimals == 0 {
					req.Source.TokenDecimals = t.Decimals
				}
			}
		}
	}
	if req.Destination.TokenAddress == "" && req.Destination.ChainID != 0 && req.Destination.Asset != "" {
		if m, ok := bridges.TokenByChainAndSymbol[bridges.ChainID(req.Destination.ChainID)]; ok {
			if t, ok := m[strings.ToUpper(req.Destination.Asset)]; ok {
				req.Destination.TokenAddress = t.Address
				if req.Destination.TokenDecimals == 0 {
					req.Destination.TokenDecimals = t.Decimals
				}
			}
		}
	}
	return req
}

// Quote returns routes for the given request using registered adapters and DEX adapters.
func Quote(ctx context.Context, adapters []bridges.Adapter, dexAdapters []dex.Adapter, req models.QuoteRequest) (*models.QuoteResponse, error) {
	req = EnrichQuoteRequest(req)
	routes, err := router.QuoteUnified(ctx, adapters, dexAdapters, req)
	if err != nil {
		if errors.Is(err, router.ErrNoRoutes) {
			return &models.QuoteResponse{Routes: nil}, err
		}
		return nil, err
	}
	routes = filterSaneRoutes(routes)
	routes = filterSaneRoutesWithReference(ctx, routes)
	routes = filterMinInputValue(ctx, routes)
	if len(routes) == 0 {
		return &models.QuoteResponse{Routes: nil}, router.ErrNoRoutes
	}
	return &models.QuoteResponse{Routes: routes}, nil
}

// filterSaneRoutes drops clearly invalid/unsafe routes before they reach the UI:
// - missing/zero/non-numeric output amount
// - absurdly large output amount (likely decimals/parsing bug)
func filterSaneRoutes(routes []models.Route) []models.Route {
	out := make([]models.Route, 0, len(routes))
	maxReasonable := new(big.Int).Exp(big.NewInt(10), big.NewInt(40), nil) // 1e40 base units hard ceiling
	for _, r := range routes {
		if r.EstimatedOutputAmount == "" {
			continue
		}
		v, ok := new(big.Int).SetString(r.EstimatedOutputAmount, 10)
		if !ok || v.Sign() <= 0 {
			continue
		}
		if v.Cmp(maxReasonable) > 0 {
			continue
		}
		out = append(out, r)
	}
	return out
}

// filterMinInputValue drops cross-chain routes where the input USD value is
// too low for economical bridging. On Ethereum L1, gas for a bridge deposit
// is $5-100+, so bridging $0.50 makes no sense. The threshold is conservative
// to avoid silent drops — Across and others have their own minimums on-chain.
const minBridgeInputUSD = 0.50

func filterMinInputValue(ctx context.Context, routes []models.Route) []models.Route {
	if len(routes) == 0 {
		return routes
	}
	prices, err := fetchReferencePricesUSD(ctx)
	if err != nil || len(prices) == 0 {
		return routes
	}
	out := make([]models.Route, 0, len(routes))
	for _, r := range routes {
		if len(r.Hops) == 0 {
			out = append(out, r)
			continue
		}
		first := r.Hops[0]
		isCrossChain := first.FromChain != r.Hops[len(r.Hops)-1].ToChain
		if !isCrossChain {
			out = append(out, r)
			continue
		}
		srcUSD, ok := amountUSD(first.FromChain, first.FromAsset, first.AmountInBaseUnits, prices)
		if !ok || srcUSD >= minBridgeInputUSD {
			out = append(out, r)
			continue
		}
		// Drop: input value is below minimum for bridging.
	}
	return out
}

var coingeckoIDBySymbol = map[string]string{
	"ETH":    "ethereum",
	"WETH":   "ethereum",
	"MATIC":  "matic-network",
	"POL":    "matic-network",
	"WMATIC": "matic-network",
	"USDC":   "usd-coin",
	"USDT":   "tether",
	"DAI":    "dai",
	"WBTC":   "wrapped-bitcoin",
}

// filterSaneRoutesWithReference enforces strict market sanity on top of numeric sanity.
// It drops routes whose implied USD value is wildly off relative to source input.
func filterSaneRoutesWithReference(ctx context.Context, routes []models.Route) []models.Route {
	if len(routes) == 0 {
		return routes
	}
	prices, err := fetchReferencePricesUSD(ctx)
	if err != nil || len(prices) == 0 {
		// Fail-open on reference API outage; numeric sanity already applied.
		return routes
	}

	out := make([]models.Route, 0, len(routes))
	for _, r := range routes {
		if len(r.Hops) == 0 {
			continue
		}
		first := r.Hops[0]
		last := r.Hops[len(r.Hops)-1]

		srcUSD, okIn := amountUSD(first.FromChain, first.FromAsset, first.AmountInBaseUnits, prices)
		dstUSD, okOut := amountUSD(last.ToChain, last.ToAsset, r.EstimatedOutputAmount, prices)
		if !okIn || !okOut || srcUSD <= 0 || dstUSD <= 0 {
			// If we cannot price both sides reliably, keep route (avoid false negatives).
			out = append(out, r)
			continue
		}

		ratio := dstUSD / srcUSD
		// Conservative production rails:
		// - below 40% value retention is likely broken quoting/decimals/liquidity anomaly
		// - above 250% is likely a pricing/parsing bug
		if ratio < 0.40 || ratio > 2.50 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
			continue
		}
		out = append(out, r)
	}
	return out
}

func amountUSD(chainName, symbol, baseUnits string, prices map[string]float64) (float64, bool) {
	if baseUnits == "" {
		return 0, false
	}
	priceID, ok := coingeckoIDBySymbol[strings.ToUpper(symbol)]
	if !ok {
		return 0, false
	}
	price, ok := prices[priceID]
	if !ok || price <= 0 {
		return 0, false
	}
	decimals := tokenDecimalsByChainSymbol(chainName, symbol)
	if decimals <= 0 {
		return 0, false
	}
	amt, err := formatUnitsFloat(baseUnits, decimals)
	if err != nil || amt <= 0 {
		return 0, false
	}
	return amt * price, true
}

func tokenDecimalsByChainSymbol(chainName, symbol string) int {
	chainID := bridges.ChainIDFromName(chainName)
	if chainID == 0 {
		return 0
	}
	m, ok := bridges.TokenByChainAndSymbol[bridges.ChainID(chainID)]
	if !ok {
		return 0
	}
	t, ok := m[strings.ToUpper(symbol)]
	if !ok {
		return 0
	}
	return t.Decimals
}

func formatUnitsFloat(baseUnits string, decimals int) (float64, error) {
	v, ok := new(big.Int).SetString(baseUnits, 10)
	if !ok {
		return 0, fmt.Errorf("invalid big integer")
	}
	if decimals == 0 {
		f, _ := new(big.Rat).SetInt(v).Float64()
		return f, nil
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	r := new(big.Rat).SetFrac(v, scale)
	f, _ := r.Float64()
	return f, nil
}

func fetchReferencePricesUSD(ctx context.Context) (map[string]float64, error) {
	ids := make([]string, 0, len(coingeckoIDBySymbol))
	seen := map[string]bool{}
	for _, id := range coingeckoIDBySymbol {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	u, _ := url.Parse("https://api.coingecko.com/api/v3/simple/price")
	q := u.Query()
	q.Set("ids", strings.Join(ids, ","))
	q.Set("vs_currencies", "usd")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reference pricing status=%d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var raw map[string]map[string]float64
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	out := make(map[string]float64, len(raw))
	for id, row := range raw {
		if usd, ok := row["usd"]; ok && usd > 0 {
			out[id] = usd
		}
	}
	return out, nil
}
