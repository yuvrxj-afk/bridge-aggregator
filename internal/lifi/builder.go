// Package lifi translates internal route models into LiFi Diamond contract parameters.
// The LiFi Diamond (0x1231DEB6f5749EF6cE6943a275A1D3E7486F4EaE, deployed identically on
// every EVM chain) accepts a single transaction that atomically executes a source swap,
// bridge, and schedules a destination swap via an on-chain message — all in one user signature.
package lifi

import (
	"encoding/json"
	"fmt"
	"strings"

	"bridge-aggregator/internal/models"

	"github.com/google/uuid"
)

// LiFiDiamond is the EIP-2535 Diamond proxy address — identical on every supported EVM chain.
const LiFiDiamond = "0x1231DEB6f5749EF6cE6943a275A1D3E7486F4EaE"

// ZeroAddress is the canonical placeholder for native ETH in LiFi contracts.
const ZeroAddress = "0x0000000000000000000000000000000000000000"

// Integrator tag embedded in every LiFi transaction for fee tracking.
const Integrator = "syv-bridge-aggregator"

// receiverAcrossV3 maps destination chain IDs to the deployed ReceiverAcrossV3 contract.
// This contract receives bridged tokens on the destination chain and executes the swap
// described in the Across message field.  Same address on all chains (CREATE2 deploy).
var receiverAcrossV3 = map[int]string{
	1:     "0xca6e6B692F568055adA0bF72A06D1EBbC938Fb23", // Ethereum
	10:    "0xca6e6B692F568055adA0bF72A06D1EBbC938Fb23", // Optimism
	137:   "0xca6e6B692F568055adA0bF72A06D1EBbC938Fb23", // Polygon
	8453:  "0xca6e6B692F568055adA0bF72A06D1EBbC938Fb23", // Base
	42161: "0xca6e6B692F568055adA0bF72A06D1EBbC938Fb23", // Arbitrum
	43114: "0xca6e6B692F568055adA0bF72A06D1EBbC938Fb23", // Avalanche
}

// chainIDFromName maps chain names used in routes to numeric IDs.
var chainIDFromName = map[string]int{
	"ethereum": 1,
	"optimism": 10,
	"polygon":  137,
	"base":     8453,
	"arbitrum": 42161,
	"avalanche": 43114,
	// testnets
	"sepolia":          11155111,
	"base-sepolia":     84532,
	"arbitrum-sepolia": 421614,
	"op-sepolia":       11155420,
}

// BuildTransaction translates a scored route into LiFi Diamond execution parameters.
// Supported bridge types:
// - across (AcrossFacetV3, with optional source/destination swaps)
// - cctp (CelerCircleBridgeFacet, direct one-click path)
//
// Supported route shapes:
//
//	[bridge]               → startBridgeTokensViaAcrossV3
//	[swap, bridge]         → swapAndStartBridgeTokensViaAcrossV3
//	[bridge, swap]         → startBridgeTokensViaAcrossV3  (dest call via message)
//	[swap, bridge, swap]   → swapAndStartBridgeTokensViaAcrossV3 (src swap + dest call)
func BuildTransaction(route models.Route, fromAddress string) (*models.LiFiBuildResponse, error) {
	hops := route.Hops
	if len(hops) == 0 {
		return nil, fmt.Errorf("route has no hops")
	}

	// Identify the bridge hop and optional surrounding swap hops.
	bridgeIdx, srcSwapIdx, dstSwapIdx, err := classifyHops(hops)
	if err != nil {
		return nil, err
	}

	bridgeHop := hops[bridgeIdx]

	txID := newTxID()
	srcChainID := resolveChainID(bridgeHop.FromChain)
	dstChainID := resolveChainID(bridgeHop.ToChain)

	hasSourceSwap := srcSwapIdx >= 0
	hasDestSwap := dstSwapIdx >= 0

	// Determine receiver: if there's a destination swap (Across only), funds go to ReceiverAcrossV3;
	// otherwise they go directly to the user wallet.
	receiver := fromAddress
	if hasDestSwap && bridgeHop.BridgeID == "across" {
		r, ok := receiverAcrossV3[dstChainID]
		if !ok {
			return nil, fmt.Errorf("lifi builder: no ReceiverAcrossV3 address for destination chain %d", dstChainID)
		}
		receiver = r
	}

	// --- BridgeData ---
	// sendingAssetId: the token that arrives at the bridge.
	// If there's a source swap, the bridge receives the swap's output (USDC, etc.).
	// If there's no source swap, the bridge receives the original input token.
	var sendingAssetID string
	var minAmount string
	if hasSourceSwap {
		srcHop := hops[srcSwapIdx]
		sendingAssetID = normaliseAddress(srcHop.FromTokenAddress) // original input (e.g. ETH)
		minAmount = srcHop.AmountInBaseUnits
	} else {
		sendingAssetID = normaliseAddress(bridgeHop.FromTokenAddress)
		minAmount = bridgeHop.AmountInBaseUnits
	}

	bridgeName := bridgeHop.BridgeID
	if bridgeName == "cctp" {
		bridgeName = "celerCircleBridge"
	}
	bridgeData := &models.LiFiBridgeData{
		TransactionID:      txID,
		Bridge:             bridgeName,
		Integrator:         Integrator,
		Referrer:           ZeroAddress,
		SendingAssetID:     sendingAssetID,
		Receiver:           receiver,
		MinAmount:          minAmount,
		DestinationChainID: dstChainID,
		HasSourceSwaps:     hasSourceSwap,
		HasDestinationCall: hasDestSwap,
	}

	// --- Source SwapData ---
	var swapData []models.LiFiSwapData
	if hasSourceSwap {
		sd, err := extractSwapData(hops[srcSwapIdx], true)
		if err != nil {
			return nil, fmt.Errorf("lifi builder: source swap: %w", err)
		}
		swapData = append(swapData, sd)
	}

	// Across path (supports destination message + optional source swap).
	if bridgeHop.BridgeID == "across" {
		deposit, err := extractAcrossDeposit(bridgeHop)
		if err != nil {
			return nil, fmt.Errorf("lifi builder: %w", err)
		}

		acrossData := &models.LiFiAcrossV3Data{
			ReceiverAddress:     receiver,
			RefundAddress:       fromAddress,
			ReceivingAssetID:    normaliseAddress(deposit.OutputToken),
			OutputAmount:        deposit.OutputAmount,
			OutputAmountPercent: "0",
			ExclusiveRelayer:    deposit.ExclusiveRelayer,
			QuoteTimestamp:      deposit.QuoteTimestamp,
			FillDeadline:        deposit.FillDeadline,
			ExclusivityDeadline: deposit.ExclusivityDeadline,
			Message:             "0x", // frontend sets this from AcrossMessage when HasDestinationCall=true
		}

		var acrossMsg *models.LiFiAcrossMessage
		if hasDestSwap {
			sd, err := extractSwapData(hops[dstSwapIdx], false)
			if err != nil {
				return nil, fmt.Errorf("lifi builder: dest swap: %w", err)
			}
			acrossMsg = &models.LiFiAcrossMessage{
				TransactionID: txID,
				SwapData:      []models.LiFiSwapData{sd},
				Receiver:      fromAddress,
			}
		}

		fn := "startBridgeTokensViaAcrossV3"
		if hasSourceSwap {
			fn = "swapAndStartBridgeTokensViaAcrossV3"
		}

		// Value to attach: ETH amount when sending native token, else "0".
		value := "0"
		if isNativeToken(sendingAssetID) {
			value = minAmount
		}
		return &models.LiFiBuildResponse{
			Diamond:       LiFiDiamond,
			ChainID:       srcChainID,
			Function:      fn,
			Value:         value,
			BridgeData:    bridgeData,
			SwapData:      swapData,
			AcrossV3Data:  acrossData,
			AcrossMessage: acrossMsg,
		}, nil
	}

	// CCTP/CelerCircle one-click path (no destination calls; source swaps currently disabled).
	if bridgeHop.BridgeID == "cctp" {
		if hasDestSwap {
			return nil, fmt.Errorf("lifi builder: cctp destination swaps are not supported in one-click path")
		}
		if hasSourceSwap {
			return nil, fmt.Errorf("lifi builder: cctp source swaps are not supported in one-click path")
		}
		celer := &models.LiFiCelerCircleData{
			MaxFee:               "0",
			MinFinalityThreshold: 1000,
		}
		return &models.LiFiBuildResponse{
			Diamond:          LiFiDiamond,
			ChainID:          srcChainID,
			Function:         "startBridgeTokensViaCelerCircleBridge",
			Value:            "0",
			BridgeData:       bridgeData,
			CelerCircleData:  celer,
		}, nil
	}

	return nil, fmt.Errorf("lifi builder: unsupported bridge %q", bridgeHop.BridgeID)
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// classifyHops returns the indices of bridge, optional source swap, and optional dest swap hops.
// Returns an error if the pattern is unrecognised or unsupported.
func classifyHops(hops []models.Hop) (bridgeIdx, srcSwapIdx, dstSwapIdx int, err error) {
	bridgeIdx = -1
	srcSwapIdx = -1
	dstSwapIdx = -1

	for i, h := range hops {
		ht := h.HopType
		if ht == "" {
			ht = models.HopTypeBridge
		}
		switch ht {
		case models.HopTypeBridge:
			if bridgeIdx >= 0 {
				return 0, 0, 0, fmt.Errorf("multiple bridge hops not supported")
			}
			bridgeIdx = i
		case models.HopTypeSwap:
			if bridgeIdx < 0 {
				srcSwapIdx = i
			} else {
				dstSwapIdx = i
			}
		}
	}

	if bridgeIdx < 0 {
		return 0, 0, 0, fmt.Errorf("no bridge hop found in route")
	}
	return bridgeIdx, srcSwapIdx, dstSwapIdx, nil
}

// acrossDepositParams holds the fields we need from provider_data.deposit.
type acrossDepositParams struct {
	OutputToken         string `json:"outputToken"`
	OutputAmount        string `json:"outputAmount"`
	ExclusiveRelayer    string `json:"exclusiveRelayer"`
	QuoteTimestamp      int64  `json:"quoteTimestamp"`
	FillDeadline        int64  `json:"fillDeadline"`
	ExclusivityDeadline int64  `json:"exclusivityDeadline"`
}

func extractAcrossDeposit(hop models.Hop) (*acrossDepositParams, error) {
	if len(hop.ProviderData) == 0 {
		return nil, fmt.Errorf("across hop has no provider_data")
	}
	var pd map[string]json.RawMessage
	if err := json.Unmarshal(hop.ProviderData, &pd); err != nil {
		return nil, fmt.Errorf("invalid provider_data: %w", err)
	}

	// swapTx-style Across routes use Across's own contract, not LiFi AcrossV3 builder path.
	// The ExecutePanel handles these via /route/stepTransaction instead.
	if crossSwapType, _ := pd["cross_swap_type"]; len(crossSwapType) > 0 {
		var cst string
		_ = json.Unmarshal(crossSwapType, &cst)
		switch cst {
		case "anyToBridgeable", "bridgeableToAny", "anyToAny":
			return nil, fmt.Errorf("this route uses Across swapTx flow (%s): use stepTransaction execution, not LiFi Diamond", cst)
		}
	}
	if swapTxRaw, ok := pd["swap_tx"]; ok && len(swapTxRaw) > 0 && string(swapTxRaw) != "null" {
		return nil, fmt.Errorf("this route uses Across swapTx flow: use stepTransaction execution, not LiFi Diamond")
	}

	depositRaw, ok := pd["deposit"]
	if !ok || len(depositRaw) == 0 || string(depositRaw) == "null" {
		return nil, fmt.Errorf("provider_data missing deposit params — re-quote to refresh")
	}
	var dep acrossDepositParams
	if err := json.Unmarshal(depositRaw, &dep); err != nil {
		return nil, fmt.Errorf("invalid deposit params: %w", err)
	}
	if dep.OutputToken == "" || dep.OutputAmount == "" {
		return nil, fmt.Errorf("deposit params missing outputToken or outputAmount")
	}
	return &dep, nil
}

// swapProviderData is the envelope stored in swap hop provider_data by DEX adapters.
type swapProviderData struct {
	Quote json.RawMessage `json:"quote"`
}

// zeroExTx is the transaction fragment inside a 0x quote.
type zeroExTx struct {
	To    string `json:"to"`
	Data  string `json:"data"`
	Value string `json:"value"`
}

// extractSwapData builds a LiFiSwapData from a swap hop's provider_data.
// requiresDeposit should be true for source swaps (Diamond must pull tokens before calling)
// and false for destination swaps (ReceiverAcrossV3 already holds the tokens).
func extractSwapData(hop models.Hop, requiresDeposit bool) (models.LiFiSwapData, error) {
	if len(hop.ProviderData) == 0 {
		return models.LiFiSwapData{}, fmt.Errorf("swap hop %q has no provider_data", hop.BridgeID)
	}
	var pd swapProviderData
	if err := json.Unmarshal(hop.ProviderData, &pd); err != nil {
		return models.LiFiSwapData{}, fmt.Errorf("invalid provider_data: %w", err)
	}
	if len(pd.Quote) == 0 {
		return models.LiFiSwapData{}, fmt.Errorf("no quote in provider_data for hop %q", hop.BridgeID)
	}

	// 0x and Uniswap both store the final tx under .transaction inside the quote object.
	var quote struct {
		Transaction zeroExTx `json:"transaction"`
	}
	if err := json.Unmarshal(pd.Quote, &quote); err != nil {
		return models.LiFiSwapData{}, fmt.Errorf("could not parse quote.transaction: %w", err)
	}
	tx := quote.Transaction
	if tx.To == "" || tx.Data == "" {
		return models.LiFiSwapData{}, fmt.Errorf("quote.transaction missing to/data for hop %q", hop.BridgeID)
	}

	return models.LiFiSwapData{
		CallTo:           tx.To,
		ApproveTo:        tx.To, // 0x and Uniswap routers approve-to = call-to
		SendingAssetID:   normaliseAddress(hop.FromTokenAddress),
		ReceivingAssetID: normaliseAddress(hop.ToTokenAddress),
		FromAmount:       hop.AmountInBaseUnits,
		CallData:         tx.Data,
		RequiresDeposit:  requiresDeposit,
	}, nil
}

// newTxID generates a bytes32 transaction ID as a 0x-prefixed 64-hex-char string.
// UUID supplies the first 16 bytes; the remaining 16 bytes are zero-padded.
func newTxID() string {
	id := uuid.New()
	b := [16]byte(id)
	// bytes32 = 32 bytes = 64 hex chars. UUID is 16 bytes → pad 16 zero bytes.
	return fmt.Sprintf("0x%x%032x", b, 0)
}

// normaliseAddress ensures a token address is lowercase and 0x-prefixed.
// The zero address and empty strings are mapped to ZeroAddress (native ETH sentinel).
func normaliseAddress(addr string) string {
	if addr == "" {
		return ZeroAddress
	}
	lower := strings.ToLower(addr)
	if !strings.HasPrefix(lower, "0x") {
		lower = "0x" + lower
	}
	return lower
}

// isNativeToken returns true if the address is the zero address sentinel (native ETH/MATIC etc.).
func isNativeToken(addr string) bool {
	return strings.EqualFold(addr, ZeroAddress)
}

// resolveChainID maps chain name strings to numeric IDs, falling back to 0 if unknown.
func resolveChainID(chain string) int {
	return chainIDFromName[strings.ToLower(chain)]
}
