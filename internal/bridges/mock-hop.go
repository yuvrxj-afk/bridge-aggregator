package bridges

import "bridge-aggregator/internal/models"

type HopAdapter struct{}

func (h HopAdapter) Name() string {
	return "Hop"
}

func (h HopAdapter) GetQuote(req models.QuoteRequest) (models.Quote, error) {

	return models.Quote{
		Bridge:        "Hop",
		Fee:           3.0,
		EstimatedTime: 90,
	}, nil
}