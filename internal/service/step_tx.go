package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"bridge-aggregator/internal/bridges"
	"bridge-aggregator/internal/dex"
	"bridge-aggregator/internal/models"
)

// BridgeClients holds live API clients for bridges that need real-time data at execution.
type BridgeClients struct {
	Stargate *bridges.StargateClient
	Mayan    *bridges.MayanAdapter
	Across   *bridges.AcrossClient
}

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
func PopulateStepTransaction(ctx context.Context, dexAdapters []dex.Adapter, bc *BridgeClients, req models.StepTransactionRequest) (*models.StepTransactionResponse, error) {
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
	return populateBridgeStep(ctx, h, req, bc)
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
	case "blockdaemon_dex":
		resp, err = populateBlockdaemonDEXTx(ctx, adapter.(*dex.BlockdaemonDEXAdapter), h, req)
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

func populateBlockdaemonDEXTx(ctx context.Context, ba *dex.BlockdaemonDEXAdapter, h models.Hop, req models.StepTransactionRequest) (*models.StepTransactionResponse, error) {
	var pd swapProviderData
	if len(h.ProviderData) > 0 {
		if err := json.Unmarshal(h.ProviderData, &pd); err != nil {
			return nil, fmt.Errorf("blockdaemon dex: invalid provider_data: %w", err)
		}
	}
	if len(pd.Quote) == 0 {
		return nil, fmt.Errorf("blockdaemon dex: missing provider quote data for hop")
	}
	txData, err := ba.CreateSwapTx(ctx, dex.QuoteRequest{}, pd.Quote)
	if err != nil {
		return nil, err
	}
	to, _ := txData["to"].(string)
	data, _ := txData["data"].(string)
	value, _ := txData["value"].(string)
	if to == "" || data == "" {
		return nil, fmt.Errorf("blockdaemon dex: transaction missing to/data")
	}
	return &models.StepTransactionResponse{
		HopIndex: req.HopIndex,
		Tx: &models.TransactionRequest{
			To:    to,
			Data:  data,
			Value: firstNonEmpty(value, "0"),
		},
	}, nil
}

// ── Bridge hops ───────────────────────────────────────────────────────────────

func populateBridgeStep(ctx context.Context, h models.Hop, req models.StepTransactionRequest, bc *BridgeClients) (*models.StepTransactionResponse, error) {
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
		return populateCCTPStep(h, pd, req)
	case protocol == "across_v3":
		return populateAcrossStep(ctx, h, pd, req, bc)
	case protocol == "canonical" && jsonString(pd["bridge"]) == "base":
		return populateCanonicalStep(h, pd, req, "base")
	case protocol == "canonical" && jsonString(pd["bridge"]) == "optimism":
		return populateCanonicalStep(h, pd, req, "optimism")
	case protocol == "canonical" && jsonString(pd["bridge"]) == "arbitrum":
		return populateCanonicalStep(h, pd, req, "arbitrum")
	case protocol == "layerzero_stargate_v2":
		return populateStargateStep(ctx, h, pd, req, bc)
	case isProtocolMayan(protocol):
		return populateMayanStep(ctx, h, pd, req, bc)
	default:
		return nil, fmt.Errorf("step transaction not yet supported for bridge %q (protocol=%q)", h.BridgeID, protocol)
	}
}

func isProtocolMayan(p string) bool {
	return p == "mayan_swift" || p == "mayan_wh" || p == "mayan_mctp" ||
		p == "mayan_fast_mctp" || p == "mayan"
}

// CCTP — Circle Cross-Chain Transfer Protocol
//
// On-chain flow:
//  1. approve(TokenMessenger, amount)
//  2. TokenMessenger.depositForBurn(amount, destinationDomain, mintRecipient, burnToken)
//  3. (off-chain) poll https://iris-api.circle.com for attestation
//  4. (on dst chain) MessageTransmitter.receiveMessage(message, attestation)
const cctpDepositForBurnABI = `{"name":"depositForBurn","type":"function","stateMutability":"nonpayable","inputs":[{"name":"amount","type":"uint256"},{"name":"destinationDomain","type":"uint32"},{"name":"mintRecipient","type":"bytes32"},{"name":"burnToken","type":"address"}],"outputs":[{"name":"nonce","type":"uint64"}]}`

func populateCCTPStep(h models.Hop, pd map[string]json.RawMessage, req models.StepTransactionRequest) (*models.StepTransactionResponse, error) {
	tokenMessenger := jsonString(pd["token_messenger_src"])
	burnToken := jsonString(pd["burn_token"])
	amount := jsonString(pd["amount"])
	dstDomain := jsonNumber(pd["dst_domain"])
	srcChainID, err := requireChainID(h, true)
	if err != nil {
		return nil, fmt.Errorf("cctp: source %w", err)
	}
	dstChainID, err := requireChainID(h, false)
	if err != nil {
		return nil, fmt.Errorf("cctp: destination %w", err)
	}

	if tokenMessenger == "" || burnToken == "" || amount == "" {
		return nil, fmt.Errorf("cctp provider_data incomplete: need token_messenger_src, burn_token, amount")
	}

	// mintRecipient must be the destination EVM address left-padded to bytes32.
	recipient := firstNonEmpty(req.ReceiverAddress, req.SenderAddress)
	if recipient == "" {
		return nil, fmt.Errorf("cctp: receiver_address (or sender_address fallback) is required for depositForBurn")
	}
	mintRecipient, err := addressToBytes32(recipient)
	if err != nil {
		return nil, fmt.Errorf("cctp: invalid recipient address %q: %w", recipient, err)
	}

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
		HopIndex: req.HopIndex,
		HopType:  models.HopTypeBridge,
		BridgeParams: &models.BridgeStepParams{
			Protocol: "circle_cctp",
			Steps:    steps,
			Notes:    "After step 2 completes, retrieve the MessageSent event bytes and attest via https://iris-api.circle.com/v1/attestations/{keccak256(message)} before executing step 3 on the destination chain.",
		},
	}, nil
}

func addressToBytes32(addr string) (string, error) {
	a := strings.TrimSpace(addr)
	if len(a) != 42 || !strings.HasPrefix(strings.ToLower(a), "0x") {
		return "", fmt.Errorf("expected 0x-prefixed 20-byte address")
	}
	hexPart := strings.ToLower(a[2:])
	for _, c := range hexPart {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return "", fmt.Errorf("address contains non-hex characters")
		}
	}
	return "0x000000000000000000000000" + hexPart, nil
}

// Across v3 — SpokePool.depositV3()
const acrossDepositV3ABI = `{"name":"depositV3","type":"function","stateMutability":"payable","inputs":[{"name":"depositor","type":"address"},{"name":"recipient","type":"address"},{"name":"inputToken","type":"address"},{"name":"outputToken","type":"address"},{"name":"inputAmount","type":"uint256"},{"name":"outputAmount","type":"uint256"},{"name":"destinationChainId","type":"uint256"},{"name":"exclusiveRelayer","type":"address"},{"name":"quoteTimestamp","type":"uint32"},{"name":"fillDeadline","type":"uint32"},{"name":"exclusivityDeadline","type":"uint32"},{"name":"message","type":"bytes"}],"outputs":[]}`

func populateAcrossStep(ctx context.Context, h models.Hop, pd map[string]json.RawMessage, req models.StepTransactionRequest, bc *BridgeClients) (*models.StepTransactionResponse, error) {
	hopIndex := req.HopIndex
	crossSwapType := jsonString(pd["cross_swap_type"])

	// swapTx-based Across routes: Across did swap logic internally and returned a pre-built
	// swapTx calldata + required approvals. Execute these directly — no deposit params needed.
	if crossSwapType == "anyToBridgeable" || crossSwapType == "bridgeableToAny" || crossSwapType == "anyToAny" {
		return populateAcrossSwapTxStep(h, pd, hopIndex)
	}
	// Backward/shape fallback: if swap_tx is present, always use swapTx path.
	if raw, ok := pd["swap_tx"]; ok && len(raw) > 0 && string(raw) != "null" {
		return populateAcrossSwapTxStep(h, pd, hopIndex)
	}

	// "bridgeable" (same-token direct bridge): use deposit params for SpokePool.depositV3().
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

	depositRaw, hasDeposit := pd["deposit"]
	if !hasDeposit || len(depositRaw) == 0 || string(depositRaw) == "null" {
		// Deposit params missing from quote (common for same-token routes like USDC Eth→Polygon
		// via /swap/approval). Fall back to /suggested-fees to fetch fresh params on-demand.
		if bc != nil && bc.Across != nil {
			walletAddr := req.SenderAddress
			if walletAddr == "" {
				walletAddr = req.ReceiverAddress
			}
			srcChainID, err := requireChainID(h, true)
			if err != nil {
				return nil, fmt.Errorf("across: source %w", err)
			}
			dstChainID, err := requireChainID(h, false)
			if err != nil {
				return nil, fmt.Errorf("across: destination %w", err)
			}
			fresh, err := bc.Across.FetchDeposit(ctx,
				int64(srcChainID), int64(dstChainID),
				h.FromTokenAddress, h.ToTokenAddress,
				h.AmountInBaseUnits, walletAddr,
			)
			if err != nil {
				return nil, fmt.Errorf("across: could not fetch deposit params: %w", err)
			}
			raw, _ := json.Marshal(fresh)
			depositRaw = raw
		} else {
			return nil, fmt.Errorf("across provider_data missing deposit params; re-quote to refresh")
		}
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

// populateAcrossSwapTxStep handles "anyToBridgeable" Across routes where the API
// returned a pre-built swapTx calldata. Execution is: approve token → send swapTx.
func populateAcrossSwapTxStep(h models.Hop, pd map[string]json.RawMessage, hopIndex int) (*models.StepTransactionResponse, error) {
	type swapTxObj struct {
		ChainID int    `json:"chainId"`
		To      string `json:"to"`
		Data    string `json:"data"`
		Gas     string `json:"gas,omitempty"`
	}

	swapTxRaw, ok := pd["swap_tx"]
	if !ok || len(swapTxRaw) == 0 || string(swapTxRaw) == "null" {
		return nil, fmt.Errorf("across anyToBridgeable: missing swap_tx in provider_data; re-quote to refresh")
	}
	var swapTx swapTxObj
	if err := json.Unmarshal(swapTxRaw, &swapTx); err != nil {
		return nil, fmt.Errorf("across anyToBridgeable: invalid swap_tx: %w", err)
	}
	if swapTx.To == "" || swapTx.Data == "" {
		return nil, fmt.Errorf("across anyToBridgeable: swap_tx missing to/data")
	}

	// Parse approval transactions — typically a single ERC-20 approve.
	var steps []models.BridgeStepCall
	var approvalTxns []swapTxObj
	if raw, ok := pd["approval_txns"]; ok && len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &approvalTxns); err != nil {
			return nil, fmt.Errorf("across anyToBridgeable: invalid approval_txns: %w", err)
		}
	}
	for _, appr := range approvalTxns {
		if appr.To == "" || appr.Data == "" {
			continue
		}
		// Decode the approve calldata to extract spender and amount for a cleaner UI.
		// Fall back to a raw tx step if we can't decode.
		steps = append(steps, models.BridgeStepCall{
			StepType: "approve",
			Tx: &models.BridgeTxCall{
				ChainID:     appr.ChainID,
				Contract:    appr.To,
				Function:    "approve",
				Params:      map[string]any{"data": appr.Data},
				ABIFragment: `{"name":"approve","type":"function","inputs":[],"outputs":[]}`,
				Value:       "0",
			},
		})
	}

	// The main swap+bridge transaction.
	srcChainID := swapTx.ChainID
	if srcChainID == 0 {
		srcChainID = chainIDFromHop(h, true)
	}
	steps = append(steps, models.BridgeStepCall{
		StepType: "deposit",
		Tx: &models.BridgeTxCall{
			ChainID:     srcChainID,
			Contract:    swapTx.To,
			Function:    "swapAndBridge",
			Params:      map[string]any{"data": swapTx.Data},
			ABIFragment: `{"name":"swapAndBridge","type":"function","inputs":[],"outputs":[]}`,
		},
	})

	return &models.StepTransactionResponse{
		HopIndex: hopIndex,
		HopType:  models.HopTypeBridge,
		BridgeParams: &models.BridgeStepParams{
			Protocol: "across_v3_swap",
			Steps:    steps,
			Notes:    "Across handles the origin swap and bridge atomically. Approve the token first, then send the swap transaction.",
		},
	}, nil
}

// Canonical L2 bridges (Base, Optimism, Arbitrum)
// L1→L2 deposits
const opStackDepositETHABI = `{"name":"depositETH","type":"function","stateMutability":"payable","inputs":[{"name":"_minGasLimit","type":"uint32"},{"name":"_extraData","type":"bytes"}],"outputs":[]}`
const opStackDepositERC20ABI = `{"name":"depositERC20","type":"function","stateMutability":"nonpayable","inputs":[{"name":"_l1Token","type":"address"},{"name":"_l2Token","type":"address"},{"name":"_amount","type":"uint256"},{"name":"_minGasLimit","type":"uint32"},{"name":"_extraData","type":"bytes"}],"outputs":[]}`
// L2→L1 withdrawals (OP-stack L2StandardBridge at 0x4200000000000000000000000000000000000010)
const opStackBridgeETHToABI = `{"name":"bridgeETHTo","type":"function","stateMutability":"payable","inputs":[{"name":"_to","type":"address"},{"name":"_minGasLimit","type":"uint32"},{"name":"_extraData","type":"bytes"}],"outputs":[]}`
const opStackBridgeERC20ToABI = `{"name":"bridgeERC20To","type":"function","stateMutability":"nonpayable","inputs":[{"name":"_localToken","type":"address"},{"name":"_remoteToken","type":"address"},{"name":"_to","type":"address"},{"name":"_amount","type":"uint256"},{"name":"_minGasLimit","type":"uint32"},{"name":"_extraData","type":"bytes"}],"outputs":[]}`
// Arbitrum L1→L2
const arbitrumDepositETHABI = `{"name":"depositEth","type":"function","stateMutability":"payable","inputs":[],"outputs":[{"name":"","type":"uint256"}]}`
const arbitrumOutboundTransferABI = `{"name":"outboundTransfer","type":"function","stateMutability":"payable","inputs":[{"name":"_token","type":"address"},{"name":"_to","type":"address"},{"name":"_amount","type":"uint256"},{"name":"_maxGas","type":"uint256"},{"name":"_gasPriceBid","type":"uint256"},{"name":"_data","type":"bytes"}],"outputs":[{"name":"","type":"bytes"}]}`
// Arbitrum L2→L1 (ArbSys precompile at 0x0000000000000000000000000000000000000064)
const arbSysWithdrawEthABI = `{"name":"withdrawEth","type":"function","stateMutability":"payable","inputs":[{"name":"destination","type":"address"}],"outputs":[{"name":"","type":"uint256"}]}`

func populateCanonicalStep(h models.Hop, pd map[string]json.RawMessage, req models.StepTransactionRequest, network string) (*models.StepTransactionResponse, error) {
	hopIndex := req.HopIndex
	amount := jsonString(pd["amount"])
	inputToken := jsonString(pd["input_token"])
	outputToken := jsonString(pd["output_token"])
	depositOnL1Raw := jsonString(pd["deposit_on_l1"])
	depositOnL1 := depositOnL1Raw == "true"
	srcChainID, err := requireChainID(h, true)
	if err != nil {
		return nil, fmt.Errorf("canonical_%s: source %w", network, err)
	}

	isETH := inputToken == "0x0000000000000000000000000000000000000000" ||
		inputToken == "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE"

	var steps []models.BridgeStepCall

	switch network {
	case "base", "optimism":
		recipient := req.SenderAddress
		if recipient == "" {
			recipient = req.ReceiverAddress
		}

		if depositOnL1 {
			// ── L1→L2 deposit ──
			bridgeContract := jsonString(pd["l1_bridge"])
			if bridgeContract == "" {
				return nil, fmt.Errorf("canonical_%s provider_data missing l1_bridge", network)
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
		} else {
			// ── L2→L1 withdrawal ──
			l2Bridge := jsonString(pd["l2_bridge"])
			if l2Bridge == "" {
				l2Bridge = "0x4200000000000000000000000000000000000010"
			}
			if recipient == "" {
				recipient = "0x0000000000000000000000000000000000000000"
			}
			if isETH {
				steps = append(steps, models.BridgeStepCall{
					StepType: "deposit",
					Tx: &models.BridgeTxCall{
						ChainID:  srcChainID,
						Contract: l2Bridge,
						Function: "bridgeETHTo",
						Params: map[string]any{
							"_to":          recipient,
							"_minGasLimit": 200000,
							"_extraData":   "0x",
						},
						Value:       amount,
						ABIFragment: opStackBridgeETHToABI,
					},
				})
			} else {
				steps = append(steps, models.BridgeStepCall{
					StepType: "approve",
					Approval: &models.TokenApproval{
						ChainID:       srcChainID,
						TokenContract: inputToken,
						Spender:       l2Bridge,
						Amount:        amount,
					},
				})
				steps = append(steps, models.BridgeStepCall{
					StepType: "deposit",
					Tx: &models.BridgeTxCall{
						ChainID:  srcChainID,
						Contract: l2Bridge,
						Function: "bridgeERC20To",
						Params: map[string]any{
							"_localToken":  inputToken,
							"_remoteToken": outputToken,
							"_to":          recipient,
							"_amount":      amount,
							"_minGasLimit": 200000,
							"_extraData":   "0x",
						},
						ABIFragment: opStackBridgeERC20ToABI,
					},
				})
			}
		}

	case "arbitrum":
		if isETH && depositOnL1 {
			l1Inbox := jsonString(pd["l1_inbox"])
			if l1Inbox == "" {
				return nil, fmt.Errorf("canonical_arbitrum provider_data missing l1_inbox")
			}
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
		} else if !isETH && depositOnL1 {
			gatewayRouter := jsonString(pd["l1_gateway_router"])
			erc20Gateway := jsonString(pd["l1_erc20_gateway"])
			if gatewayRouter == "" || erc20Gateway == "" {
				return nil, fmt.Errorf("canonical_arbitrum provider_data missing l1_gateway_router or l1_erc20_gateway")
			}

			// Retryable ticket parameters (conservative defaults).
			// In production these should be fetched from Inbox.calculateRetryableSubmissionFee().
			maxSubmissionCost := "500000000000000" // 0.0005 ETH
			maxGas := "300000"
			gasPriceBid := "200000000" // 0.2 gwei

			// msg.value = maxSubmissionCost + maxGas * gasPriceBid
			ethValue := arbRetryableValue(maxSubmissionCost, maxGas, gasPriceBid)

			// _data = abi.encode(uint256 maxSubmissionCost, bytes(""))
			encodedData := encodeGatewayRouterData(maxSubmissionCost)

			recipient := req.SenderAddress
			if recipient == "" {
				recipient = req.ReceiverAddress
			}
			if recipient == "" {
				recipient = "0x0000000000000000000000000000000000000000"
			}

			steps = append(steps,
				models.BridgeStepCall{
					StepType: "approve",
					Approval: &models.TokenApproval{
						ChainID:       srcChainID,
						TokenContract: inputToken,
						Spender:       erc20Gateway,
						Amount:        amount,
					},
				},
				models.BridgeStepCall{
					StepType: "deposit",
					Tx: &models.BridgeTxCall{
						ChainID:  srcChainID,
						Contract: gatewayRouter,
						Function: "outboundTransfer",
						Params: map[string]any{
							"_token":       inputToken,
							"_to":          recipient,
							"_amount":      amount,
							"_maxGas":      maxGas,
							"_gasPriceBid": gasPriceBid,
							"_data":        encodedData,
						},
						Value:       ethValue,
						ABIFragment: arbitrumOutboundTransferABI,
					},
				},
			)
		} else if !depositOnL1 && isETH {
			// Arbitrum L2→L1 ETH withdrawal via ArbSys precompile.
			const arbSysAddress = "0x0000000000000000000000000000000000000064"
			recipient := req.SenderAddress
			if recipient == "" {
				recipient = req.ReceiverAddress
			}
			if recipient == "" {
				recipient = "0x0000000000000000000000000000000000000000"
			}
			steps = append(steps, models.BridgeStepCall{
				StepType: "deposit",
				Tx: &models.BridgeTxCall{
					ChainID:     srcChainID,
					Contract:    arbSysAddress,
					Function:    "withdrawEth",
					Params:      map[string]any{"destination": recipient},
					Value:       amount,
					ABIFragment: arbSysWithdrawEthABI,
				},
			})
		} else {
			return nil, fmt.Errorf("canonical_arbitrum: L2→L1 ERC-20 withdrawals not yet supported (use Across or CCTP)")
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

// ── Stargate (LayerZero VT API) ───────────────────────────────────────────────

func populateStargateStep(ctx context.Context, h models.Hop, pd map[string]json.RawMessage, req models.StepTransactionRequest, bc *BridgeClients) (*models.StepTransactionResponse, error) {
	if bc == nil || bc.Stargate == nil {
		return nil, fmt.Errorf("stargate: client not configured for step transaction")
	}

	srcChainKey := jsonString(pd["src_chain_key"])
	dstChainKey := jsonString(pd["dst_chain_key"])
	srcToken := jsonString(pd["src_token_address"])
	dstToken := jsonString(pd["dst_token_address"])
	amount := jsonString(pd["amount"])

	// Use the real wallet address from the request, falling back to stored addresses.
	srcWallet := req.SenderAddress
	if srcWallet == "" {
		srcWallet = jsonString(pd["src_wallet"])
	}
	dstWallet := req.ReceiverAddress
	if dstWallet == "" {
		dstWallet = jsonString(pd["dst_wallet"])
	}
	if srcWallet == "" || dstWallet == "" {
		return nil, fmt.Errorf("stargate: sender/receiver address required for step transaction")
	}

	txSteps, err := bc.Stargate.GetTransactionSteps(ctx, amount, srcToken, dstToken, srcChainKey, dstChainKey, srcWallet, dstWallet)
	if err != nil {
		return nil, fmt.Errorf("stargate step: %w", err)
	}

	srcChainID := chainIDFromHop(h, true)

	var steps []models.BridgeStepCall
	for _, ts := range txSteps {
		chainID := ts.ChainID
		if chainID == 0 {
			chainID = srcChainID
		}
		if ts.StepType == "approve" {
			steps = append(steps, models.BridgeStepCall{
				StepType: "approve",
				Tx: &models.BridgeTxCall{
					ChainID:  chainID,
					Contract: ts.To,
					Params:   map[string]any{"data": ts.Data},
					Value:    firstNonEmpty(ts.Value, "0"),
				},
			})
		} else {
			steps = append(steps, models.BridgeStepCall{
				StepType: "deposit",
				Tx: &models.BridgeTxCall{
					ChainID:  chainID,
					Contract: ts.To,
					Params:   map[string]any{"data": ts.Data},
					Value:    firstNonEmpty(ts.Value, "0"),
				},
			})
		}
	}

	return &models.StepTransactionResponse{
		HopIndex: req.HopIndex,
		HopType:  models.HopTypeBridge,
		BridgeParams: &models.BridgeStepParams{
			Protocol: "layerzero_stargate_v2",
			Steps:    steps,
			Notes:    "Stargate (LayerZero) cross-chain transfer. The VT API provides pre-built transaction data.",
		},
	}, nil
}

// ── Mayan Finance ─────────────────────────────────────────────────────────────

const mayanTxBuilderURL = "https://tx-builder.mayan.finance"
const mayanForwarderAddress = "0x337685fdaB40D39bd02028545a4FfA7D287cC3E2"

func populateMayanStep(ctx context.Context, h models.Hop, pd map[string]json.RawMessage, req models.StepTransactionRequest, bc *BridgeClients) (*models.StepTransactionResponse, error) {
	if bc == nil || bc.Mayan == nil {
		return nil, fmt.Errorf("mayan: adapter not configured for step transaction")
	}

	senderAddr := req.SenderAddress
	recipientAddr := req.ReceiverAddress
	if recipientAddr == "" {
		recipientAddr = senderAddr
	}
	if senderAddr == "" {
		return nil, fmt.Errorf("mayan: sender address required for step transaction")
	}

	srcChainID, err := requireChainID(h, true)
	if err != nil {
		return nil, fmt.Errorf("mayan: source %w", err)
	}
	isSolanaSource := h.FromChain == "solana" || srcChainID == 900

	isNative := h.FromTokenAddress == "0x0000000000000000000000000000000000000000" ||
		h.FromTokenAddress == "0xEeeeeEeeeEeeeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE"

	// Re-quote through Mayan's tx-builder to get a signed quote object,
	// then POST /build to get the unsigned transaction.
	txData, err := bc.Mayan.GetTransactionData(ctx, h, senderAddr, recipientAddr, srcChainID)
	if err != nil {
		return nil, fmt.Errorf("mayan step: %w", err)
	}

	// Solana-source: the tx-builder returns a serialized Solana transaction.
	// Return it directly — the frontend signs it with the Solana wallet adapter.
	if txData.IsSolana || isSolanaSource {
		return &models.StepTransactionResponse{
			HopIndex: req.HopIndex,
			HopType:  models.HopTypeBridge,
			SolanaTx: &models.SolanaTransactionRequest{
				SerializedTx:    txData.SerializedTx,
				SignerPublicKey: txData.SignerPublicKey,
			},
		}, nil
	}

	var steps []models.BridgeStepCall

	// ERC-20 tokens need approval to the Mayan Forwarder.
	if !isNative {
		steps = append(steps, models.BridgeStepCall{
			StepType: "approve",
			Approval: &models.TokenApproval{
				ChainID:       srcChainID,
				TokenContract: h.FromTokenAddress,
				Spender:       mayanForwarderAddress,
				Amount:        h.AmountInBaseUnits,
			},
		})
	}

	steps = append(steps, models.BridgeStepCall{
		StepType: "deposit",
		Tx: &models.BridgeTxCall{
			ChainID:  srcChainID,
			Contract: txData.To,
			Params:   map[string]any{"data": txData.Data},
			Value:    firstNonEmpty(txData.Value, "0"),
		},
	})

	return &models.StepTransactionResponse{
		HopIndex: req.HopIndex,
		HopType:  models.HopTypeBridge,
		BridgeParams: &models.BridgeStepParams{
			Protocol: jsonString(pd["protocol"]),
			Steps:    steps,
			Notes:    "Mayan Finance (Wormhole) cross-chain swap. Approve the Mayan Forwarder for ERC-20 tokens.",
		},
	}, nil
}

// ── Arbitrum GatewayRouter helpers ─────────────────────────────────────────────

// encodeGatewayRouterData builds the _data parameter for outboundTransfer:
// abi.encode(uint256 maxSubmissionCost, bytes(""))
func encodeGatewayRouterData(maxSubmissionCost string) string {
	mc := new(big.Int)
	mc.SetString(maxSubmissionCost, 10)
	return "0x" +
		fmt.Sprintf("%064x", mc) +
		fmt.Sprintf("%064x", 64) + // offset to bytes
		fmt.Sprintf("%064x", 0) // bytes length = 0
}

// arbRetryableValue computes the ETH value: maxSubmissionCost + maxGas * gasPriceBid.
func arbRetryableValue(maxSubmissionCost, maxGas, gasPriceBid string) string {
	mc := new(big.Int)
	mc.SetString(maxSubmissionCost, 10)
	mg := new(big.Int)
	mg.SetString(maxGas, 10)
	gp := new(big.Int)
	gp.SetString(gasPriceBid, 10)
	total := new(big.Int).Add(mc, new(big.Int).Mul(mg, gp))
	return total.String()
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

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func chainIDFromHop(h models.Hop, src bool) int {
	name := h.ToChain
	if src {
		name = h.FromChain
	}
	// Use the global ChainNameToID map (populated with testnet entries when NETWORK=testnet).
	if id := bridges.ChainIDFromName(name); id != 0 {
		return int(id)
	}
	// Aliases not in the canonical map.
	aliases := map[string]int{
		"eth": 1, "mainnet": 1, "arbitrum_one": 42161,
		"op": 10, "matic": 137,
		"bnb": 56, "avax": 43114,
		"linea": 59144, "scroll": 534352,
		"zksync": 324, "zksync_era": 324,
		"mantle": 5000, "celo": 42220,
		"gnosis": 100, "xdai": 100,
	}
	return aliases[name]
}

// requireChainID returns the chain ID for a hop side, or an error if the chain name is unrecognised.
func requireChainID(h models.Hop, src bool) (int, error) {
	id := chainIDFromHop(h, src)
	if id == 0 {
		side := h.ToChain
		if src {
			side = h.FromChain
		}
		return 0, fmt.Errorf("unrecognised chain %q — chain_id cannot be resolved", side)
	}
	return id, nil
}
