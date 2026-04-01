package bridges

import (
	"context"

	"bridge-aggregator/internal/models"
)

// Adapter is the bridge provider adapter interface.
// Each implementation returns a single Route (e.g. one hop) for a quote request.
type Adapter interface {
	ID() string
	// Tier returns the adapter's production readiness tier.
	// Only tier 1 (TierProduction) and tier 2 (TierDegraded) participate in the quote fan-out.
	Tier() models.AdapterTier
	GetQuote(ctx context.Context, req models.QuoteRequest) (*models.Route, error)
}
