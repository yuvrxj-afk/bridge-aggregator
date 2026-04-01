package dex_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"bridge-aggregator/internal/dex"
	"bridge-aggregator/internal/models"
)

// ── 0x (ZeroEx) adapter ───────────────────────────────────────────────────────

const testTaker = "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045"

func zeroExHappyResponse() map[string]any {
	return map[string]any{
		"liquidityAvailable": true,
		"buyAmount":          "2142959229232459",
		"sellAmount":         "5000000",
		"minBuyAmount":       "2121529636940134",
		"transaction": map[string]any{
			"to":       "0x0000000000001ff3684f28c67538d4d072c22734",
			"data":     "0xdeadbeef1234",
			"value":    "0",
			"gas":      "200000",
			"gasPrice": "1000000000",
		},
		"fees": map[string]any{
			"gasFee": map[string]any{
				"amount": "50000",
				"token":  "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2",
			},
		},
	}
}

func zeroExQuoteReq() dex.QuoteRequest {
	return dex.QuoteRequest{
		TokenInChainID:  8453,
		TokenOutChainID: 8453,
		TokenIn:         "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913",
		TokenOut:        "0x4200000000000000000000000000000000000006",
		Amount:          "5000000",
		Swapper:         testTaker,
	}
}

func TestZeroExAdapter_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(zeroExHappyResponse())
	}))
	defer srv.Close()

	adapter := dex.NewZeroExAdapter(srv.URL, "test-key", testTaker)
	q, err := adapter.GetQuote(context.Background(), zeroExQuoteReq())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q == nil {
		t.Fatal("quote is nil")
	}
	if q.EstimatedOutputAmount != "2142959229232459" {
		t.Errorf("EstimatedOutputAmount = %q, want 2142959229232459", q.EstimatedOutputAmount)
	}
	if q.DEXID != "zeroex" {
		t.Errorf("DEXID = %q, want zeroex", q.DEXID)
	}
	// ProviderQuote must be non-nil (used by CreateSwapTx)
	if len(q.ProviderQuote) == 0 {
		t.Error("ProviderQuote is empty — CreateSwapTx will fail")
	}
}

func TestZeroExAdapter_API500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"message": "internal error"})
	}))
	defer srv.Close()

	adapter := dex.NewZeroExAdapter(srv.URL, "test-key", testTaker)
	_, err := adapter.GetQuote(context.Background(), zeroExQuoteReq())
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestZeroExAdapter_NoLiquidity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := zeroExHappyResponse()
		resp["liquidityAvailable"] = false
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	adapter := dex.NewZeroExAdapter(srv.URL, "test-key", testTaker)
	_, err := adapter.GetQuote(context.Background(), zeroExQuoteReq())
	if err == nil {
		t.Fatal("expected error when liquidityAvailable=false")
	}
}

func TestZeroExAdapter_UnexpectedSchema(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`not valid json`))
	}))
	defer srv.Close()

	adapter := dex.NewZeroExAdapter(srv.URL, "test-key", testTaker)
	_, err := adapter.GetQuote(context.Background(), zeroExQuoteReq())
	if err == nil {
		t.Fatal("expected error for malformed JSON response")
	}
}

func TestZeroExAdapter_MissingAPIKey(t *testing.T) {
	adapter := dex.NewZeroExAdapter("https://api.0x.org", "", testTaker)
	_, err := adapter.GetQuote(context.Background(), zeroExQuoteReq())
	if err == nil {
		t.Fatal("expected error when API key is missing")
	}
}

func TestZeroExAdapter_MissingTaker(t *testing.T) {
	adapter := dex.NewZeroExAdapter("https://api.0x.org", "test-key", "")
	req := zeroExQuoteReq()
	req.Swapper = "" // no swapper in request either
	_, err := adapter.GetQuote(context.Background(), req)
	if err == nil {
		t.Fatal("expected error when taker address is missing")
	}
}

func TestZeroExAdapter_CrossChainRejected(t *testing.T) {
	adapter := dex.NewZeroExAdapter("https://api.0x.org", "test-key", testTaker)
	req := zeroExQuoteReq()
	req.TokenOutChainID = 42161 // different from TokenInChainID=8453
	_, err := adapter.GetQuote(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for cross-chain request (0x is same-chain only)")
	}
}

func TestZeroExAdapter_Tier_WithTaker(t *testing.T) {
	adapter := dex.NewZeroExAdapter("https://api.0x.org", "test-key", testTaker)
	if adapter.Tier() != models.TierDegraded {
		t.Errorf("0x tier with taker = %d, want TierDegraded(%d)", adapter.Tier(), models.TierDegraded)
	}
}

func TestZeroExAdapter_Tier_NoTaker(t *testing.T) {
	adapter := dex.NewZeroExAdapter("https://api.0x.org", "test-key", "")
	if adapter.Tier() != models.TierConfigBroken {
		t.Errorf("0x tier without taker = %d, want TierConfigBroken(%d)", adapter.Tier(), models.TierConfigBroken)
	}
}

// ── 1inch adapter ─────────────────────────────────────────────────────────────

func oneInchQuoteReq() dex.QuoteRequest {
	return dex.QuoteRequest{
		TokenInChainID:  1,
		TokenOutChainID: 1,
		TokenIn:         "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", // USDC
		TokenOut:        "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", // WETH
		Amount:          "5000000",
		Swapper:         testTaker,
	}
}

func TestOneInchAdapter_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"dstAmount": "1421000000000000",
		})
	}))
	defer srv.Close()

	adapter := dex.NewOneInchAdapter(srv.URL, "test-key", "v6.1", testTaker)
	q, err := adapter.GetQuote(context.Background(), oneInchQuoteReq())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.EstimatedOutputAmount != "1421000000000000" {
		t.Errorf("EstimatedOutputAmount = %q, want 1421000000000000", q.EstimatedOutputAmount)
	}
	if q.DEXID != "oneinch" {
		t.Errorf("DEXID = %q, want oneinch", q.DEXID)
	}
}

func TestOneInchAdapter_API500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()

	adapter := dex.NewOneInchAdapter(srv.URL, "test-key", "v6.1", testTaker)
	_, err := adapter.GetQuote(context.Background(), oneInchQuoteReq())
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestOneInchAdapter_MissingDstAmount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Valid JSON but missing dstAmount
		json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}))
	defer srv.Close()

	adapter := dex.NewOneInchAdapter(srv.URL, "test-key", "v6.1", testTaker)
	_, err := adapter.GetQuote(context.Background(), oneInchQuoteReq())
	if err == nil {
		t.Fatal("expected error when dstAmount is missing")
	}
}

func TestOneInchAdapter_MissingAPIKey(t *testing.T) {
	adapter := dex.NewOneInchAdapter("https://api.1inch.com", "", "v6.1", testTaker)
	_, err := adapter.GetQuote(context.Background(), oneInchQuoteReq())
	if err == nil {
		t.Fatal("expected error when API key is missing")
	}
}

func TestOneInchAdapter_CrossChainRejected(t *testing.T) {
	adapter := dex.NewOneInchAdapter("https://api.1inch.com", "test-key", "v6.1", testTaker)
	req := oneInchQuoteReq()
	req.TokenOutChainID = 8453
	_, err := adapter.GetQuote(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for cross-chain request (1inch is same-chain only)")
	}
}

func TestOneInchAdapter_Tier_WithKey(t *testing.T) {
	adapter := dex.NewOneInchAdapter("https://api.1inch.com", "test-key", "v6.1", testTaker)
	if adapter.Tier() != models.TierProduction {
		t.Errorf("1inch tier with key = %d, want TierProduction(%d)", adapter.Tier(), models.TierProduction)
	}
}

func TestOneInchAdapter_Tier_NoKey(t *testing.T) {
	adapter := dex.NewOneInchAdapter("https://api.1inch.com", "", "v6.1", testTaker)
	if adapter.Tier() != models.TierUncredentialed {
		t.Errorf("1inch tier without key = %d, want TierUncredentialed(%d)", adapter.Tier(), models.TierUncredentialed)
	}
}

// ── Uniswap Trading API adapter ───────────────────────────────────────────────

func uniswapQuoteReq() dex.QuoteRequest {
	return dex.QuoteRequest{
		TokenInChainID:  8453,
		TokenOutChainID: 8453,
		TokenIn:         "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913", // USDC Base
		TokenOut:        "0x4200000000000000000000000000000000000006", // WETH Base
		Amount:          "5000000",
		Swapper:         testTaker,
	}
}

func uniswapHappyResponse() map[string]any {
	return map[string]any{
		"routing": "CLASSIC",
		"quote": map[string]any{
			"input":  map[string]any{"amount": "5000000"},
			"output": map[string]any{"amount": "2000000000000000"},
		},
		"permitData": nil,
	}
}

func TestUniswapAdapter_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(uniswapHappyResponse())
	}))
	defer srv.Close()

	adapter := dex.NewUniswapTradingAdapter(srv.URL, "test-key", testTaker)
	q, err := adapter.GetQuote(context.Background(), uniswapQuoteReq())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q == nil {
		t.Fatal("quote is nil")
	}
	if q.EstimatedOutputAmount != "2000000000000000" {
		t.Errorf("EstimatedOutputAmount = %q, want 2000000000000000", q.EstimatedOutputAmount)
	}
	if q.DEXID != "uniswap_trading_api" {
		t.Errorf("DEXID = %q, want uniswap_trading_api", q.DEXID)
	}
}

func TestUniswapAdapter_API500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	adapter := dex.NewUniswapTradingAdapter(srv.URL, "test-key", testTaker)
	_, err := adapter.GetQuote(context.Background(), uniswapQuoteReq())
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestUniswapAdapter_MissingAPIKey(t *testing.T) {
	adapter := dex.NewUniswapTradingAdapter("https://trading-api.example.com", "", testTaker)
	_, err := adapter.GetQuote(context.Background(), uniswapQuoteReq())
	if err == nil {
		t.Fatal("expected error when API key is missing")
	}
}

func TestUniswapAdapter_Tier_WithKey(t *testing.T) {
	adapter := dex.NewUniswapTradingAdapter("https://example.com", "test-key", testTaker)
	if adapter.Tier() != models.TierProduction {
		t.Errorf("Uniswap tier with key = %d, want TierProduction(%d)", adapter.Tier(), models.TierProduction)
	}
}

func TestUniswapAdapter_Tier_NoKey(t *testing.T) {
	adapter := dex.NewUniswapTradingAdapter("https://example.com", "", testTaker)
	if adapter.Tier() != models.TierUncredentialed {
		t.Errorf("Uniswap tier without key = %d, want TierUncredentialed(%d)", adapter.Tier(), models.TierUncredentialed)
	}
}

// ── Blockdaemon DEX adapter ───────────────────────────────────────────────────

func bdDEXHappyResponse() map[string]any {
	return map[string]any{
		"status": 200,
		"msg":    "ok",
		"data": []map[string]any{
			{
				"dexId":      "uniswap_v3",
				"dexName":    "Uniswap V3",
				"amountIn":   "5000000",
				"amountsOut": []string{"5000000", "2100000000000000"},
				"transactionData": map[string]any{
					"to":    "0x1234567890abcdef",
					"data":  "0xabcdef",
					"value": "0",
				},
			},
		},
	}
}

func TestBlockdaemonDEXAdapter_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(bdDEXHappyResponse())
	}))
	defer srv.Close()

	client := dex.NewBlockdaemonDEXClient(srv.URL, "test-key")
	adapter := dex.NewBlockdaemonDEXAdapter(client)
	req := dex.QuoteRequest{
		TokenInChainID:  1,
		TokenOutChainID: 1,
		TokenIn:         "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		TokenOut:        "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2",
		Amount:          "5000000",
	}

	q, err := adapter.GetQuote(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q == nil {
		t.Fatal("quote is nil")
	}
	if q.EstimatedOutputAmount != "2100000000000000" {
		t.Errorf("EstimatedOutputAmount = %q, want 2100000000000000", q.EstimatedOutputAmount)
	}
}

func TestBlockdaemonDEXAdapter_API500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := dex.NewBlockdaemonDEXClient(srv.URL, "test-key")
	adapter := dex.NewBlockdaemonDEXAdapter(client)
	req := dex.QuoteRequest{
		TokenInChainID:  1, TokenOutChainID: 1,
		TokenIn: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		TokenOut: "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2",
		Amount:  "5000000",
	}
	_, err := adapter.GetQuote(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestBlockdaemonDEXAdapter_NoTransactionData(t *testing.T) {
	// Blockdaemon known issue: returns quotes without transactionData
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status": 200, "msg": "ok",
			"data": []map[string]any{
				{
					"dexId":      "uniswap_v3",
					"amountIn":   "5000000",
					"amountsOut": []string{"5000000", "2100000000000000"},
					// transactionData intentionally missing
				},
			},
		})
	}))
	defer srv.Close()

	client := dex.NewBlockdaemonDEXClient(srv.URL, "test-key")
	adapter := dex.NewBlockdaemonDEXAdapter(client)
	req := dex.QuoteRequest{
		TokenInChainID:  1, TokenOutChainID: 1,
		TokenIn: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		TokenOut: "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2",
		Amount:  "5000000",
	}
	_, err := adapter.GetQuote(context.Background(), req)
	if err == nil {
		t.Fatal("expected error when all quotes are missing transactionData")
	}
}

func TestBlockdaemonDEXAdapter_Tier(t *testing.T) {
	adapter := dex.NewBlockdaemonDEXAdapter(nil)
	if adapter.Tier() != models.TierDegraded {
		t.Errorf("Blockdaemon DEX tier = %d, want TierDegraded(%d)", adapter.Tier(), models.TierDegraded)
	}
}

// ── Timeout test ──────────────────────────────────────────────────────────────

func TestZeroExAdapter_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(zeroExHappyResponse())
	}))
	defer srv.Close()

	// Use a very short timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	adapter := dex.NewZeroExAdapter(srv.URL, "test-key", testTaker)
	_, err := adapter.GetQuote(ctx, zeroExQuoteReq())
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
