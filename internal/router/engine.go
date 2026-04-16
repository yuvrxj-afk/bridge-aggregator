package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"sort"
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
var ErrInvalidRouteFee = errors.New("invalid route fee")

// quoteIncompleteReason validates that a route has the minimum fields required for
// execution. Returns a non-empty string describing why the quote is incomplete, or
// empty string if it is complete. Quote data does not include tx material — full
// tx validation happens at stepTransaction time. Here we check structural completeness.
func quoteIncompleteReason(r *models.Route) string {
	if r.EstimatedOutputAmount == "" || r.EstimatedOutputAmount == "0" {
		return "estimated_output_amount is zero or missing"
	}
	for i, hop := range r.Hops {
		if hop.AmountInBaseUnits == "" || hop.AmountInBaseUnits == "0" {
			return fmt.Sprintf("hop %d has zero amount_in_base_units", i)
		}
	}
	return ""
}

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

	// Exclude tier 3 (uncredentialed) and tier 4 (config broken) adapters from fan-out.
	// They are guaranteed to fail and waste the timeout budget.
	eligible := make([]bridges.Adapter, 0, len(adapters))
	for _, a := range adapters {
		if a.Tier() <= models.TierDegraded {
			eligible = append(eligible, a)
		} else {
			log.Printf("[router] skipping adapter=%s tier=%d (not eligible for fan-out)", a.ID(), a.Tier())
		}
	}
	adapters = eligible

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
				log.Printf("[router] quote adapter=%s tier=%d err=%s", adapter.ID(), adapter.Tier(), truncateForLog(err.Error(), maxLogErrorLen))
				return
			}
			if route == nil || len(route.Hops) == 0 {
				return
			}
			if reason := quoteIncompleteReason(route); reason != "" {
				log.Printf("[router] dropped incomplete quote adapter=%s reason=%s", adapter.ID(), reason)
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
	scored := make([]*models.Route, 0, len(routes))
	for _, r := range routes {
		if _, err := parseNonNegativeDecimal(r.TotalFee); err != nil {
			log.Printf("[router] dropped route=%s reason=invalid total_fee: %s", r.RouteID, truncateForLog(err.Error(), maxLogErrorLen))
			continue
		}
		r.Score = scoreRoute(r, priority)
		scored = append(scored, r)
	}
	if len(scored) == 0 {
		return nil, ErrNoRoutes
	}
	sort.Slice(scored, func(i, j int) bool {
		return compareRoutes(scored[i], scored[j], priority) > 0
	})

	out := make([]models.Route, len(scored))
	for i, r := range scored {
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
	fee, err := parseNonNegativeDecimal(r.TotalFee)
	if err != nil {
		return 0
	}
	penalty := new(big.Rat).Add(routeScoreFeePenaltyRat(r), routeScoreExecutionPenaltyRat(r))
	feeWithPenalty := new(big.Rat).Add(fee, penalty)

	switch priority {
	case "fastest":
		timeNorm := big.NewRat(r.EstimatedTimeSeconds, 60)
		if timeNorm.Cmp(big.NewRat(1, 1)) < 0 {
			timeNorm = big.NewRat(1, 1)
		}
		score := new(big.Rat).Quo(big.NewRat(1000, 1), timeNorm)
		f, _ := score.Float64()
		return f
	case "cheapest", "":
		fallthrough
	default:
		den := new(big.Rat).Add(big.NewRat(1, 1), feeWithPenalty)
		score := new(big.Rat).Quo(big.NewRat(1000, 1), den)
		f, _ := score.Float64()
		return f
	}
}

func routeScoreExecutionPenaltyRat(r *models.Route) *big.Rat {
	if r == nil || r.Execution == nil {
		return big.NewRat(1, 2)
	}
	if !r.Execution.Supported {
		return big.NewRat(50, 1)
	}
	switch r.Execution.Intent {
	case "atomic_one_click":
		return new(big.Rat)
	case "guided_two_step":
		return big.NewRat(1, 10)
	case "async_claim":
		return big.NewRat(3, 2)
	default:
		return big.NewRat(1, 2)
	}
}

func routeScoreFeePenaltyRat(r *models.Route) *big.Rat {
	switch routeTier(r) {
	case "aggregator":
		return big.NewRat(1, 4)
	case "placeholder":
		return big.NewRat(2, 1)
	default:
		return new(big.Rat)
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

func jsonString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

func parseNonNegativeDecimal(s string) (*big.Rat, error) {
	if strings.TrimSpace(s) == "" {
		return nil, fmt.Errorf("%w: empty", ErrInvalidRouteFee)
	}
	if s != strings.TrimSpace(s) {
		return nil, fmt.Errorf("%w: whitespace not allowed", ErrInvalidRouteFee)
	}
	if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		return nil, fmt.Errorf("%w: sign not allowed", ErrInvalidRouteFee)
	}
	dotCount := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '.' {
			dotCount++
			if dotCount > 1 {
				return nil, fmt.Errorf("%w: invalid decimal format", ErrInvalidRouteFee)
			}
			continue
		}
		if ch < '0' || ch > '9' {
			return nil, fmt.Errorf("%w: invalid character", ErrInvalidRouteFee)
		}
	}
	if s == "." {
		return nil, fmt.Errorf("%w: invalid decimal format", ErrInvalidRouteFee)
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, fmt.Errorf("%w: parse failed", ErrInvalidRouteFee)
	}
	if r.Sign() < 0 {
		return nil, fmt.Errorf("%w: negative", ErrInvalidRouteFee)
	}
	return r, nil
}

func formatRatDecimal(r *big.Rat) string {
	if r == nil {
		return "0"
	}
	return r.FloatString(18)
}

func compareRoutes(a, b *models.Route, priority string) int {
	if priority == "fastest" {
		ta := a.EstimatedTimeSeconds
		tb := b.EstimatedTimeSeconds
		if ta < tb {
			return 1
		}
		if ta > tb {
			return -1
		}
	} else {
		fa, errA := parseNonNegativeDecimal(a.TotalFee)
		fb, errB := parseNonNegativeDecimal(b.TotalFee)
		if errA == nil && errB == nil {
			pa := new(big.Rat).Add(fa, routeScoreFeePenaltyRat(a))
			pa.Add(pa, routeScoreExecutionPenaltyRat(a))
			pb := new(big.Rat).Add(fb, routeScoreFeePenaltyRat(b))
			pb.Add(pb, routeScoreExecutionPenaltyRat(b))
			if c := pb.Cmp(pa); c != 0 {
				return c
			}
		}
	}
	if a.Score > b.Score {
		return 1
	}
	if a.Score < b.Score {
		return -1
	}
	return 0
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
	filtered := make([]models.Route, 0, len(out))
	for i := range out {
		out[i].Execution = deriveExecutionProfile(&out[i])
		if _, err := parseNonNegativeDecimal(out[i].TotalFee); err != nil {
			log.Printf("[router] dropped route=%s reason=invalid total_fee: %s", out[i].RouteID, truncateForLog(err.Error(), maxLogErrorLen))
			continue
		}
		out[i].Score = scoreRoute(&out[i], priority)
		filtered = append(filtered, out[i])
	}
	if len(filtered) == 0 {
		return nil, ErrNoRoutes
	}
	sort.Slice(filtered, func(i, j int) bool { return compareRoutes(&filtered[i], &filtered[j], priority) > 0 })
	return filtered, nil
}

// QuoteStream runs all adapters in parallel (same as QuoteUnified) and fires onRoute
// immediately for each valid result rather than collecting them. This enables SSE
// streaming: callers receive routes as they arrive instead of waiting for all adapters.
// The priority parameter controls per-route scoring ("cheapest" / "fastest" / "best").
// Context cancellation propagates to all in-flight HTTP requests.
func QuoteStream(ctx context.Context, adapters []bridges.Adapter, dexAdapters []dex.Adapter, req models.QuoteRequest, onRoute func(models.Route)) {
	priority := "cheapest"
	if req.Preferences != nil && req.Preferences.Priority != "" {
		priority = req.Preferences.Priority
	}

	emit := func(r *models.Route) {
		if r == nil || len(r.Hops) == 0 {
			return
		}
		r.Execution = deriveExecutionProfile(r)
		r.Score = scoreRoute(r, priority)
		onRoute(*r)
	}

	var wg sync.WaitGroup

	// 1. Bridge-only routes (parallel per adapter).
	if !isSameChainRequest(req) {
		allowed := make(map[string]bool)
		if req.Preferences != nil {
			for _, id := range req.Preferences.AllowedBridges {
				allowed[id] = true
			}
		}
		for _, a := range adapters {
			adapter := a
			if len(allowed) > 0 && !allowed[adapter.ID()] {
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				route, err := adapter.GetQuote(ctx, req)
				if err != nil {
					if !strings.Contains(err.Error(), "not configured") {
						log.Printf("[router/stream] bridge=%s err=%s", adapter.ID(), truncateForLog(err.Error(), maxLogErrorLen))
					}
					return
				}
				emit(route)
			}()
		}
	}

	// 2. Swap-only routes (same-chain, one goroutine per DEX adapter).
	if len(dexAdapters) > 0 && req.Source.Chain == req.Destination.Chain {
		for _, da := range dexAdapters {
			dexA := da
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, err := quoteSwapOnly(ctx, dexA, req)
				if err != nil {
					log.Printf("[router/stream] dex=%s err=%s", dexA.ID(), truncateForLog(err.Error(), maxLogErrorLen))
					return
				}
				emit(r)
			}()
		}
	}

	// 3. Cross-chain compositions (bridge+swap) — run sequentially per DEX to avoid explosion.
	if len(dexAdapters) > 0 && req.Source.Chain != req.Destination.Chain && !strings.EqualFold(req.Source.Asset, req.Destination.Asset) {
		for _, da := range dexAdapters {
			dexA := da
			wg.Add(1)
			go func() {
				defer wg.Done()
				if rts, err := quoteBridgeThenSwap(ctx, adapters, dexA, req); err == nil {
					for i := range rts {
						emit(&rts[i])
					}
				}
				if rts, err := quoteSwapThenBridge(ctx, adapters, dexA, req); err == nil {
					for i := range rts {
						emit(&rts[i])
					}
				}
			}()
		}
		// Full 3-hop compositions — one goroutine.
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rts, err := quoteSwapBridgeSwap(ctx, adapters, dexAdapters, req); err == nil {
				for i := range rts {
					emit(&rts[i])
				}
			}
		}()
	}

	wg.Wait()
}

func deriveExecutionProfile(r *models.Route) *models.ExecutionProfile {
	p := &models.ExecutionProfile{
		Supported: false,
		Intent:    "unsupported",
		Guarantee: "unknown",
		Recovery:  "manual",
	}
	if r == nil || len(r.Hops) == 0 {
		p.Reasons = []string{"empty_route"}
		return p
	}

	bridgeHops := 0
	var bridgeHop *models.Hop
	for i := range r.Hops {
		h := &r.Hops[i]
		if h.HopType == models.HopTypeBridge || (h.HopType == "" && h.BridgeID != "") {
			bridgeHops++
			bridgeHop = h
		}
	}
	if bridgeHops == 0 {
		// Same-chain swap route.
		p.Supported = true
		p.Intent = "guided_two_step"
		p.Guarantee = "manual_recovery_required"
		p.Recovery = "resumable_guided"
		p.Requirements = []string{"wallet_connected", "source_network_selected", "approval_if_needed"}
		p.Metadata = map[string]string{"mode": "swap_only"}
		return p
	}
	if bridgeHops > 1 {
		p.Reasons = []string{"multi_bridge_not_supported"}
		return p
	}

	if bridgeHop == nil {
		p.Reasons = []string{"bridge_hop_missing"}
		return p
	}

	// Provider-sourced metadata.
	pd := map[string]json.RawMessage{}
	if len(bridgeHop.ProviderData) > 0 {
		_ = json.Unmarshal(bridgeHop.ProviderData, &pd)
	}
	protocol := jsonString(pd["protocol"])
	crossSwapType := jsonString(pd["cross_swap_type"])

	switch bridgeHop.BridgeID {
	case "across":
		p.Supported = true
		p.Guarantee = "relay_fill_or_refund"
		p.Recovery = "resumable_guided"
		p.Requirements = []string{
			"wallet_connected",
			"source_network_selected",
			"allowance_or_approval",
			"fresh_quote_before_submit",
		}
		if crossSwapType == "anyToBridgeable" || crossSwapType == "bridgeableToAny" || crossSwapType == "anyToAny" {
			p.Intent = "guided_two_step"
			p.Metadata = map[string]string{
				"execution_path":  "across_swap_tx",
				"protocol":        firstNonEmpty(protocol, "across_v3"),
				"cross_swap_type": crossSwapType,
			}
			return p
		}
		// bridgeable / bridgeableToBridgeable routes normally use LiFi Diamond one-click.
		// However if the router has composed a destination-side swap hop after the bridge
		// (e.g. bridge USDC then swap USDC→USDT on destination), LiFi Diamond cannot
		// handle that second hop — downgrade to guided_two_step so the frontend routes
		// through the hop-by-hop stepTransaction path instead.
		hasDestSwap := false
		for i := range r.Hops {
			if &r.Hops[i] == bridgeHop {
				for _, h := range r.Hops[i+1:] {
					if h.HopType == models.HopTypeSwap {
						hasDestSwap = true
					}
				}
				break
			}
		}
		if hasDestSwap {
			p.Intent = "guided_two_step"
			p.Metadata = map[string]string{
				"execution_path":  "step_transaction_bridge_then_swap",
				"protocol":        firstNonEmpty(protocol, "across_v3"),
				"cross_swap_type": firstNonEmpty(crossSwapType, "bridgeable"),
			}
		} else {
			p.Intent = "atomic_one_click"
			p.Metadata = map[string]string{
				"execution_path":  "lifi_diamond_across",
				"protocol":        firstNonEmpty(protocol, "across_v3"),
				"cross_swap_type": firstNonEmpty(crossSwapType, "bridgeable"),
			}
		}
		return p
	case "cctp":
		p.Supported = true
		p.Intent = "async_claim"
		p.Guarantee = "manual_recovery_required"
		p.Recovery = "resumable_guided"
		p.Requirements = []string{
			"wallet_connected",
			"source_network_selected",
			"approval_if_needed",
			"attestation_fetch_required",
			"destination_claim_required",
		}
		p.Metadata = map[string]string{"protocol": firstNonEmpty(protocol, "circle_cctp")}
		return p
	case "stargate":
		if hopIsExecutable(*bridgeHop) {
			p.Supported = true
			p.Intent = "guided_two_step"
			p.Guarantee = "relay_fill_or_refund"
			p.Recovery = "resumable_guided"
			p.Requirements = []string{"wallet_connected", "source_network_selected", "approval_if_needed"}
			p.Metadata = map[string]string{
				"protocol": "layerzero_stargate_v2",
				"provider": firstNonEmpty(protocol, "stargate"),
			}
			return p
		}
		p.Reasons = []string{"stargate_execution_not_integrated"}
		return p
	case "mayan":
		if hopIsExecutable(*bridgeHop) {
			p.Supported = true
			p.Intent = "guided_two_step"
			p.Guarantee = "relay_fill_or_refund"
			p.Recovery = "resumable_guided"
			p.Requirements = []string{"wallet_connected", "source_network_selected", "approval_if_needed"}
			p.Metadata = map[string]string{"protocol": firstNonEmpty(protocol, "mayan")}
			return p
		}
		p.Reasons = []string{"mayan_execution_not_integrated"}
		return p
	case "blockdaemon":
		p.Reasons = []string{"quote_only_aggregator"}
		p.Metadata = map[string]string{"protocol": firstNonEmpty(protocol, "blockdaemon_defi_api")}
		return p
	case "canonical_base", "canonical_optimism", "canonical_arbitrum":
		depositOnL1 := jsonString(pd["deposit_on_l1"])

		// L1→L2 deposits are supported via stepTransaction.
		if depositOnL1 == "true" {
			p.Supported = true
			p.Intent = "guided_two_step"
			p.Guarantee = "relay_fill_or_refund"
			p.Recovery = "manual"
			p.Requirements = []string{
				"wallet_connected",
				"source_network_selected",
				"approval_if_needed",
			}
			p.Metadata = map[string]string{
				"protocol":       firstNonEmpty(protocol, "canonical"),
				"execution_path": "step_transaction_canonical",
				"bridge":         bridgeHop.BridgeID,
			}
			return p
		}

		// L2→L1 withdrawals: initiation is supported, but claim requires a separate step
		// after the 7-day finality window (OP/Base) or challenge period (Arbitrum).
		isETH := jsonString(pd["input_token"]) == "0x0000000000000000000000000000000000000000" ||
			jsonString(pd["input_token"]) == "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE"

		// Arbitrum L2→L1 ERC-20 is not yet supported (no GatewayRouter on L2 implemented).
		if bridgeHop.BridgeID == "canonical_arbitrum" && !isETH {
			p.Reasons = []string{"canonical_arbitrum_l2_erc20_withdrawal_not_supported"}
			p.Metadata = map[string]string{"protocol": firstNonEmpty(protocol, "canonical")}
			return p
		}

		p.Supported = true
		p.Intent = "async_claim"
		p.Guarantee = "finality_then_claim"
		p.Recovery = "manual"
		p.Requirements = []string{
			"wallet_connected",
			"source_network_selected",
			"approval_if_needed",
			"claim_after_finality",
		}
		p.Metadata = map[string]string{
			"protocol":       firstNonEmpty(protocol, "canonical"),
			"execution_path": "step_transaction_withdrawal",
			"bridge":         bridgeHop.BridgeID,
			"finality":       "7_days",
		}
		return p
	default:
		p.Reasons = []string{"unsupported_bridge_execution"}
		p.Metadata = map[string]string{"bridge_id": bridgeHop.BridgeID}
		return p
	}
}

func quoteSwapOnly(ctx context.Context, dexAdapter dex.Adapter, req models.QuoteRequest) (*models.Route, error) {
	if req.Source.ChainID == 0 || req.Source.TokenAddress == "" || req.Destination.ChainID == 0 || req.Destination.TokenAddress == "" {
		return nil, errors.New("swap-only requires source/destination chain_id and token_address")
	}
	amountIn := firstNonEmpty(req.AmountBaseUnits, req.Amount)
	swapSlippage := 0
	if req.Preferences != nil {
		swapSlippage = req.Preferences.MaxSlippageBps
	}
	tokenIn := normalizeDexTokenAddress(req.Source.ChainID, req.Source.TokenAddress)
	tokenOut := normalizeDexTokenAddress(req.Destination.ChainID, req.Destination.TokenAddress)
	dq, err := dexAdapter.GetQuote(ctx, dex.QuoteRequest{
		TokenInChainID:  req.Source.ChainID,
		TokenOutChainID: req.Destination.ChainID,
		TokenIn:         tokenIn,
		TokenOut:        tokenOut,
		Amount:          amountIn,
		Swapper:         req.Source.Address,
		MaxSlippageBps:  swapSlippage,
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
				FromTokenAddress:   tokenIn,
				ToTokenAddress:     tokenOut,
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

	// Exclude canonical (L1↔L2 finality) bridges from bridge+swap compositions.
	// Canonical bridges have ~15-minute finality and only move the same token — they
	// are not suitable as the bridge leg of a bridge→swap route.
	var composableAdapters []bridges.Adapter
	for _, a := range adapters {
		if !strings.HasPrefix(a.ID(), "canonical_") {
			composableAdapters = append(composableAdapters, a)
		}
	}

	// Quote bridging the source asset across (so we have the bridged token addresses on destination).
	brReq := req
	brReq.Destination.Asset = req.Source.Asset
	brReq.Destination.TokenAddress = "" // bridge adapters determine this internally
	brRoutes, err := Quote(ctx, composableAdapters, brReq)
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
		// Skip bridge hops that are missing execution data (e.g. Across without deposit params).
		// Such routes can be quoted but not executed, so omit them from composed routes.
		if !hopIsExecutable(h) {
			continue
		}
		// Use the bridge's actual output as the DEX swap input amount (not the original request amount).
		bridgeOutputAmount := firstNonEmpty(br.EstimatedOutputAmount, firstNonEmpty(req.AmountBaseUnits, req.Amount))
		// Use destination address if provided; fall back to source address.
		// On EVM, the user's wallet is the same address on every chain.
		// This ensures the Uniswap calldata sets the correct output recipient.
		swapper := firstNonEmpty(req.Destination.Address, req.Source.Address)
		bridgeSwapSlippage := 0
		if req.Preferences != nil {
			bridgeSwapSlippage = req.Preferences.MaxSlippageBps
		}
		dq, derr := dexAdapter.GetQuote(ctx, dex.QuoteRequest{
			TokenInChainID:  req.Destination.ChainID,
			TokenOutChainID: req.Destination.ChainID,
			TokenIn:         tokenIn,
			TokenOut:        req.Destination.TokenAddress,
			Amount:          bridgeOutputAmount,
			Swapper:         swapper,
			MaxSlippageBps:  bridgeSwapSlippage,
		})
		if derr != nil {
			continue
		}

		totalFee, feeErr := sumFeeStrings(br.TotalFee, dq.EstimatedFeeAmount)
		if feeErr != nil {
			log.Printf("[router] dropped composed route bridge=%s dex=%s reason=%s", h.BridgeID, dq.DEXID, truncateForLog(feeErr.Error(), maxLogErrorLen))
			continue
		}
		route := models.Route{
			RouteID:               "bridge:" + h.BridgeID + "->swap:" + dq.DEXID,
			EstimatedOutputAmount: dq.EstimatedOutputAmount,
			EstimatedTimeSeconds:  br.EstimatedTimeSeconds + 30,
			TotalFee:              totalFee,
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
	stbSlippage := 0
	if req.Preferences != nil {
		stbSlippage = req.Preferences.MaxSlippageBps
	}
	tokenIn := normalizeDexTokenAddress(req.Source.ChainID, req.Source.TokenAddress)
	tokenOut := normalizeDexTokenAddress(req.Source.ChainID, rawOutAddr)
	dq, err := dexAdapter.GetQuote(ctx, dex.QuoteRequest{
		TokenInChainID:  req.Source.ChainID,
		TokenOutChainID: req.Source.ChainID,
		TokenIn:         tokenIn,
		TokenOut:        tokenOut,
		Amount:          amountIn,
		Swapper:         req.Source.Address,
		MaxSlippageBps:  stbSlippage,
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
	// Exclude canonical bridges from swap→bridge compositions:
	// - Canonical bridges only move the same token (no cross-token bridging)
	// - They have long finality and are not suitable as the bridge leg after a swap
	var composableAdapters []bridges.Adapter
	for _, a := range adapters {
		if !strings.HasPrefix(a.ID(), "canonical_") {
			composableAdapters = append(composableAdapters, a)
		}
	}
	brRoutes, berr := Quote(ctx, composableAdapters, brReq)
	if berr != nil {
		return nil, berr
	}

	var out []models.Route
	for _, br := range brRoutes {
		totalFee, feeErr := sumFeeStrings(dq.EstimatedFeeAmount, br.TotalFee)
		if feeErr != nil {
			log.Printf("[router] dropped composed route dex=%s bridge=%s reason=%s", dq.DEXID, br.RouteID, truncateForLog(feeErr.Error(), maxLogErrorLen))
			continue
		}
		route := models.Route{
			RouteID:               "swap:" + dq.DEXID + "->bridge:" + br.RouteID,
			EstimatedOutputAmount: br.EstimatedOutputAmount,
			EstimatedTimeSeconds:  30 + br.EstimatedTimeSeconds,
			TotalFee:              totalFee,
			Hops: []models.Hop{
				{
					HopType:            models.HopTypeSwap,
					BridgeID:           dq.DEXID,
					FromChain:          req.Source.Chain,
					ToChain:            req.Source.Chain,
					FromAsset:          req.Source.Asset,
					ToAsset:            req.Destination.Asset,
					FromTokenAddress:   tokenIn,
					ToTokenAddress:     tokenOut,
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
			// Canonical bridges are not valid bridge legs for swap→bridge→swap.
			// They only move the same token and can produce contradictory provider_data
			// when asked to bridge an intermediate like USDC.
			if strings.HasPrefix(ba.ID(), "canonical_") {
				continue
			}
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
				compSlippage := 0
				if req.Preferences != nil {
					compSlippage = req.Preferences.MaxSlippageBps
				}
				tokenIn := normalizeDexTokenAddress(req.Source.ChainID, req.Source.TokenAddress)
				step1, err := da.GetQuote(ctx, dex.QuoteRequest{
					TokenInChainID:  req.Source.ChainID,
					TokenOutChainID: req.Source.ChainID,
					TokenIn:         tokenIn,
					TokenOut:        srcInterim.Address,
					Amount:          amountIn,
					Swapper:         req.Source.Address,
					MaxSlippageBps:  compSlippage,
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

				// Skip bridge results that are missing execution data (e.g. Across without deposit params).
				if len(bridgeRoute.Hops) == 0 || !hopIsExecutable(bridgeRoute.Hops[len(bridgeRoute.Hops)-1]) {
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
					MaxSlippageBps:  compSlippage,
				})
				if err != nil {
					continue
				}

				totalFee, feeErr := sumFeeStrings(step1.EstimatedFeeAmount, bridgeRoute.TotalFee, step3.EstimatedFeeAmount)
				if feeErr != nil {
					log.Printf("[router] dropped 3-hop route bridge=%s dex=%s reason=%s", ba.ID(), da.ID(), truncateForLog(feeErr.Error(), maxLogErrorLen))
					continue
				}

				routeID := fmt.Sprintf("swap:%s->bridge:%s->swap:%s", da.ID(), ba.ID(), da.ID())
				route := models.Route{
					RouteID:               routeID,
					EstimatedOutputAmount: step3.EstimatedOutputAmount,
					EstimatedTimeSeconds:  30 + bridgeRoute.EstimatedTimeSeconds + 30,
					TotalFee:              totalFee,
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

func sumFeeStrings(fees ...string) (string, error) {
	total := new(big.Rat)
	for _, fee := range fees {
		r, err := parseNonNegativeDecimal(fee)
		if err != nil {
			return "", err
		}
		total.Add(total, r)
	}
	return formatRatDecimal(total), nil
}

// hopIsExecutable returns true when a bridge hop carries enough provider_data to build
// an on-chain transaction via PopulateStepTransaction. It rejects hops that only have
// a quote (e.g. Across routes where the /swap/approval response omitted deposit params).
func hopIsExecutable(h models.Hop) bool {
	if h.HopType == models.HopTypeSwap {
		return true // swap hops: execution data is embedded in provider_data by DEX adapters
	}
	if len(h.ProviderData) == 0 {
		return false
	}
	var pd map[string]json.RawMessage
	if err := json.Unmarshal(h.ProviderData, &pd); err != nil {
		return false
	}
	protocol := ""
	if raw, ok := pd["protocol"]; ok {
		_ = json.Unmarshal(raw, &protocol)
	}
	switch protocol {
	case "across_v3":
		// bridgeable / bridgeableToBridgeable: deposit params are fetched fresh at build time
		// via /suggested-fees, so a missing deposit in cached provider_data is OK.
		dep, ok := pd["deposit"]
		if ok && len(dep) > 0 && string(dep) != "null" {
			return true
		}
		// "bridgeableToBridgeable": same symbol, different chain addresses (e.g. USDC Arb→Polygon).
		// /swap/approval returns no deposit, but /suggested-fees always works for these.
		crossType := ""
		if raw, ok2 := pd["cross_swap_type"]; ok2 {
			_ = json.Unmarshal(raw, &crossType)
		}
		if crossType == "bridgeableToBridgeable" || crossType == "bridgeable" {
			return true
		}
		// anyToBridgeable: pre-built swapTx from Across
		swapTx, ok3 := pd["swap_tx"]
		return ok3 && len(swapTx) > 0 && string(swapTx) != "null"
	case "circle_cctp":
		_, hasSrc := pd["token_messenger_src"]
		_, hasBurn := pd["burn_token"]
		return hasSrc && hasBurn
	case "canonical":
		_, hasL1 := pd["l1_bridge"]
		_, hasL1Inbox := pd["l1_inbox"]
		return hasL1 || hasL1Inbox
	case "layerzero_stargate_v2":
		_, hasSrc := pd["src_chain_key"]
		_, hasDst := pd["dst_chain_key"]
		return hasSrc && hasDst
	case "mayan_swift", "mayan_wh", "mayan_mctp", "mayan_fast_mctp":
		return true // Mayan tx-builder provides pre-built transactions at execution time
	case "blockdaemon_defi_api":
		return false // aggregator quote-only; no direct deposit call
	default:
		return false
	}
}

func normalizeDexTokenAddress(chainID int, tokenAddress string) string {
	addr := strings.TrimSpace(tokenAddress)
	if addr == "" {
		return addr
	}
	if strings.EqualFold(addr, "0x0000000000000000000000000000000000000000") ||
		strings.EqualFold(addr, "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE") {
		if info, ok := bridges.TokenByChainAndSymbol[bridges.ChainID(chainID)]["WETH"]; ok && info.Address != "" {
			return info.Address
		}
	}
	return addr
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

	fallbackSlippage := 0
	if req.Preferences != nil {
		fallbackSlippage = req.Preferences.MaxSlippageBps
	}
	dq, derr := dexAdapter.GetQuote(ctx, dex.QuoteRequest{
		TokenInChainID:  req.Source.ChainID,
		TokenOutChainID: req.Destination.ChainID,
		TokenIn:         req.Source.TokenAddress,
		TokenOut:        req.Destination.TokenAddress,
		Amount:          firstNonEmpty(req.AmountBaseUnits, req.Amount),
		MaxSlippageBps:  fallbackSlippage,
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
