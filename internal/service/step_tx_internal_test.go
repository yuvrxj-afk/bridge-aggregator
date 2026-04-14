package service

import "testing"

func TestArbRetryableValue_StrictParse(t *testing.T) {
	if _, err := arbRetryableValue("500000000000000", "300000", "200000000"); err != nil {
		t.Fatalf("expected valid retryable value params, got %v", err)
	}
	if _, err := arbRetryableValue("1e18", "300000", "200000000"); err == nil {
		t.Fatalf("expected strict parse error for scientific notation")
	}
}
