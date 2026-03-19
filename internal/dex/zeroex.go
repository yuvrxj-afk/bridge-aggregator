package dex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const zeroExID = "zeroex"
const nativeTokenZeroAddress = "0x0000000000000000000000000000000000000000"

// 0x allowance-holder/permit2 endpoints require token addresses (not "ETH").
// When the caller uses the zero address to represent native ETH, we translate it
// to the wrapped native token address for that chain.
func normalize0xTokenParam(chainID int, addr string) (string, error) {
	if !strings.EqualFold(addr, nativeTokenZeroAddress) {
		return addr, nil
	}
	switch chainID {
	case 1: // Ethereum → WETH
		return "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", nil
	case 8453: // Base → WETH
		return "0x4200000000000000000000000000000000000006", nil
	case 10: // Optimism → WETH
		return "0x4200000000000000000000000000000000000006", nil
	case 42161: // Arbitrum → WETH
		return "0x82aF49447D8a07e3bd95BD0d56f35241523fBab1", nil
	case 137: // Polygon → WMATIC (native is MATIC)
		return "0x0d500B1d8E8eF31E21C99d1Db9A6444d3ADf1270", nil
	case 56: // BSC → WBNB (native is BNB)
		return "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c", nil
	case 43114: // Avalanche C-Chain → WAVAX (native is AVAX)
		return "0xB31f66AA3C1e785363F0875A1B74E27b85FD66c7", nil
	default:
		return "", fmt.Errorf("0x requires token addresses; chain %d needs wrapped native address mapping", chainID)
	}
}

// ZeroExAdapter calls the 0x Swap API allowance-holder quote endpoint.
// Docs: https://docs.0x.org/api-reference/openapi-json/swap/allowanceholder-getquote
type ZeroExAdapter struct {
	baseURL string
	apiKey  string
	taker   string
	client  *http.Client
}

// NewZeroExAdapter returns a 0x DEX adapter. baseURL is the 0x API root (e.g. https://api.0x.org).
// taker is the address that holds sellToken and will set allowance; if empty, quotes may fail.
func NewZeroExAdapter(baseURL, apiKey, taker string) *ZeroExAdapter {
	if baseURL == "" {
		baseURL = "https://api.0x.org"
	}
	return &ZeroExAdapter{
		baseURL: baseURL,
		apiKey:  apiKey,
		taker:   taker,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (a *ZeroExAdapter) ID() string {
	return zeroExID
}

// zeroExQuoteResponse matches the 200 response when liquidityAvailable is true.
type zeroExQuoteResponse struct {
	LiquidityAvailable bool   `json:"liquidityAvailable"`
	BuyAmount          string `json:"buyAmount"`
	SellAmount         string `json:"sellAmount"`
	MinBuyAmount       string `json:"minBuyAmount"`
	Transaction        struct {
		To       string `json:"to"`
		Data     string `json:"data"`
		Value    string `json:"value"`
		Gas      string `json:"gas"`
		GasPrice string `json:"gasPrice"`
	} `json:"transaction"`
	Fees struct {
		GasFee *struct {
			Amount string `json:"amount"`
			Token  string `json:"token"`
		} `json:"gasFee"`
	} `json:"fees"`
}

// GetQuote requests a firm quote from 0x GET /swap/allowance-holder/quote.
// Only same-chain swaps are supported (TokenInChainID must equal TokenOutChainID).
func (a *ZeroExAdapter) GetQuote(ctx context.Context, req QuoteRequest) (*Quote, error) {
	if a.apiKey == "" {
		return nil, fmt.Errorf("0x api key must be configured")
	}
	if req.TokenInChainID != req.TokenOutChainID {
		return nil, fmt.Errorf("0x adapter only supports same-chain swaps; chain ids must match")
	}
	if req.TokenInChainID == 0 || req.TokenIn == "" || req.TokenOut == "" || req.Amount == "" {
		return nil, fmt.Errorf("tokenInChainId, tokenIn, tokenOut, and amount are required")
	}

	// Prefer request-level swapper if it is a valid address; discard placeholders like "0xYourWallet".
	taker := req.Swapper
	if !IsValidEVMAddress(taker) {
		taker = a.taker
	}
	if !IsValidEVMAddress(taker) {
		return nil, fmt.Errorf("0x requires taker address (set ZEROEX_TAKER or UNISWAP_SWAPPER_WALLET env var)")
	}

	path := a.baseURL + "/swap/allowance-holder/quote"
	params := url.Values{}
	params.Set("chainId", fmt.Sprintf("%d", req.TokenInChainID))
	sellToken, err := normalize0xTokenParam(req.TokenInChainID, req.TokenIn)
	if err != nil {
		return nil, err
	}
	buyToken, err := normalize0xTokenParam(req.TokenInChainID, req.TokenOut)
	if err != nil {
		return nil, err
	}
	params.Set("sellToken", sellToken)
	params.Set("buyToken", buyToken)
	params.Set("sellAmount", req.Amount)
	params.Set("taker", taker)

	u, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	u.RawQuery = params.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("0x-api-key", a.apiKey)
	httpReq.Header.Set("0x-version", "v2")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		if errBody.Message != "" {
			return nil, fmt.Errorf("0x api %s: %s", resp.Status, errBody.Message)
		}
		return nil, fmt.Errorf("0x api %s", resp.Status)
	}

	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	var q zeroExQuoteResponse
	if err := json.Unmarshal(raw, &q); err != nil {
		return nil, err
	}
	if !q.LiquidityAvailable {
		return nil, fmt.Errorf("0x: no liquidity available for this pair")
	}

	feeAmount := ""
	if q.Fees.GasFee != nil {
		feeAmount = q.Fees.GasFee.Amount
	}

	return &Quote{
		DEXID:                 zeroExID,
		EstimatedOutputAmount: q.BuyAmount,
		EstimatedFeeAmount:   feeAmount,
		ProviderQuote:         raw,
		PermitData:            nil,
		Routing:               "",
	}, nil
}
