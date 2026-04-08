package intent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// ParseResult is the structured intent extracted from natural language.
type ParseResult struct {
	Amount   string `json:"amount"`
	SrcToken string `json:"src_token"`
	DstToken string `json:"dst_token"`
	SrcChain string `json:"src_chain"`
	DstChain string `json:"dst_chain"`
}

const openRouterURL = "https://openrouter.ai/api/v1/chat/completions"
const model = "meta-llama/llama-3.1-8b-instruct:free"

const systemPrompt = `Extract bridge/swap intent from user text. Return ONLY valid JSON with no extra text or markdown:
{"amount":"<number or empty string>","src_token":"<TOKEN symbol or empty>","dst_token":"<TOKEN symbol or empty>","src_chain":"<chain-slug or empty>","dst_chain":"<chain-slug or empty>"}

Chain slugs: ethereum, base, arbitrum, optimism, polygon, solana, sepolia, base-sepolia, arbitrum-sepolia, op-sepolia
Token examples: ETH, USDC, USDT, DAI, WBTC, WETH, SOL, MATIC, BNB
Rules:
- amount: the source amount (number string, no currency symbol)
- src_token: token being sent
- dst_token: token to receive (may differ from src_token for swaps)
- If chain is only mentioned once with no from/to context, treat it as dst_chain
- Use "" for any field that cannot be determined`

const maxIntentLen = 500

// blockedPatterns contains substrings that indicate a prompt injection attempt.
var blockedPatterns = []string{"<|", "{{", "system:", "ignore previous", "ignore all instructions"}

// Parse calls OpenRouter with a free LLM model and extracts structured intent from text.
// Returns an error if the key is empty, the request fails, or the response cannot be parsed.
func Parse(ctx context.Context, apiKey, text string) (*ParseResult, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("openrouter key not configured")
	}
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("empty input text")
	}
	if len(text) > maxIntentLen {
		return nil, fmt.Errorf("input too long (max %d characters)", maxIntentLen)
	}
	lower := strings.ToLower(text)
	for _, pat := range blockedPatterns {
		if strings.Contains(lower, pat) {
			return nil, errors.New("invalid input")
		}
	}

	body, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": text},
		},
		"max_tokens":  200,
		"temperature": 0,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openRouterURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "bridge-aggregator")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		log.Printf("openrouter error (status %d): %s", resp.StatusCode, truncate(string(raw), 200))
		return nil, fmt.Errorf("intent service unavailable")
	}

	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &completion); err != nil {
		return nil, fmt.Errorf("parse completion: %w", err)
	}
	if len(completion.Choices) == 0 {
		return nil, errors.New("openrouter returned no choices")
	}

	content := strings.TrimSpace(completion.Choices[0].Message.Content)
	// Strip markdown code fences if the model wrapped the JSON.
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var result ParseResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("parse JSON from model output %q: %w", truncate(content, 100), err)
	}

	// Normalize token symbols to uppercase.
	result.SrcToken = strings.ToUpper(result.SrcToken)
	result.DstToken = strings.ToUpper(result.DstToken)
	// Normalize chain slugs to lowercase.
	result.SrcChain = strings.ToLower(result.SrcChain)
	result.DstChain = strings.ToLower(result.DstChain)

	return &result, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
