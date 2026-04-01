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
			Timeout: 5 * time.Second,
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

// TransactionStatus represents the status of a cross-chain transaction.
type TransactionStatus struct {
	TxHash      string `json:"txHash"`
	Status      string `json:"status"`           // "pending", "completed", "failed"
	BridgeUsed  string `json:"bridgeUsed"`
	Progress    int    `json:"progress"`         // 0-100
	Timestamp   int64  `json:"timestamp"`
	Message     string `json:"message,omitempty"`
}

// TransactionStatusResponse is the GET /defi/v1/transaction/status/{txHash} response.
type TransactionStatusResponse struct {
	Status int    `json:"status"`
	Msg    string `json:"msg"`
	Data   struct {
		TxHash      string `json:"txHash"`
		Status      string `json:"status"`
		BridgeName  string `json:"bridgeName"`
		Progress    int    `json:"progress"`
		Timestamp   int64  `json:"timestamp"`
	} `json:"data"`
}

// GetTransactionStatus calls GET /defi/v1/transaction/status/{txHash}.
// Returns the status of a cross-chain transaction for tracking purposes.
func (c *BlockdaemonClient) GetTransactionStatus(ctx context.Context, txHash string) (*TransactionStatus, error) {
	if txHash == "" {
		return nil, fmt.Errorf("blockdaemon: txHash is required")
	}

	u := fmt.Sprintf("%s/defi/v1/transaction/status/%s", c.BaseURL, txHash)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
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

	var data TransactionStatusResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("blockdaemon status response decode: %w", err)
	}
	if data.Status != 200 {
		return nil, fmt.Errorf("blockdaemon: status check failed (status=%d msg=%s)", data.Status, data.Msg)
	}

	return &TransactionStatus{
		TxHash:     data.Data.TxHash,
		Status:     data.Data.Status,
		BridgeUsed: data.Data.BridgeName,
		Progress:   data.Data.Progress,
		Timestamp:  data.Data.Timestamp,
		Message:    data.Msg,
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
