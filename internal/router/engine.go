package router

import (
	"bridge-aggregator/internal/bridges"
	"bridge-aggregator/internal/models"
)

func FindBestRoute(adapters []bridges.BridgeAdapter, req models.QuoteRequest) (models.Quote, error) {

	var best models.Quote

	for i, adapter := range adapters {

		quote, err := adapter.GetQuote(req)
		if err != nil {
			continue
		}

		if i == 0 || quote.Fee < best.Fee {
			best = quote
		}
	}

	return best, nil
}