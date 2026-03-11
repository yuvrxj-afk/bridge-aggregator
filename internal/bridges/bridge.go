package bridges

import "bridge-aggregator/internal/models"

type BridgeAdapter interface {
	Name() string
	GetQuote(req models.QuoteRequest) (models.Quote, error)
}