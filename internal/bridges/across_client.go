package bridges

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultAcrossAPIURL = "https://app.across.to/api"

// AcrossQuoteResponse is the relevant part of Across GET /swap/approval response.
type AcrossQuoteResponse struct {
	ExpectedOutputAmount string `json:"expectedOutputAmount"`
	MinOutputAmount     string `json:"minOutputAmount"`
	ExpectedFillTime    int    `json:"expectedFillTime"`
	InputAmount         string `json:"inputAmount"`
	Fees                *struct {
		Total *struct {
			Amount string `json:"amount"`
			Token  *struct {
				Decimals int `json:"decimals"`
			} `json:"token"`
		} `json:"total"`
	} `json:"fees"`
}

// AcrossClient calls Across Swap API for quotes.
type AcrossClient struct {
	BaseURL    string
	HTTPClient *http.Client
	APIKey     string
}

// NewAcrossClient returns an Across client. BaseURL defaults to production if empty.
func NewAcrossClient(baseURL string, apiKey string) *AcrossClient {
	if baseURL == "" {
		baseURL = defaultAcrossAPIURL
	}
	return &AcrossClient{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		APIKey: apiKey,
	}
}

// QuoteResult holds the parsed quote for our aggregator.
type QuoteResult struct {
	ExpectedOutputAmount string // human-readable
	ExpectedFillTimeSec  int64
	TotalFeeAmount       string // human-readable
	InputAmount          string // human-readable (echo)
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
	}
	result.ExpectedOutputAmount, _ = toHumanAmount(data.ExpectedOutputAmount, decimals)
	result.TotalFeeAmount = "0"
	if data.Fees != nil && data.Fees.Total != nil && data.Fees.Total.Amount != "" {
		result.TotalFeeAmount, _ = toHumanAmount(data.Fees.Total.Amount, decimals)
	}
	result.InputAmount, _ = toHumanAmount(data.InputAmount, decimals)
	return result, nil
}

func toHumanAmount(smallest string, decimals int) (string, error) {
	if smallest == "" {
		return "0", nil
	}
	n, err := strconv.ParseInt(smallest, 10, 64)
	if err != nil {
		return "", err
	}
	if decimals <= 0 {
		return strconv.FormatInt(n, 10), nil
	}
	div := int64(1)
	for i := 0; i < decimals; i++ {
		div *= 10
	}
	whole := n / div
	frac := n % div
	if frac < 0 {
		frac = -frac
	}
	if frac == 0 {
		return strconv.FormatInt(whole, 10), nil
	}
	fracStr := strconv.FormatInt(frac, 10)
	for len(fracStr) < decimals {
		fracStr = "0" + fracStr
	}
	fracStr = strings.TrimRight(fracStr, "0")
	return strconv.FormatInt(whole, 10) + "." + fracStr, nil
}

// HumanToSmallest converts a human amount string to smallest units (e.g. "100" + 6 decimals -> "100000000").
func HumanToSmallest(human string, decimals int) (string, error) {
	if human == "" || decimals < 0 {
		return "0", nil
	}
	// Use big.Rat to avoid float rounding issues.
	r, ok := new(big.Rat).SetString(human)
	if !ok {
		return "", fmt.Errorf("invalid decimal amount: %q", human)
	}

	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	r.Mul(r, new(big.Rat).SetInt(scale))

	// Round half up to nearest integer.
	num := r.Num()
	den := r.Denom()
	quo, rem := new(big.Int).QuoRem(num, den, new(big.Int))
	if rem.Sign() != 0 {
		// compare 2*rem with den for half-up rounding
		twoRem := new(big.Int).Mul(rem, big.NewInt(2))
		if twoRem.Cmp(den) >= 0 {
			quo.Add(quo, big.NewInt(1))
		}
	}
	if quo.Sign() < 0 {
		return "", fmt.Errorf("amount must be non-negative")
	}
	return quo.String(), nil
}
