package router

import (
	"context"
	"testing"

	"bridge-aggregator/internal/bridges"
	"bridge-aggregator/internal/models"
)

type feeTestAdapter struct {
	id    string
	route *models.Route
}

func (m *feeTestAdapter) ID() string               { return m.id }
func (m *feeTestAdapter) Tier() models.AdapterTier { return models.TierProduction }
func (m *feeTestAdapter) GetQuote(_ context.Context, _ models.QuoteRequest) (*models.Route, error) {
	return m.route, nil
}

func TestParseNonNegativeDecimal_Strict(t *testing.T) {
	if _, err := parseNonNegativeDecimal("0.25"); err != nil {
		t.Fatalf("expected valid decimal fee: %v", err)
	}
	invalid := []string{"", "-1", "1e3", "1/2", "NaN", " 1", "+1"}
	for _, v := range invalid {
		if _, err := parseNonNegativeDecimal(v); err == nil {
			t.Fatalf("expected parse failure for %q", v)
		}
	}
}

func TestQuote_DropsInvalidFeeRoutes(t *testing.T) {
	req := models.QuoteRequest{
		Source:          models.Endpoint{ChainID: 1, Chain: "ethereum", Asset: "USDC"},
		Destination:     models.Endpoint{ChainID: 8453, Chain: "base", Asset: "USDC"},
		AmountBaseUnits: "1000000",
	}
	valid := &models.Route{
		RouteID:               "valid",
		EstimatedOutputAmount: "1000",
		EstimatedTimeSeconds:  60,
		TotalFee:              "0.25",
		Hops: []models.Hop{
			{HopType: models.HopTypeBridge, BridgeID: "across", AmountInBaseUnits: "1000"},
		},
	}
	invalid := &models.Route{
		RouteID:               "invalid",
		EstimatedOutputAmount: "1000",
		EstimatedTimeSeconds:  30,
		TotalFee:              "1e9",
		Hops: []models.Hop{
			{HopType: models.HopTypeBridge, BridgeID: "across", AmountInBaseUnits: "1000"},
		},
	}
	routes, err := Quote(context.Background(), []bridges.Adapter{
		&feeTestAdapter{id: "valid", route: valid},
		&feeTestAdapter{id: "invalid", route: invalid},
	}, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(routes) != 1 || routes[0].RouteID != "valid" {
		t.Fatalf("invalid fee route must be dropped before ranking; got %+v", routes)
	}
}
