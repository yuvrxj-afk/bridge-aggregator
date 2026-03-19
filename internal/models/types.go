package models

import "encoding/json"

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

	// Optional fields for DEX/tx-building integrations.
	ChainID       int    `json:"chain_id,omitempty"`
	TokenAddress  string `json:"token_address,omitempty"`
	TokenDecimals int    `json:"token_decimals,omitempty"`
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
	// Amount is the input amount. Historically this was a human-readable decimal string.
	// For DEX and composition work, prefer AmountBaseUnits.
	Amount string `json:"amount"`

	// AmountBaseUnits is the input amount in token base units (e.g. wei for ETH).
	AmountBaseUnits string `json:"amount_base_units,omitempty"`
	Preferences *QuotePreferences `json:"preferences,omitempty"`
	Metadata    map[string]any   `json:"metadata,omitempty"`
}

// HopType identifies what kind of hop this is.
const (
	HopTypeBridge = "bridge"
	HopTypeSwap   = "swap"
)

// Hop represents a single hop in a route (bridge or swap).
type Hop struct {
	// HopType is optional for backward compatibility. If empty, treat as "bridge".
	HopType string `json:"hop_type,omitempty"`

	// BridgeID identifies the provider to use. For bridge hops it's a bridge adapter ID
	// (e.g. "across"). For swap hops it's a DEX adapter ID (e.g. "uniswap_trading_api")
	// or a prefixed form like "dex:uniswap_trading_api" depending on the producer.
	BridgeID string `json:"bridge_id"`

	FromChain string `json:"from_chain"`
	ToChain   string `json:"to_chain"`
	FromAsset string `json:"from_asset"`
	ToAsset   string `json:"to_asset"`

	// Optional token/amount details for tx building and better composition.
	FromTokenAddress   string `json:"from_token_address,omitempty"`
	ToTokenAddress     string `json:"to_token_address,omitempty"`
	AmountInBaseUnits  string `json:"amount_in_base_units,omitempty"`
	AmountOutBaseUnits string `json:"amount_out_base_units,omitempty"`

	// ProviderData can hold opaque provider-specific JSON used later for tx building
	// (e.g. Uniswap Trading API quote object for /swap).
	ProviderData json.RawMessage `json:"provider_data,omitempty"`

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

// StepTransactionRequest asks the server to populate a single hop with transaction data.
// This mirrors LiFi's stepTransaction idea. The server does NOT broadcast; it only returns
// an unsigned transaction request when supported by the hop provider.
type StepTransactionRequest struct {
	Route     Route  `json:"route"`
	HopIndex  int    `json:"hop_index"`
	Signature string `json:"signature,omitempty"`
}

// TransactionRequest is an unsigned transaction payload suitable for a wallet to sign.
type TransactionRequest struct {
	To       string `json:"to"`
	From     string `json:"from,omitempty"`
	Data     string `json:"data"`
	Value    string `json:"value,omitempty"`
	ChainID  int    `json:"chain_id,omitempty"`
	GasLimit string `json:"gas_limit,omitempty"`
}

// BridgeTxCall describes a single smart-contract call the client must submit.
// Feed Contract + ABIFragment + Params directly to viem's writeContract() or wagmi's
// useWriteContract(); or build calldata manually using encodeFunctionData().
type BridgeTxCall struct {
	ChainID     int            `json:"chain_id"`
	Contract    string         `json:"contract"`
	Function    string         `json:"function"`
	Params      map[string]any `json:"params"`
	Value       string         `json:"value,omitempty"` // ETH value in wei (payable calls)
	ABIFragment string         `json:"abi_fragment"`    // JSON ABI of just this function
}

// TokenApproval describes an ERC-20 approve() call needed before a bridge deposit.
type TokenApproval struct {
	ChainID       int    `json:"chain_id"`
	TokenContract string `json:"token_contract"`
	Spender       string `json:"spender"`
	Amount        string `json:"amount"`
}

// BridgeStepParams contains structured parameters for a bridge hop's on-chain execution.
// It returns everything the client needs to call the bridge contracts with viem/wagmi
// without the server needing to ABI-encode the calldata itself.
type BridgeStepParams struct {
	// Protocol identifies the bridge: "cctp", "across_v3", "canonical_base", etc.
	Protocol string `json:"protocol"`
	// Steps lists each on-chain call in execution order.
	// Index 0 is typically the ERC-20 approval (if needed), index 1 is the deposit.
	Steps []BridgeStepCall `json:"steps"`
	// Notes contains human-readable guidance about off-chain steps (e.g. Iris attestation).
	Notes string `json:"notes,omitempty"`
}

// BridgeStepCall is one step inside BridgeStepParams: either an approval or a contract call.
type BridgeStepCall struct {
	StepType string         `json:"step_type"` // "approve" | "deposit" | "claim"
	Approval *TokenApproval `json:"approval,omitempty"`
	Tx       *BridgeTxCall  `json:"tx,omitempty"`
}

// StepTransactionResponse returns the populated transaction request for the hop.
// For swap hops (Uniswap/0x/1inch) Tx is populated.
// For bridge hops BridgeParams is populated with structured call parameters.
type StepTransactionResponse struct {
	HopIndex     int               `json:"hop_index"`
	HopType      string            `json:"hop_type"`               // "swap" | "bridge"
	Tx           *TransactionRequest `json:"tx,omitempty"`           // swap hops only
	BridgeParams *BridgeStepParams   `json:"bridge_params,omitempty"` // bridge hops only
}

// OperationResponse is the response body for GET /api/v1/operations/{id}.
type OperationResponse struct {
	OperationID       string `json:"operation_id"`
	Status            string `json:"status"`
	TxHash            string `json:"tx_hash,omitempty"`
	Route             Route  `json:"route"`
	ClientReferenceID string `json:"client_reference_id,omitempty"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

// UpdateOperationStatusRequest is the request body for PATCH /api/v1/operations/{id}/status.
type UpdateOperationStatusRequest struct {
	// Status must be one of: submitted, completed, failed.
	Status string `json:"status" binding:"required"`
	// TxHash is the on-chain transaction hash (optional; can be set on any status transition).
	TxHash string `json:"tx_hash,omitempty"`
}

// ErrorEnvelope is the standard error response for non-2xx responses.
type ErrorEnvelope struct {
	Error struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details,omitempty"`
	} `json:"error"`
}
