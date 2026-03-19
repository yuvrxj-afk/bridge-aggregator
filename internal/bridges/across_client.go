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

	"bridge-aggregator/internal/ethutil"
)

const defaultAcrossAPIURL = "https://app.across.to/api"

// AcrossQuoteResponse is the relevant part of Across GET /swap/approval response.
type AcrossQuoteResponse struct {
	ExpectedOutputAmount string `json:"expectedOutputAmount"`
	MinOutputAmount      string `json:"minOutputAmount"`
	ExpectedFillTime     int    `json:"expectedFillTime"`
	InputAmount          string `json:"inputAmount"`
	Fees                 *struct {
		Total *struct {
			Amount string `json:"amount"`
			Token  *struct {
				Decimals int `json:"decimals"`
			} `json:"token"`
		} `json:"total"`
	} `json:"fees"`
	// Deposit holds the parameters for the SpokePool.depositV3() on-chain call.
	Deposit *AcrossDepositParams `json:"deposit,omitempty"`
}

// AcrossDepositParams are the params for SpokePool.depositV3() returned by the Across API.
// These map directly to the function signature on-chain.
type AcrossDepositParams struct {
	Depositor            string `json:"depositor"`
	Recipient            string `json:"recipient"`
	InputToken           string `json:"inputToken"`
	OutputToken          string `json:"outputToken"`
	InputAmount          string `json:"inputAmount"`
	OutputAmount         string `json:"outputAmount"`
	DestinationChainID   int64  `json:"destinationChainId"`
	ExclusiveRelayer     string `json:"exclusiveRelayer"`
	QuoteTimestamp       int64  `json:"quoteTimestamp"`
	FillDeadline         int64  `json:"fillDeadline"`
	ExclusivityDeadline  int64  `json:"exclusivityDeadline"`
	Message              string `json:"message"`
	SpokePoolAddress     string `json:"spokePoolAddress"`
}

// AcrossClient calls Across Swap API for quotes.
type AcrossClient struct {
	BaseURL    string
	HTTPClient *http.Client
	APIKey     string
	// Depositor is the default wallet address used for the depositor parameter.
	// Required by Across to check on-chain allowances/balances. Set via ACROSS_DEPOSITOR env var.
	Depositor string
}

// NewAcrossClient returns an Across client. BaseURL defaults to production if empty.
func NewAcrossClient(baseURL, apiKey, depositor string) *AcrossClient {
	if baseURL == "" {
		baseURL = defaultAcrossAPIURL
	}
	return &AcrossClient{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		APIKey:    apiKey,
		Depositor: depositor,
	}
}

// QuoteResult holds the parsed quote for our aggregator.
type QuoteResult struct {
	ExpectedOutputAmount string               // base units (smallest denomination of the output token)
	ExpectedFillTimeSec  int64
	TotalFeeAmount       string               // human-readable (fee in fee-token units, used for scoring)
	InputAmount          string               // human-readable (echo)
	Deposit              *AcrossDepositParams // params for the on-chain depositV3() call, if available
}

// GetQuote calls Across GET /swap/approval and returns parsed quote.
// Amount is in smallest units (e.g. 100 USDC = 100000000).
func (c *AcrossClient) GetQuote(ctx context.Context, originChainID, destChainID int64, inputToken, outputToken, amountSmallestUnits, depositor string) (*QuoteResult, error) {
	u, err := url.Parse(c.BaseURL + "/swap/approval")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("tradeType", "exactInput")
	q.Set("amount", amountSmallestUnits)
	q.Set("inputToken", inputToken)
	q.Set("outputToken", outputToken)
	q.Set("originChainId", strconv.FormatInt(originChainID, 10))
	q.Set("destinationChainId", strconv.FormatInt(destChainID, 10))
	q.Set("depositor", depositor)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	req.Header.Set("Accept", "application/json")

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
		return nil, fmt.Errorf("across api %s: %d %s", u.String(), resp.StatusCode, string(body))
	}

	var data AcrossQuoteResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("across response decode: %w", err)
	}

	decimals := 6
	if data.Fees != nil && data.Fees.Total != nil && data.Fees.Total.Token != nil {
		decimals = data.Fees.Total.Token.Decimals
	}

	result := &QuoteResult{
		ExpectedFillTimeSec: int64(data.ExpectedFillTime),
		Deposit:             data.Deposit,
	}
	// Keep output amount in raw base units — the output token may differ from the fee token
	// (e.g. USDC→ETH anyToAny: output is ETH wei, fee token is USDC with 6 decimals).
	// Using fee-token decimals to convert would produce a wildly wrong human number.
	result.ExpectedOutputAmount = data.ExpectedOutputAmount
	if result.ExpectedOutputAmount == "" {
		result.ExpectedOutputAmount = "0"
	}
	result.TotalFeeAmount = "0"
	if data.Fees != nil && data.Fees.Total != nil && data.Fees.Total.Amount != "" {
		result.TotalFeeAmount, _ = toHumanAmount(data.Fees.Total.Amount, decimals)
	}
	result.InputAmount, _ = toHumanAmount(data.InputAmount, decimals)
	return result, nil
}

// toHumanAmount is a package-level alias for ethutil.FormatUnits.
// Kept for backward compatibility with callers inside the bridges package.
func toHumanAmount(baseUnits string, decimals int) (string, error) {
	return ethutil.FormatUnits(baseUnits, decimals)
}

// HumanToSmallest is a package-level alias for ethutil.ParseUnitsString.
// Kept for backward compatibility with callers inside the bridges package.
func HumanToSmallest(human string, decimals int) (string, error) {
	return ethutil.ParseUnitsString(human, decimals)
}
