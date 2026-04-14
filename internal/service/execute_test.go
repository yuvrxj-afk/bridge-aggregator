package service

import (
	"testing"
	"time"

	"bridge-aggregator/internal/models"
)

func TestValidateExecuteRequest_QuoteExpiryRequired(t *testing.T) {
	req := models.ExecuteRequest{
		Route: &models.Route{
			RouteID: "r1",
			Hops: []models.Hop{
				{HopType: models.HopTypeBridge, BridgeID: "across"},
			},
		},
	}
	if err := ValidateExecuteRequest(req, map[string]bool{"across": true}); err != ErrQuoteExpiryRequired {
		t.Fatalf("expected ErrQuoteExpiryRequired, got %v", err)
	}
}

func TestValidateExecuteRequest_QuoteExpired(t *testing.T) {
	req := models.ExecuteRequest{
		Route: &models.Route{
			RouteID:               "r1",
			QuoteExpiresAt:        time.Now().Add(-1 * time.Minute).UTC().Format(time.RFC3339),
			EstimatedOutputAmount: "1",
			Hops: []models.Hop{
				{HopType: models.HopTypeBridge, BridgeID: "across"},
			},
		},
	}
	if err := ValidateExecuteRequest(req, map[string]bool{"across": true}); err != ErrQuoteExpired {
		t.Fatalf("expected ErrQuoteExpired, got %v", err)
	}
}
