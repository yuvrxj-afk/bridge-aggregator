package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"bridge-aggregator/internal/dex"
	"bridge-aggregator/internal/models"
)

var (
	ErrHopIndexOutOfRange = errors.New("hop_index out of range")
	ErrHopNotSupported    = errors.New("hop does not support transaction population")
	ErrPermitSignatureReq = errors.New("permit signature required for this quote")
)

type swapProviderData struct {
	Quote      json.RawMessage `json:"quote"`
	PermitData json.RawMessage `json:"permitData"`
	Routing    string          `json:"routing"`
}

// zeroExTx is the transaction fragment inside a 0x quote response.
type zeroExTx struct {
	To       string `json:"to"`
	Data     string `json:"data"`
	Value    string `json:"value"`
	Gas      string `json:"gas"`
	GasPrice string `json:"gasPrice"`
}

// PopulateStepTransaction returns transaction parameters for a single hop.
//
//   - Swap hops (Uniswap / 0x / 1inch): returns a ready-to-broadcast unsigned tx in Tx.
//   - Bridge hops (CCTP / Across / canonical): returns structured BridgeParams with the
//     contract addresses, function signatures, call parameters, and ABI fragments the
//     client can feed directly to viem's writeContract() or wagmi's useWriteContract().
func PopulateStepTransaction(ctx context.Context, dexAdapters []dex.Adapter, req models.StepTransactionRequest) (*models.StepTransactionResponse, error) {
	if req.HopIndex < 0 || req.HopIndex >= len(req.Route.Hops) {
		return nil, ErrHopIndexOutOfRange
	}

	h := req.Route.Hops[req.HopIndex]
	hopType := h.HopType
	if hopType == "" {
		hopType = models.HopTypeBridge
	}

	if hopType == models.HopTypeSwap {
		return populateSwapStep(ctx, dexAdapters, h, req)
	}
	return populateBridgeStep(h, req)
}

// ── Swap hops ────────────────────────────────────────────────────────────────

func populateSwapStep(ctx context.Context, dexAdapters []dex.Adapter, h models.Hop, req models.StepTransactionRequest) (*models.StepTransactionResponse, error) {
	var adapter dex.Adapter
	for _, a := range dexAdapters {
		if a != nil && h.BridgeID == a.ID() {
			adapter = a
			break
		}
	}
	if adapter == nil {
		return nil, ErrHopNotSupported
	}

	var resp *models.StepTransactionResponse
	var err error
	switch adapter.ID() {
	case "zeroex":
		resp, err = populateZeroExTx(h.ProviderData, req.HopIndex)
	case "oneinch":
		resp, err = populateOneInchTx(ctx, adapter.(*dex.OneInchAdapter), h, req)
	case "uniswap_trading_api":
		resp, err = populateUniswapTx(ctx, adapter.(*dex.UniswapTradingAdapter), h, req)
	default:
		return nil, ErrHopNotSupported
	}
	if err != nil {
		return nil, err
	}
	resp.HopType = models.HopTypeSwap
	return resp, nil
}

func populateZeroExTx(providerData json.RawMessage, hopIndex int) (*models.StepTransactionResponse, error) {
	if len(providerData) == 0 {
		return nil, fmt.Errorf("missing provider_data for 0x hop")
	}
	var wrapper swapProviderData
	if err := json.Unmarshal(providerData, &wrapper); err != nil {
		return nil, fmt.Errorf("invalid provider_data: %w", err)
	}
	if len(wrapper.Quote) == 0 {
		return nil, fmt.Errorf("missing quote in provider_data for 0x hop")
	}
	var quote struct {
		Transaction zeroExTx `json:"transaction"`
	}
	if err := json.Unmarshal(wrapper.Quote, &quote); err != nil {
		return nil, fmt.Errorf("invalid 0x quote in provider_data: %w", err)
	}
	tx := quote.Transaction
	if tx.To == "" || tx.Data == "" {
		return nil, fmt.Errorf("0x quote missing transaction to/data")
	}
	return &models.StepTransactionResponse{
		HopIndex: hopIndex,
		Tx: &models.TransactionRequest{
			To:       tx.To,
			Data:     tx.Data,
			Value:    tx.Value,
			GasLimit: tx.Gas,
		},
	}, nil
}

func populateUniswapTx(ctx context.Context, ua *dex.UniswapTradingAdapter, h models.Hop, req models.StepTransactionRequest) (*models.StepTransactionResponse, error) {
	var pd swapProviderData
	if len(h.ProviderData) > 0 {
		if err := json.Unmarshal(h.ProviderData, &pd); err != nil {
			return nil, fmt.Errorf("invalid provider_data: %w", err)
		}
	}
	if len(pd.Quote) == 0 {
		return nil, fmt.Errorf("missing provider quote data for hop")
	}
	if len(pd.PermitData) > 0 && string(pd.PermitData) != "null" && req.Signature == "" {
		return nil, ErrPermitSignatureReq
	}

	tx, err := ua.CreateSwapTx(ctx, pd.Quote, pd.PermitData, req.Signature)
	if err != nil {
		return nil, err
	}
	if tx.To == "" || tx.Data == "" || tx.Data == "0x" {
		return nil, fmt.Errorf("invalid tx returned: missing to/data")
	}
	return &models.StepTransactionResponse{
		HopIndex: req.HopIndex,
		Tx:       &tx,
	}, nil
}

func populateOneInchTx(ctx context.Context, oa *dex.OneInchAdapter, h models.Hop, req models.StepTransactionRequest) (*models.StepTransactionResponse, error) {
	var pd swapProviderData
	if len(h.ProviderData) > 0 {
		if err := json.Unmarshal(h.ProviderData, &pd); err != nil {
			return nil, fmt.Errorf("invalid provider_data: %w", err)
		}
	}
	if len(pd.Quote) == 0 {
		return nil, fmt.Errorf("missing provider quote data for hop")
	}

	tx, err := oa.CreateSwapTx(ctx, pd.Quote)
	if err != nil {
		return nil, err
	}
	if tx.To == "" || tx.Data == "" || tx.Data == "0x" {
		return nil, fmt.Errorf("invalid tx returned: missing to/data")
	}
	return &models.StepTransactionResponse{
		HopIndex: req.HopIndex,
		Tx:       &tx,
	}, nil
}

// ── Bridge hops ───────────────────────────────────────────────────────────────

func populateBridgeStep(h models.Hop, req models.StepTransactionRequest) (*models.StepTransactionResponse, error) {
	if len(h.ProviderData) == 0 {
		return nil, fmt.Errorf("bridge hop %q has no provider_data; re-quote to obtain execution parameters", h.BridgeID)
	}

	var pd map[string]json.RawMessage
	if err := json.Unmarshal(h.ProviderData, &pd); err != nil {
		return nil, fmt.Errorf("invalid provider_data for bridge hop: %w", err)
	}

	protocol := jsonString(pd["protocol"])

	switch {
	case protocol == "circle_cctp":
		return populateCCTPStep(h, pd, req.HopIndex)
	case protocol == "across_v3":
		return populateAcrossStep(h, pd, req.HopIndex)
	case protocol == "canonical" && jsonString(pd["bridge"]) == "base":
		return populateCanonicalStep(h, pd, req.HopIndex, "base")
	case protocol == "canonical" && jsonString(pd["bridge"]) == "optimism":
		return populateCanonicalStep(h, pd, req.HopIndex, "optimism")
	case protocol == "canonical" && jsonString(pd["bridge"]) == "arbitrum":
		return populateCanonicalStep(h, pd, req.HopIndex, "arbitrum")
	default:
		return nil, fmt.Errorf("step transaction not yet supported for bridge %q (protocol=%q)", h.BridgeID, protocol)
	}
}

// CCTP — Circle Cross-Chain Transfer Protocol
//
// On-chain flow:
//  1. approve(TokenMessenger, amount)
//  2. TokenMessenger.depositForBurn(amount, destinationDomain, mintRecipient, burnToken)
//  3. (off-chain) poll https://iris-api.circle.com for attestation
//  4. (on dst chain) MessageTransmitter.receiveMessage(message, attestation)
const cctpDepositForBurnABI = `{"name":"depositForBurn","type":"function","stateMutability":"nonpayable","inputs":[{"name":"amount","type":"uint256"},{"name":"destinationDomain","type":"uint32"},{"name":"mintRecipient","type":"bytes32"},{"name":"burnToken","type":"address"}],"outputs":[{"name":"nonce","type":"uint64"}]}`

func populateCCTPStep(h models.Hop, pd map[string]json.RawMessage, hopIndex int) (*models.StepTransactionResponse, error) {
	tokenMessenger := jsonString(pd["token_messenger_src"])
	burnToken := jsonString(pd["burn_token"])
	amount := jsonString(pd["amount"])
	dstDomain := jsonNumber(pd["dst_domain"])
	srcChainID := chainIDFromHop(h, true)
	dstChainID := chainIDFromHop(h, false)

	if tokenMessenger == "" || burnToken == "" || amount == "" {
		return nil, fmt.Errorf("cctp provider_data incomplete: need token_messenger_src, burn_token, amount")
	}

	// mintRecipient: the recipient address left-padded to 32 bytes (bytes32).
	// When no explicit recipient is set, use the zero address as a placeholder the
	// client MUST replace before signing.
	mintRecipient := "0x0000000000000000000000000000000000000000000000000000000000000000"

	steps := []models.BridgeStepCall{
		{
			StepType: "approve",
			Approval: &models.TokenApproval{
				ChainID:       srcChainID,
				TokenContract: burnToken,
				Spender:       tokenMessenger,
				Amount:        amount,
			},
		},
		{
			StepType: "deposit",
			Tx: &models.BridgeTxCall{
				ChainID:  srcChainID,
				Contract: tokenMessenger,
				Function: "depositForBurn",
				Params: map[string]any{
					"amount":            amount,
					"destinationDomain": dstDomain,
					"mintRecipient":     mintRecipient,
					"burnToken":         burnToken,
				},
				ABIFragment: cctpDepositForBurnABI,
			},
		},
		{
			StepType: "claim",
			Tx: &models.BridgeTxCall{
				ChainID:  dstChainID,
				Contract: jsonString(pd["message_transmitter_dst"]),
				Function: "receiveMessage",
				Params: map[string]any{
					"message":     "<obtain from depositForBurn event log>",
					"attestation": "<obtain from https://iris-api.circle.com/v1/attestations/{messageHash}>",
				},
				ABIFragment: `{"name":"receiveMessage","type":"function","stateMutability":"nonpayable","inputs":[{"name":"message","type":"bytes"},{"name":"attestation","type":"bytes"}],"outputs":[{"name":"success","type":"bool"}]}`,
			},
		},
	}

	return &models.StepTransactionResponse{
		HopIndex: hopIndex,
		HopType:  models.HopTypeBridge,
		BridgeParams: &models.BridgeStepParams{
			Protocol: "circle_cctp",
			Steps:    steps,
			Notes:    "After step 2 completes, retrieve the MessageSent event bytes and attest via https://iris-api.circle.com/v1/attestations/{keccak256(message)} before executing step 3 on the destination chain.",
		},
	}, nil
}

// Across v3 — SpokePool.depositV3()
const acrossDepositV3ABI = `{"name":"depositV3","type":"function","stateMutability":"payable","inputs":[{"name":"depositor","type":"address"},{"name":"recipient","type":"address"},{"name":"inputToken","type":"address"},{"name":"outputToken","type":"address"},{"name":"inputAmount","type":"uint256"},{"name":"outputAmount","type":"uint256"},{"name":"destinationChainId","type":"uint256"},{"name":"exclusiveRelayer","type":"address"},{"name":"quoteTimestamp","type":"uint32"},{"name":"fillDeadline","type":"uint32"},{"name":"exclusivityDeadline","type":"uint32"},{"name":"message","type":"bytes"}],"outputs":[]}`

func populateAcrossStep(h models.Hop, pd map[string]json.RawMessage, hopIndex int) (*models.StepTransactionResponse, error) {
	// Extract the deposit sub-object Across returns in the API response.
	var deposit struct {
		Depositor           string `json:"depositor"`
		Recipient           string `json:"recipient"`
		InputToken          string `json:"inputToken"`
		OutputToken         string `json:"outputToken"`
		InputAmount         string `json:"inputAmount"`
		OutputAmount        string `json:"outputAmount"`
		DestinationChainID  int64  `json:"destinationChainId"`
		ExclusiveRelayer    string `json:"exclusiveRelayer"`
		QuoteTimestamp      int64  `json:"quoteTimestamp"`
		FillDeadline        int64  `json:"fillDeadline"`
		ExclusivityDeadline int64  `json:"exclusivityDeadline"`
		Message             string `json:"message"`
		SpokePoolAddress    string `json:"spokePoolAddress"`
	}

	depositRaw, ok := pd["deposit"]
	if !ok || len(depositRaw) == 0 || string(depositRaw) == "null" {
		return nil, fmt.Errorf("across provider_data missing deposit params; re-quote to refresh")
	}
	if err := json.Unmarshal(depositRaw, &deposit); err != nil {
		return nil, fmt.Errorf("invalid across deposit params: %w", err)
	}
	if deposit.SpokePoolAddress == "" {
		return nil, fmt.Errorf("across deposit params missing spokePoolAddress")
	}

	srcChainID := chainIDFromHop(h, true)
	isETH := deposit.InputToken == "0x0000000000000000000000000000000000000000"

	var steps []models.BridgeStepCall

	// Only add an ERC-20 approval step when the input token is not the native coin.
	if !isETH && deposit.InputToken != "" {
		steps = append(steps, models.BridgeStepCall{
			StepType: "approve",
			Approval: &models.TokenApproval{
				ChainID:       srcChainID,
				TokenContract: deposit.InputToken,
				Spender:       deposit.SpokePoolAddress,
				Amount:        deposit.InputAmount,
			},
		})
	}

	value := ""
	if isETH {
		value = deposit.InputAmount
	}

	steps = append(steps, models.BridgeStepCall{
		StepType: "deposit",
		Tx: &models.BridgeTxCall{
			ChainID:  srcChainID,
			Contract: deposit.SpokePoolAddress,
			Function: "depositV3",
			Params: map[string]any{
				"depositor":           deposit.Depositor,
				"recipient":           deposit.Recipient,
				"inputToken":          deposit.InputToken,
				"outputToken":         deposit.OutputToken,
				"inputAmount":         deposit.InputAmount,
				"outputAmount":        deposit.OutputAmount,
				"destinationChainId":  deposit.DestinationChainID,
				"exclusiveRelayer":    deposit.ExclusiveRelayer,
				"quoteTimestamp":      deposit.QuoteTimestamp,
				"fillDeadline":        deposit.FillDeadline,
				"exclusivityDeadline": deposit.ExclusivityDeadline,
				"message":             deposit.Message,
			},
			Value:       value,
			ABIFragment: acrossDepositV3ABI,
		},
	})

	return &models.StepTransactionResponse{
		HopIndex: hopIndex,
		HopType:  models.HopTypeBridge,
		BridgeParams: &models.BridgeStepParams{
			Protocol: "across_v3",
			Steps:    steps,
			Notes:    "Across relayers fill the deposit on the destination chain automatically. Monitor status at https://app.across.to.",
		},
	}, nil
}

// Canonical L2 bridges (Base, Optimism, Arbitrum)
const opStackDepositETHABI = `{"name":"depositETH","type":"function","stateMutability":"payable","inputs":[{"name":"_minGasLimit","type":"uint32"},{"name":"_extraData","type":"bytes"}],"outputs":[]}`
const opStackDepositERC20ABI = `{"name":"depositERC20","type":"function","stateMutability":"nonpayable","inputs":[{"name":"_l1Token","type":"address"},{"name":"_l2Token","type":"address"},{"name":"_amount","type":"uint256"},{"name":"_minGasLimit","type":"uint32"},{"name":"_extraData","type":"bytes"}],"outputs":[]}`
const arbitrumDepositETHABI = `{"name":"depositEth","type":"function","stateMutability":"payable","inputs":[],"outputs":[{"name":"","type":"uint256"}]}`

func populateCanonicalStep(h models.Hop, pd map[string]json.RawMessage, hopIndex int, network string) (*models.StepTransactionResponse, error) {
	amount := jsonString(pd["amount"])
	inputToken := jsonString(pd["input_token"])
	outputToken := jsonString(pd["output_token"])
	depositOnL1Raw := jsonString(pd["deposit_on_l1"])
	depositOnL1 := depositOnL1Raw == "true"
	srcChainID := chainIDFromHop(h, true)

	isETH := inputToken == "0x0000000000000000000000000000000000000000" ||
		inputToken == "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE"

	var steps []models.BridgeStepCall

	switch network {
	case "base", "optimism":
		bridgeKey := "l1_bridge"
		if !depositOnL1 {
			bridgeKey = "l2_bridge"
		}
		bridgeContract := jsonString(pd[bridgeKey])
		if bridgeContract == "" {
			return nil, fmt.Errorf("canonical_%s provider_data missing %s", network, bridgeKey)
		}

		if isETH {
			steps = append(steps, models.BridgeStepCall{
				StepType: "deposit",
				Tx: &models.BridgeTxCall{
					ChainID:     srcChainID,
					Contract:    bridgeContract,
					Function:    "depositETH",
					Params:      map[string]any{"_minGasLimit": 200000, "_extraData": "0x"},
					Value:       amount,
					ABIFragment: opStackDepositETHABI,
				},
			})
		} else {
			steps = append(steps, models.BridgeStepCall{
				StepType: "approve",
				Approval: &models.TokenApproval{
					ChainID:       srcChainID,
					TokenContract: inputToken,
					Spender:       bridgeContract,
					Amount:        amount,
				},
			})
			steps = append(steps, models.BridgeStepCall{
				StepType: "deposit",
				Tx: &models.BridgeTxCall{
					ChainID:  srcChainID,
					Contract: bridgeContract,
					Function: "depositERC20",
					Params: map[string]any{
						"_l1Token":     inputToken,
						"_l2Token":     outputToken,
						"_amount":      amount,
						"_minGasLimit": 200000,
						"_extraData":   "0x",
					},
					ABIFragment: opStackDepositERC20ABI,
				},
			})
		}

	case "arbitrum":
		l1Inbox := jsonString(pd["l1_inbox"])
		if l1Inbox == "" {
			return nil, fmt.Errorf("canonical_arbitrum provider_data missing l1_inbox")
		}
		if isETH && depositOnL1 {
			steps = append(steps, models.BridgeStepCall{
				StepType: "deposit",
				Tx: &models.BridgeTxCall{
					ChainID:     srcChainID,
					Contract:    l1Inbox,
					Function:    "depositEth",
					Params:      map[string]any{},
					Value:       amount,
					ABIFragment: arbitrumDepositETHABI,
				},
			})
		} else {
			return nil, fmt.Errorf("canonical_arbitrum ERC-20 deposits require the GatewayRouter; re-quote using Across or CCTP for ERC-20 transfers")
		}
	}

	notes := fmt.Sprintf("Canonical %s bridge. Finality: Base/Optimism ~1–7 min; Arbitrum ~7 days (challenge period). No claim step needed for deposits to L2.", network)
	if !depositOnL1 {
		notes = fmt.Sprintf("Canonical %s bridge withdrawal from L2. A 7-day (Optimism/Base) or challenge period may apply before funds are claimable on L1.", network)
	}

	return &models.StepTransactionResponse{
		HopIndex: hopIndex,
		HopType:  models.HopTypeBridge,
		BridgeParams: &models.BridgeStepParams{
			Protocol: "canonical_" + network,
			Steps:    steps,
			Notes:    notes,
		},
	}, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func jsonString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// Try number → string fallback.
	return string(raw)
}

func jsonNumber(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	return 0
}

func chainIDFromHop(h models.Hop, src bool) int {
	// Hop doesn't carry chain IDs directly; derive from chain name via a best-effort map.
	// Callers should use the route's source.chain_id when available.
	chainNames := map[string]int{
		"ethereum": 1, "base": 8453, "arbitrum": 42161,
		"optimism": 10, "polygon": 137, "bsc": 56, "avalanche": 43114,
	}
	if src {
		if id, ok := chainNames[h.FromChain]; ok {
			return id
		}
	} else {
		if id, ok := chainNames[h.ToChain]; ok {
			return id
		}
	}
	return 0
}
