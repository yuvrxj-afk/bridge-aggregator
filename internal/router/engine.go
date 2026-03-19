package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"bridge-aggregator/internal/bridges"
	"bridge-aggregator/internal/dex"
	"bridge-aggregator/internal/models"
)

const maxLogErrorLen = 120

// truncateForLog shortens s for readable one-line logs; preserves runes.
func truncateForLog(s string, max int) string {
	if max <= 0 {
		max = maxLogErrorLen
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	n := 0
	for i := range s {
		if n == max {
			return s[:i] + " …"
		}
		n++
	}
	return s + " …"
}

// ErrNoRoutes is returned when no adapter returns a valid route for the request.
var ErrNoRoutes = errors.New("no available routes for the requested pair")

// Quote returns routes from all adapters in parallel, scored by fees and estimated time (best first).
// Preferences.Priority can be "cheapest" (default) or "fastest".
// Preferences.AllowedBridges, if set, restricts which adapters are queried.
func Quote(ctx context.Context, adapters []bridges.Adapter, req models.QuoteRequest) ([]models.Route, error) {
	// If request is same-chain, do not query bridge adapters. This avoids polluting
	// swap-only requests with placeholder/canonical bridge routes.
	if isSameChainRequest(req) {
		return nil, ErrNoRoutes
	}

	allowed := make(map[string]bool)
	if req.Preferences != nil && len(req.Preferences.AllowedBridges) > 0 {
		for _, id := range req.Preferences.AllowedBridges {
			allowed[id] = true
		}
	}

	var filtered []bridges.Adapter
	for _, a := range adapters {
		if len(allowed) == 0 || allowed[a.ID()] {
			filtered = append(filtered, a)
		}
	}
	adapters = filtered

	if len(adapters) == 0 {
		return nil, ErrNoRoutes
	}

	var mu sync.Mutex
	var routes []*models.Route
	var wg sync.WaitGroup

	for _, a := range adapters {
		adapter := a
		wg.Add(1)
		go func() {
			defer wg.Done()
			route, err := adapter.GetQuote(ctx, req)
			if err != nil {
				// Don't spam logs for adapters that are intentionally not configured in baseline.
				if strings.Contains(err.Error(), "not configured") {
					return
				}
				log.Printf("[router] quote adapter=%s err=%s", adapter.ID(), truncateForLog(err.Error(), maxLogErrorLen))
				return
			}
			if route == nil || len(route.Hops) == 0 {
				return
			}
			mu.Lock()
			routes = append(routes, route)
			mu.Unlock()
		}()
	}

	wg.Wait()

	if len(routes) == 0 {
		return nil, ErrNoRoutes
	}

	priority := "cheapest"
	if req.Preferences != nil && req.Preferences.Priority != "" {
		priority = req.Preferences.Priority
	}

	// Score and sort: lower fee and lower time are better.
	for _, r := range routes {
		r.Score = scoreRoute(r, priority)
	}
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Score > routes[j].Score
	})

	out := make([]models.Route, len(routes))
	for i, r := range routes {
		out[i] = *r
	}
	return out, nil
}

func isSameChainRequest(req models.QuoteRequest) bool {
	// Prefer chain_id when present.
	if req.Source.ChainID != 0 && req.Destination.ChainID != 0 {
		return req.Source.ChainID == req.Destination.ChainID
	}
	// Fallback to chain string.
	if req.Source.Chain != "" && req.Destination.Chain != "" {
		return strings.EqualFold(req.Source.Chain, req.Destination.Chain)
	}
	return false
}

func scoreRoute(r *models.Route, priority string) float64 {
	fee, _ := strconv.ParseFloat(r.TotalFee, 64)
	timeNorm := float64(r.EstimatedTimeSeconds) / 60.0
	if timeNorm < 1 {
		timeNorm = 1
	}

	fee = fee + routeScoreFeePenalty(r)

	switch priority {
	case "fastest":
		return 1000.0 / timeNorm
	case "cheapest", "":
		fallthrough
	default:
		return 1000.0 / (1 + fee)
	}
}

func routeScoreFeePenalty(r *models.Route) float64 {
	switch routeTier(r) {
	case "aggregator":
		return 0.25
	case "placeholder":
		return 2.0
	default:
		return 0
	}
}

func routeTier(r *models.Route) string {
	if r == nil {
		return "unknown"
	}
	for _, h := range r.Hops {
		if h.HopType != models.HopTypeBridge || len(h.ProviderData) == 0 {
			continue
		}
		var v struct {
			Source string `json:"source"`
		}
		if json.Unmarshal(h.ProviderData, &v) == nil && v.Source != "" {
			return v.Source
		}
	}
	// Fallback heuristics.
	if isAggregatorSourced(r) {
		return "aggregator"
	}
	if strings.HasPrefix(r.RouteID, "canonical_") {
		return "placeholder"
	}
	return "direct"
}

func isAggregatorSourced(r *models.Route) bool {
	if r == nil {
		return false
	}
	if strings.HasPrefix(r.RouteID, "blockdaemon") {
		return true
	}
	for _, h := range r.Hops {
		if h.HopType == models.HopTypeBridge && h.BridgeID == "blockdaemon" {
			return true
		}
	}
	return false
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func mustParseFloat(s string) float64 {
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}

func buildSwapProviderData(dq *dex.Quote) json.RawMessage {
	if dq == nil {
		return nil
	}
	payload, _ := json.Marshal(map[string]any{
		"quote":      json.RawMessage(dq.ProviderQuote),
		"permitData": json.RawMessage(dq.PermitData),
		"routing":    dq.Routing,
	})
	return payload
}

// QuoteUnified returns a unified set of candidate routes, including:
// - bridge-only routes
// - swap-only routes (same-chain, one route per DEX adapter that returns a quote)
// - simple compositions (rule-based Phase 2): bridge→swap and swap→bridge when possible
func QuoteUnified(ctx context.Context, adapters []bridges.Adapter, dexAdapters []dex.Adapter, req models.QuoteRequest) ([]models.Route, error) {
	var out []models.Route

	// 1) Bridge-only routes (existing behavior).
	bridgeRoutes, bridgeErr := Quote(ctx, adapters, req)
	if bridgeErr == nil {
		out = append(out, bridgeRoutes...)
	}

	// 2) Swap-only routes on same chain (try each DEX adapter).
	if len(dexAdapters) > 0 && req.Source.Chain == req.Destination.Chain {
		for _, da := range dexAdapters {
			swapRoute, err := quoteSwapOnly(ctx, da, req)
			if err != nil {
				log.Printf("[router] dex quote adapter=%s err=%s", da.ID(), truncateForLog(err.Error(), maxLogErrorLen))
				continue
			}
			if swapRoute != nil {
				out = append(out, *swapRoute)
			}
		}
	}

	// 3) Cross-chain compositions (try each DEX adapter).
	if len(dexAdapters) > 0 && req.Source.Chain != req.Destination.Chain && !strings.EqualFold(req.Source.Asset, req.Destination.Asset) {
		for _, da := range dexAdapters {
			if rts, err := quoteBridgeThenSwap(ctx, adapters, da, req); err == nil {
				out = append(out, rts...)
			} else if !errors.Is(err, ErrNoRoutes) {
				log.Printf("[router] dex quote adapter=%s composition=bridgeThenSwap err=%s", da.ID(), truncateForLog(err.Error(), maxLogErrorLen))
			}
			if rts, err := quoteSwapThenBridge(ctx, adapters, da, req); err == nil {
				out = append(out, rts...)
			} else if !errors.Is(err, ErrNoRoutes) {
				log.Printf("[router] dex quote adapter=%s composition=swapThenBridge err=%s", da.ID(), truncateForLog(err.Error(), maxLogErrorLen))
			}
		}
		// 4) Full 3-hop swap→bridge→swap for any token on any supported chain pair.
		if rts, err := quoteSwapBridgeSwap(ctx, adapters, dexAdapters, req); err == nil {
			out = append(out, rts...)
		} else if !errors.Is(err, ErrNoRoutes) {
			log.Printf("[router] composition=swapBridgeSwap err=%s", truncateForLog(err.Error(), maxLogErrorLen))
		}
	}

	if len(out) == 0 {
		if bridgeErr != nil {
			return nil, bridgeErr
		}
		return nil, ErrNoRoutes
	}

	// Score/sort unified routes.
	priority := "cheapest"
	if req.Preferences != nil && req.Preferences.Priority != "" {
		priority = req.Preferences.Priority
	}
	for i := range out {
		out[i].Score = scoreRoute(&out[i], priority)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, nil
}

func quoteSwapOnly(ctx context.Context, dexAdapter dex.Adapter, req models.QuoteRequest) (*models.Route, error) {
	if req.Source.ChainID == 0 || req.Source.TokenAddress == "" || req.Destination.ChainID == 0 || req.Destination.TokenAddress == "" {
		return nil, errors.New("swap-only requires source/destination chain_id and token_address")
	}
	amountIn := firstNonEmpty(req.AmountBaseUnits, req.Amount)
	dq, err := dexAdapter.GetQuote(ctx, dex.QuoteRequest{
		TokenInChainID:  req.Source.ChainID,
		TokenOutChainID: req.Destination.ChainID,
		TokenIn:         req.Source.TokenAddress,
		TokenOut:        req.Destination.TokenAddress,
		Amount:          amountIn,
		Swapper:         req.Source.Address,
	})
	if err != nil {
		return nil, err
	}
	route := models.Route{
		RouteID:               "swap:" + dq.DEXID,
		EstimatedOutputAmount: dq.EstimatedOutputAmount,
		EstimatedTimeSeconds:  30,
		TotalFee:              dq.EstimatedFeeAmount,
		Hops: []models.Hop{
			{
				HopType:            models.HopTypeSwap,
				BridgeID:           dq.DEXID,
				FromChain:          req.Source.Chain,
				ToChain:            req.Source.Chain,
				FromAsset:          req.Source.Asset,
				ToAsset:            req.Destination.Asset,
				FromTokenAddress:   req.Source.TokenAddress,
				ToTokenAddress:     req.Destination.TokenAddress,
				AmountInBaseUnits:  amountIn,
				AmountOutBaseUnits: dq.EstimatedOutputAmount,
				ProviderData:       buildSwapProviderData(dq),
				EstimatedFee:       dq.EstimatedFeeAmount,
			},
		},
	}
	return &route, nil
}

func quoteBridgeThenSwap(ctx context.Context, adapters []bridges.Adapter, dexAdapter dex.Adapter, req models.QuoteRequest) ([]models.Route, error) {
	if req.Destination.ChainID == 0 || req.Destination.TokenAddress == "" {
		return nil, errors.New("bridge→swap requires destination chain_id and token_address")
	}

	// Quote bridging the source asset across (so we have the bridged token addresses on destination).
	brReq := req
	brReq.Destination.Asset = req.Source.Asset
	brReq.Destination.TokenAddress = "" // bridge adapters determine this internally
	brRoutes, err := Quote(ctx, adapters, brReq)
	if err != nil {
		return nil, err
	}

	var out []models.Route
	for _, br := range brRoutes {
		if len(br.Hops) == 0 {
			continue
		}
		h := br.Hops[len(br.Hops)-1]
		tokenIn := h.ToTokenAddress
		if tokenIn == "" {
			continue
		}
		// Use the bridge's actual output as the DEX swap input amount (not the original request amount).
		bridgeOutputAmount := firstNonEmpty(br.EstimatedOutputAmount, firstNonEmpty(req.AmountBaseUnits, req.Amount))
		dq, derr := dexAdapter.GetQuote(ctx, dex.QuoteRequest{
			TokenInChainID:  req.Destination.ChainID,
			TokenOutChainID: req.Destination.ChainID,
			TokenIn:         tokenIn,
			TokenOut:        req.Destination.TokenAddress,
			Amount:          bridgeOutputAmount,
			Swapper:         req.Destination.Address,
		})
		if derr != nil {
			continue
		}

		totalFee := mustParseFloat(br.TotalFee) + mustParseFloat(dq.EstimatedFeeAmount)
		route := models.Route{
			RouteID:               "bridge:" + h.BridgeID + "->swap:" + dq.DEXID,
			EstimatedOutputAmount: dq.EstimatedOutputAmount,
			EstimatedTimeSeconds:  br.EstimatedTimeSeconds + 30,
			TotalFee:              strconv.FormatFloat(totalFee, 'f', -1, 64),
			Hops:                  append([]models.Hop{}, br.Hops...),
		}
		route.Hops = append(route.Hops, models.Hop{
			HopType:            models.HopTypeSwap,
			BridgeID:           dq.DEXID,
			FromChain:          req.Destination.Chain,
			ToChain:            req.Destination.Chain,
			FromAsset:          brReq.Destination.Asset,
			ToAsset:            req.Destination.Asset,
			FromTokenAddress:   tokenIn,
			ToTokenAddress:     req.Destination.TokenAddress,
			AmountInBaseUnits:  bridgeOutputAmount,
			AmountOutBaseUnits: dq.EstimatedOutputAmount,
			ProviderData:       buildSwapProviderData(dq),
			EstimatedFee:       dq.EstimatedFeeAmount,
		})
		out = append(out, route)
	}
	if len(out) == 0 {
		return nil, ErrNoRoutes
	}
	return out, nil
}

func quoteSwapThenBridge(ctx context.Context, adapters []bridges.Adapter, dexAdapter dex.Adapter, req models.QuoteRequest) ([]models.Route, error) {
	// This composition requires knowing destination token address on SOURCE chain for the swap output.
	// We support it when the caller provides source-side token_address/chain_id for the destination asset
	// via metadata: source_swap_token_out_address, source_swap_token_out_decimals (optional).
	rawOutAddr, ok := req.Metadata["source_swap_token_out_address"].(string)
	if !ok || rawOutAddr == "" || req.Source.ChainID == 0 {
		return nil, ErrNoRoutes
	}

	amountIn := firstNonEmpty(req.AmountBaseUnits, req.Amount)
	dq, err := dexAdapter.GetQuote(ctx, dex.QuoteRequest{
		TokenInChainID:  req.Source.ChainID,
		TokenOutChainID: req.Source.ChainID,
		TokenIn:         req.Source.TokenAddress,
		TokenOut:        rawOutAddr,
		Amount:          amountIn,
		Swapper:         req.Source.Address,
	})
	if err != nil {
		return nil, err
	}

	// Bridge the swap output asset across by asking bridge adapters for a quote where the
	// source asset symbol is unchanged but the token address isn't directly used by bridge adapters.
	// For now this is best-effort and works cleanly when the bridged token is already in our bridge token registry.
	brReq := req
	brReq.Source.Asset = req.Destination.Asset
	brReq.AmountBaseUnits = dq.EstimatedOutputAmount
	brRoutes, berr := Quote(ctx, adapters, brReq)
	if berr != nil {
		return nil, berr
	}

	var out []models.Route
	for _, br := range brRoutes {
		route := models.Route{
			RouteID:               "swap:" + dq.DEXID + "->bridge:" + br.RouteID,
			EstimatedOutputAmount: br.EstimatedOutputAmount,
			EstimatedTimeSeconds:  30 + br.EstimatedTimeSeconds,
			TotalFee:              br.TotalFee,
			Hops: []models.Hop{
				{
					HopType:            models.HopTypeSwap,
					BridgeID:           dq.DEXID,
					FromChain:          req.Source.Chain,
					ToChain:            req.Source.Chain,
					FromAsset:          req.Source.Asset,
					ToAsset:            req.Destination.Asset,
					FromTokenAddress:   req.Source.TokenAddress,
					ToTokenAddress:     rawOutAddr,
					AmountInBaseUnits:  amountIn,
					AmountOutBaseUnits: dq.EstimatedOutputAmount,
					ProviderData:       buildSwapProviderData(dq),
					EstimatedFee:       dq.EstimatedFeeAmount,
				},
			},
		}
		route.Hops = append(route.Hops, br.Hops...)
		out = append(out, route)
	}
	if len(out) == 0 {
		return nil, ErrNoRoutes
	}
	return out, nil
}

// quoteSwapBridgeSwap composes full 3-hop routes: DEX swap on source chain → bridge → DEX swap
// on destination chain. This enables any-token-any-chain routing for bridges that do not
// natively handle arbitrary token pairs (CCTP, Stargate, canonical L2s).
//
// For each registered DEX adapter × bridge adapter × candidate intermediate token, it:
//  1. Quotes a DEX swap (srcToken → intermediate) on the source chain.
//  2. Quotes the bridge (intermediate srcChain → intermediate dstChain).
//  3. Quotes a DEX swap (intermediate → dstToken) on the destination chain.
//
// Across and Mayan are excluded from this composition because they handle arbitrary
// token pairs internally via their own routing APIs.
func quoteSwapBridgeSwap(ctx context.Context, adapters []bridges.Adapter, dexAdapters []dex.Adapter, req models.QuoteRequest) ([]models.Route, error) {
	if req.Source.ChainID == 0 || req.Source.TokenAddress == "" ||
		req.Destination.ChainID == 0 || req.Destination.TokenAddress == "" {
		return nil, errors.New("swap→bridge→swap requires chain_id and token_address for both endpoints")
	}

	srcChainID := bridges.ChainID(req.Source.ChainID)
	dstChainID := bridges.ChainID(req.Destination.ChainID)
	if srcChainID == dstChainID {
		return nil, ErrNoRoutes
	}

	amountIn := firstNonEmpty(req.AmountBaseUnits, req.Amount)

	var out []models.Route

	for _, da := range dexAdapters {
		for _, ba := range adapters {
			// Skip bridges that handle arbitrary tokens natively — their GetQuote already
			// does swap+bridge+swap internally, so a 3-hop composition would duplicate them.
			switch ba.ID() {
			case "across", "mayan", "blockdaemon":
				continue
			}

			candidates := bridges.BridgeIntermediateCandidates(ba.ID(), srcChainID, dstChainID)
			if len(candidates) == 0 {
				continue
			}

			for _, symbol := range candidates {
				srcInterim, ok := bridges.TokenByChainAndSymbol[srcChainID][symbol]
				if !ok {
					continue
				}
				dstInterim, ok := bridges.TokenByChainAndSymbol[dstChainID][symbol]
				if !ok {
					continue
				}

				// Skip if intermediate equals source token (bridge→swap covers it).
				if strings.EqualFold(srcInterim.Address, req.Source.TokenAddress) {
					continue
				}
				// Skip if intermediate equals destination token (swap→bridge covers it).
				if strings.EqualFold(dstInterim.Address, req.Destination.TokenAddress) {
					continue
				}

				// Step 1: DEX swap on source chain: srcToken → intermediate.
				step1, err := da.GetQuote(ctx, dex.QuoteRequest{
					TokenInChainID:  req.Source.ChainID,
					TokenOutChainID: req.Source.ChainID,
					TokenIn:         req.Source.TokenAddress,
					TokenOut:        srcInterim.Address,
					Amount:          amountIn,
					Swapper:         req.Source.Address,
				})
				if err != nil {
					continue
				}

				// Step 2: Bridge intermediate token across chains.
				brReq := req
				brReq.Source.Asset = symbol
				brReq.Source.TokenAddress = srcInterim.Address
				brReq.Source.TokenDecimals = srcInterim.Decimals
				brReq.Destination.Asset = symbol
				brReq.Destination.TokenAddress = dstInterim.Address
				brReq.Destination.TokenDecimals = dstInterim.Decimals
				brReq.AmountBaseUnits = step1.EstimatedOutputAmount
				brReq.Amount = ""

				bridgeRoute, berr := ba.GetQuote(ctx, brReq)
				if berr != nil {
					continue
				}

				// Step 3: DEX swap on destination chain: intermediate → dstToken.
				bridgeOut := firstNonEmpty(bridgeRoute.EstimatedOutputAmount, step1.EstimatedOutputAmount)
				step3, err := da.GetQuote(ctx, dex.QuoteRequest{
					TokenInChainID:  req.Destination.ChainID,
					TokenOutChainID: req.Destination.ChainID,
					TokenIn:         dstInterim.Address,
					TokenOut:        req.Destination.TokenAddress,
					Amount:          bridgeOut,
					Swapper:         req.Destination.Address,
				})
				if err != nil {
					continue
				}

				totalFee := mustParseFloat(step1.EstimatedFeeAmount) +
					mustParseFloat(bridgeRoute.TotalFee) +
					mustParseFloat(step3.EstimatedFeeAmount)

				routeID := fmt.Sprintf("swap:%s->bridge:%s->swap:%s", da.ID(), ba.ID(), da.ID())
				route := models.Route{
					RouteID:               routeID,
					EstimatedOutputAmount: step3.EstimatedOutputAmount,
					EstimatedTimeSeconds:  30 + bridgeRoute.EstimatedTimeSeconds + 30,
					TotalFee:              strconv.FormatFloat(totalFee, 'f', -1, 64),
					Hops: []models.Hop{
						{
							HopType:            models.HopTypeSwap,
							BridgeID:           da.ID(),
							FromChain:          req.Source.Chain,
							ToChain:            req.Source.Chain,
							FromAsset:          req.Source.Asset,
							ToAsset:            symbol,
							FromTokenAddress:   req.Source.TokenAddress,
							ToTokenAddress:     srcInterim.Address,
							AmountInBaseUnits:  amountIn,
							AmountOutBaseUnits: step1.EstimatedOutputAmount,
							ProviderData:       buildSwapProviderData(step1),
							EstimatedFee:       step1.EstimatedFeeAmount,
						},
					},
				}
				// Append all bridge hops (usually one, but could be multiple for multi-hop bridges).
				route.Hops = append(route.Hops, bridgeRoute.Hops...)
				// Append destination swap hop.
				route.Hops = append(route.Hops, models.Hop{
					HopType:            models.HopTypeSwap,
					BridgeID:           da.ID(),
					FromChain:          req.Destination.Chain,
					ToChain:            req.Destination.Chain,
					FromAsset:          symbol,
					ToAsset:            req.Destination.Asset,
					FromTokenAddress:   dstInterim.Address,
					ToTokenAddress:     req.Destination.TokenAddress,
					AmountInBaseUnits:  bridgeOut,
					AmountOutBaseUnits: step3.EstimatedOutputAmount,
					ProviderData:       buildSwapProviderData(step3),
					EstimatedFee:       step3.EstimatedFeeAmount,
				})

				out = append(out, route)
			}
		}
	}

	if len(out) == 0 {
		return nil, ErrNoRoutes
	}
	return out, nil
}

// QuoteWithDEX wraps Quote and, when no bridge routes are available, falls back to a
// same-chain DEX swap using the provided dex.Adapter. It returns a single synthetic
// Route representing the swap.
func QuoteWithDEX(ctx context.Context, adapters []bridges.Adapter, dexAdapter dex.Adapter, req models.QuoteRequest) ([]models.Route, error) {
	// First try normal bridge routes.
	routes, err := Quote(ctx, adapters, req)
	if err == nil && len(routes) > 0 {
		return routes, nil
	}
	if !errors.Is(err, ErrNoRoutes) {
		return nil, err
	}
	if dexAdapter == nil {
		return nil, err
	}
	// Only use DEX for same-chain swaps.
	if req.Source.Chain != req.Destination.Chain {
		return nil, ErrNoRoutes
	}

	dq, derr := dexAdapter.GetQuote(ctx, dex.QuoteRequest{
		TokenInChainID:  req.Source.ChainID,
		TokenOutChainID: req.Destination.ChainID,
		TokenIn:         req.Source.TokenAddress,
		TokenOut:        req.Destination.TokenAddress,
		Amount:          firstNonEmpty(req.AmountBaseUnits, req.Amount),
	})
	if derr != nil {
		log.Printf("[router] dex quote err=%s", truncateForLog(derr.Error(), maxLogErrorLen))
		return nil, ErrNoRoutes
	}

	route := models.Route{
		RouteID:               "dex-" + dq.DEXID,
		EstimatedOutputAmount: dq.EstimatedOutputAmount,
		EstimatedTimeSeconds:  30,
		TotalFee:              dq.EstimatedFeeAmount,
		Hops: []models.Hop{
			{
				HopType:            models.HopTypeSwap,
				BridgeID:           dq.DEXID,
				FromChain:          req.Source.Chain,
				ToChain:            req.Source.Chain,
				FromAsset:          req.Source.Asset,
				ToAsset:            req.Destination.Asset,
				FromTokenAddress:   req.Source.TokenAddress,
				ToTokenAddress:     req.Destination.TokenAddress,
				AmountInBaseUnits:  firstNonEmpty(req.AmountBaseUnits, req.Amount),
				AmountOutBaseUnits: dq.EstimatedOutputAmount,
				ProviderData:       buildSwapProviderData(dq),
				EstimatedFee:       dq.EstimatedFeeAmount,
			},
		},
	}
	route.Score = scoreRoute(&route, "cheapest")
	return []models.Route{route}, nil
}
