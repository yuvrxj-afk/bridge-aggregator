package bridges_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bridge-aggregator/internal/bridges"
	"bridge-aggregator/internal/models"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func jsonBody(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func acrossQuoteReq() models.QuoteRequest {
	return models.QuoteRequest{
		Source: models.Endpoint{
			ChainID: 42161, Chain: "arbitrum", Asset: "USDC",
			TokenAddress:  "0xaf88d065e77c8cC2239327C5EDb3A432268e5831",
			TokenDecimals: 6,
			Address:       "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045",
		},
		Destination: models.Endpoint{
			ChainID: 8453, Chain: "base", Asset: "USDC",
			TokenAddress:  "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913",
			TokenDecimals: 6,
		},
		AmountBaseUnits: "5000000",
	}
}

func acrossHappyResponse() map[string]any {
	return map[string]any{
		"crossSwapType":        "bridgeable",
		"expectedOutputAmount": "4993000",
		"minOutputAmount":      "4980000",
		"expectedFillTime":     120,
		"inputAmount":          "5000000",
		"isAmountTooLow":       false,
		"fees": map[string]any{
			"total": map[string]any{
				"amount": "7000",
				"token":  map[string]any{"decimals": 6},
			},
		},
		"deposit": map[string]any{
			"depositor":           "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045",
			"recipient":           "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045",
			"inputToken":          "0xaf88d065e77c8cC2239327C5EDb3A432268e5831",
			"outputToken":         "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913",
			"inputAmount":         "5000000",
			"outputAmount":        "4993000",
			"destinationChainId":  8453,
			"exclusiveRelayer":    "0x0000000000000000000000000000000000000000",
			"quoteTimestamp":      1234567890,
			"fillDeadline":        1234568490,
			"exclusivityDeadline": 0,
			"message":             "0x",
			"spokePoolAddress":    "0xe35e9842fceaCA96570B734083f4a58e8F7C5f2A",
		},
	}
}

// newAcrossAdapter returns an AcrossAdapter pointed at a test server.
func newAcrossAdapter(srv *httptest.Server) bridges.AcrossAdapter {
	return bridges.AcrossAdapter{
		Client: bridges.NewAcrossClient(srv.URL, "test-key", "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045"),
	}
}

// ── Across adapter ────────────────────────────────────────────────────────────

func TestAcrossAdapter_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(acrossHappyResponse())
	}))
	defer srv.Close()

	adapter := newAcrossAdapter(srv)
	route, err := adapter.GetQuote(context.Background(), acrossQuoteReq())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if route == nil {
		t.Fatal("route is nil")
	}
	if route.EstimatedOutputAmount != "4993000" {
		t.Errorf("EstimatedOutputAmount = %q, want 4993000", route.EstimatedOutputAmount)
	}
	if len(route.Hops) != 1 {
		t.Fatalf("expected 1 hop, got %d", len(route.Hops))
	}
	if route.Hops[0].HopType != models.HopTypeBridge {
		t.Errorf("HopType = %q, want bridge", route.Hops[0].HopType)
	}
	// provider_data must include deposit params
	var pd map[string]any
	if err := json.Unmarshal(route.Hops[0].ProviderData, &pd); err != nil {
		t.Fatalf("provider_data not valid JSON: %v", err)
	}
	if pd["deposit"] == nil {
		t.Error("provider_data.deposit is nil — stepTransaction will fail")
	}
}

func TestAcrossAdapter_API500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer srv.Close()

	adapter := newAcrossAdapter(srv)
	route, err := adapter.GetQuote(context.Background(), acrossQuoteReq())
	if err == nil {
		t.Fatalf("expected error for 500 response, got route: %+v", route)
	}
	if route != nil {
		t.Error("route should be nil on error")
	}
}

func TestAcrossAdapter_AmountTooLow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := acrossHappyResponse()
		resp["isAmountTooLow"] = true
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	adapter := newAcrossAdapter(srv)
	_, err := adapter.GetQuote(context.Background(), acrossQuoteReq())
	if err == nil {
		t.Fatal("expected error for isAmountTooLow=true")
	}
	if !strings.Contains(err.Error(), "too low") {
		t.Errorf("error message %q should mention 'too low'", err.Error())
	}
}

func TestAcrossAdapter_UnexpectedSchema(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Valid JSON but completely wrong schema
		w.Write([]byte(`{"foo":"bar","nested":{"x":1}}`))
	}))
	defer srv.Close()

	adapter := newAcrossAdapter(srv)
	// Should not panic; may return a route with empty output or error
	route, err := adapter.GetQuote(context.Background(), acrossQuoteReq())
	// With missing deposit, it tries /suggested-fees which will also fail (no mock)
	// So expect an error here — the key thing is it doesn't panic.
	if err == nil && (route == nil || route.EstimatedOutputAmount == "") {
		// empty route without error is also acceptable
	}
	// Main assertion: didn't panic
}

func TestAcrossAdapter_MissingDepositor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(acrossHappyResponse())
	}))
	defer srv.Close()

	// No depositor set in client, no address in request
	adapter := bridges.AcrossAdapter{
		Client: bridges.NewAcrossClient(srv.URL, "test-key", ""),
	}
	req := acrossQuoteReq()
	req.Source.Address = ""
	_, err := adapter.GetQuote(context.Background(), req)
	if err == nil {
		t.Fatal("expected error when depositor address is missing")
	}
}

func TestAcrossAdapter_Tier(t *testing.T) {
	adapter := bridges.AcrossAdapter{}
	if adapter.Tier() != models.TierProduction {
		t.Errorf("Across tier = %d, want TierProduction(%d)", adapter.Tier(), models.TierProduction)
	}
}

// ── Blockdaemon bridge adapter ────────────────────────────────────────────────

func bdBridgeHappyResponse() map[string]any {
	return map[string]any{
		"status": 200,
		"msg":    "ok",
		"data": []map[string]any{
			{
				"bridgeId":       "squid",
				"bridgeName":     "Squid",
				"srcChainId":     "1",
				"dstChainId":     "8453",
				"srcTokenSymbol": "USDC",
				"dstTokenSymbol": "USDC",
				"amountIn":       "5000000",
				"amountsOut":     []string{"5000000", "4985000"},
			},
		},
	}
}

func TestBlockdaemonBridgeAdapter_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(bdBridgeHappyResponse())
	}))
	defer srv.Close()

	adapter := bridges.BlockdaemonAdapter{Client: bridges.NewBlockdaemonClient(srv.URL, "test-key")}
	req := models.QuoteRequest{
		Source: models.Endpoint{
			ChainID: 1, Chain: "ethereum", Asset: "USDC",
			TokenAddress:  "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
			TokenDecimals: 6,
		},
		Destination: models.Endpoint{
			ChainID: 8453, Chain: "base", Asset: "USDC",
			TokenAddress:  "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913",
			TokenDecimals: 6,
		},
		AmountBaseUnits: "5000000",
	}

	route, err := adapter.GetQuote(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if route == nil {
		t.Fatal("route is nil")
	}
	if route.EstimatedOutputAmount != "4985000" {
		t.Errorf("EstimatedOutputAmount = %q, want 4985000", route.EstimatedOutputAmount)
	}
}

func TestBlockdaemonBridgeAdapter_API500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	adapter := bridges.BlockdaemonAdapter{Client: bridges.NewBlockdaemonClient(srv.URL, "test-key")}
	req := models.QuoteRequest{
		Source:          models.Endpoint{ChainID: 1, Asset: "USDC", TokenAddress: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", TokenDecimals: 6},
		Destination:     models.Endpoint{ChainID: 8453, Asset: "USDC", TokenAddress: "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913", TokenDecimals: 6},
		AmountBaseUnits: "5000000",
	}
	_, err := adapter.GetQuote(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestBlockdaemonBridgeAdapter_EmptyData(t *testing.T) {
	// Blockdaemon known issue: returns 200 with empty data array
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status": 200, "msg": "ok", "data": []any{},
		})
	}))
	defer srv.Close()

	adapter := bridges.BlockdaemonAdapter{Client: bridges.NewBlockdaemonClient(srv.URL, "test-key")}
	req := models.QuoteRequest{
		Source:          models.Endpoint{ChainID: 1, Asset: "USDC", TokenAddress: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", TokenDecimals: 6},
		Destination:     models.Endpoint{ChainID: 8453, Asset: "USDC", TokenAddress: "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913", TokenDecimals: 6},
		AmountBaseUnits: "5000000",
	}
	_, err := adapter.GetQuote(context.Background(), req)
	if err == nil {
		t.Fatal("expected error when data array is empty")
	}
}

func TestBlockdaemonBridgeAdapter_Tier(t *testing.T) {
	adapter := bridges.BlockdaemonAdapter{}
	if adapter.Tier() != models.TierDegraded {
		t.Errorf("Blockdaemon bridge tier = %d, want TierDegraded(%d)", adapter.Tier(), models.TierDegraded)
	}
}

// ── Mayan adapter ─────────────────────────────────────────────────────────────

func mayanHappyResponse() []map[string]any {
	return []map[string]any{
		{
			"type":              "SWIFT",
			"expectedAmountOut": "9.85",
			"minAmountOut":      "9.70",
			"eta":               60,
			"bridgeFee":         map[string]any{"amount": "0.05", "symbol": "USDC"},
			"toToken":           map[string]any{"contract": "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", "decimals": 6},
		},
	}
}

func newMayanAdapter(srv *httptest.Server) bridges.MayanAdapter {
	a := bridges.NewMayanAdapter()
	a.PriceAPIURL = srv.URL
	return a
}

func TestMayanAdapter_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mayanHappyResponse())
	}))
	defer srv.Close()

	adapter := newMayanAdapter(srv)
	req := models.QuoteRequest{
		Source: models.Endpoint{
			ChainID: 1, Chain: "ethereum", Asset: "USDC",
			TokenAddress:  "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
			TokenDecimals: 6,
		},
		Destination: models.Endpoint{
			ChainID: 900, Chain: "solana", Asset: "USDC",
			TokenAddress:  "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
			TokenDecimals: 6,
		},
		AmountBaseUnits: "10000000",
	}

	route, err := adapter.GetQuote(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if route == nil {
		t.Fatal("route is nil")
	}
	if len(route.Hops) != 1 {
		t.Fatalf("expected 1 hop, got %d", len(route.Hops))
	}
}

func TestMayanAdapter_API500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"upstream failure"}`))
	}))
	defer srv.Close()

	adapter := newMayanAdapter(srv)
	req := models.QuoteRequest{
		Source:          models.Endpoint{ChainID: 1, Chain: "ethereum", Asset: "USDC", TokenAddress: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", TokenDecimals: 6},
		Destination:     models.Endpoint{ChainID: 900, Chain: "solana", Asset: "USDC", TokenAddress: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", TokenDecimals: 6},
		AmountBaseUnits: "10000000",
	}
	_, err := adapter.GetQuote(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestMayanAdapter_CrossTokenRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should never be called for cross-token pair
		t.Error("unexpected HTTP call for cross-token pair")
	}))
	defer srv.Close()

	adapter := newMayanAdapter(srv)
	req := models.QuoteRequest{
		Source:          models.Endpoint{ChainID: 1, Chain: "ethereum", Asset: "USDC", TokenAddress: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", TokenDecimals: 6},
		Destination:     models.Endpoint{ChainID: 900, Chain: "solana", Asset: "ETH", TokenAddress: "0x0000000000000000000000000000000000000000", TokenDecimals: 18},
		AmountBaseUnits: "10000000",
	}
	_, err := adapter.GetQuote(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for cross-token pair (USDC→ETH)")
	}
	if !strings.Contains(err.Error(), "cross-token") {
		t.Errorf("error %q should mention cross-token", err.Error())
	}
}

func TestMayanAdapter_EmptyRoutes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	adapter := newMayanAdapter(srv)
	req := models.QuoteRequest{
		Source:          models.Endpoint{ChainID: 1, Chain: "ethereum", Asset: "USDC", TokenAddress: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", TokenDecimals: 6},
		Destination:     models.Endpoint{ChainID: 900, Chain: "solana", Asset: "USDC", TokenAddress: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", TokenDecimals: 6},
		AmountBaseUnits: "10000000",
	}
	_, err := adapter.GetQuote(context.Background(), req)
	if err == nil {
		t.Fatal("expected error when Mayan returns empty routes array")
	}
}

func TestMayanAdapter_Tier(t *testing.T) {
	adapter := bridges.NewMayanAdapter()
	if adapter.Tier() != models.TierDegraded {
		t.Errorf("Mayan tier = %d, want TierDegraded(%d)", adapter.Tier(), models.TierDegraded)
	}
}

// ── Adapter timeout tests ─────────────────────────────────────────────────────

func TestAcrossAdapter_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow server
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(acrossHappyResponse())
	}))
	defer srv.Close()

	client := bridges.NewAcrossClient(srv.URL, "test-key", "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045")
	// Use a very short timeout
	client.HTTPClient = &http.Client{Timeout: 10 * time.Millisecond}
	adapter := bridges.AcrossAdapter{Client: client}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := adapter.GetQuote(ctx, acrossQuoteReq())
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
