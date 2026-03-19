package bridges

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

// Blockdaemon DeFi API: bridge quotes (swap quotes from multiple bridges).
// Docs: https://docs.blockdaemon.com/docs/defi-api-overview
const defaultBlockdaemonAPIURL = "https://svc.blockdaemon.com"

// BlockdaemonQuotesResponse is the GET /defi/v1/bridge/quotes response.
type BlockdaemonQuotesResponse struct {
	Status int    `json:"status"`
	Msg    string `json:"msg"`
	Data   []struct {
		BridgeID       string   `json:"bridgeId"`
		BridgeName     string   `json:"bridgeName"`
		SrcChainID     string   `json:"srcChainId"`
		DstChainID     string   `json:"dstChainId"`
		SrcTokenSymbol string   `json:"srcTokenSymbol"`
		DstTokenSymbol string   `json:"dstTokenSymbol"`
		AmountIn       string   `json:"amountIn"`
		AmountsOut      []string `json:"amountsOut"` // [inputAmount, expectedOutputAmount] in smallest unit
	} `json:"data"`
}

// BlockdaemonClient calls Blockdaemon DeFi API for aggregated bridge quotes.
type BlockdaemonClient struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// NewBlockdaemonClient returns a client for the Blockdaemon DeFi API.
func NewBlockdaemonClient(baseURL string, apiKey string) *BlockdaemonClient {
	if baseURL == "" {
		baseURL = defaultBlockdaemonAPIURL
	}
	return &BlockdaemonClient{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

// BlockdaemonQuoteResult is the best quote from the aggregator (first in sorted list).
type BlockdaemonQuoteResult struct {
	BridgeID         string
	BridgeName       string
	AmountOut        string // base units (smallest denomination of the destination token)
	EstimatedTimeSec int64
}

// GetBridgeQuotes calls GET /defi/v1/bridge/quotes. Chain IDs are numeric (e.g. 1, 137).
// amountInSmallest is in token smallest units. Returns the best quote (first in API response).
func (c *BlockdaemonClient) GetBridgeQuotes(ctx context.Context, srcChainID, dstChainID int64, srcTokenSymbol, dstTokenSymbol string, amountInSmallest string) (*BlockdaemonQuoteResult, error) {
	u, err := url.Parse(c.BaseURL + "/defi/v1/bridge/quotes")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("srcChainId", strconv.FormatInt(srcChainID, 10))
	q.Set("dstChainId", strconv.FormatInt(dstChainID, 10))
	q.Set("srcTokenSymbol", srcTokenSymbol)
	q.Set("dstTokenSymbol", dstTokenSymbol)
	q.Set("amountIn", amountInSmallest)
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
		return nil, fmt.Errorf("blockdaemon: %d %s", resp.StatusCode, shortHTTPErrorBlockdaemon(resp.StatusCode, body))
	}

	var data BlockdaemonQuotesResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("blockdaemon response decode: %w", err)
	}
	if data.Status != 200 || len(data.Data) == 0 {
		return nil, fmt.Errorf("blockdaemon: no quotes (status=%d msg=%s)", data.Status, data.Msg)
	}

	best := &data.Data[0]
	// AmountsOut[1] is the expected output in the destination token's smallest units.
	// Store it as-is so callers receive consistent base-unit amounts.
	amountOut := ""
	if len(best.AmountsOut) >= 2 {
		amountOut = best.AmountsOut[1]
	}
	if amountOut == "" {
		amountOut = "0"
	}

	return &BlockdaemonQuoteResult{
		BridgeID:         best.BridgeID,
		BridgeName:       best.BridgeName,
		AmountOut:        amountOut,
		EstimatedTimeSec: 120,
	}, nil
}

func shortHTTPErrorBlockdaemon(statusCode int, body []byte) string {
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
