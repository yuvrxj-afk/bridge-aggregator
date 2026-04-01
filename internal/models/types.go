package models

import "encoding/json"

// OperationStatus represents the lifecycle state of an operation.
const (
	OperationStatusPending   = "pending"
	OperationStatusSubmitted = "submitted"
	OperationStatusCompleted = "completed"
	OperationStatusFailed    = "failed"
)

// ErrorType classifies errors by their recoverability.
// The frontend uses this to determine what recovery action to offer the user.
const (
	ErrorTypeRetryable   = "retryable"    // network blip, rate limit — retry same params
	ErrorTypeUserAction  = "user_action"  // wallet rejected, wrong chain — user must act
	ErrorTypeRequote     = "requote"      // quote expired, no routes — get a fresh quote
	ErrorTypeTerminal    = "terminal"     // contract reverted, invalid data — start over
)

// AdapterTier classifies an adapter's production readiness.
// Only tier 1 and tier 2 participate in the quote fan-out.
type AdapterTier int

const (
	TierProduction     AdapterTier = 1 // Fully configured, executable end-to-end
	TierDegraded       AdapterTier = 2 // Functional but intermittent; quotes are validated before scoring
	TierUncredentialed AdapterTier = 3 // Missing required API keys — excluded from fan-out
	TierConfigBroken   AdapterTier = 4 // Config present but unusable — excluded from fan-out
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
	Source      Endpoint `json:"source"`
	Destination Endpoint `json:"destination"`
	// Amount is the input amount. Historically this was a human-readable decimal string.
	// For DEX and composition work, prefer AmountBaseUnits.
	Amount string `json:"amount"`

	// AmountBaseUnits is the input amount in token base units (e.g. wei for ETH).
	AmountBaseUnits string            `json:"amount_base_units,omitempty"`
	Preferences     *QuotePreferences `json:"preferences,omitempty"`
	Metadata        map[string]any    `json:"metadata,omitempty"`
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
	RouteID               string            `json:"route_id"`
	Score                 float64           `json:"score"`
	EstimatedOutputAmount string            `json:"estimated_output_amount"`
	EstimatedTimeSeconds  int64             `json:"estimated_time_seconds"`
	TotalFee              string            `json:"total_fee"`
	Hops                  []Hop             `json:"hops"`
	Execution             *ExecutionProfile `json:"execution,omitempty"`
}

// ExecutionProfile describes how safely and directly a route can be executed.
// This is a control-plane contract between routing and UI/orchestration layers.
type ExecutionProfile struct {
	Supported    bool              `json:"supported"`
	Intent       string            `json:"intent"`    // atomic_one_click | guided_two_step | async_claim | unsupported
	Guarantee    string            `json:"guarantee"` // relay_fill_or_refund | manual_recovery_required | unknown
	Recovery     string            `json:"recovery"`  // automatic | resumable_guided | manual
	Requirements []string          `json:"requirements,omitempty"`
	Reasons      []string          `json:"reasons,omitempty"` // machine-readable unsupported reasons
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// QuoteResponse is the success response for POST /api/v1/quote (routes best-first).
type QuoteResponse struct {
	Routes []Route `json:"routes"`
}

// ExecuteRequest is the request body for POST /api/v1/execute.
type ExecuteRequest struct {
	RouteID           string `json:"route_id,omitempty"`
	Route             *Route `json:"route,omitempty"`
	IdempotencyKey    string `json:"idempotency_key,omitempty"`
	ClientReferenceID string `json:"client_reference_id,omitempty"`
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
	Route           Route  `json:"route"`
	HopIndex        int    `json:"hop_index"`
	Signature       string `json:"signature,omitempty"`
	SenderAddress   string `json:"sender_address,omitempty"`
	ReceiverAddress string `json:"receiver_address,omitempty"`
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

// SolanaTransactionRequest is an unsigned Solana transaction ready for wallet signing.
type SolanaTransactionRequest struct {
	// SerializedTx is the base64-encoded transaction bytes (versioned or legacy).
	SerializedTx    string `json:"serialized_tx"`
	// SignerPublicKey is the expected signer's base58 public key.
	SignerPublicKey string `json:"signer_public_key,omitempty"`
}

// StepTransactionResponse returns the populated transaction request for the hop.
// For swap hops (Uniswap/0x/1inch) Tx is populated.
// For bridge hops BridgeParams is populated with structured call parameters.
// For Solana-source Mayan hops SolanaTx is populated instead.
type StepTransactionResponse struct {
	HopIndex     int                       `json:"hop_index"`
	HopType      string                    `json:"hop_type"`                // "swap" | "bridge"
	Tx           *TransactionRequest       `json:"tx,omitempty"`            // EVM swap hops
	BridgeParams *BridgeStepParams         `json:"bridge_params,omitempty"` // EVM bridge hops
	SolanaTx     *SolanaTransactionRequest `json:"solana_tx,omitempty"`     // Solana-source hops
}

// OperationResponse is the response body for GET /api/v1/operations/{id}.
type OperationResponse struct {
	OperationID       string   `json:"operation_id"`
	Status            string   `json:"status"`
	TxHash            string   `json:"tx_hash,omitempty"`
	Route             Route    `json:"route"`
	ClientReferenceID string   `json:"client_reference_id,omitempty"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
	NextAction        string   `json:"next_action,omitempty"`
	RecoveryHints     []string `json:"recovery_hints,omitempty"`
}

type OperationEventResponse struct {
	ID         int64  `json:"id"`
	EventType  string `json:"event_type"`
	FromStatus string `json:"from_status,omitempty"`
	ToStatus   string `json:"to_status,omitempty"`
	TxHash     string `json:"tx_hash,omitempty"`
	Metadata   string `json:"metadata,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// CapabilitiesResponse exposes runtime-execution capabilities for UI/ops introspection.
type CapabilitiesResponse struct {
	BuildTransaction map[string]any `json:"build_transaction"`
	StepTransaction  map[string]any `json:"step_transaction"`
	Operations       map[string]any `json:"operations"`
}

// AdapterHealth represents health of one bridge/DEX integration.
type AdapterHealth struct {
	Service   string `json:"service"`
	Kind      string `json:"kind"`   // "bridge" | "dex"
	Tier      int    `json:"tier"`   // 1=production 2=degraded 3=uncredentialed 4=config_broken
	Status    string `json:"status"` // "healthy" | "degraded" | "down"
	Reason    string `json:"reason,omitempty"`
	LatencyMS int64  `json:"latency_ms"`
}

// AdapterHealthResponse is the response body for GET /api/v1/health/adapters.
type AdapterHealthResponse struct {
	Status   string          `json:"status"` // overall status
	Adapters []AdapterHealth `json:"adapters"`
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
		Code      string         `json:"code"`
		Message   string         `json:"message"`
		Details   map[string]any `json:"details,omitempty"`
		// ErrorType classifies recoverability: "retryable" | "user_action" | "requote" | "terminal"
		ErrorType string         `json:"error_type,omitempty"`
		// ErrorCode is a machine-readable sub-type, e.g. "no_routes", "timeout", "invalid_route"
		ErrorCode string         `json:"error_code,omitempty"`
	} `json:"error"`
}

// ── LiFi Diamond execution types ─────────────────────────────────────────────

// LiFiBuildRequest is the request body for POST /api/v1/route/buildTransaction.
// The backend translates the route into LiFi Diamond contract parameters the
// frontend can ABI-encode and submit as a single transaction.
type LiFiBuildRequest struct {
	Route       Route  `json:"route"`
	FromAddress string `json:"from_address"` // user wallet — becomes the token recipient
}

// LiFiBridgeData mirrors ILiFi.BridgeData from the LiFi Diamond contracts.
// All numeric amounts are decimal strings (not hex) to avoid precision loss in JSON.
type LiFiBridgeData struct {
	TransactionID      string `json:"transactionId"` // bytes32 as 0x-prefixed hex
	Bridge             string `json:"bridge"`        // "across", "cctp", etc.
	Integrator         string `json:"integrator"`
	Referrer           string `json:"referrer"`       // zero address when unused
	SendingAssetID     string `json:"sendingAssetId"` // source token (native = 0x000…000)
	Receiver           string `json:"receiver"`       // ReceiverAcrossV3 or user wallet
	MinAmount          string `json:"minAmount"`      // source amount in base units
	DestinationChainID int    `json:"destinationChainId"`
	HasSourceSwaps     bool   `json:"hasSourceSwaps"`
	HasDestinationCall bool   `json:"hasDestinationCall"`
}

// LiFiSwapData mirrors LibSwap.SwapData from the LiFi Diamond contracts.
type LiFiSwapData struct {
	CallTo           string `json:"callTo"`           // DEX router address
	ApproveTo        string `json:"approveTo"`        // address that receives token approval
	SendingAssetID   string `json:"sendingAssetId"`   // token in (native = 0x000…000)
	ReceivingAssetID string `json:"receivingAssetId"` // token out
	FromAmount       string `json:"fromAmount"`       // input amount in base units
	CallData         string `json:"callData"`         // 0x-prefixed encoded swap calldata
	RequiresDeposit  bool   `json:"requiresDeposit"`  // true for source swaps, false for dest
}

// LiFiAcrossV3Data mirrors AcrossFacetV3.AcrossV3Data from the LiFi contracts.
type LiFiAcrossV3Data struct {
	ReceiverAddress     string `json:"receiverAddress"`     // LiFi ReceiverAcrossV3 or user wallet
	RefundAddress       string `json:"refundAddress"`       // user wallet (for failed bridge refunds)
	ReceivingAssetID    string `json:"receivingAssetId"`    // output token on destination chain
	OutputAmount        string `json:"outputAmount"`        // minimum output in base units
	OutputAmountPercent string `json:"outputAmountPercent"` // "0" when not used
	ExclusiveRelayer    string `json:"exclusiveRelayer"`
	QuoteTimestamp      int64  `json:"quoteTimestamp"`
	FillDeadline        int64  `json:"fillDeadline"`
	ExclusivityDeadline int64  `json:"exclusivityDeadline"`
	// Message is the ABI-encoded payload for the destination ReceiverAcrossV3 contract.
	// Empty ("0x") when there is no destination call (no dest swap).
	// Frontend must encode this from AcrossMessage using viem encodeAbiParameters before
	// calling the Diamond: abi.encode(bytes32 txId, SwapData[] swapData, address receiver).
	Message string `json:"message"`
}

// LiFiAcrossMessage is the DECODED form of the Across cross-chain message payload.
// When AcrossV3Data.Message is empty, the frontend must ABI-encode this struct
// using viem's encodeAbiParameters and set it as AcrossV3Data.Message before
// encoding the final Diamond call.
//
// Encoding scheme (matches ReceiverAcrossV3.handleV3AcrossMessage):
//
//	abi.encode(bytes32 transactionId, LibSwap.SwapData[] swapData, address receiver)
type LiFiAcrossMessage struct {
	TransactionID string         `json:"transaction_id"` // bytes32 hex — same as BridgeData.TransactionID
	SwapData      []LiFiSwapData `json:"swap_data"`      // destination chain swap(s)
	Receiver      string         `json:"receiver"`       // final recipient (user wallet)
}

// LiFiCelerCircleData mirrors CelerCircleBridgeFacet.CelerCircleData.
type LiFiCelerCircleData struct {
	MaxFee               string `json:"maxFee"`
	MinFinalityThreshold uint32 `json:"minFinalityThreshold"`
}

// LiFiBuildResponse is the response for POST /api/v1/route/buildTransaction.
// The frontend uses viem to:
//  1. If AcrossMessage is set: encode it with encodeAbiParameters and set AcrossV3Data.Message.
//  2. Call encodeFunctionData(Function, [BridgeData, SwapData, AcrossV3Data]).
//  3. sendTransaction({ to: Diamond, data: calldata, value: Value }).
type LiFiBuildResponse struct {
	Diamond  string `json:"diamond"`  // LiFi Diamond address (same on all chains)
	ChainID  int    `json:"chain_id"` // source chain to send the transaction on
	Function string `json:"function"` // Diamond function to call
	// Value is the native ETH amount to attach (wei, decimal string). "0" for ERC-20 inputs.
	Value string `json:"value"`

	BridgeData   *LiFiBridgeData   `json:"bridge_data,omitempty"`
	SwapData     []LiFiSwapData    `json:"swap_data,omitempty"` // source swap(s); empty if none
	AcrossV3Data *LiFiAcrossV3Data `json:"across_v3_data,omitempty"`

	// AcrossMessage is present when there is a destination swap (HasDestinationCall=true).
	// The frontend MUST ABI-encode this and insert it into AcrossV3Data.Message.
	AcrossMessage   *LiFiAcrossMessage   `json:"across_message,omitempty"`
	CelerCircleData *LiFiCelerCircleData `json:"celer_circle_data,omitempty"`
}
