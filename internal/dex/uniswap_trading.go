package dex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// UniswapTradingAdapter calls the Uniswap Trading API /v1/quote endpoint.
// Docs: https://docs.uniswap.org/api/trading/overview
type UniswapTradingAdapter struct {
	baseURL string
	apiKey  string
	signer  string
	client  *http.Client
}

func NewUniswapTradingAdapter(baseURL, apiKey, signer string) *UniswapTradingAdapter {
	return &UniswapTradingAdapter{
		baseURL: baseURL,
		apiKey:  apiKey,
		signer:  signer,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (a *UniswapTradingAdapter) ID() string {
	return "uniswap_trading_api"
}

// uniswapQuoteRequest is a minimal request body for EXACT_INPUT swaps on mainnet.
type uniswapQuoteRequest struct {
	GeneratePermitAsTransaction bool   `json:"generatePermitAsTransaction"`
	AutoSlippage                string `json:"autoSlippage"`
	RoutingPreference           string `json:"routingPreference"`
	SpreadOptimization          string `json:"spreadOptimization"`
	Urgency                     string `json:"urgency"`
	PermitAmount                string `json:"permitAmount"`
	Type                        string `json:"type"`
	Amount                      string `json:"amount"`
	TokenInChainID              int    `json:"tokenInChainId"`
	TokenOutChainID             int    `json:"tokenOutChainId"`
	TokenIn                     string `json:"tokenIn"`
	TokenOut                    string `json:"tokenOut"`
	Swapper                     string `json:"swapper"`
}

// uniswapQuoteResponse is a minimal subset of the Trading API /quote response.
type uniswapQuoteResponse struct {
	Routing   string          `json:"routing"`
	PermitData json.RawMessage `json:"permitData"`
	Quote struct {
		Input struct {
			Amount string `json:"amount"`
		} `json:"input"`
		Output struct {
			Amount string `json:"amount"`
		} `json:"output"`
	} `json:"quote"`
	QuoteRaw json.RawMessage `json:"-"`
}

// GetQuote calls Uniswap /v1/quote for an EXACT_INPUT swap using explicit chain IDs and token addresses.
// Amount must be in base units (e.g. wei for ETH, 6-decimals for USDC).
func (a *UniswapTradingAdapter) GetQuote(ctx context.Context, req QuoteRequest) (*Quote, error) {
	if a.apiKey == "" {
		return nil, fmt.Errorf("uniswap trading api key must be configured")
	}
	if req.TokenInChainID == 0 || req.TokenOutChainID == 0 || req.TokenIn == "" || req.TokenOut == "" || req.Amount == "" {
		return nil, fmt.Errorf("tokenInChainId, tokenOutChainId, tokenIn, tokenOut, and amount are required")
	}
	swapper := req.Swapper
	if swapper == "" {
		swapper = a.signer
	}
	if swapper == "" {
		return nil, fmt.Errorf("uniswap swapper wallet must be configured (either request.swapper or uniswap_swapper_wallet)")
	}

	body := uniswapQuoteRequest{
		GeneratePermitAsTransaction: false,
		AutoSlippage:                "DEFAULT",
		RoutingPreference:           "BEST_PRICE",
		SpreadOptimization:          "EXECUTION",
		Urgency:                     "urgent",
		PermitAmount:                "FULL",
		Type:                        "EXACT_INPUT",
		Amount:                      req.Amount,
		TokenInChainID:              req.TokenInChainID,
		TokenOutChainID:             req.TokenOutChainID,
		TokenIn:                     req.TokenIn,
		TokenOut:                    req.TokenOut,
		Swapper:                     swapper,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("uniswap quote marshal: %w", err)
	}

	url := a.baseURL + "/quote"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("uniswap quote request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("x-universal-router-version", "2.0")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("uniswap quote http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("uniswap quote: unexpected status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("uniswap quote read body: %w", err)
	}

	var uq uniswapQuoteResponse
	if err := json.Unmarshal(bodyBytes, &uq); err != nil {
		return nil, fmt.Errorf("uniswap quote decode: %w", err)
	}
	// capture raw quote object
	var raw struct {
		Quote json.RawMessage `json:"quote"`
	}
	_ = json.Unmarshal(bodyBytes, &raw)
	uq.QuoteRaw = raw.Quote
	if uq.Quote.Output.Amount == "" {
		return nil, fmt.Errorf("uniswap quote: missing output amount")
	}

	// Compute a simple fee estimate in input units: (input - output_in_input_units) if possible.
	// For now we just leave EstimatedFeeAmount as "0" because the Trading API does not provide
	// a direct fee field in the quote; fee is implicit in price and gas.
	// You can extend this later by mapping USDC output back to ETH using price data.

	return &Quote{
		DEXID:                 a.ID(),
		EstimatedOutputAmount: uq.Quote.Output.Amount,
		EstimatedFeeAmount:    "0",
		ProviderQuote:         uq.QuoteRaw,
		PermitData:            uq.PermitData,
		Routing:               uq.Routing,
	}, nil
}

