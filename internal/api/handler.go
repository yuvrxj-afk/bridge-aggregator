package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"bridge-aggregator/internal/bridges"
	"bridge-aggregator/internal/dex"
	"bridge-aggregator/internal/ethutil"
	"bridge-aggregator/internal/intent"
	"bridge-aggregator/internal/lifi"
	"bridge-aggregator/internal/models"
	"bridge-aggregator/internal/router"
	"bridge-aggregator/internal/service"
	"bridge-aggregator/internal/store"

	"github.com/gin-gonic/gin"
)

// QuoteHandler returns a Gin handler that uses the quote service to return routes.
func QuoteHandler(adapters []bridges.Adapter, dexAdapters []dex.Adapter) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req models.QuoteRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error(), nil)
			return
		}

		hasSourceSymbol := req.Source.Chain != "" && req.Source.Asset != ""
		hasSourceAddress := req.Source.ChainID != 0 && req.Source.TokenAddress != ""
		hasDestSymbol := req.Destination.Chain != "" && req.Destination.Asset != ""
		hasDestAddress := req.Destination.ChainID != 0 && req.Destination.TokenAddress != ""
		hasAmount := req.Amount != "" || req.AmountBaseUnits != ""

		if (!hasSourceSymbol && !hasSourceAddress) || (!hasDestSymbol && !hasDestAddress) || !hasAmount {
			RespondError(
				c,
				http.StatusBadRequest,
				CodeInvalidRequest,
				"require (source.chain+source.asset OR source.chain_id+source.token_address), (destination.chain+destination.asset OR destination.chain_id+destination.token_address), and (amount OR amount_base_units)",
				nil,
			)
			return
		}
		if req.AmountBaseUnits != "" {
			if err := ethutil.ValidatePositiveUint256String(req.AmountBaseUnits); err != nil {
				RespondErrorTyped(c, http.StatusBadRequest, CodeInvalidRequest,
					"amount_base_units must be an unsigned uint256 integer string > 0",
					models.ErrorTypeUserAction, "invalid_amount_base_units", nil)
				return
			}
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 7*time.Second)
		defer cancel()
		resp, err := service.Quote(ctx, adapters, dexAdapters, req)
		if err != nil {
			if errors.Is(err, router.ErrNoRoutes) {
				RespondNoRoutes(c) // typed: requote / no_routes
				return
			}
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				RespondErrorTyped(c, http.StatusGatewayTimeout, CodeInternal,
					"quote timed out — try again", models.ErrorTypeRetryable, "timeout", nil)
				return
			}
			RespondErrorTyped(c, http.StatusInternalServerError, CodeInternal,
				err.Error(), models.ErrorTypeTerminal, "internal", nil)
			return
		}

		c.JSON(http.StatusOK, resp)
	}
}

// StreamQuoteHandler streams routes as Server-Sent Events as each adapter returns results.
// POST /api/v1/quote/stream — same request body as /quote, SSE response.
// Each SSE "data:" line is a single JSON-encoded Route object.
// Ends with "event: done" when all adapters complete, or "event: error" if none found.
func StreamQuoteHandler(adapters []bridges.Adapter, dexAdapters []dex.Adapter) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req models.QuoteRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error(), nil)
			return
		}

		hasSourceSymbol := req.Source.Chain != "" && req.Source.Asset != ""
		hasSourceAddress := req.Source.ChainID != 0 && req.Source.TokenAddress != ""
		hasDestSymbol := req.Destination.Chain != "" && req.Destination.Asset != ""
		hasDestAddress := req.Destination.ChainID != 0 && req.Destination.TokenAddress != ""
		hasAmount := req.Amount != "" || req.AmountBaseUnits != ""

		if (!hasSourceSymbol && !hasSourceAddress) || (!hasDestSymbol && !hasDestAddress) || !hasAmount {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest,
				"require source/destination (chain+asset or chain_id+token_address) and amount", nil)
			return
		}
		if req.AmountBaseUnits != "" {
			if err := ethutil.ValidatePositiveUint256String(req.AmountBaseUnits); err != nil {
				RespondErrorTyped(c, http.StatusBadRequest, CodeInvalidRequest,
					"amount_base_units must be an unsigned uint256 integer string > 0",
					models.ErrorTypeUserAction, "invalid_amount_base_units", nil)
				return
			}
		}

		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			RespondError(c, http.StatusInternalServerError, CodeInternal, "streaming not supported", nil)
			return
		}

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		c.Header("Access-Control-Allow-Origin", "*")

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		var mu sync.Mutex
		var sent int

		req = service.EnrichQuoteRequest(req)
		router.QuoteStream(ctx, adapters, dexAdapters, req, func(route models.Route) {
			data, err := json.Marshal(route)
			if err != nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			flusher.Flush()
			sent++
		})

		mu.Lock()
		defer mu.Unlock()
		if sent == 0 {
			fmt.Fprintf(c.Writer, "event: error\ndata: {\"message\":\"no routes found\"}\n\n")
		} else {
			fmt.Fprintf(c.Writer, "event: done\ndata: {\"count\":%d}\n\n", sent)
		}
		flusher.Flush()
	}
}

// HealthHandler responds with service status and version.
func HealthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "version": "v1"})
}

func classifyHealth(err error) (string, string) {
	if err == nil {
		return "healthy", ""
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "api key"), strings.Contains(lower, "unauthorized"),
		strings.Contains(lower, "forbidden"), strings.Contains(lower, "must be configured"),
		strings.Contains(lower, "not configured"):
		return "down", msg
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "deadline"),
		strings.Contains(lower, "no routes"), strings.Contains(lower, "no liquidity"),
		strings.Contains(lower, "500"), strings.Contains(lower, "internal server error"),
		strings.Contains(lower, "no quotes"):
		return "degraded", msg
	default:
		return "degraded", msg
	}
}

func bridgeProbeRequest(id, network string) models.QuoteRequest {
	testnet := network == "testnet"
	switch id {
	case "mayan":
		// Mayan is mainnet-only. Always probe against Ethereum→Solana regardless of network mode.
		return models.QuoteRequest{
			Source: models.Endpoint{
				ChainID:       1,
				Chain:         "ethereum",
				Asset:         "USDC",
				TokenAddress:  "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
				TokenDecimals: 6,
			},
			Destination: models.Endpoint{
				ChainID:       900,
				Chain:         "solana",
				Asset:         "USDC",
				TokenAddress:  "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
				TokenDecimals: 6,
			},
			AmountBaseUnits: "10000000", // 10 USDC
		}
	case "canonical_base":
		if testnet {
			return models.QuoteRequest{
				Source:          models.Endpoint{ChainID: 11155111, Chain: "sepolia", Asset: "ETH", TokenAddress: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", TokenDecimals: 18},
				Destination:     models.Endpoint{ChainID: 84532, Chain: "base-sepolia", Asset: "ETH", TokenAddress: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", TokenDecimals: 18},
				AmountBaseUnits: "1000000000000000",
			}
		}
		return models.QuoteRequest{
			Source:          models.Endpoint{ChainID: 1, Chain: "ethereum", Asset: "ETH", TokenAddress: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", TokenDecimals: 18},
			Destination:     models.Endpoint{ChainID: 8453, Chain: "base", Asset: "ETH", TokenAddress: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", TokenDecimals: 18},
			AmountBaseUnits: "1000000000000000",
		}
	case "canonical_optimism":
		if testnet {
			return models.QuoteRequest{
				Source:          models.Endpoint{ChainID: 11155111, Chain: "sepolia", Asset: "ETH", TokenAddress: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", TokenDecimals: 18},
				Destination:     models.Endpoint{ChainID: 11155420, Chain: "op-sepolia", Asset: "ETH", TokenAddress: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", TokenDecimals: 18},
				AmountBaseUnits: "1000000000000000",
			}
		}
		return models.QuoteRequest{
			Source:          models.Endpoint{ChainID: 1, Chain: "ethereum", Asset: "ETH", TokenAddress: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", TokenDecimals: 18},
			Destination:     models.Endpoint{ChainID: 10, Chain: "optimism", Asset: "ETH", TokenAddress: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", TokenDecimals: 18},
			AmountBaseUnits: "1000000000000000",
		}
	case "canonical_arbitrum":
		if testnet {
			return models.QuoteRequest{
				Source:          models.Endpoint{ChainID: 11155111, Chain: "sepolia", Asset: "ETH", TokenAddress: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", TokenDecimals: 18},
				Destination:     models.Endpoint{ChainID: 421614, Chain: "arbitrum-sepolia", Asset: "ETH", TokenAddress: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", TokenDecimals: 18},
				AmountBaseUnits: "1000000000000000",
			}
		}
		return models.QuoteRequest{
			Source:          models.Endpoint{ChainID: 1, Chain: "ethereum", Asset: "ETH", TokenAddress: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", TokenDecimals: 18},
			Destination:     models.Endpoint{ChainID: 42161, Chain: "arbitrum", Asset: "ETH", TokenAddress: "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", TokenDecimals: 18},
			AmountBaseUnits: "1000000000000000",
		}
	default:
		// Default probe: Across and any other bridge.
		// Use testnet chain IDs when NETWORK=testnet — the testnet Across API only accepts testnet chain IDs.
		if testnet {
			return models.QuoteRequest{
				Source: models.Endpoint{
					ChainID:       11155111,
					Chain:         "sepolia",
					Asset:         "USDC",
					TokenAddress:  "0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238",
					TokenDecimals: 6,
					Address:       "0x1111111111111111111111111111111111111111",
				},
				Destination: models.Endpoint{
					ChainID:       421614,
					Chain:         "arbitrum-sepolia",
					Asset:         "USDC",
					TokenAddress:  "0x75faf114eafb1BDbe2F0316DF893fd58CE46AA4d",
					TokenDecimals: 6,
				},
				AmountBaseUnits: "5000000", // 5 testnet USDC
			}
		}
		return models.QuoteRequest{
			Source: models.Endpoint{
				ChainID:       42161,
				Chain:         "arbitrum",
				Asset:         "USDC",
				TokenAddress:  "0xaf88d065e77c8cC2239327C5EDb3A432268e5831",
				TokenDecimals: 6,
				Address:       "0x1111111111111111111111111111111111111111",
			},
			Destination: models.Endpoint{
				ChainID:       8453,
				Chain:         "base",
				Asset:         "USDC",
				TokenAddress:  "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913",
				TokenDecimals: 6,
			},
			AmountBaseUnits: "1000000",
		}
	}
}

// AdapterHealthHandler runs live probe quotes and reports per-adapter health.
// Tier 3/4 adapters are reported as "down" immediately without probing — they
// are guaranteed to fail and probing them wastes the 8s timeout budget.
// network should be "mainnet" or "testnet" — it selects appropriate probe chain IDs.
func AdapterHealthHandler(adapters []bridges.Adapter, dexAdapters []dex.Adapter, network string) gin.HandlerFunc {
	return func(c *gin.Context) {
		out := make([]models.AdapterHealth, 0, len(adapters)+len(dexAdapters))

		for _, a := range adapters {
			if a == nil {
				continue
			}
			tier := int(a.Tier())
			if a.Tier() > models.TierDegraded {
				// Tier 3/4: skip probe, report down immediately.
				out = append(out, models.AdapterHealth{
					Service: a.ID(), Kind: "bridge", Tier: tier,
					Status: "down", Reason: "not eligible for fan-out (tier " + fmt.Sprintf("%d", tier) + ")",
				})
				continue
			}
			start := time.Now()
			ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
			_, err := a.GetQuote(ctx, bridgeProbeRequest(a.ID(), network))
			cancel()
			status, reason := classifyHealth(err)
			out = append(out, models.AdapterHealth{
				Service:   a.ID(),
				Kind:      "bridge",
				Tier:      tier,
				Status:    status,
				Reason:    reason,
				LatencyMS: time.Since(start).Milliseconds(),
			})
		}

		for _, a := range dexAdapters {
			if a == nil {
				continue
			}
			tier := int(a.Tier())
			if a.Tier() > models.TierDegraded {
				out = append(out, models.AdapterHealth{
					Service: a.ID(), Kind: "dex", Tier: tier,
					Status: "down", Reason: "not eligible for fan-out (tier " + fmt.Sprintf("%d", tier) + ")",
				})
				continue
			}
			start := time.Now()
			ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
			// USDC→USDT on Ethereum mainnet. No Swapper: each adapter falls back to
			// its own configured wallet. Dead/zero addresses are rejected by some DEX
			// APIs (e.g. 0x v2 treats them as non-signable takers).
			_, err := a.GetQuote(ctx, dex.QuoteRequest{
				TokenInChainID:  1,
				TokenOutChainID: 1,
				TokenIn:         "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", // USDC on Ethereum
				TokenOut:        "0xdac17f958d2ee523a2206206994597c13d831ec7", // USDT on Ethereum
				Amount:          "1000000",
			})
			cancel()
			status, reason := classifyHealth(err)
			out = append(out, models.AdapterHealth{
				Service:   a.ID(),
				Kind:      "dex",
				Tier:      tier,
				Status:    status,
				Reason:    reason,
				LatencyMS: time.Since(start).Milliseconds(),
			})
		}

		overall := "healthy"
		for _, h := range out {
			if h.Status == "down" {
				overall = "down"
				break
			}
			if h.Status == "degraded" {
				overall = "degraded"
			}
		}

		c.JSON(http.StatusOK, models.AdapterHealthResponse{
			Status:   overall,
			Adapters: out,
		})
	}
}

// CapabilitiesHandler returns runtime feature capabilities for UI/ops introspection.
func CapabilitiesHandler(adapters []bridges.Adapter, dexAdapters []dex.Adapter) gin.HandlerFunc {
	return func(c *gin.Context) {
		bridgeIDs := make([]string, 0, len(adapters))
		for _, a := range adapters {
			if a == nil {
				continue
			}
			bridgeIDs = append(bridgeIDs, a.ID())
		}
		dexIDs := make([]string, 0, len(dexAdapters))
		for _, a := range dexAdapters {
			if a == nil {
				continue
			}
			dexIDs = append(dexIDs, a.ID())
		}
		resp := models.CapabilitiesResponse{
			BuildTransaction: map[string]any{
				"supported_bridge_paths": []string{
					"across_depositv3_one_click",
					"cctp_celercircle_one_click",
				},
				"unsupported_bridge_paths": []string{
					"across_swap_tx_variants",
					"layerzero_stargate_one_click_pending",
				},
				"diamond_address": lifi.LiFiDiamond,
			},
			StepTransaction: map[string]any{
				"supports_swap_hops":   true,
				"supports_bridge_hops": true,
				"dex_adapters":         dexIDs,
			},
			Operations: map[string]any{
				"statuses": []string{
					models.OperationStatusPending,
					models.OperationStatusSubmitted,
					models.OperationStatusCompleted,
					models.OperationStatusFailed,
				},
				"requires_tx_hash_on_submitted": true,
				"bridge_adapters":               bridgeIDs,
				"confirmed_bridge_adapters":     []string{"across", "cctp"},
			},
		}
		c.JSON(http.StatusOK, resp)
	}
}

// ExecuteHandler returns a Gin handler that creates an operation for a selected route.
func ExecuteHandler(s *store.Store, adapters []bridges.Adapter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if s == nil {
			RespondError(c, http.StatusServiceUnavailable, CodeInternal, "database not configured; set DATABASE_URL to enable execute", nil)
			return
		}
		var req models.ExecuteRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error(), nil)
			return
		}

		resp, err := service.Execute(c.Request.Context(), s, adapters, req)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrRouteRequired), errors.Is(err, service.ErrRouteHopsEmpty), errors.Is(err, service.ErrUnknownBridgeID), errors.Is(err, service.ErrQuoteExpiryRequired):
				RespondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error(), nil)
				return
			case errors.Is(err, service.ErrQuoteExpired):
				RespondErrorTyped(c, http.StatusConflict, CodeInvalidRequest,
					"quote has expired; please requote",
					models.ErrorTypeRequote, "quote_expired", nil)
				return
			}
			RespondError(c, http.StatusInternalServerError, CodeInternal, err.Error(), nil)
			return
		}

		c.JSON(http.StatusOK, resp)
	}
}

// ListOperationsHandler returns the most recent operations for a wallet address, newest-first.
// Requires ?wallet=0x... query parameter.
func ListOperationsHandler(s *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Validate wallet before checking store — gives the caller a useful error either way.
		wallet := c.Query("wallet")
		if wallet == "" {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, "wallet query parameter is required", nil)
			return
		}
		if s == nil {
			RespondError(c, http.StatusServiceUnavailable, CodeInternal, "database not configured; set DATABASE_URL to enable operations", nil)
			return
		}
		limit := 50
		if q := c.Query("limit"); q != "" {
			if n, err := strconv.Atoi(q); err == nil && n > 0 {
				limit = n
			}
		}
		scope := c.Query("scope") // "mainnet", "testnet", or "" for all
		ops, err := s.ListOperations(limit, scope, wallet)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, CodeInternal, err.Error(), nil)
			return
		}
		out := make([]models.OperationResponse, 0, len(ops))
		for _, op := range ops {
			out = append(out, models.OperationResponse{
				OperationID:       op.ID,
				Status:            op.Status,
				TxHash:            op.TxHash,
				Route:             op.Route,
				ClientReferenceID: op.ClientReferenceID,
				CreatedAt:         op.CreatedAt.Format(time.RFC3339),
				UpdatedAt:         op.UpdatedAt.Format(time.RFC3339),
			})
		}
		c.JSON(http.StatusOK, gin.H{"operations": out})
	}
}

// GetOperationHandler returns a Gin handler that fetches an operation by ID.
func GetOperationHandler(s *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if s == nil {
			RespondError(c, http.StatusServiceUnavailable, CodeInternal, "database not configured; set DATABASE_URL to enable operations", nil)
			return
		}
		id := c.Param("id")
		if id == "" {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, "operation id is required", nil)
			return
		}

		resp, err := service.GetOperation(c.Request.Context(), s, id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, CodeInternal, err.Error(), nil)
			return
		}
		if resp == nil {
			RespondError(c, http.StatusNotFound, CodeInvalidRequest, "operation not found", nil)
			return
		}

		c.JSON(http.StatusOK, resp)
	}
}

// PatchOperationStatusHandler updates the status (and optionally tx_hash) of an operation.
func PatchOperationStatusHandler(s *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if s == nil {
			RespondError(c, http.StatusServiceUnavailable, CodeInternal, "database not configured; set DATABASE_URL to enable operations", nil)
			return
		}
		id := c.Param("id")
		if id == "" {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, "operation id is required", nil)
			return
		}
		var req models.UpdateOperationStatusRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error(), nil)
			return
		}
		if err := service.UpdateOperationStatus(c.Request.Context(), s, id, req); err != nil {
			switch {
			case errors.Is(err, service.ErrInvalidStatus), errors.Is(err, service.ErrInvalidTransition), errors.Is(err, service.ErrTxHashRequired):
				RespondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error(), nil)
			case errors.Is(err, sql.ErrNoRows), errors.Is(err, store.ErrNotFound):
				RespondError(c, http.StatusNotFound, CodeInvalidRequest, "operation not found", nil)
			default:
				RespondError(c, http.StatusInternalServerError, CodeInternal, err.Error(), nil)
			}
			return
		}
		c.JSON(http.StatusOK, gin.H{"operation_id": id, "status": req.Status})
	}
}

// GetOperationEventsHandler returns immutable operation lifecycle events for recovery/audit.
func GetOperationEventsHandler(s *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if s == nil {
			RespondError(c, http.StatusServiceUnavailable, CodeInternal, "database not configured; set DATABASE_URL to enable operations", nil)
			return
		}
		id := c.Param("id")
		if id == "" {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, "operation id is required", nil)
			return
		}
		limit := 50
		if q := c.Query("limit"); q != "" {
			if n, err := strconv.Atoi(q); err == nil {
				limit = n
			}
		}
		events, err := service.GetOperationEvents(c.Request.Context(), s, id, limit)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				RespondError(c, http.StatusNotFound, CodeInvalidRequest, "operation not found", nil)
				return
			}
			RespondError(c, http.StatusInternalServerError, CodeInternal, err.Error(), nil)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"operation_id": id,
			"events":       events,
		})
	}
}

// DEXQuoteHandler returns a Gin handler that tries DEX adapters in order and returns the first successful quote.
func DEXQuoteHandler(adapters []dex.Adapter) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req dex.QuoteRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error(), nil)
			return
		}
		if req.TokenInChainID == 0 || req.TokenOutChainID == 0 || req.TokenIn == "" || req.TokenOut == "" || req.Amount == "" {
			RespondError(
				c,
				http.StatusBadRequest,
				CodeInvalidRequest,
				"tokenInChainId, tokenOutChainId, tokenIn, tokenOut, and amount (base units) are required",
				nil,
			)
			return
		}

		for _, adapter := range adapters {
			if adapter == nil {
				continue
			}
			quote, err := adapter.GetQuote(c.Request.Context(), req)
			if err != nil {
				continue
			}
			c.JSON(http.StatusOK, quote)
			return
		}
		RespondError(c, http.StatusBadRequest, CodeInvalidRequest, "no DEX adapter could return a quote for this request", nil)
	}
}

// BuildTransactionHandler translates a route into a single LiFi Diamond transaction.
// POST /api/v1/route/buildTransaction
//
// For "bridgeable" Across routes (same-token direct bridge), this always fetches fresh
// deposit params from the Across API using the user's actual wallet address. This handles
// two failure modes: (1) quotes that expired, (2) routes where Across returned no deposit
// object at quote time (can happen when the depositor address had no on-chain balance).
//
// For "anyToBridgeable" routes (cross-token), the frontend routes to /route/stepTransaction.
func BuildTransactionHandler(adapters []bridges.Adapter) gin.HandlerFunc {
	// Pre-find the Across client once at handler construction time.
	var acrossClient *bridges.AcrossClient
	for _, a := range adapters {
		if aa, ok := a.(bridges.AcrossAdapter); ok && aa.Client != nil {
			acrossClient = aa.Client
			break
		}
	}

	return func(c *gin.Context) {
		var req models.LiFiBuildRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error(), nil)
			return
		}
		if req.FromAddress == "" {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, "from_address is required", nil)
			return
		}
		if len(req.Route.Hops) == 0 {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, "route.hops must not be empty", nil)
			return
		}

		// Refresh Across deposit params for "bridgeable" hops using the user's wallet.
		// This is always done (not just when missing) to ensure timestamps are fresh.
		route := req.Route
		if acrossClient != nil {
			route = refreshBridgeableAcrossDeposits(c, acrossClient, route, req.FromAddress)
		}

		resp, err := lifi.BuildTransaction(route, req.FromAddress)
		if err != nil {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error(), nil)
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// refreshBridgeableAcrossDeposits fetches fresh deposit params from Across for every "bridgeable"
// bridge hop in the route, using walletAddress as both depositor and recipient.
// anyToBridgeable hops are skipped — they use swapTx, not deposit params.
func refreshBridgeableAcrossDeposits(c *gin.Context, client *bridges.AcrossClient, route models.Route, walletAddress string) models.Route {
	for i, hop := range route.Hops {
		if hop.HopType != models.HopTypeBridge || hop.BridgeID != "across" {
			continue
		}

		// Skip swapTx-style Across routes: these do not use depositV3 params.
		if isAcrossSwapTxHop(hop) {
			continue
		}

		originChainID := bridges.ChainIDFromName(hop.FromChain)
		destChainID := bridges.ChainIDFromName(hop.ToChain)
		if originChainID == 0 || destChainID == 0 {
			log.Printf("[buildTx] unknown chain names: %s→%s", hop.FromChain, hop.ToChain)
			continue
		}

		deposit, err := client.FetchDeposit(
			c.Request.Context(),
			originChainID, destChainID,
			hop.FromTokenAddress, hop.ToTokenAddress,
			hop.AmountInBaseUnits,
			walletAddress,
		)
		if err != nil {
			log.Printf("[buildTx] across deposit refresh failed: %v", err)
			continue
		}

		updated, err := bridges.InjectAcrossDeposit(hop, deposit)
		if err != nil {
			log.Printf("[buildTx] inject deposit failed: %v", err)
			continue
		}
		route.Hops[i] = updated
		log.Printf("[buildTx] refreshed Across deposit for hop %d (%s→%s)", i, hop.FromChain, hop.ToChain)
	}
	return route
}

// isAcrossSwapTxHop returns true for Across routes that must execute via swap_tx
// (not via depositV3/LiFi AcrossV3 builder path).
func isAcrossSwapTxHop(hop models.Hop) bool {
	if len(hop.ProviderData) == 0 {
		return false
	}
	var pd map[string]json.RawMessage
	if err := json.Unmarshal(hop.ProviderData, &pd); err != nil {
		return false
	}
	// Most robust signal: swap_tx exists.
	if raw, ok := pd["swap_tx"]; ok && len(raw) > 0 && string(raw) != "null" {
		return true
	}
	raw, ok := pd["cross_swap_type"]
	if !ok {
		return false
	}
	var cst string
	_ = json.Unmarshal(raw, &cst)
	switch cst {
	case "anyToBridgeable", "bridgeableToAny", "anyToAny":
		return true
	default:
		return false
	}
}

// StepTransactionHandler populates a single hop with tx data when supported.
func StepTransactionHandler(dexAdapters []dex.Adapter, bc *service.BridgeClients) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req models.StepTransactionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error(), nil)
			return
		}
		resp, err := service.PopulateStepTransaction(c.Request.Context(), dexAdapters, bc, req)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrHopIndexOutOfRange), errors.Is(err, service.ErrHopNotSupported), errors.Is(err, service.ErrPermitSignatureReq):
				RespondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error(), nil)
				return
			case errors.Is(err, service.ErrInvalidAmountField), errors.Is(err, service.ErrInvalidValueField), errors.Is(err, service.ErrInvalidFeeField), errors.Is(err, service.ErrEncodingGuard):
				RespondErrorTyped(c, http.StatusBadRequest, CodeInvalidRequest, err.Error(), models.ErrorTypeTerminal, "invalid_numeric_field", nil)
				return
			}
			RespondError(c, http.StatusInternalServerError, CodeInternal, err.Error(), nil)
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// TransactionStatusHandler returns the status of a cross-chain transaction.
// It uses the Blockdaemon DeFi API to track transaction progress.
func TransactionStatusHandler(blockdaemonClient *bridges.BlockdaemonClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		if blockdaemonClient == nil || blockdaemonClient.APIKey == "" {
			RespondError(c, http.StatusServiceUnavailable, CodeInternal, "transaction status tracking not configured", nil)
			return
		}

		txHash := c.Param("txHash")
		if txHash == "" {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, "txHash is required", nil)
			return
		}

		status, err := blockdaemonClient.GetTransactionStatus(c.Request.Context(), txHash)
		if err != nil {
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
				RespondError(c, http.StatusNotFound, CodeInvalidRequest, "transaction not found", nil)
				return
			}
			RespondError(c, http.StatusInternalServerError, CodeInternal, err.Error(), nil)
			return
		}

		c.JSON(http.StatusOK, status)
	}
}

var messageHashRE = regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`)

// isValidMessageHash returns true if h is a 0x-prefixed 32-byte hex string.
func isValidMessageHash(h string) bool {
	return messageHashRE.MatchString(h)
}

// CCTPAttestationHandler proxies Circle Iris attestation requests through the backend,
// avoiding browser CORS restrictions on direct calls to iris-api.circle.com.
//
//	GET /api/v1/cctp/attestation/:messageHash
//
// Passes the response status and body through verbatim so the frontend can handle
// 404 (not yet attested) and 200 (complete) identically to the direct API.
func CCTPAttestationHandler(attestationURL string) gin.HandlerFunc {
	client := &http.Client{Timeout: 10 * time.Second}
	return func(c *gin.Context) {
		messageHash := c.Param("messageHash")
		if messageHash == "" {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, "messageHash is required", nil)
			return
		}
		if !isValidMessageHash(messageHash) {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, "messageHash must be a 0x-prefixed 32-byte hex string", nil)
			return
		}
		url := attestationURL + "/v1/attestations/" + messageHash
		resp, err := client.Get(url)
		if err != nil {
			RespondError(c, http.StatusBadGateway, CodeInternal, "Circle Iris unreachable: "+err.Error(), nil)
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			RespondError(c, http.StatusBadGateway, CodeInternal, "failed to read Iris response", nil)
			return
		}
		c.Data(resp.StatusCode, "application/json", body)
	}
}

// CCTPAttestationStreamHandler streams Circle Iris attestation status via SSE.
// It polls internally every 12 seconds and pushes an event when attestation arrives.
// The stream closes automatically once the attestation is complete.
//
//	GET /api/v1/cctp/attestation/stream/:messageHash
func CCTPAttestationStreamHandler(attestationURL string) gin.HandlerFunc {
	client := &http.Client{Timeout: 10 * time.Second}
	return func(c *gin.Context) {
		messageHash := c.Param("messageHash")
		if messageHash == "" {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, "messageHash is required", nil)
			return
		}
		if !isValidMessageHash(messageHash) {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, "messageHash must be a 0x-prefixed 32-byte hex string", nil)
			return
		}

		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			RespondError(c, http.StatusInternalServerError, CodeInternal, "streaming not supported", nil)
			return
		}

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")

		poll := func() (string, bool) {
			url := attestationURL + "/v1/attestations/" + messageHash
			resp, err := client.Get(url)
			if err != nil {
				return "", false
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil || resp.StatusCode != http.StatusOK {
				return "", false
			}
			var v struct {
				Status      string `json:"status"`
				Attestation string `json:"attestation"`
			}
			if json.Unmarshal(body, &v) != nil {
				return "", false
			}
			if v.Status == "complete" && v.Attestation != "" {
				return v.Attestation, true
			}
			return "", false
		}

		ticker := time.NewTicker(12 * time.Second)
		defer ticker.Stop()
		ctx := c.Request.Context()

		// Immediate first check before the first tick.
		if att, done := poll(); done {
			fmt.Fprintf(c.Writer, "data: {\"status\":\"complete\",\"attestation\":%q}\n\n", att)
			flusher.Flush()
			return
		}
		fmt.Fprintf(c.Writer, "data: {\"status\":\"pending\"}\n\n")
		flusher.Flush()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if att, done := poll(); done {
					fmt.Fprintf(c.Writer, "data: {\"status\":\"complete\",\"attestation\":%q}\n\n", att)
					flusher.Flush()
					return
				}
				fmt.Fprintf(c.Writer, "data: {\"status\":\"pending\"}\n\n")
				flusher.Flush()
			}
		}
	}
}

// IntentParseHandler parses a natural language intent string using configured LLM providers.
// POST /api/v1/intent/parse — body: {"text":"bridge 10 USDC from Ethereum to Base"}
// Returns: {"amount":"10","src_token":"USDC","dst_token":"USDC","src_chain":"ethereum","dst_chain":"base"}
func IntentParseHandler(intentCfg intent.ProviderConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Text string `json:"text"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Text) == "" {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, "text is required", nil)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		result, err := intent.Parse(ctx, intentCfg, req.Text)
		if err != nil {
			if strings.Contains(err.Error(), "intent provider not configured") {
				RespondError(c, http.StatusServiceUnavailable, "INTENT_PROVIDER_DISABLED", "Intent parsing is not enabled on this server.", nil)
				return
			}
			RespondError(c, http.StatusUnprocessableEntity, "INTENT_PARSE_FAILED", err.Error(), nil)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}
