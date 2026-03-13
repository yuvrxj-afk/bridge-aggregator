package service

import (
	"context"
	"errors"

	"bridge-aggregator/internal/bridges"
	"bridge-aggregator/internal/models"
	"bridge-aggregator/internal/router"
)

// Quote returns routes for the given request using registered adapters.
func Quote(ctx context.Context, adapters []bridges.Adapter, req models.QuoteRequest) (*models.QuoteResponse, error) {
	routes, err := router.Quote(ctx, adapters, req)
	if err != nil {
		if errors.Is(err, router.ErrNoRoutes) {
			return &models.QuoteResponse{Routes: nil}, err
		}
		return nil, err
	}
	return &models.QuoteResponse{Routes: routes}, nil
}
