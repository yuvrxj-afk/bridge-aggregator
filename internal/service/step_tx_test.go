package service_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"bridge-aggregator/internal/dex"
	"bridge-aggregator/internal/models"
	"bridge-aggregator/internal/service"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func makeRoute(hops ...models.Hop) models.Route {
	return models.Route{
		RouteID:               "test-route",
		EstimatedOutputAmount: "5000000",
		EstimatedTimeSeconds:  300,
		TotalFee:              "0",
		Hops:                  hops,
	}
}

// cctpHop builds a CCTP bridge hop with the provider_data that cctp.go now stores.
func cctpHop() models.Hop {
	pd := mustMarshal(map[string]any{
		"source":                  "direct",
		"protocol":                "circle_cctp",
		"src_domain":              3,  // Arbitrum
		"dst_domain":              6,  // Base
		"token_messenger_src":     "0x19330d10D9Cc8751218eaf51E8885D058642E08A",
		"token_messenger_dst":     "0x1682Ae6375C4E4A97e4B583BC394c861A46D8962",
		"message_transmitter_dst": "0xAD09780d193884d503182aD4588450C416D6F9D4",
		"burn_token":              "0xaf88d065e77c8cC2239327C5EDb3A432268e5831",
		"amount":                  "5000000",
	})
	return models.Hop{
		HopType:          models.HopTypeBridge,
		BridgeID:         "cctp",
		FromChain:        "arbitrum",
		ToChain:          "base",
		FromAsset:        "USDC",
		ToAsset:          "USDC",
		FromTokenAddress: "0xaf88d065e77c8cC2239327C5EDb3A432268e5831",
		ToTokenAddress:   "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913",
		AmountInBaseUnits: "5000000",
		ProviderData:     pd,
	}
}

// acrossHop builds an Across v3 bridge hop with the deposit params the API returns.
func acrossHop() models.Hop {
	pd := mustMarshal(map[string]any{
		"source":   "direct",
		"protocol": "across_v3",
		"deposit": map[string]any{
			"depositor":           "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045",
			"recipient":           "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045",
			"inputToken":          "0xaf88d065e77c8cC2239327C5EDb3A432268e5831",
			"outputToken":         "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913",
			"inputAmount":         "5000000",
			"outputAmount":        "4993000",
			"destinationChainId":  8453,
			"exclusiveRelayer":    "0x0000000000000000000000000000000000000000",
			"quoteTimestamp":      1234567890,
			"fillDeadline":        1234568490,
			"exclusivityDeadline": 0,
			"message":             "0x",
			"spokePoolAddress":    "0xe35e9842fceaCA96570B734083f4a58e8F7C5f2A",
		},
	})
	return models.Hop{
		HopType:           models.HopTypeBridge,
		BridgeID:          "across",
		FromChain:         "arbitrum",
		ToChain:           "base",
		FromAsset:         "USDC",
		ToAsset:           "USDC",
		FromTokenAddress:  "0xaf88d065e77c8cC2239327C5EDb3A432268e5831",
		ToTokenAddress:    "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913",
		AmountInBaseUnits: "5000000",
		ProviderData:      pd,
	}
}

// canonicalBaseETHHop builds a canonical Base bridge hop (ETH deposit L1→L2).
func canonicalBaseETHHop() models.Hop {
	pd := mustMarshal(map[string]any{
		"source":        "direct",
		"protocol":      "canonical",
		"bridge":        "base",
		"l1_bridge":     "0x3154Cf16ccdb4C6d922629664174b904d80F2C35",
		"l2_bridge":     "0x4200000000000000000000000000000000000010",
		"deposit_on_l1": true,
		"amount":        "10000000000000000",
		"input_token":   "0x0000000000000000000000000000000000000000",
		"output_token":  "0x0000000000000000000000000000000000000000",
	})
	return models.Hop{
		HopType:           models.HopTypeBridge,
		BridgeID:          "canonical_base",
		FromChain:         "ethereum",
		ToChain:           "base",
		FromAsset:         "ETH",
		ToAsset:           "ETH",
		AmountInBaseUnits: "10000000000000000",
		ProviderData:      pd,
	}
}

// zeroexSwapHop builds a 0x swap hop with embedded transaction data.
func zeroexSwapHop() models.Hop {
	pd := mustMarshal(map[string]any{
		"quote": map[string]any{
			"transaction": map[string]any{
				"to":       "0x0000000000001ff3684f28c67538d4d072c22734",
				"data":     "0xdeadbeef1234abcd",
				"value":    "0",
				"gas":      "200000",
				"gasPrice": "1000000000",
			},
		},
		"permitData": nil,
		"routing":    "",
	})
	return models.Hop{
		HopType:           models.HopTypeSwap,
		BridgeID:          "zeroex",
		FromChain:         "base",
		ToChain:           "base",
		FromAsset:         "USDC",
		ToAsset:           "ETH",
		AmountInBaseUnits: "5000000",
		ProviderData:      pd,
	}
}

// mockZeroExAdapter implements dex.Adapter with ID "zeroex" (used for stepTransaction routing).
type mockZeroExAdapter struct{}

func (m *mockZeroExAdapter) ID() string { return "zeroex" }
func (m *mockZeroExAdapter) GetQuote(_ context.Context, _ dex.QuoteRequest) (*dex.Quote, error) {
	return &dex.Quote{DEXID: "zeroex", EstimatedOutputAmount: "2142959229232459"}, nil
}

// ── CCTP step transaction ─────────────────────────────────────────────────────

func TestPopulateStepTransaction_CCTP(t *testing.T) {
	route := makeRoute(cctpHop())
	req := models.StepTransactionRequest{Route: route, HopIndex: 0}

	resp, err := service.PopulateStepTransaction(context.Background(), nil, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.HopType != models.HopTypeBridge {
		t.Errorf("HopType = %q, want %q", resp.HopType, models.HopTypeBridge)
	}
	if resp.Tx != nil {
		t.Errorf("Tx should be nil for bridge hops; got %+v", resp.Tx)
	}
	bp := resp.BridgeParams
	if bp == nil {
		t.Fatal("BridgeParams is nil")
	}
	if bp.Protocol != "circle_cctp" {
		t.Errorf("Protocol = %q, want circle_cctp", bp.Protocol)
	}
	if len(bp.Steps) < 3 {
		t.Fatalf("expected ≥3 steps (approve, deposit, claim), got %d", len(bp.Steps))
	}

	// Step 0 — approve
	approve := bp.Steps[0]
	if approve.StepType != "approve" {
		t.Errorf("step[0].StepType = %q, want approve", approve.StepType)
	}
	if approve.Approval == nil {
		t.Fatal("step[0].Approval is nil")
	}
	if approve.Approval.Spender != "0x19330d10D9Cc8751218eaf51E8885D058642E08A" {
		t.Errorf("step[0].Approval.Spender = %q", approve.Approval.Spender)
	}
	if approve.Approval.Amount != "5000000" {
		t.Errorf("step[0].Approval.Amount = %q, want 5000000", approve.Approval.Amount)
	}

	// Step 1 — depositForBurn
	deposit := bp.Steps[1]
	if deposit.StepType != "deposit" {
		t.Errorf("step[1].StepType = %q, want deposit", deposit.StepType)
	}
	if deposit.Tx == nil {
		t.Fatal("step[1].Tx is nil")
	}
	if deposit.Tx.Function != "depositForBurn" {
		t.Errorf("step[1].Tx.Function = %q, want depositForBurn", deposit.Tx.Function)
	}
	if deposit.Tx.Contract != "0x19330d10D9Cc8751218eaf51E8885D058642E08A" {
		t.Errorf("step[1].Tx.Contract = %q", deposit.Tx.Contract)
	}
	// ABI fragment must be valid JSON
	var abiCheck []map[string]any
	if err := json.Unmarshal([]byte("["+deposit.Tx.ABIFragment+"]"), &abiCheck); err != nil {
		t.Errorf("ABIFragment is not valid JSON: %v — fragment: %s", err, deposit.Tx.ABIFragment)
	}

	// Step 2 — receiveMessage (destination chain claim)
	claim := bp.Steps[2]
	if claim.StepType != "claim" {
		t.Errorf("step[2].StepType = %q, want claim", claim.StepType)
	}
	if claim.Tx != nil && !strings.Contains(claim.Tx.Contract, "0x") {
		t.Errorf("claim contract doesn't look like an address: %q", claim.Tx.Contract)
	}

	// Notes should mention Iris attestation API
	if !strings.Contains(bp.Notes, "iris-api.circle.com") {
		t.Errorf("Notes should reference iris-api.circle.com, got: %q", bp.Notes)
	}
}

// ── Across v3 step transaction ────────────────────────────────────────────────

func TestPopulateStepTransaction_Across(t *testing.T) {
	route := makeRoute(acrossHop())
	req := models.StepTransactionRequest{Route: route, HopIndex: 0}

	resp, err := service.PopulateStepTransaction(context.Background(), nil, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bp := resp.BridgeParams
	if bp == nil {
		t.Fatal("BridgeParams is nil")
	}
	if bp.Protocol != "across_v3" {
		t.Errorf("Protocol = %q, want across_v3", bp.Protocol)
	}
	// USDC input → ERC-20 approval required, so 2 steps
	if len(bp.Steps) < 2 {
		t.Fatalf("expected ≥2 steps for ERC-20 Across deposit, got %d", len(bp.Steps))
	}

	approve := bp.Steps[0]
	if approve.StepType != "approve" {
		t.Errorf("step[0].StepType = %q, want approve", approve.StepType)
	}
	if approve.Approval.TokenContract != "0xaf88d065e77c8cC2239327C5EDb3A432268e5831" {
		t.Errorf("approval token = %q", approve.Approval.TokenContract)
	}
	if approve.Approval.Spender != "0xe35e9842fceaCA96570B734083f4a58e8F7C5f2A" {
		t.Errorf("approval spender (SpokePool) = %q", approve.Approval.Spender)
	}

	deposit := bp.Steps[1]
	if deposit.StepType != "deposit" {
		t.Errorf("step[1].StepType = %q, want deposit", deposit.StepType)
	}
	if deposit.Tx.Function != "depositV3" {
		t.Errorf("step[1].Tx.Function = %q, want depositV3", deposit.Tx.Function)
	}
	// SpokePool address must be the one from provider_data
	if deposit.Tx.Contract != "0xe35e9842fceaCA96570B734083f4a58e8F7C5f2A" {
		t.Errorf("SpokePool contract = %q", deposit.Tx.Contract)
	}
	// All 12 depositV3 params should be present
	requiredParams := []string{
		"depositor", "recipient", "inputToken", "outputToken",
		"inputAmount", "outputAmount", "destinationChainId", "exclusiveRelayer",
		"quoteTimestamp", "fillDeadline", "exclusivityDeadline", "message",
	}
	for _, p := range requiredParams {
		if _, ok := deposit.Tx.Params[p]; !ok {
			t.Errorf("depositV3 param %q missing from Params", p)
		}
	}
	// Value should be empty for ERC-20 (no ETH sent)
	if deposit.Tx.Value != "" {
		t.Errorf("Value should be empty for ERC-20 deposit, got %q", deposit.Tx.Value)
	}
}

func TestPopulateStepTransaction_AcrossETH(t *testing.T) {
	// For native ETH input, no approval step should be added.
	pd := mustMarshal(map[string]any{
		"source":   "direct",
		"protocol": "across_v3",
		"deposit": map[string]any{
			"depositor":           "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045",
			"recipient":           "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045",
			"inputToken":          "0x0000000000000000000000000000000000000000", // native ETH
			"outputToken":         "0x0000000000000000000000000000000000000000",
			"inputAmount":         "10000000000000000",
			"outputAmount":        "9993000000000000",
			"destinationChainId":  8453,
			"exclusiveRelayer":    "0x0000000000000000000000000000000000000000",
			"quoteTimestamp":      1234567890,
			"fillDeadline":        1234568490,
			"exclusivityDeadline": 0,
			"message":             "0x",
			"spokePoolAddress":    "0xe35e9842fceaCA96570B734083f4a58e8F7C5f2A",
		},
	})
	hop := models.Hop{
		HopType:          models.HopTypeBridge,
		BridgeID:         "across",
		FromChain:        "arbitrum",
		ToChain:          "base",
		ProviderData:     pd,
	}
	resp, err := service.PopulateStepTransaction(context.Background(), nil, models.StepTransactionRequest{
		Route: makeRoute(hop), HopIndex: 0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	bp := resp.BridgeParams
	if len(bp.Steps) != 1 {
		t.Fatalf("expected 1 step for ETH deposit (no approval), got %d", len(bp.Steps))
	}
	if bp.Steps[0].StepType != "deposit" {
		t.Errorf("step[0].StepType = %q, want deposit", bp.Steps[0].StepType)
	}
	// ETH deposit: value must equal inputAmount
	if bp.Steps[0].Tx.Value != "10000000000000000" {
		t.Errorf("Value = %q, want 10000000000000000", bp.Steps[0].Tx.Value)
	}
}

// ── Canonical Base bridge ─────────────────────────────────────────────────────

func TestPopulateStepTransaction_CanonicalBase_ETH(t *testing.T) {
	route := makeRoute(canonicalBaseETHHop())
	resp, err := service.PopulateStepTransaction(context.Background(), nil, models.StepTransactionRequest{
		Route: route, HopIndex: 0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	bp := resp.BridgeParams
	if bp == nil {
		t.Fatal("BridgeParams is nil")
	}
	if bp.Protocol != "canonical_base" {
		t.Errorf("Protocol = %q, want canonical_base", bp.Protocol)
	}
	if len(bp.Steps) != 1 {
		t.Fatalf("ETH deposit: expected 1 step, got %d", len(bp.Steps))
	}
	step := bp.Steps[0]
	if step.StepType != "deposit" {
		t.Errorf("StepType = %q, want deposit", step.StepType)
	}
	if step.Tx.Function != "depositETH" {
		t.Errorf("Function = %q, want depositETH", step.Tx.Function)
	}
	if step.Tx.Contract != "0x3154Cf16ccdb4C6d922629664174b904d80F2C35" {
		t.Errorf("Contract = %q (not the official Base L1StandardBridge)", step.Tx.Contract)
	}
	// Payable call: ETH value must be set
	if step.Tx.Value != "10000000000000000" {
		t.Errorf("Value = %q, want 10000000000000000", step.Tx.Value)
	}
}

// ── 0x swap step transaction ──────────────────────────────────────────────────

func TestPopulateStepTransaction_ZeroEx(t *testing.T) {
	route := makeRoute(zeroexSwapHop())
	req := models.StepTransactionRequest{Route: route, HopIndex: 0}

	resp, err := service.PopulateStepTransaction(context.Background(), []dex.Adapter{&mockZeroExAdapter{}}, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.HopType != models.HopTypeSwap {
		t.Errorf("HopType = %q, want swap", resp.HopType)
	}
	if resp.BridgeParams != nil {
		t.Error("BridgeParams should be nil for swap hops")
	}
	tx := resp.Tx
	if tx == nil {
		t.Fatal("Tx is nil for 0x swap hop")
	}
	if tx.To != "0x0000000000001ff3684f28c67538d4d072c22734" {
		t.Errorf("Tx.To = %q", tx.To)
	}
	if tx.Data != "0xdeadbeef1234abcd" {
		t.Errorf("Tx.Data = %q", tx.Data)
	}
	if tx.Value != "0" {
		t.Errorf("Tx.Value = %q", tx.Value)
	}
}

// ── Error cases ───────────────────────────────────────────────────────────────

func TestPopulateStepTransaction_HopIndexOutOfRange(t *testing.T) {
	route := makeRoute(cctpHop())

	_, err := service.PopulateStepTransaction(context.Background(), nil, models.StepTransactionRequest{
		Route: route, HopIndex: 5, // only 1 hop
	})
	if err != service.ErrHopIndexOutOfRange {
		t.Errorf("expected ErrHopIndexOutOfRange, got %v", err)
	}

	_, err = service.PopulateStepTransaction(context.Background(), nil, models.StepTransactionRequest{
		Route: route, HopIndex: -1,
	})
	if err != service.ErrHopIndexOutOfRange {
		t.Errorf("expected ErrHopIndexOutOfRange for -1, got %v", err)
	}
}

func TestPopulateStepTransaction_BridgeHopNoProviderData(t *testing.T) {
	hop := models.Hop{
		HopType:  models.HopTypeBridge,
		BridgeID: "cctp",
		// No ProviderData
	}
	route := makeRoute(hop)
	_, err := service.PopulateStepTransaction(context.Background(), nil, models.StepTransactionRequest{
		Route: route, HopIndex: 0,
	})
	if err == nil {
		t.Error("expected error for bridge hop without provider_data, got nil")
	}
}

func TestPopulateStepTransaction_UnknownProtocol(t *testing.T) {
	pd := mustMarshal(map[string]any{
		"source":   "direct",
		"protocol": "unsupported_bridge_xyz",
	})
	hop := models.Hop{
		HopType:      models.HopTypeBridge,
		BridgeID:     "xyz",
		ProviderData: pd,
	}
	route := makeRoute(hop)
	_, err := service.PopulateStepTransaction(context.Background(), nil, models.StepTransactionRequest{
		Route: route, HopIndex: 0,
	})
	if err == nil {
		t.Error("expected error for unsupported bridge protocol, got nil")
	}
}

func TestPopulateStepTransaction_SwapHopNoAdapter(t *testing.T) {
	route := makeRoute(zeroexSwapHop())
	// Pass empty adapter list — should fail with ErrHopNotSupported
	_, err := service.PopulateStepTransaction(context.Background(), []dex.Adapter{}, models.StepTransactionRequest{
		Route: route, HopIndex: 0,
	})
	if err != service.ErrHopNotSupported {
		t.Errorf("expected ErrHopNotSupported when no adapter matches, got %v", err)
	}
}
