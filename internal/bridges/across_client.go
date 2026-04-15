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

// AcrossSwapTx is a pre-built unsigned transaction returned by the Across API.
// For "anyToBridgeable" routes (cross-token), Across returns a swapTx that atomically
// does an origin swap + SpokePool deposit in their own UniversalSwapAndBridge contract.
type AcrossSwapTx struct {
	ChainID int    `json:"chainId"`
	To      string `json:"to"`
	Data    string `json:"data"`
	Gas     string `json:"gas,omitempty"`
}

// AcrossQuoteResponse is the full Across GET /swap/approval response.
// Two route shapes exist, distinguished by CrossSwapType:
//
//   - "bridgeable": same-token direct bridge. Returns Deposit params.
//     Execution: depositV3() on SpokePool (or via LiFi Diamond).
//
//   - "anyToBridgeable": cross-token swap+bridge. Across does an origin swap first.
//     Returns SwapTx (ready-to-send calldata) + ApprovalTxns.
//     Execution: approve token → send SwapTx to Across UniversalSwapAndBridge.
type AcrossQuoteResponse struct {
	CrossSwapType        string `json:"crossSwapType"` // "bridgeable" | "anyToBridgeable"
	ExpectedOutputAmount string `json:"expectedOutputAmount"`
	MinOutputAmount      string `json:"minOutputAmount"`
	ExpectedFillTime     int    `json:"expectedFillTime"`
	InputAmount          string `json:"inputAmount"`
	IsAmountTooLow       bool   `json:"isAmountTooLow"`
	Fees                 *struct {
		Total *struct {
			Amount string `json:"amount"`
			Token  *struct {
				Decimals int `json:"decimals"`
			} `json:"token"`
		} `json:"total"`
	} `json:"fees"`
	// Deposit is present for "bridgeable" routes: decoded SpokePool.depositV3() params.
	Deposit *AcrossDepositParams `json:"deposit,omitempty"`
	// SwapTx is present for "anyToBridgeable" routes: complete pre-built calldata.
	SwapTx *AcrossSwapTx `json:"swapTx,omitempty"`
	// ApprovalTxns lists required ERC-20 approvals to send before SwapTx.
	ApprovalTxns []AcrossSwapTx `json:"approvalTxns,omitempty"`
}

// AcrossDepositParams are the params for SpokePool.depositV3() returned by the Across API.
// These map directly to the function signature on-chain.
type AcrossDepositParams struct {
	Depositor           string `json:"depositor"`
	Recipient           string `json:"recipient"`
	InputToken          string `json:"inputToken"`
	OutputToken         string `json:"outputToken"`
	InputAmount         string `json:"inputAmount"`
	OutputAmount        string `json:"outputAmount"`
	DestinationChainID  int64  `json:"destinationChainId"`
	ExclusiveRelayer    string `json:"exclusiveRelayer"`
	QuoteTimestamp      int64  `json:"quoteTimestamp"`
	FillDeadline        int64  `json:"fillDeadline"`
	ExclusivityDeadline int64  `json:"exclusivityDeadline"`
	Message             string `json:"message"`
	SpokePoolAddress    string `json:"spokePoolAddress"`
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
			Timeout: 5 * time.Second,
		},
		APIKey:    apiKey,
		Depositor: depositor,
	}
}

// QuoteResult holds the parsed quote for our aggregator.
type QuoteResult struct {
	ExpectedOutputAmount string // base units (smallest denomination of the output token)
	ExpectedFillTimeSec  int64
	TotalFeeAmount       string               // human-readable (fee in fee-token units, used for scoring)
	InputAmount          string               // human-readable (echo)
	CrossSwapType        string               // "bridgeable" | "anyToBridgeable"
	Deposit              *AcrossDepositParams // params for SpokePool.depositV3() — bridgeable only
	SwapTx               *AcrossSwapTx        // pre-built execution tx — anyToBridgeable only
	ApprovalTxns         []AcrossSwapTx       // required ERC-20 approvals before SwapTx
}

// GetQuote calls Across GET /swap/approval and returns parsed quote.
// Amount is in smallest units (e.g. 100 USDC = 100000000).
func (c *AcrossClient) GetQuote(ctx context.Context, originChainID, destChainID int64, inputToken, outputToken, amountSmallestUnits, depositor string) (*QuoteResult, error) {
	u, err := url.Parse(c.BaseURL + "/swap/approval")
	if err != nil {
		return nil, err
	}
	// Across API parameter validation is strict; normalize addresses to lowercase hex.
	inputToken = strings.ToLower(strings.TrimSpace(inputToken))
	outputToken = strings.ToLower(strings.TrimSpace(outputToken))
	depositor = strings.ToLower(strings.TrimSpace(depositor))

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

	if data.IsAmountTooLow {
		return nil, fmt.Errorf("across: amount too low for this token pair (minimum not met)")
	}

	decimals := 6
	if data.Fees != nil && data.Fees.Total != nil && data.Fees.Total.Token != nil {
		decimals = data.Fees.Total.Token.Decimals
	}

	result := &QuoteResult{
		ExpectedFillTimeSec: int64(data.ExpectedFillTime),
		CrossSwapType:       data.CrossSwapType,
		Deposit:             data.Deposit,
		SwapTx:              data.SwapTx,
		ApprovalTxns:        data.ApprovalTxns,
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

// suggestedFeesResponse is the response from Across GET /suggested-fees.
// This endpoint is purpose-built for direct SpokePool.depositV3() deposits and always
// returns fee parameters — unlike /swap/approval which returns no deposit params for
// "bridgeableToBridgeable" routes (same symbol, different chain addresses like USDC Arb→Polygon).
//
// Note: timestamp and fillDeadline are returned as decimal strings by the API.
type suggestedFeesResponse struct {
	EstimatedFillTimeSec int    `json:"estimatedFillTimeSec"`
	OutputAmount         string `json:"outputAmount"`
	ExclusiveRelayer     string `json:"exclusiveRelayer"`
	Timestamp            string `json:"timestamp"`    // decimal string, used as quoteTimestamp
	FillDeadline         string `json:"fillDeadline"` // decimal string
	ExclusivityDeadline  int64  `json:"exclusivityDeadline"`
	SpokePoolAddress     string `json:"spokePoolAddress"`
	IsAmountTooLow       bool   `json:"isAmountTooLow"`
}

// FetchDeposit fetches fresh SpokePool.depositV3() parameters for the given token pair
// using the Across /suggested-fees endpoint. This endpoint is reliable for all token pairs
// that Across supports — unlike /swap/approval which omits deposit params for
// "bridgeableToBridgeable" routes (e.g. USDC on Arbitrum → USDC on Polygon).
func (c *AcrossClient) FetchDeposit(
	ctx context.Context,
	originChainID, destChainID int64,
	inputToken, outputToken, amountSmallestUnits, walletAddress string,
) (*AcrossDepositParams, error) {
	u, err := url.Parse(c.BaseURL + "/suggested-fees")
	if err != nil {
		return nil, err
	}
	// Normalize addresses for strict API validation.
	inputToken = strings.ToLower(strings.TrimSpace(inputToken))
	outputToken = strings.ToLower(strings.TrimSpace(outputToken))
	walletAddress = strings.ToLower(strings.TrimSpace(walletAddress))

	q := u.Query()
	q.Set("inputToken", inputToken)
	q.Set("outputToken", outputToken)
	q.Set("originChainId", strconv.FormatInt(originChainID, 10))
	q.Set("destinationChainId", strconv.FormatInt(destChainID, 10))
	q.Set("amount", amountSmallestUnits)
	q.Set("depositor", walletAddress)
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
		return nil, fmt.Errorf("across /suggested-fees: %d %s", resp.StatusCode, string(body))
	}

	var fees suggestedFeesResponse
	if err := json.Unmarshal(body, &fees); err != nil {
		return nil, fmt.Errorf("across suggested-fees decode: %w", err)
	}
	if fees.IsAmountTooLow {
		return nil, fmt.Errorf("across: amount too low to bridge this token pair")
	}
	if fees.OutputAmount == "" || fees.OutputAmount == "0" {
		return nil, fmt.Errorf("across: suggested-fees returned zero output amount — pair may not be supported")
	}

	// Parse the decimal-string timestamps returned by the API.
	quoteTimestamp, _ := strconv.ParseInt(fees.Timestamp, 10, 64)
	fillDeadline, _ := strconv.ParseInt(fees.FillDeadline, 10, 64)

	return &AcrossDepositParams{
		Depositor:           walletAddress,
		Recipient:           walletAddress,
		InputToken:          inputToken,
		OutputToken:         outputToken,
		InputAmount:         amountSmallestUnits,
		OutputAmount:        fees.OutputAmount,
		DestinationChainID:  destChainID,
		ExclusiveRelayer:    fees.ExclusiveRelayer,
		QuoteTimestamp:      quoteTimestamp,
		FillDeadline:        fillDeadline,
		ExclusivityDeadline: fees.ExclusivityDeadline,
		Message:             "",
		SpokePoolAddress:    fees.SpokePoolAddress,
	}, nil
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
