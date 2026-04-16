package router

import (
	"context"
	"testing"

	"bridge-aggregator/internal/bridges"
	"bridge-aggregator/internal/dex"
	"bridge-aggregator/internal/models"
)

type dexTokenInAssertingAdapter struct {
	wantTokenIn string
	quoteOut    string
}

func (a *dexTokenInAssertingAdapter) ID() string               { return "dex_test" }
func (a *dexTokenInAssertingAdapter) Tier() models.AdapterTier { return models.TierProduction }
func (a *dexTokenInAssertingAdapter) GetQuote(_ context.Context, req dex.QuoteRequest) (*dex.Quote, error) {
	if a.wantTokenIn != "" && req.TokenIn != a.wantTokenIn {
		return nil, &testErr{msg: "TokenIn mismatch: got " + req.TokenIn + " want " + a.wantTokenIn}
	}
	return &dex.Quote{
		DEXID:                 a.ID(),
		EstimatedOutputAmount: a.quoteOut,
		EstimatedFeeAmount:    "0.0",
	}, nil
}

type bridgePassthroughAdapter struct {
	id string
}

func (a *bridgePassthroughAdapter) ID() string               { return a.id }
func (a *bridgePassthroughAdapter) Tier() models.AdapterTier { return models.TierProduction }
func (a *bridgePassthroughAdapter) GetQuote(_ context.Context, req models.QuoteRequest) (*models.Route, error) {
	return &models.Route{
		RouteID:               a.id,
		EstimatedOutputAmount: req.AmountBaseUnits,
		EstimatedTimeSeconds:  60,
		TotalFee:              "0.0",
		Hops: []models.Hop{
			{
				HopType:           models.HopTypeBridge,
				BridgeID:          a.id,
				FromChain:         req.Source.Chain,
				ToChain:           req.Destination.Chain,
				FromAsset:         req.Source.Asset,
				ToAsset:           req.Destination.Asset,
				AmountInBaseUnits: req.AmountBaseUnits,
				ProviderData:      []byte(`{"protocol":"circle_cctp","token_messenger_src":"0x1","burn_token":"0x2"}`),
			},
		},
	}, nil
}

type testErr struct{ msg string }

func (e *testErr) Error() string { return e.msg }

func TestQuoteSwapThenBridge_NormalizesNativeETHToWETHForDEX(t *testing.T) {
	bridges.RegisterTestnetChains()

	sepoliaWETH := bridges.TokenByChainAndSymbol[bridges.ChainID(bridges.ChainIDSepolia)]["WETH"].Address
	if sepoliaWETH == "" {
		t.Fatalf("missing sepolia WETH in chainmap")
	}

	req := models.QuoteRequest{
		Source: models.Endpoint{
			ChainID:       int(bridges.ChainIDSepolia),
			Chain:         "sepolia",
			Asset:         "ETH",
			TokenAddress:  "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE",
			TokenDecimals: 18,
			Address:       "0x000000000000000000000000000000000000dEaD",
		},
		Destination: models.Endpoint{
			ChainID: int(bridges.ChainIDBaseSepolia),
			Chain:   "base-sepolia",
			Asset:   "USDC",
		},
		AmountBaseUnits: "1200000000000000",
		Metadata: map[string]interface{}{
			"source_swap_token_out_address": bridges.TokenByChainAndSymbol[bridges.ChainID(bridges.ChainIDSepolia)]["USDC"].Address,
		},
	}

	d := &dexTokenInAssertingAdapter{wantTokenIn: sepoliaWETH, quoteOut: "723762"}
	b := &bridgePassthroughAdapter{id: "cctp"}

	routes, err := quoteSwapThenBridge(context.Background(), []bridges.Adapter{b}, d, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(routes) == 0 {
		t.Fatalf("expected at least one composed route")
	}
	if got := routes[0].Hops[0].FromTokenAddress; got != sepoliaWETH {
		t.Fatalf("swap hop FromTokenAddress = %q, want sepolia WETH %q", got, sepoliaWETH)
	}
}

