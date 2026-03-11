package models

type QuoteRequest struct {
	FromChain string  `json:"fromChain"`
	ToChain   string  `json:"toChain"`
	Token     string  `json:"token"`
	Amount    float64 `json:"amount"`
}

type Quote struct {
	Bridge        string  `json:"bridge"`
	Fee           float64 `json:"fee"`
	EstimatedTime int     `json:"estimatedTime"`
}

type RouteResponse struct {
	BestRoute Quote `json:"bestRoute"`
}