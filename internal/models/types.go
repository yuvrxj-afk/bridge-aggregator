package models

// OperationStatus represents the lifecycle state of an operation.
const (
	OperationStatusPending   = "pending"
	OperationStatusSubmitted = "submitted"
	OperationStatusCompleted = "completed"
	OperationStatusFailed    = "failed"
)

// Endpoint identifies a value location (chain, asset, address).
type Endpoint struct {
	Chain   string `json:"chain"`
	Asset   string `json:"asset"`
	Address string `json:"address"`
}

// QuotePreferences holds optional quote preferences.
type QuotePreferences struct {
	MaxSlippageBps int      `json:"max_slippage_bps"`
	MaxFee         string   `json:"max_fee"`
	Priority       string   `json:"priority"` // e.g. "fastest", "cheapest"
	AllowedBridges []string `json:"allowed_bridges"`
}

// QuoteRequest is the request body for POST /api/v1/quote.
type QuoteRequest struct {
	Source      Endpoint         `json:"source"`
	Destination Endpoint         `json:"destination"`
	Amount      string           `json:"amount"`
	Preferences *QuotePreferences `json:"preferences,omitempty"`
	Metadata    map[string]any   `json:"metadata,omitempty"`
}

// Hop represents a single bridge hop in a route.
type Hop struct {
	BridgeID     string `json:"bridge_id"`
	FromChain    string `json:"from_chain"`
	ToChain      string `json:"to_chain"`
	FromAsset    string `json:"from_asset"`
	ToAsset      string `json:"to_asset"`
	EstimatedFee string `json:"estimated_fee"`
}

// Route is one quoted route (one or more hops) with score and totals.
type Route struct {
	RouteID               string  `json:"route_id"`
	Score                 float64 `json:"score"`
	EstimatedOutputAmount string  `json:"estimated_output_amount"`
	EstimatedTimeSeconds  int64   `json:"estimated_time_seconds"`
	TotalFee              string  `json:"total_fee"`
	Hops                  []Hop   `json:"hops"`
}

// QuoteResponse is the success response for POST /api/v1/quote (routes best-first).
type QuoteResponse struct {
	Routes []Route `json:"routes"`
}

// ExecuteRequest is the request body for POST /api/v1/execute.
type ExecuteRequest struct {
	RouteID           string  `json:"route_id,omitempty"`
	Route             *Route  `json:"route,omitempty"`
	IdempotencyKey    string  `json:"idempotency_key,omitempty"`
	ClientReferenceID string  `json:"client_reference_id,omitempty"`
}

// ExecuteResponse is the response body for POST /api/v1/execute.
type ExecuteResponse struct {
	OperationID       string `json:"operation_id"`
	Status            string `json:"status"`
	ClientReferenceID string `json:"client_reference_id,omitempty"`
}

// OperationResponse is the response body for GET /api/v1/operations/{id}.
type OperationResponse struct {
	OperationID       string `json:"operation_id"`
	Status            string `json:"status"`
	Route             Route  `json:"route"`
	ClientReferenceID string `json:"client_reference_id,omitempty"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

// ErrorEnvelope is the standard error response for non-2xx responses.
type ErrorEnvelope struct {
	Error struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details,omitempty"`
	} `json:"error"`
}
