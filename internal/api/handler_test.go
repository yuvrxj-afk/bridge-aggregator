package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"bridge-aggregator/internal/api"
	"bridge-aggregator/internal/bridges"
	"bridge-aggregator/internal/dex"
	"bridge-aggregator/internal/models"
	"bridge-aggregator/internal/router"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ── Mock adapters ─────────────────────────────────────────────────────────────

type mockBridge struct {
	id    string
	route *models.Route
	err   error
}

func (m *mockBridge) ID() string               { return m.id }
func (m *mockBridge) Tier() models.AdapterTier { return models.TierProduction }
func (m *mockBridge) GetQuote(_ context.Context, _ models.QuoteRequest) (*models.Route, error) {
	return m.route, m.err
}

type mockDEX struct {
	id    string
	quote *dex.Quote
	err   error
}

func (m *mockDEX) ID() string               { return m.id }
func (m *mockDEX) Tier() models.AdapterTier { return models.TierProduction }
func (m *mockDEX) GetQuote(_ context.Context, _ dex.QuoteRequest) (*dex.Quote, error) {
	return m.quote, m.err
}

// ── Test router setup ─────────────────────────────────────────────────────────

func mustMarshal(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// newTestRouter builds a Gin router wired up exactly like main.go, but with
// mock adapters and no database (store = nil).
func newTestRouter(bridgeAdapters []bridges.Adapter, dexAdapters []dex.Adapter) *gin.Engine {
	r := gin.New()
	r.GET("/health", api.HealthHandler)
	v1 := r.Group("/api/v1")
	{
		v1.GET("/health/adapters", api.AdapterHealthHandler(bridgeAdapters, dexAdapters, "mainnet"))
		v1.POST("/quote", api.QuoteHandler(bridgeAdapters, dexAdapters))
		v1.POST("/execute", api.ExecuteHandler(nil, bridgeAdapters))             // nil store
		v1.GET("/operations/:id", api.GetOperationHandler(nil))                  // nil store
		v1.PATCH("/operations/:id/status", api.PatchOperationStatusHandler(nil)) // nil store
		v1.POST("/dex/quote", api.DEXQuoteHandler(dexAdapters))
		v1.POST("/route/stepTransaction", api.StepTransactionHandler(dexAdapters, nil))
	}
	return r
}

func doRequest(r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	var buf *bytes.Buffer
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewBuffer(b)
	} else {
		buf = &bytes.Buffer{}
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ── /health ───────────────────────────────────────────────────────────────────

func TestHealthHandler(t *testing.T) {
	r := newTestRouter(nil, nil)
	w := doRequest(r, http.MethodGet, "/health", nil)
	if w.Code != http.StatusOK {
		t.Errorf("health: status %d, want 200", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("health: status field = %q, want ok", resp["status"])
	}
}

func TestAdapterHealthHandler(t *testing.T) {
	r := newTestRouter(
		[]bridges.Adapter{
			&mockBridge{id: "cctp", route: &models.Route{RouteID: "cctp", Hops: []models.Hop{{HopType: models.HopTypeBridge}}}},
			&mockBridge{id: "across", err: errors.New("across api key must be configured")},
		},
		[]dex.Adapter{
			&mockDEX{id: "uniswap_trading_api", quote: &dex.Quote{DEXID: "uniswap_trading_api", EstimatedOutputAmount: "1"}},
			&mockDEX{id: "zeroex", err: errors.New("timeout contacting upstream")},
		},
	)
	w := doRequest(r, http.MethodGet, "/api/v1/health/adapters", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("adapter health: status %d, body: %s", w.Code, w.Body.String())
	}
	var resp models.AdapterHealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode adapter health: %v", err)
	}
	if len(resp.Adapters) != 4 {
		t.Fatalf("expected 4 adapter rows, got %d", len(resp.Adapters))
	}
	if resp.Status != "down" {
		t.Fatalf("overall status = %q, want down", resp.Status)
	}
}

// ── /api/v1/quote ─────────────────────────────────────────────────────────────

func TestQuoteHandler_CrossChain_CCTP(t *testing.T) {
	// A mock CCTP bridge that returns a valid USDC Arbitrum→Base route.
	cctpRoute := &models.Route{
		RouteID:               "cctp",
		EstimatedOutputAmount: "5000000",
		EstimatedTimeSeconds:  420,
		TotalFee:              "0",
		Hops: []models.Hop{
			{
				HopType:           models.HopTypeBridge,
				BridgeID:          "cctp",
				FromChain:         "arbitrum",
				ToChain:           "base",
				FromAsset:         "USDC",
				ToAsset:           "USDC",
				FromTokenAddress:  "0xaf88d065e77c8cC2239327C5EDb3A432268e5831",
				ToTokenAddress:    "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913",
				AmountInBaseUnits: "5000000",
				ProviderData: mustMarshal(map[string]any{
					"source":   "direct",
					"protocol": "circle_cctp",
				}),
				EstimatedFee: "0",
			},
		},
	}

	r := newTestRouter([]bridges.Adapter{&mockBridge{id: "cctp", route: cctpRoute}}, nil)

	reqBody := models.QuoteRequest{
		Source: models.Endpoint{
			ChainID:       42161,
			Chain:         "arbitrum",
			Asset:         "USDC",
			TokenAddress:  "0xaf88d065e77c8cC2239327C5EDb3A432268e5831",
			TokenDecimals: 6,
		},
		Destination: models.Endpoint{
			ChainID:       8453,
			Chain:         "base",
			Asset:         "USDC",
			TokenAddress:  "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913",
			TokenDecimals: 6,
		},
		AmountBaseUnits: "5000000",
	}

	w := doRequest(r, http.MethodPost, "/api/v1/quote", reqBody)
	if w.Code != http.StatusOK {
		t.Fatalf("quote: status %d, body: %s", w.Code, w.Body.String())
	}

	var resp models.QuoteResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Routes) == 0 {
		t.Fatal("expected at least one route, got 0")
	}
	found := false
	for _, rt := range resp.Routes {
		if rt.RouteID == "cctp" {
			found = true
			if rt.EstimatedOutputAmount != "5000000" {
				t.Errorf("EstimatedOutputAmount = %q, want 5000000", rt.EstimatedOutputAmount)
			}
			if len(rt.Hops) != 1 {
				t.Errorf("expected 1 hop, got %d", len(rt.Hops))
			}
		}
	}
	if !found {
		t.Error("cctp route not found in response")
	}
}

func TestQuoteHandler_SameChain_Swap(t *testing.T) {
	// Same-chain request → only DEX adapters should fire.
	dexAdapter := &mockDEX{
		id: "uniswap_trading_api",
		quote: &dex.Quote{
			DEXID:                 "uniswap_trading_api",
			EstimatedOutputAmount: "2146179246131503",
			EstimatedFeeAmount:    "0",
		},
	}

	r := newTestRouter(nil, []dex.Adapter{dexAdapter})

	reqBody := models.QuoteRequest{
		Source: models.Endpoint{
			ChainID:       8453,
			Chain:         "base",
			Asset:         "USDC",
			TokenAddress:  "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913",
			TokenDecimals: 6,
		},
		Destination: models.Endpoint{
			ChainID:       8453,
			Chain:         "base",
			Asset:         "ETH",
			TokenAddress:  "0x0000000000000000000000000000000000000000",
			TokenDecimals: 18,
		},
		AmountBaseUnits: "5000000",
	}

	w := doRequest(r, http.MethodPost, "/api/v1/quote", reqBody)
	if w.Code != http.StatusOK {
		t.Fatalf("quote: status %d, body: %s", w.Code, w.Body.String())
	}

	var resp models.QuoteResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Routes) == 0 {
		t.Fatal("expected at least one swap route, got 0")
	}
	if resp.Routes[0].Hops[0].HopType != models.HopTypeSwap {
		t.Errorf("hop type = %q, want swap", resp.Routes[0].Hops[0].HopType)
	}
}

func TestQuoteHandler_BadRequest_MissingAmount(t *testing.T) {
	r := newTestRouter(nil, nil)
	reqBody := models.QuoteRequest{
		Source:      models.Endpoint{ChainID: 1, TokenAddress: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", TokenDecimals: 6},
		Destination: models.Endpoint{ChainID: 8453, TokenAddress: "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913", TokenDecimals: 6},
		// No Amount or AmountBaseUnits
	}
	w := doRequest(r, http.MethodPost, "/api/v1/quote", reqBody)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400", w.Code)
	}
}

func TestQuoteHandler_BadRequest_InvalidAmountBaseUnits(t *testing.T) {
	r := newTestRouter(nil, nil)
	reqBody := models.QuoteRequest{
		Source:          models.Endpoint{ChainID: 1, Chain: "ethereum", Asset: "USDC", TokenAddress: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", TokenDecimals: 6},
		Destination:     models.Endpoint{ChainID: 8453, Chain: "base", Asset: "USDC", TokenAddress: "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913", TokenDecimals: 6},
		AmountBaseUnits: "1e6",
	}
	w := doRequest(r, http.MethodPost, "/api/v1/quote", reqBody)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", w.Code)
	}
}

func TestQuoteHandler_NoRoutes(t *testing.T) {
	// Adapter returns error for every request.
	r := newTestRouter(
		[]bridges.Adapter{&mockBridge{id: "across", err: router.ErrNoRoutes}},
		nil,
	)
	reqBody := models.QuoteRequest{
		Source:          models.Endpoint{ChainID: 1, Chain: "ethereum", Asset: "ETH", TokenAddress: "0x0000000000000000000000000000000000000000", TokenDecimals: 18},
		Destination:     models.Endpoint{ChainID: 8453, Chain: "base", Asset: "ETH", TokenAddress: "0x0000000000000000000000000000000000000000", TokenDecimals: 18},
		AmountBaseUnits: "10000000000000000",
	}
	w := doRequest(r, http.MethodPost, "/api/v1/quote", reqBody)
	// 422 Unprocessable or 200 with empty routes — both are acceptable NO_ROUTES responses.
	if w.Code != http.StatusUnprocessableEntity && w.Code != http.StatusOK {
		t.Errorf("status %d, want 422 or 200 with empty routes", w.Code)
	}
}

// ── /api/v1/execute (nil store) ───────────────────────────────────────────────

func TestExecuteHandler_NilStore(t *testing.T) {
	r := newTestRouter(nil, nil)
	reqBody := models.ExecuteRequest{
		Route: &models.Route{
			RouteID: "cctp",
			Hops: []models.Hop{
				{HopType: models.HopTypeBridge, BridgeID: "cctp"},
			},
		},
	}
	w := doRequest(r, http.MethodPost, "/api/v1/execute", reqBody)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("execute with nil store: status %d, want 503", w.Code)
	}
}

// ── /api/v1/operations/:id (nil store) ───────────────────────────────────────

func TestGetOperationHandler_NilStore(t *testing.T) {
	r := newTestRouter(nil, nil)
	w := doRequest(r, http.MethodGet, "/api/v1/operations/some-id", nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("get operation with nil store: status %d, want 503", w.Code)
	}
}

func TestPatchOperationStatusHandler_NilStore(t *testing.T) {
	r := newTestRouter(nil, nil)
	reqBody := models.UpdateOperationStatusRequest{Status: "submitted", TxHash: "0xdeadbeef"}
	w := doRequest(r, http.MethodPatch, "/api/v1/operations/some-id/status", reqBody)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("patch status with nil store: status %d, want 503", w.Code)
	}
}

// ── /api/v1/dex/quote ─────────────────────────────────────────────────────────

func TestDEXQuoteHandler(t *testing.T) {
	adapter := &mockDEX{
		id: "uniswap_trading_api",
		quote: &dex.Quote{
			DEXID:                 "uniswap_trading_api",
			EstimatedOutputAmount: "2146179246131503",
		},
	}
	r := newTestRouter(nil, []dex.Adapter{adapter})

	reqBody := dex.QuoteRequest{
		TokenInChainID:  8453,
		TokenOutChainID: 8453,
		TokenIn:         "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913",
		TokenOut:        "0x0000000000000000000000000000000000000000",
		Amount:          "5000000",
	}
	w := doRequest(r, http.MethodPost, "/api/v1/dex/quote", reqBody)
	if w.Code != http.StatusOK {
		t.Fatalf("dex/quote: status %d, body: %s", w.Code, w.Body.String())
	}
	var resp dex.Quote
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.EstimatedOutputAmount != "2146179246131503" {
		t.Errorf("EstimatedOutputAmount = %q", resp.EstimatedOutputAmount)
	}
}

func TestDEXQuoteHandler_MissingFields(t *testing.T) {
	r := newTestRouter(nil, []dex.Adapter{&mockDEX{id: "uniswap_trading_api"}})
	reqBody := dex.QuoteRequest{
		// Missing required fields
		TokenIn: "0x...",
	}
	w := doRequest(r, http.MethodPost, "/api/v1/dex/quote", reqBody)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400", w.Code)
	}
}

// ── /api/v1/route/stepTransaction ─────────────────────────────────────────────

func TestStepTransactionHandler_ZeroExSwap(t *testing.T) {
	zeroexAdapter := &mockDEX{id: "zeroex"}
	r := newTestRouter(nil, []dex.Adapter{zeroexAdapter})

	pd := mustMarshal(map[string]any{
		"quote": map[string]any{
			"transaction": map[string]any{
				"to":    "0x0000000000001ff3684f28c67538d4d072c22734",
				"data":  "0xdeadbeef1234abcd",
				"value": "0",
				"gas":   "200000",
			},
		},
		"permitData": nil,
		"routing":    "",
	})

	reqBody := models.StepTransactionRequest{
		Route: models.Route{
			RouteID: "bridge:cctp->swap:zeroex",
			Hops: []models.Hop{
				{
					HopType:      models.HopTypeSwap,
					BridgeID:     "zeroex",
					FromChain:    "base",
					ToChain:      "base",
					ProviderData: pd,
				},
			},
		},
		HopIndex: 0,
	}

	w := doRequest(r, http.MethodPost, "/api/v1/route/stepTransaction", reqBody)
	if w.Code != http.StatusOK {
		t.Fatalf("stepTransaction: status %d, body: %s", w.Code, w.Body.String())
	}

	var resp models.StepTransactionResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Tx == nil {
		t.Fatal("Tx is nil for 0x swap step")
	}
	if resp.Tx.To != "0x0000000000001ff3684f28c67538d4d072c22734" {
		t.Errorf("Tx.To = %q", resp.Tx.To)
	}
}

func TestStepTransactionHandler_CCTPBridge(t *testing.T) {
	r := newTestRouter(nil, nil)

	pd := mustMarshal(map[string]any{
		"source":                  "direct",
		"protocol":                "circle_cctp",
		"src_domain":              3,
		"dst_domain":              6,
		"token_messenger_src":     "0x19330d10D9Cc8751218eaf51E8885D058642E08A",
		"token_messenger_dst":     "0x1682Ae6375C4E4A97e4B583BC394c861A46D8962",
		"message_transmitter_dst": "0xAD09780d193884d503182aD4588450C416D6F9D4",
		"burn_token":              "0xaf88d065e77c8cC2239327C5EDb3A432268e5831",
		"amount":                  "5000000",
	})

	reqBody := models.StepTransactionRequest{
		Route: models.Route{
			RouteID: "cctp",
			Hops: []models.Hop{
				{
					HopType:           models.HopTypeBridge,
					BridgeID:          "cctp",
					FromChain:         "arbitrum",
					ToChain:           "base",
					AmountInBaseUnits: "5000000",
					ProviderData:      pd,
				},
			},
		},
		HopIndex:        0,
		ReceiverAddress: "0x4f8bbccc89d443e6998e52d7b57ce2ae09476328",
	}

	w := doRequest(r, http.MethodPost, "/api/v1/route/stepTransaction", reqBody)
	if w.Code != http.StatusOK {
		t.Fatalf("stepTransaction CCTP: status %d, body: %s", w.Code, w.Body.String())
	}

	var resp models.StepTransactionResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.BridgeParams == nil {
		t.Fatal("BridgeParams is nil")
	}
	if resp.BridgeParams.Protocol != "circle_cctp" {
		t.Errorf("Protocol = %q, want circle_cctp", resp.BridgeParams.Protocol)
	}
	if len(resp.BridgeParams.Steps) < 2 {
		t.Errorf("expected ≥2 steps, got %d", len(resp.BridgeParams.Steps))
	}
}

func TestStepTransactionHandler_InvalidHopIndex(t *testing.T) {
	r := newTestRouter(nil, nil)
	reqBody := models.StepTransactionRequest{
		Route:    models.Route{Hops: []models.Hop{{HopType: models.HopTypeBridge, BridgeID: "cctp"}}},
		HopIndex: 99,
	}
	w := doRequest(r, http.MethodPost, "/api/v1/route/stepTransaction", reqBody)
	if w.Code != http.StatusBadRequest {
		t.Errorf("out-of-range hop index: status %d, want 400", w.Code)
	}
}

// compile-time check that mockBridge satisfies bridges.Adapter
var _ bridges.Adapter = (*mockBridge)(nil)
