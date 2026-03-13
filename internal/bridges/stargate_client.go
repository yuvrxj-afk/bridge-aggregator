package bridges

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// LayerZero Value Transfer API (replaces deprecated Stargate API).
// Docs: https://transfer.layerzero-api.com/v1/docs
const defaultLayerZeroVTAPIURL = "https://transfer.layerzero-api.com/v1"

// shortHTTPError returns a one-line summary for 4xx/5xx responses (no full body in logs).
func shortHTTPError(statusCode int, body []byte) string {
	status := http.StatusText(statusCode)
	if status == "" {
		status = "error"
	}
	msg := fmt.Sprintf("%d %s", statusCode, status)
	var v struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &v) == nil && v.Message != "" {
		s := v.Message
		if len(s) > 80 {
			s = s[:77] + "..."
		}
		msg = msg + ": " + s
	}
	return msg
}

// StargateClient calls the LayerZero Value Transfer API for bridge quotes.
// Uses POST /v1/quotes (see https://transfer.layerzero-api.com/v1/docs).
// APIKey is required for quotes; set via config (stargate_api_key) or 401 will be returned.
type StargateClient struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// NewStargateClient returns a client for the LayerZero VT API. BaseURL defaults to production if empty.
// apiKey is sent as Authorization Bearer when non-empty; required for successful quotes.
func NewStargateClient(baseURL string, apiKey string) *StargateClient {
	if baseURL == "" {
		baseURL = defaultLayerZeroVTAPIURL
	}
	return &StargateClient{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// layerZeroQuoteRequest is the POST /quotes body.
type layerZeroQuoteRequest struct {
	SrcChainKey      string `json:"srcChainKey"`
	DstChainKey      string `json:"dstChainKey"`
	SrcTokenAddress  string `json:"srcTokenAddress"`
	DstTokenAddress  string `json:"dstTokenAddress"`
	SrcWalletAddress string `json:"srcWalletAddress"`
	DstWalletAddress string `json:"dstWalletAddress"`
	Amount           string `json:"amount"`
}

// layerZeroQuoteResponse is the POST /quotes response.
type layerZeroQuoteResponse struct {
	Error  *struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
		Issues  []struct {
			Message string `json:"message"`
		} `json:"issues"`
	} `json:"error"`
	Quotes []struct {
		ID          string `json:"id"`
		SrcAmount   string `json:"srcAmount"`
		DstAmount   string `json:"dstAmount"`
		DstAmountMin string `json:"dstAmountMin"`
		FeeUsd      string `json:"feeUsd"`
		FeePercent  string `json:"feePercent"`
		Duration    *struct {
			Estimated string `json:"estimated"` // milliseconds
		} `json:"duration"`
		Fees []struct {
			ChainKey   string `json:"chainKey"`
			Type       string `json:"type"`
			Amount     string `json:"amount"`
			Address    string `json:"address"`
		} `json:"fees"`
	} `json:"quotes"`
}

// StargateQuoteResult holds the parsed quote for our aggregator.
type StargateQuoteResult struct {
	DstAmount        string
	TotalFeeAmount   string
	EstimatedTimeSec int64
}

// GetQuote calls LayerZero VT API POST /v1/quotes.
// amountSmallestUnits, srcToken, dstToken are contract addresses and amount in smallest units.
// srcChainKey, dstChainKey are e.g. ethereum, arbitrum, optimism, polygon.
func (c *StargateClient) GetQuote(ctx context.Context, amountSmallestUnits, srcToken, dstToken, srcChainKey, dstChainKey, srcAddress, dstAddress string) (*StargateQuoteResult, error) {
	body := layerZeroQuoteRequest{
		SrcChainKey:      srcChainKey,
		DstChainKey:      dstChainKey,
		SrcTokenAddress:  srcToken,
		DstTokenAddress:  dstToken,
		SrcWalletAddress: srcAddress,
		DstWalletAddress: dstAddress,
		Amount:           amountSmallestUnits,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("stargate request: %w", err)
	}

	u := c.BaseURL + "/quotes"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		msg := shortHTTPError(resp.StatusCode, respBody)
		return nil, fmt.Errorf("stargate: %s", msg)
	}

	var data layerZeroQuoteResponse
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, fmt.Errorf("stargate response decode: %w", err)
	}

	if data.Error != nil {
		msg := data.Error.Message
		if msg == "" {
			msg = "quote failed"
		}
		if len(data.Error.Issues) > 0 {
			msg = msg + ": " + data.Error.Issues[0].Message
		}
		return nil, fmt.Errorf("stargate: %s", msg)
	}

	if len(data.Quotes) == 0 {
		return nil, fmt.Errorf("stargate: no quotes returned")
	}

	q0 := &data.Quotes[0]
	decimals := 6
	if strings.EqualFold(dstToken, "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE") {
		decimals = 18
	}

	result := &StargateQuoteResult{}
	result.DstAmount, _ = toHumanAmount(q0.DstAmount, decimals)
	result.TotalFeeAmount = q0.FeeUsd
	if result.TotalFeeAmount == "" {
		result.TotalFeeAmount = "0"
		for _, f := range q0.Fees {
			if f.Amount != "" {
				feeHuman, _ := toHumanAmount(f.Amount, decimals)
				result.TotalFeeAmount = feeHuman
				break
			}
		}
	}

	if q0.Duration != nil && q0.Duration.Estimated != "" {
		ms, _ := strconv.ParseInt(q0.Duration.Estimated, 10, 64)
		result.EstimatedTimeSec = ms / 1000
	}
	if result.EstimatedTimeSec <= 0 {
		result.EstimatedTimeSec = 120
	}

	return result, nil
}
