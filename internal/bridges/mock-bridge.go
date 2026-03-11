package bridges

import "bridge-aggregator/internal/models"

type AcrossAdapter struct{}

func (a AcrossAdapter) Name() string {
	return "Across"
}

func (a AcrossAdapter) GetQuote(req models.QuoteRequest) (models.Quote, error) {

	return models.Quote{
		Bridge:        "Across",
		Fee:           2.1,
		EstimatedTime: 120,
	}, nil
}