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

func TestIsValidTransition_pendingToCompletedWithTxHash(t *testing.T) {
	if isValidTransition(models.OperationStatusPending, models.OperationStatusCompleted, "") {
		t.Fatal("pending→completed must require non-empty tx hash")
	}
	if !isValidTransition(models.OperationStatusPending, models.OperationStatusCompleted, "0xabc") {
		t.Fatal("pending→completed should be allowed when tx hash is set")
	}
	if !isValidTransition(models.OperationStatusPending, models.OperationStatusSubmitted, "") {
		t.Fatal("pending→submitted remains valid (tx hash enforced in UpdateOperationStatus)")
	}
}
