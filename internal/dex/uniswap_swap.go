package dex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"bridge-aggregator/internal/models"
)

type uniswapSwapRequest struct {
	Quote               json.RawMessage `json:"quote"`
	PermitData          json.RawMessage `json:"permitData,omitempty"`
	Signature           string          `json:"signature,omitempty"`
	RefreshGasPrice     bool            `json:"refreshGasPrice,omitempty"`
	SimulateTransaction bool            `json:"simulateTransaction,omitempty"`
}

type uniswapSwapResponse struct {
	Swap models.TransactionRequest `json:"swap"`
}

// CreateSwapTx calls Uniswap Trading API POST /swap to build an unsigned transaction.
// It requires the `quote` object returned from /quote. If permitData is non-null, a signature is required.
// Docs: https://api-docs.uniswap.org/api-reference/swapping/create_protocol_swap
func (a *UniswapTradingAdapter) CreateSwapTx(ctx context.Context, quote json.RawMessage, permitData json.RawMessage, signature string) (models.TransactionRequest, error) {
	if a.apiKey == "" {
		return models.TransactionRequest{}, fmt.Errorf("uniswap trading api key must be configured")
	}
	if len(quote) == 0 {
		return models.TransactionRequest{}, fmt.Errorf("quote is required")
	}

	reqBody := uniswapSwapRequest{
		Quote: quote,
	}
	if len(permitData) > 0 && string(permitData) != "null" {
		if signature == "" {
			return models.TransactionRequest{}, fmt.Errorf("signature required when permitData is present")
		}
		reqBody.PermitData = permitData
		reqBody.Signature = signature
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return models.TransactionRequest{}, fmt.Errorf("uniswap swap marshal: %w", err)
	}

	url := a.baseURL + "/swap"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return models.TransactionRequest{}, fmt.Errorf("uniswap swap request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("x-universal-router-version", "2.0")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return models.TransactionRequest{}, fmt.Errorf("uniswap swap http: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return models.TransactionRequest{}, fmt.Errorf("uniswap swap: status %d body=%s", resp.StatusCode, string(bodyBytes))
	}

	var sr uniswapSwapResponse
	if err := json.Unmarshal(bodyBytes, &sr); err != nil {
		return models.TransactionRequest{}, fmt.Errorf("uniswap swap decode: %w", err)
	}
	return sr.Swap, nil
}

