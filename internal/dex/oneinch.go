package dex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"bridge-aggregator/internal/models"
)

const oneInchID = "oneinch"

type OneInchAdapter struct {
	baseURL string
	apiKey  string
	version string
	swapper string
	client  *http.Client
}

// NewOneInchAdapter constructs a 1inch Classic Swap adapter.
// baseURL defaults to https://api.1inch.com
// version defaults to v6.1
// apiKey is the 1inch Business API key used as Authorization: Bearer <key>
// swapper is used as the "from" parameter for /swap tx generation.
func NewOneInchAdapter(baseURL, apiKey, version, swapper string) *OneInchAdapter {
	if baseURL == "" {
		baseURL = "https://api.1inch.com"
	}
	if version == "" {
		version = "v6.1"
	}
	return &OneInchAdapter{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		version: version,
		swapper: swapper,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (a *OneInchAdapter) ID() string { return oneInchID }

func normalizeWrappedNative(chainID int, addr string) (string, error) {
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
		return "", fmt.Errorf("1inch requires token addresses; chain %d needs wrapped native address mapping", chainID)
	}
}

func (a *OneInchAdapter) endpoint(chainID int, path string) (string, error) {
	if chainID <= 0 {
		return "", fmt.Errorf("chain id is required")
	}
	return fmt.Sprintf("%s/swap/%s/%d%s", a.baseURL, a.version, chainID, path), nil
}

type oneInchQuoteResponse struct {
	DstAmount string `json:"dstAmount"`
}

type oneInchSwapResponse struct {
	DstAmount string `json:"dstAmount"`
	Tx        struct {
		To    string `json:"to"`
		Data  string `json:"data"`
		Value string `json:"value"`
		Gas   string `json:"gas"`
	} `json:"tx"`
}

type oneInchProviderQuote struct {
	SwapParams map[string]string `json:"swapParams"`
	QuoteRaw   json.RawMessage   `json:"quoteRaw"`
}

// GetQuote calls 1inch Classic Swap /quote and returns dstAmount.
// Only same-chain swaps are supported (TokenInChainID must equal TokenOutChainID).
func (a *OneInchAdapter) GetQuote(ctx context.Context, req QuoteRequest) (*Quote, error) {
	if a.apiKey == "" {
		return nil, fmt.Errorf("1inch api key must be configured")
	}
	if req.TokenInChainID != req.TokenOutChainID {
		return nil, fmt.Errorf("1inch adapter only supports same-chain swaps; chain ids must match")
	}
	if req.TokenInChainID == 0 || req.TokenIn == "" || req.TokenOut == "" || req.Amount == "" {
		return nil, fmt.Errorf("tokenInChainId, tokenIn, tokenOut, and amount are required")
	}

	src, err := normalizeWrappedNative(req.TokenInChainID, req.TokenIn)
	if err != nil {
		return nil, err
	}
	dst, err := normalizeWrappedNative(req.TokenInChainID, req.TokenOut)
	if err != nil {
		return nil, err
	}

	quoteURL, err := a.endpoint(req.TokenInChainID, "/quote")
	if err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("src", src)
	q.Set("dst", dst)
	q.Set("amount", req.Amount)

	u, _ := url.Parse(quoteURL)
	u.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("1inch api %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var qr oneInchQuoteResponse
	if err := json.Unmarshal(body, &qr); err != nil {
		return nil, err
	}
	if qr.DstAmount == "" {
		return nil, fmt.Errorf("1inch quote missing dstAmount")
	}

	// Store swap params needed for stepTransaction (we will call /swap).
	// Discard placeholder addresses like "0xYourWallet".
	swapper := req.Swapper
	if !IsValidEVMAddress(swapper) {
		swapper = a.swapper
	}
	pq, _ := json.Marshal(oneInchProviderQuote{
		SwapParams: map[string]string{
			"chainId":          strconv.Itoa(req.TokenInChainID),
			"src":              src,
			"dst":              dst,
			"amount":           req.Amount,
			"from":             strings.ToLower(swapper),
			"slippage":         "1",
			"disableEstimate":  "false",
			"allowPartialFill": "false",
		},
		QuoteRaw: json.RawMessage(body),
	})

	return &Quote{
		DEXID:                 oneInchID,
		EstimatedOutputAmount: qr.DstAmount,
		EstimatedFeeAmount:    "",
		ProviderQuote:         pq,
	}, nil
}

// CreateSwapTx calls 1inch Classic Swap /swap using swap params stored in providerQuote.
func (a *OneInchAdapter) CreateSwapTx(ctx context.Context, providerQuote json.RawMessage) (models.TransactionRequest, error) {
	var pq oneInchProviderQuote
	if err := json.Unmarshal(providerQuote, &pq); err != nil {
		return models.TransactionRequest{}, fmt.Errorf("invalid 1inch provider quote: %w", err)
	}
	chainIDStr := pq.SwapParams["chainId"]
	if chainIDStr == "" {
		return models.TransactionRequest{}, fmt.Errorf("missing chainId in 1inch provider quote")
	}
	chainID, _ := strconv.Atoi(chainIDStr)
	swapURL, err := a.endpoint(chainID, "/swap")
	if err != nil {
		return models.TransactionRequest{}, err
	}

	u, _ := url.Parse(swapURL)
	q := url.Values{}
	for k, v := range pq.SwapParams {
		if k == "chainId" || v == "" {
			continue
		}
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return models.TransactionRequest{}, err
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return models.TransactionRequest{}, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return models.TransactionRequest{}, fmt.Errorf("1inch api %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var sr oneInchSwapResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return models.TransactionRequest{}, err
	}
	if sr.Tx.To == "" || sr.Tx.Data == "" || sr.Tx.Data == "0x" {
		return models.TransactionRequest{}, fmt.Errorf("1inch swap returned invalid tx")
	}
	return models.TransactionRequest{
		To:       sr.Tx.To,
		Data:     sr.Tx.Data,
		Value:    sr.Tx.Value,
		ChainID:  chainID,
		GasLimit: sr.Tx.Gas,
	}, nil
}

