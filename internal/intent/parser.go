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
	"net/url"
	"regexp"
	"sort"
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

type ProviderConfig struct {
	GeminiAPIKey  string
	GeminiModel   string
	OpenRouterKey string
}

const openRouterURL = "https://openrouter.ai/api/v1/chat/completions"
const openRouterModel = "meta-llama/llama-3.1-8b-instruct:free"
const defaultGeminiModel = "gemini-2.0-flash"

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

var amountRE = regexp.MustCompile(`\b(\d+(?:\.\d+)?)\b`)
var tokenAfterAmountRE = regexp.MustCompile(`\b\d+(?:\.\d+)?\s+([A-Za-z0-9]+)\b`)

var chainAliases = map[string]string{
	"eth":              "ethereum",
	"ethereum":         "ethereum",
	"base":             "base",
	"arb":              "arbitrum",
	"arbitrum":         "arbitrum",
	"op":               "optimism",
	"optimism":         "optimism",
	"poly":             "polygon",
	"polygon":          "polygon",
	"avax":             "avalanche",
	"avalanche":        "avalanche",
	"bnb":              "bsc",
	"bsc":              "bsc",
	"bnb chain":        "bsc",
	"sol":              "solana",
	"solana":           "solana",
	"sepolia":          "sepolia",
	"base sepolia":     "base-sepolia",
	"base-sepolia":     "base-sepolia",
	"arb sepolia":      "arbitrum-sepolia",
	"arbitrum sepolia": "arbitrum-sepolia",
	"arbitrum-sepolia": "arbitrum-sepolia",
	"op sepolia":       "op-sepolia",
	"optimism sepolia": "op-sepolia",
	"op-sepolia":       "op-sepolia",
}

type doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Parse extracts structured intent from text.
// Provider precedence: Gemini (if configured), then OpenRouter.
func Parse(ctx context.Context, cfg ProviderConfig, text string) (*ParseResult, error) {
	return parseWithClient(ctx, http.DefaultClient, cfg, text)
}

func parseWithClient(ctx context.Context, client doer, cfg ProviderConfig, text string) (*ParseResult, error) {
	normalizedInput, err := validateUserInput(text)
	if err != nil {
		return nil, err
	}
	heuristic := parseHeuristic(normalizedInput)
	if strings.TrimSpace(cfg.GeminiAPIKey) == "" && strings.TrimSpace(cfg.OpenRouterKey) == "" {
		if hasSignal(heuristic) {
			return heuristic, nil
		}
		return nil, errors.New("intent provider not configured")
	}

	if strings.TrimSpace(cfg.GeminiAPIKey) != "" {
		model := strings.TrimSpace(cfg.GeminiModel)
		if model == "" {
			model = defaultGeminiModel
		}
		res, err := parseWithGemini(ctx, client, cfg.GeminiAPIKey, model, normalizedInput)
		if err == nil {
			return mergeResults(res, heuristic), nil
		}
		log.Printf("gemini parse failed, falling back to openrouter: %s", truncate(err.Error(), 200))
	}
	if strings.TrimSpace(cfg.OpenRouterKey) == "" {
		if hasSignal(heuristic) {
			return heuristic, nil
		}
		return nil, fmt.Errorf("intent service unavailable")
	}
	res, err := parseWithOpenRouter(ctx, client, cfg.OpenRouterKey, normalizedInput)
	if err == nil {
		return mergeResults(res, heuristic), nil
	}
	if hasSignal(heuristic) {
		log.Printf("openrouter parse failed, falling back to heuristic parse: %s", truncate(err.Error(), 200))
		return heuristic, nil
	}
	return nil, err
}

func validateUserInput(text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", errors.New("empty input text")
	}
	if len(text) > maxIntentLen {
		return "", fmt.Errorf("input too long (max %d characters)", maxIntentLen)
	}
	lower := strings.ToLower(text)
	for _, pat := range blockedPatterns {
		if strings.Contains(lower, pat) {
			return "", errors.New("invalid input")
		}
	}
	return text, nil
}

func parseWithOpenRouter(ctx context.Context, client doer, apiKey, text string) (*ParseResult, error) {
	body, err := json.Marshal(map[string]any{
		"model": openRouterModel,
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

	resp, err := client.Do(req)
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

	content := completion.Choices[0].Message.Content
	result, err := parseResultJSON(content)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func parseWithGemini(ctx context.Context, client doer, apiKey, model, text string) (*ParseResult, error) {
	endpoint := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		url.PathEscape(model), url.QueryEscape(apiKey))
	body, err := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]string{
					{"text": systemPrompt + "\n\nUser input: " + text},
				},
			},
		},
		"generationConfig": map[string]any{
			"temperature":      0,
			"maxOutputTokens":  200,
			"responseMimeType": "application/json",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal gemini request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build gemini request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read gemini response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		log.Printf("gemini error (status %d): %s", resp.StatusCode, truncate(string(raw), 200))
		return nil, fmt.Errorf("gemini service unavailable")
	}

	var payload struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("parse gemini response: %w", err)
	}
	if len(payload.Candidates) == 0 || len(payload.Candidates[0].Content.Parts) == 0 {
		return nil, errors.New("gemini returned no candidates")
	}
	content := payload.Candidates[0].Content.Parts[0].Text
	return parseResultJSON(content)
}

func parseResultJSON(content string) (*ParseResult, error) {
	raw := strings.TrimSpace(content)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	first := strings.Index(raw, "{")
	last := strings.LastIndex(raw, "}")
	if first < 0 || last < first {
		return nil, fmt.Errorf("parse JSON from model output %q: missing object", truncate(raw, 100))
	}
	if first > 0 || last < len(raw)-1 {
		raw = raw[first : last+1]
	}

	var result ParseResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("parse JSON from model output %q: %w", truncate(raw, 100), err)
	}
	result.Amount = strings.TrimSpace(result.Amount)
	result.SrcToken = strings.ToUpper(strings.TrimSpace(result.SrcToken))
	result.DstToken = strings.ToUpper(strings.TrimSpace(result.DstToken))
	result.SrcChain = normalizeChain(strings.TrimSpace(result.SrcChain))
	result.DstChain = normalizeChain(strings.TrimSpace(result.DstChain))

	return &result, nil
}

func normalizeChain(chain string) string {
	if chain == "" {
		return ""
	}
	chain = strings.ToLower(chain)
	if canonical, ok := chainAliases[chain]; ok {
		return canonical
	}
	return chain
}

func mergeResults(primary, fallback *ParseResult) *ParseResult {
	if primary == nil {
		return fallback
	}
	if fallback == nil {
		return primary
	}
	out := *primary
	if strings.TrimSpace(out.Amount) == "" {
		out.Amount = fallback.Amount
	}
	if strings.TrimSpace(out.SrcToken) == "" {
		out.SrcToken = fallback.SrcToken
	}
	if strings.TrimSpace(out.DstToken) == "" {
		out.DstToken = fallback.DstToken
	}
	if strings.TrimSpace(out.SrcChain) == "" {
		out.SrcChain = fallback.SrcChain
	}
	if strings.TrimSpace(out.DstChain) == "" {
		out.DstChain = fallback.DstChain
	}
	return &out
}

func hasSignal(r *ParseResult) bool {
	if r == nil {
		return false
	}
	return r.Amount != "" || r.SrcToken != "" || r.DstToken != "" || r.SrcChain != "" || r.DstChain != ""
}

func parseHeuristic(text string) *ParseResult {
	lower := strings.ToLower(text)
	out := &ParseResult{}

	if m := amountRE.FindStringSubmatch(lower); len(m) > 1 {
		out.Amount = m[1]
	}
	if m := tokenAfterAmountRE.FindStringSubmatch(text); len(m) > 1 {
		out.SrcToken = strings.ToUpper(strings.TrimSpace(m[1]))
	}

	aliases := make([]string, 0, len(chainAliases))
	for k := range chainAliases {
		aliases = append(aliases, k)
	}
	sort.Slice(aliases, func(i, j int) bool { return len(aliases[i]) > len(aliases[j]) })

	findChainAfter := func(fromIdx int, marker string) string {
		searchStart := 0
		if fromIdx >= 0 {
			searchStart = fromIdx
		}
		idx := strings.Index(lower[searchStart:], marker)
		if idx < 0 {
			return ""
		}
		idx += searchStart
		rest := strings.TrimSpace(lower[idx+len(marker):])
		for _, a := range aliases {
			if strings.HasPrefix(rest, a) {
				return chainAliases[a]
			}
		}
		return ""
	}
	fromIdx := strings.Index(lower, "from ")
	out.SrcChain = findChainAfter(0, "from ")
	if fromIdx >= 0 {
		out.DstChain = findChainAfter(fromIdx+5, "to ")
	} else {
		out.DstChain = findChainAfter(0, "to ")
	}

	if out.SrcToken == "" && out.DstToken != "" {
		out.SrcToken = out.DstToken
	}
	if out.DstToken == "" && out.SrcToken != "" {
		out.DstToken = out.SrcToken
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
