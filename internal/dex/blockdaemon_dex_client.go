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
)

// Blockdaemon DeFi API: DEX swap quotes (aggregated from multiple DEXs).
// Docs: https://docs.blockdaemon.com/docs/defi-api-execute-a-local-swap
const defaultBlockdaemonDEXURL = "https://svc.blockdaemon.com"

// BlockdaemonDEXQuotesResponse is the GET /defi/v1/dex/quotes response.
type BlockdaemonDEXQuotesResponse struct {
	Status int    `json:"status"`
	Msg    string `json:"msg"`
	Data   []struct {
		DEXID           string   `json:"dexId"`
		DEXName         string   `json:"dexName"`
		BridgeID        string   `json:"bridgeId,omitempty"`        // Sometimes returns bridges too
		BridgeName      string   `json:"bridgeName,omitempty"`
		ChainID         string   `json:"chainId,omitempty"`
		Path            []string `json:"path"` // Token addresses in swap path
		AmountIn        string   `json:"amountIn"`
		AmountsOut      []string `json:"amountsOut"` // [inputAmount, expectedOutputAmount] in smallest unit
		EstimatedGas    string   `json:"estimatedGas,omitempty"`
		TransactionData map[string]interface{} `json:"transactionData,omitempty"` // Optional tx data for execution
	} `json:"data"`
}

// BlockdaemonDEXClient calls Blockdaemon DeFi API for aggregated DEX swap quotes.
type BlockdaemonDEXClient struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// NewBlockdaemonDEXClient returns a client for the Blockdaemon DeFi API (DEX swaps).
func NewBlockdaemonDEXClient(baseURL string, apiKey string) *BlockdaemonDEXClient {
	if baseURL == "" {
		baseURL = defaultBlockdaemonDEXURL
	}
	return &BlockdaemonDEXClient{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// SwapQuoteResult is the best swap quote from the aggregator (first in sorted list).
type SwapQuoteResult struct {
	DEXID        string
	DEXName      string
	AmountOut    string // base units (smallest denomination of the output token)
	EstimatedGas string
	ProviderData json.RawMessage // Store full quote for transaction building
}

// GetSwapQuotes calls GET /defi/v1/dex/quotes. chainID is numeric (e.g. 1, 137).
// tokenIn and tokenOut are token addresses. amountIn is in token smallest units.
// Returns the best quote (first in API response).
func (c *BlockdaemonDEXClient) GetSwapQuotes(ctx context.Context, chainID int64, tokenIn, tokenOut, amountIn string) (*SwapQuoteResult, error) {
	u, err := url.Parse(c.BaseURL + "/defi/v1/dex/quotes")
	if err != nil {
		return nil, err
	}
	// Blockdaemon uses "path" parameter: comma-separated token addresses
	path := fmt.Sprintf("%s,%s", tokenIn, tokenOut)
	q := u.Query()
	q.Set("chainId", strconv.FormatInt(chainID, 10))
	q.Set("path", path)
	q.Set("amountIn", amountIn)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("X-API-Key", c.APIKey)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("blockdaemon dex: %d %s", resp.StatusCode, shortHTTPErrorBD(resp.StatusCode, body))
	}

	var data BlockdaemonDEXQuotesResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("blockdaemon dex response decode: %w", err)
	}
	if data.Status != 200 || len(data.Data) == 0 {
		return nil, fmt.Errorf("blockdaemon dex: no quotes (status=%d msg=%s)", data.Status, data.Msg)
	}

	// Find the best executable quote: one that includes transactionData.
	// Blockdaemon may return quotes without transactionData for some DEXes; those
	// cannot be executed so we skip them. Take the first that has it.
	bestIdx := -1
	for i := range data.Data {
		if data.Data[i].TransactionData != nil {
			bestIdx = i
			break
		}
	}
	if bestIdx < 0 {
		return nil, fmt.Errorf("blockdaemon dex: no executable quote returned (all quotes missing transactionData)")
	}
	best := &data.Data[bestIdx]

	// AmountsOut[1] is the expected output in the token's smallest units.
	amountOut := ""
	if len(best.AmountsOut) >= 2 {
		amountOut = best.AmountsOut[1]
	}
	if amountOut == "" {
		amountOut = "0"
	}

	// Use DEX name if available, otherwise bridge name.
	name := best.DEXName
	if name == "" {
		name = best.BridgeName
	}
	id := best.DEXID
	if id == "" {
		id = best.BridgeID
	}

	providerData, _ := json.Marshal(best)

	return &SwapQuoteResult{
		DEXID:        id,
		DEXName:      name,
		AmountOut:    amountOut,
		EstimatedGas: best.EstimatedGas,
		ProviderData: providerData,
	}, nil
}

func shortHTTPErrorBD(statusCode int, body []byte) string {
	status := http.StatusText(statusCode)
	if status == "" {
		status = "error"
	}
	msg := fmt.Sprintf("%d %s", statusCode, status)
	var v struct {
		Msg string `json:"msg"`
	}
	if json.Unmarshal(body, &v) == nil && v.Msg != "" {
		s := v.Msg
		if len(s) > 80 {
			s = s[:77] + "..."
		}
		msg = msg + ": " + s
	}
	return msg
}
