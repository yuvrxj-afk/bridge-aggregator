package bridges

import (
	"context"

	"bridge-aggregator/internal/models"
)

// Adapter is the bridge provider adapter interface.
// Each implementation returns a single Route (e.g. one hop) for a quote request.
type Adapter interface {
	ID() string
	GetQuote(ctx context.Context, req models.QuoteRequest) (*models.Route, error)
}
