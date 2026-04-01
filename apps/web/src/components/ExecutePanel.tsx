import { useEffect, useState, useCallback } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { Buffer } from "buffer";
import { formatUnits, encodeFunctionData, encodeAbiParameters, decodeAbiParameters, parseAbiParameters, keccak256, type Abi } from "viem";
import { useSendTransaction, useWriteContract, useAccount, useChainId, useSwitchChain, usePublicClient } from "wagmi";
import { useWallet, useConnection } from "@solana/wallet-adapter-react";
import { VersionedTransaction, Transaction } from "@solana/web3.js";
import {
  type Route,
  type BridgeStepCall,
  type LiFiBuildResponse,
  type LiFiSwapData,
  type LiFiAcrossMessage,
  fetchBuildTransaction,
  fetchStepTransaction,
  fetchTransactionStatus,
} from "../api";
import { TokenIcon } from "./TokenIcon";
import { ChainIcon } from "./ChainIcon";
import { TOKENS } from "../tokens";
import { LIFI_ACROSS_FACET_V3_ABI } from "../abi/lifiAcrossFacetV3";
import { LIFI_CELER_CIRCLE_BRIDGE_ABI } from "../abi/lifiCelerCircleBridgeFacet";

// ── Constants ─────────────────────────────────────────────────────────────────

const ACROSS_FILL_URL = "https://app.across.to/transactions";

const CHAIN_ID: Record<string, number> = {
  ethereum: 1, base: 8453, arbitrum: 42161, optimism: 10, polygon: 137,
  avalanche: 43114, bsc: 56, solana: 900,
  // testnets
  sepolia: 11155111, "base-sepolia": 84532,
  "arbitrum-sepolia": 421614, "op-sepolia": 11155420,
};
const EXPLORER: Record<number, string> = {
  1: "https://etherscan.io", 8453: "https://basescan.org",
  42161: "https://arbiscan.io", 10: "https://optimistic.etherscan.io",
  137: "https://polygonscan.com", 43114: "https://snowtrace.io",
  56: "https://bscscan.com",
  // testnets
  11155111: "https://sepolia.etherscan.io",
  84532:    "https://sepolia.basescan.org",
  421614:   "https://sepolia.arbiscan.io",
  11155420: "https://sepolia-optimism.etherscan.io",
};
function explorerTx(chainId: number, hash: string) {
  return `${EXPLORER[chainId] ?? "https://etherscan.io"}/tx/${hash}`;
}
function tokenDecimals(chainId: number, symbol: string) {
  return TOKENS[chainId]?.find(t => t.symbol === symbol)?.decimals ?? 18;
}
function tokenAddress(chainId: number, symbol: string) {
  return TOKENS[chainId]?.find(t => t.symbol === symbol)?.address ?? "";
}
function fmtAmt(raw: string, dec: number) {
  try {
    const n = Number(formatUnits(BigInt(raw), dec));
    return n >= 1 ? n.toFixed(4) : n.toPrecision(4);
  } catch { return raw; }
}
function hopLabel(id: string) {
  const M: Record<string, string> = {
    across: "Across", circle_cctp: "CCTP", uniswap_trading_api: "Uniswap",
    zerox: "0x", zeroex: "0x", oneinch: "1inch", mayan: "Mayan",
    stargate: "Stargate (LayerZero)", canonical_base: "Base Bridge",
    canonical_optimism: "OP Bridge", canonical_arbitrum: "Arb Bridge",
  };
  return M[id] ?? id.replace(/_/g, " ").replace(/\b\w/g, c => c.toUpperCase());
}
function intentLabel(intent?: string) {
  switch (intent) {
    case "atomic_one_click": return "One-click";
    case "guided_two_step":  return "2-step";
    case "async_claim":      return "Async claim";
    default:                 return "Unsupported";
  }
}
function chainName(c: string) {
  const M: Record<string, string> = {
    ethereum: "Ethereum", base: "Base", arbitrum: "Arbitrum",
    optimism: "Optimism", polygon: "Polygon", avalanche: "Avalanche",
    bsc: "BNB Chain",
    sepolia: "Sepolia", "base-sepolia": "Base Sepolia",
    "arbitrum-sepolia": "Arb Sepolia", "op-sepolia": "OP Sepolia",
  };
  return M[c?.toLowerCase()] ?? c;
}
function chainIdOf(c: string) {
  return CHAIN_ID[c?.toLowerCase()] ?? 0;
}
function asAddr(v: string): `0x${string}` {
  return v.toLowerCase() as `0x${string}`;
}

// ── ABI encoding helpers ──────────────────────────────────────────────────────

function encodeAcrossMessage(msg: LiFiAcrossMessage): `0x${string}` {
  return encodeAbiParameters(
    parseAbiParameters(
      "bytes32, (address callTo, address approveTo, address sendingAssetId, address receivingAssetId, uint256 fromAmount, bytes callData, bool requiresDeposit)[], address"
    ),
    [
      msg.transaction_id as `0x${string}`,
      msg.swap_data.map(sd => ({
        callTo:           sd.callTo           as `0x${string}`,
        approveTo:        sd.approveTo        as `0x${string}`,
        sendingAssetId:   sd.sendingAssetId   as `0x${string}`,
        receivingAssetId: sd.receivingAssetId as `0x${string}`,
        fromAmount:       BigInt(sd.fromAmount),
        callData:         sd.callData         as `0x${string}`,
        requiresDeposit:  sd.requiresDeposit,
      })),
      msg.receiver as `0x${string}`,
    ]
  );
}

function buildSwapDataArgs(swapData: LiFiSwapData[]) {
  return swapData.map(sd => ({
    callTo:           sd.callTo           as `0x${string}`,
    approveTo:        sd.approveTo        as `0x${string}`,
    sendingAssetId:   sd.sendingAssetId   as `0x${string}`,
    receivingAssetId: sd.receivingAssetId as `0x${string}`,
    fromAmount:       BigInt(sd.fromAmount),
    callData:         sd.callData         as `0x${string}`,
    requiresDeposit:  sd.requiresDeposit,
  }));
}

// ── Typed LiFi Diamond execution (writeContract for better wallet decoding) ──

async function executeLiFiDiamond(
  resp: LiFiBuildResponse,
  diamond: `0x${string}`,
  chainId: number | undefined,
  writeContract: ReturnType<typeof useWriteContract>["writeContractAsync"],
): Promise<string> {
  const bd = resp.bridge_data!;
  const value = BigInt(resp.value ?? "0");

  const bridgeArg = {
    transactionId:      bd.transactionId      as `0x${string}`,
    bridge:             bd.bridge,
    integrator:         bd.integrator,
    referrer:           bd.referrer            as `0x${string}`,
    sendingAssetId:     bd.sendingAssetId      as `0x${string}`,
    receiver:           bd.receiver            as `0x${string}`,
    minAmount:          BigInt(bd.minAmount),
    destinationChainId: BigInt(bd.destinationChainId),
    hasSourceSwaps:     bd.hasSourceSwaps,
    hasDestinationCall: bd.hasDestinationCall,
  };

  if (resp.function === "startBridgeTokensViaCelerCircleBridge") {
    const celer = resp.celer_circle_data!;
    return writeContract({
      address: diamond,
      abi: LIFI_CELER_CIRCLE_BRIDGE_ABI,
      functionName: "startBridgeTokensViaCelerCircleBridge",
      args: [
        bridgeArg,
        { maxFee: BigInt(celer.maxFee), minFinalityThreshold: celer.minFinalityThreshold },
      ],
      chainId,
      // LiFi Diamond checks gasleft() < 2^24 (16,777,216). Cap well below that.
      gas: 600_000n,
    });
  }

  const av3 = resp.across_v3_data!;
  const message: `0x${string}` = resp.across_message
    ? encodeAcrossMessage(resp.across_message)
    : "0x";

  const acrossArg = {
    receiverAddress:     av3.receiverAddress     as `0x${string}`,
    refundAddress:       av3.refundAddress       as `0x${string}`,
    receivingAssetId:    av3.receivingAssetId    as `0x${string}`,
    outputAmount:        BigInt(av3.outputAmount),
    outputAmountPercent: BigInt(av3.outputAmountPercent ?? "0"),
    exclusiveRelayer:    av3.exclusiveRelayer    as `0x${string}`,
    quoteTimestamp:      av3.quoteTimestamp,
    fillDeadline:        av3.fillDeadline,
    exclusivityDeadline: av3.exclusivityDeadline,
    message,
  };

  if (resp.function === "swapAndStartBridgeTokensViaAcrossV3") {
    return writeContract({
      address: diamond,
      abi: LIFI_ACROSS_FACET_V3_ABI,
      functionName: "swapAndStartBridgeTokensViaAcrossV3",
      args: [bridgeArg, buildSwapDataArgs(resp.swap_data ?? []), acrossArg],
      value,
      chainId,
      gas: 1_000_000n,
    });
  }

  return writeContract({
    address: diamond,
    abi: LIFI_ACROSS_FACET_V3_ABI,
    functionName: "startBridgeTokensViaAcrossV3",
    args: [bridgeArg, acrossArg],
    value,
    chainId,
    gas: 1_000_000n,
  });
}

// ── Route classification ──────────────────────────────────────────────────────

function isAcrossSwapTxRoute(route: Route): boolean {
  return route.hops.some(h => {
    const pd = h.provider_data as Record<string, unknown> | undefined;
    if (!pd) return false;
    if (pd.swap_tx) return true;
    const cst = pd.cross_swap_type;
    return cst === "anyToBridgeable" || cst === "bridgeableToAny" || cst === "anyToAny";
  });
}

function isLiFiCompatible(route: Route): boolean {
  const bridgeHops = route.hops.filter(h => h.hop_type === "bridge");
  if (bridgeHops.length === 0) return false;
  // Routes with a destination-side swap hop can't go through LiFi Diamond — the
  // second hop executes on the destination chain after bridge settlement.
  if (hasDestinationSideSwap(route)) return false;
  const bridgeID = bridgeHops[0].bridge_id;
  if (bridgeID === "across") {
    if (isAcrossSwapTxRoute(route)) return false;
    return true;
  }
  // CelerCircleBridgeFacet is not deployed on every chain (e.g. Sepolia testnet).
  // Use the direct hop-by-hop depositForBurn path instead.
  if (bridgeID === "cctp") return false;
  return false;
}

// ── ERC-20 ABIs ───────────────────────────────────────────────────────────────

const ERC20_APPROVE_ABI = [{
  inputs: [{ name: "spender", type: "address" }, { name: "amount", type: "uint256" }],
  name: "approve", outputs: [{ name: "", type: "bool" }],
  stateMutability: "nonpayable", type: "function",
}] as const;

const ERC20_ALLOWANCE_ABI = [{
  inputs: [{ name: "owner", type: "address" }, { name: "spender", type: "address" }],
  name: "allowance", outputs: [{ name: "", type: "uint256" }],
  stateMutability: "view", type: "function",
}] as const;
const ERC20_BALANCE_ABI = [{
  type: "function",
  name: "balanceOf",
  stateMutability: "view",
  inputs: [{ name: "owner", type: "address" }],
  outputs: [{ name: "", type: "uint256" }],
}] as const;
const MAX_UINT256 = (2n ** 256n) - 1n;

const ZERO_ADDRESS = "0x0000000000000000000000000000000000000000";
const APPROVE_SELECTOR = "0x095ea7b3";

// ── CCTP helpers ─────────────────────────────────────────────────────────────

// keccak256("MessageSent(bytes)") — emitted by TokenMessenger on depositForBurn.
const MESSAGE_SENT_TOPIC = "0x8c5261668696ce22758910d05bab8f186d6eb247ceac2af2e82c7dc17669b036";

const RECEIVE_MESSAGE_ABI = [{
  name: "receiveMessage",
  type: "function",
  stateMutability: "nonpayable",
  inputs: [
    { name: "message", type: "bytes" },
    { name: "attestation", type: "bytes" },
  ],
  outputs: [{ name: "success", type: "bool" }],
}] as const;

// Polls Circle Iris via our backend proxy (/api/v1/cctp/attestation/:messageHash).
// Proxying avoids CORS restrictions on direct browser calls to iris-api.circle.com.
async function pollCCTPAttestation(
  messageHash: string,
  onAttempt: (n: number) => void,
  maxAttempts = 180,   // 180 × 10s = 30 minutes (testnet sandbox can be slow)
  intervalMs = 10000,
): Promise<string> {
  for (let i = 0; i < maxAttempts; i++) {
    onAttempt(i + 1);
    try {
      const res = await fetch(`/api/v1/cctp/attestation/${messageHash}`);
      if (res.ok) {
        const body = await res.json() as { status?: string; attestation?: string };
        if (body.status === "complete" && body.attestation) {
          return body.attestation;
        }
      }
      // 404 = not yet attested; any other non-ok status = keep polling
    } catch { /* transient error — keep polling */ }
    await new Promise<void>(r => setTimeout(r, intervalMs));
  }
  throw new Error(
    "CCTP attestation timed out (30 min). The USDC burn is on-chain. Visit https://cctp.money to claim manually.",
  );
}

// ── Allowance helpers ─────────────────────────────────────────────────────────

function resolveApprovalInfo(
  s: BridgeStepCall,
): { token: string; spender: string; amount: bigint; chainId: number } | null {
  if (s.approval) {
    return {
      token:   s.approval.token_contract,
      spender: s.approval.spender,
      amount:  BigInt(s.approval.amount),
      chainId: s.approval.chain_id || 0,
    };
  }
  if (s.tx?.params?.data && s.tx.contract) {
    const data = s.tx.params.data as string;
    if (data.startsWith(APPROVE_SELECTOR) && data.length >= 138) {
      const spender = "0x" + data.slice(34, 74);
      const amount  = BigInt("0x" + data.slice(74, 138));
      return { token: s.tx.contract, spender, amount, chainId: s.tx.chain_id || 0 };
    }
  }
  return null;
}

function encodeFromAbiFragment(abiFragment: string, funcName: string, params: Record<string, unknown>): `0x${string}` {
  const abiItem = JSON.parse(abiFragment);
  const abi: Abi = [abiItem];
  const inputs = abiItem.inputs as { name: string; type: string }[];
  const args = inputs.map(inp => {
    const v = params[inp.name];
    if (inp.type === "uint256" || inp.type === "uint32" || inp.type === "uint64")
      return BigInt(String(v ?? "0"));
    if (inp.type === "bytes" && typeof v === "string")
      return v as `0x${string}`;
    return v;
  });
  return encodeFunctionData({ abi, functionName: funcName, args });
}

async function sendBridgeStep(
  s: BridgeStepCall,
  sendTx: ReturnType<typeof useSendTransaction>["sendTransactionAsync"],
  writeContract: ReturnType<typeof useWriteContract>["writeContractAsync"],
): Promise<`0x${string}`> {
  if (s.step_type === "approve") {
    if (s.tx) {
      const data = s.tx.params?.data as string | undefined;
      if (data) {
        return await sendTx({
          to: s.tx.contract as `0x${string}`,
          data: data as `0x${string}`,
          value: BigInt(0),
          // Don't pass chainId — switchChainAsync already selected the network.
        });
      }
    }
    if (s.approval) {
      return await writeContract({
        address: asAddr(s.approval.token_contract),
        abi: ERC20_APPROVE_ABI,
        functionName: "approve",
        args: [asAddr(s.approval.spender), BigInt(s.approval.amount)],
        // Don't pass chainId — rely on the connected wallet chain.
      });
    }
  }

  if (s.step_type === "deposit" && s.tx) {
    const rawData = s.tx.params?.data as string | undefined;
    if (rawData) {
      return await sendTx({
        to: s.tx.contract as `0x${string}`,
        data: rawData as `0x${string}`,
        value: BigInt(s.tx.value || "0"),
        // Don't pass chainId — switchChainAsync already ran; passing it causes
        // "chain: undefined" if the chain isn't registered in the wagmi config.
      });
    }
    // ABI-based encoding (canonical bridges, structured steps)
    if (s.tx.abi_fragment && s.tx.function) {
      const calldata = encodeFromAbiFragment(
        s.tx.abi_fragment,
        s.tx.function,
        s.tx.params ?? {},
      );
      return await sendTx({
        to: s.tx.contract as `0x${string}`,
        data: calldata,
        value: BigInt(s.tx.value || "0"),
        // Don't pass chainId — switchChainAsync already ran.
      });
    }
  }

  throw new Error(`Unsupported step: ${s.step_type}`);
}

// ── Execution phases ──────────────────────────────────────────────────────────

type ExecPhase =
  | "idle"
  | "preparing"               // fetching tx data from backend
  | "approving"               // requesting token approval
  | "confirming"              // waiting for approval to mine
  | "executing"               // sending the main bridge/swap tx
  | "bridge_submitted"        // bridge deposit done; waiting for settlement before dest swap
  | "cctp_waiting_attestation" // CCTP: waiting for Circle Iris attestation after depositForBurn
  | "cctp_claiming"            // CCTP: submitting receiveMessage on destination chain
  | "done"
  | "error_retryable"         // network blip, rate limit — retry button
  | "error_action_required"   // wallet rejected, wrong chain, insufficient balance — user must act
  | "error_requote"           // quote expired, no routes — get a fresh quote
  | "error_terminal";         // contract reverted, invalid data — start over

const PHASE_LABELS: Record<ExecPhase, string> = {
  idle:                         "",
  preparing:                    "Preparing transaction…",
  approving:                    "Approve token spend in wallet…",
  confirming:                   "Waiting for approval confirmation…",
  executing:                    "Confirm bridge in wallet…",
  bridge_submitted:             "Bridge submitted — waiting for settlement",
  cctp_waiting_attestation:     "Waiting for Circle attestation…",
  cctp_claiming:                "Confirm claim in wallet…",
  done:                         "Transaction submitted",
  error_retryable:              "",
  error_action_required:        "",
  error_requote:                "",
  error_terminal:               "",
};

// ── Error classifier ──────────────────────────────────────────────────────────
// Maps errors to the correct ExecPhase and recovery action so every failure has
// a clear next step for the user.

type ErrorAction = "retry" | "switch_chain" | "requote" | "start_over" | "none";

type ClassifiedError = {
  phase: ExecPhase;
  message: string;
  action: ErrorAction;
};

function classifyError(e: unknown, srcChainName?: string): ClassifiedError {
  const msg = e instanceof Error ? e.message : "Transaction failed";
  const errType = (e as { errorType?: string }).errorType ?? "terminal";
  const errCode = (e as { errorCode?: string }).errorCode ?? "";

  // Wallet rejection / user cancelled — user action, show Try Again
  if (/user rejected|user denied|cancelled|denied transaction/i.test(msg)) {
    return { phase: "error_action_required", message: "Transaction was cancelled in your wallet.", action: "retry" };
  }
  // Wrong chain connected
  if (/wrong chain|chain mismatch|switch.*chain/i.test(msg) || errCode === "wrong_chain") {
    const chain = srcChainName ? ` Switch to ${srcChainName}.` : "";
    return { phase: "error_action_required", message: `Wrong chain connected.${chain}`, action: "switch_chain" };
  }
  // Insufficient balance
  if (/insufficient.*balance|not enough.*balance/i.test(msg)) {
    return { phase: "error_action_required", message: msg.slice(0, 200), action: "none" };
  }
  // Quote expired / no routes available
  if (errType === "requote" || /expired|stale|no routes|no liquidity|no available routes/i.test(msg) || errCode === "no_routes") {
    return { phase: "error_requote", message: "This quote has expired or prices changed. Get a fresh quote.", action: "requote" };
  }
  // Network / provider transient error — retry
  if (errType === "retryable" || /timeout|network|rate limit|503|429|econnrefused/i.test(msg) || errCode === "timeout") {
    return { phase: "error_retryable", message: "Network issue. This may be temporary.", action: "retry" };
  }
  // Default: terminal
  return { phase: "error_terminal", message: msg.slice(0, 200), action: "start_over" };
}

type ApprovalHint = {
  token: string;
  spender: string;
  required: bigint;
  chainId: number;
  allowance?: bigint;
};

function hasDestinationSideSwap(route: Route): boolean {
  const bridgeIdx = route.hops.findIndex(h => h.hop_type === "bridge");
  if (bridgeIdx < 0) return false;
  return route.hops.slice(bridgeIdx + 1).some(h => h.hop_type === "swap");
}


// ── Main component ────────────────────────────────────────────────────────────

export function ExecutePanel({ route, quotedAt }: { route: Route; quotedAt?: number }) {
  const { address: walletAddress } = useAccount();
  const currentChainId = useChainId();
  const { switchChainAsync } = useSwitchChain();
  const { sendTransactionAsync } = useSendTransaction();
  const { writeContractAsync } = useWriteContract();
  const publicClient = usePublicClient();

  // Solana wallet — used when source chain is Solana.
  const { sendTransaction: sendSolanaTransaction, publicKey: solanaPublicKey } = useWallet();
  const { connection } = useConnection();

  const [phase, setPhase] = useState<ExecPhase>("idle");
  const [txHash, setTxHash] = useState("");
  const [approveTxHash, setApproveTxHash] = useState("");
  const [resumeHopIdx, setResumeHopIdx] = useState(-1);
  const [bridgeSettled, setBridgeSettled] = useState(false);
  const [resumeCountdown, setResumeCountdown] = useState<number | null>(null);
  const [quoteAgeSec, setQuoteAgeSec] = useState(0);
  const [error, setError] = useState("");
  const [errorAction, setErrorAction] = useState<ErrorAction>("none");
  const [retryCount, setRetryCount] = useState(0);
  const [approvalHints, setApprovalHints] = useState<ApprovalHint[]>([]);
  const [approvalHintsLoading, setApprovalHintsLoading] = useState(false);
  const [gasEstimate, setGasEstimate] = useState<bigint | null>(null);
  const [gasPrice, setGasPrice] = useState<bigint | null>(null);
  const [approvalMode, setApprovalMode] = useState<"exact" | "unlimited">(() => {
    if (typeof window === "undefined") return "exact";
    return window.localStorage.getItem("approval_mode") === "unlimited" ? "unlimited" : "exact";
  });
  // CCTP-specific state
  const [cctpPollCount, setCctpPollCount] = useState(0);
  const [cctpClaimDone, setCctpClaimDone] = useState(false);

  const srcHop = route.hops[0];
  const dstHop = route.hops[route.hops.length - 1];
  const srcChainId = CHAIN_ID[srcHop.from_chain?.toLowerCase()] ?? 0;
  const dstChainId = CHAIN_ID[dstHop.to_chain?.toLowerCase()] ?? 0;
  const srcDec = tokenDecimals(srcChainId, srcHop.from_asset);
  const dstDec = tokenDecimals(dstChainId, dstHop.to_asset);
  const srcIsSolana = srcHop.from_chain?.toLowerCase() === "solana";

  const useMultiStep = isAcrossSwapTxRoute(route);
  const canUseLiFi = isLiFiCompatible(route);
  const hasDestinationSwap = hasDestinationSideSwap(route);
  const executionSupported = route.execution?.supported ?? true;
  const useHopByHop = executionSupported && route.hops.length > 0;
  const canExecute = (canUseLiFi && !useMultiStep && !hasDestinationSwap) || useHopByHop;
  const intent = route.execution?.intent;
  const isAsyncClaim = intent === "async_claim";
  const isCanonicalWithdrawal = route.execution?.metadata?.execution_path === "step_transaction_withdrawal";
  const bridgeLabel = route.hops.filter(h => h.hop_type === "bridge").map(h => hopLabel(h.bridge_id)).join(" + ");

  // Public client for the destination chain — used for settlement polling.
  const dstPublicClient = usePublicClient({ chainId: dstChainId || undefined });

  // ── Quote freshness ───────────────────────────────────────────────────────
  // Track age of the current quote and warn / block when it expires.
  const acrossDeadline = (() => {
    const bh = route.hops.find(h => h.bridge_id === "across");
    if (!bh?.provider_data) return null;
    const pd = bh.provider_data as Record<string, unknown>;
    const dep = pd.deposit as { fillDeadline?: number } | undefined;
    return dep?.fillDeadline ?? null;
  })();

  useEffect(() => {
    if (!quotedAt) return;
    const id = setInterval(() => {
      setQuoteAgeSec(Math.floor((Date.now() - quotedAt) / 1000));
    }, 1000);
    return () => clearInterval(id);
  }, [quotedAt]);

  const isQuoteExpired =
    (acrossDeadline ? Math.floor(Date.now() / 1000) > acrossDeadline : false) ||
    (quotedAt ? quoteAgeSec > 90 : false);

  const quoteCountdown = acrossDeadline
    ? Math.max(0, acrossDeadline - Math.floor(Date.now() / 1000))
    : quotedAt ? Math.max(0, 90 - quoteAgeSec) : null;

  useEffect(() => {
    setPhase("idle");
    setTxHash("");
    setApproveTxHash("");
    setError("");
    setErrorAction("none");
    setRetryCount(0);
    setResumeHopIdx(-1);
    setBridgeSettled(false);
    setResumeCountdown(null);
    setGasEstimate(null);
    setGasPrice(null);
    setCctpPollCount(0);
    setCctpClaimDone(false);
  }, [route.route_id]);

  // ── Bridge settlement polling ─────────────────────────────────────────────
  // After a bridge deposit, poll the destination chain every 5s until funds arrive.
  useEffect(() => {
    if (phase !== "bridge_submitted" || resumeHopIdx < 0 || !dstPublicClient || !walletAddress) return;
    setBridgeSettled(false);

    const bridgeHop = route.hops.find(h => h.hop_type === "bridge");
    const watchToken = bridgeHop?.to_token_address ?? "";
    const isNativeWatch = !watchToken
      || watchToken === ZERO_ADDRESS
      || watchToken.toLowerCase() === "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee";

    let initialBalance: bigint | null = null;

    const poll = async () => {
      try {
        const bal: bigint = isNativeWatch
          ? await dstPublicClient.getBalance({ address: asAddr(walletAddress) })
          : await dstPublicClient.readContract({
              address: asAddr(watchToken),
              abi: ERC20_BALANCE_ABI,
              functionName: "balanceOf",
              args: [asAddr(walletAddress)],
            }) as bigint;
        if (initialBalance === null) { initialBalance = bal; return; }
        if (bal > initialBalance) setBridgeSettled(true);
      } catch { /* ignore polling errors */ }
    };

    poll();
    const balId = setInterval(poll, 5000);

    // For Across routes: also poll the backend status endpoint.
    // Whichever track fires first (balance or status) wins.
    let statusId: ReturnType<typeof setInterval> | null = null;
    if (bridgeHop?.bridge_id === "across" && txHash) {
      const pollStatus = async () => {
        try {
          const result = await fetchTransactionStatus(txHash);
          if (result.status === "completed") setBridgeSettled(true);
        } catch { /* ignore */ }
      };
      statusId = setInterval(pollStatus, 5000);
    }

    return () => {
      clearInterval(balId);
      if (statusId) clearInterval(statusId);
    };
  }, [phase, resumeHopIdx, dstPublicClient, walletAddress, route, dstChainId, txHash]);

  // ── Unified single-click execute ──────────────────────────────────────────
  const handleExecute = useCallback(async () => {
    if (srcIsSolana) {
      if (!solanaPublicKey) {
        setError("Connect Solana wallet first");
        return;
      }
    } else {
      if (!walletAddress || !publicClient) {
        setError("Connect EVM wallet first");
        return;
      }
    }
    setError("");
    setTxHash("");
    setApproveTxHash("");

    try {
      // Basic pre-submit balance check (EVM only — Solana balance checked by wallet).
      if (!srcIsSolana && publicClient && walletAddress && srcHop?.amount_in_base_units) {
        const required = BigInt(srcHop.amount_in_base_units);
        const token = tokenAddress(srcChainId, srcHop.from_asset);
        let balance = 0n;
        if (token && token !== ZERO_ADDRESS) {
          balance = await publicClient.readContract({
            address: asAddr(token),
            abi: ERC20_BALANCE_ABI,
            functionName: "balanceOf",
            args: [asAddr(walletAddress)],
          }) as bigint;
        } else {
          balance = await publicClient.getBalance({ address: asAddr(walletAddress) });
        }
        if (balance < required) {
          throw new Error(
            `Insufficient ${srcHop.from_asset} balance. Need ${fmtAmt(required.toString(), srcDec)} ${srcHop.from_asset}, have ${fmtAmt(balance.toString(), srcDec)}.`,
          );
        }
      }

      if (canUseLiFi && !useMultiStep) {
        // ── LiFi Diamond path ──
        setPhase("preparing");
        const resp = await fetchBuildTransaction(route, walletAddress);

        const sendingAsset = resp.bridge_data?.sendingAssetId ?? "";
        const minAmount = BigInt(resp.bridge_data?.minAmount ?? "0");
        const isNative = !sendingAsset || sendingAsset.toLowerCase() === ZERO_ADDRESS;
        const diamond = resp.diamond as `0x${string}`;
        const chainId = resp.chain_id || undefined;

        if (!isNative && minAmount > 0n) {
          const allowance = await publicClient.readContract({
            address: asAddr(sendingAsset),
            abi: ERC20_ALLOWANCE_ABI,
            functionName: "allowance",
            args: [asAddr(walletAddress), asAddr(diamond)],
          }) as bigint;

          if (allowance < minAmount) {
            setPhase("approving");
            const approveAmount = approvalMode === "unlimited" ? MAX_UINT256 : minAmount;
            const aHash = await writeContractAsync({
              address: asAddr(sendingAsset),
              abi: ERC20_APPROVE_ABI,
              functionName: "approve",
              args: [asAddr(diamond), approveAmount],
              chainId,
            });
            setApproveTxHash(aHash);
            setPhase("confirming");
            await publicClient.waitForTransactionReceipt({ hash: aHash, retryCount: 60 });
          }
        }

        setPhase("executing");
        const hash = await executeLiFiDiamond(resp, diamond, chainId, writeContractAsync);
        setTxHash(hash);
        setPhase("done");

      } else if (useHopByHop) {
        // ── Unified hop-by-hop stepTransaction path (swap + bridge hops) ──
        setPhase("preparing");
        for (let hopIdx = 0; hopIdx < route.hops.length; hopIdx += 1) {
          const hop = route.hops[hopIdx];
          const data = await fetchStepTransaction(route, hopIdx, walletAddress, walletAddress);

          // ── Solana-signed hop (Mayan from Solana) ──
          if (data.solana_tx) {
            if (!solanaPublicKey) throw new Error("Solana wallet not connected");
            setPhase("executing");
            const txBytes = Buffer.from(data.solana_tx.serialized_tx, "base64");
            let solanaTx: VersionedTransaction | Transaction;
            try {
              solanaTx = VersionedTransaction.deserialize(txBytes);
            } catch {
              solanaTx = Transaction.from(txBytes);
            }
            const sig = await sendSolanaTransaction(solanaTx, connection);
            setTxHash(sig);
            setPhase("done");
            return;
          }

          if (data.hop_type === "swap") {
            if (!data.tx?.to || !data.tx?.data) {
              throw new Error(`No executable swap transaction returned for hop ${hopIdx + 1}`);
            }
            setPhase("executing");
            const swapGasLimit = data.tx.gas_limit && data.tx.gas_limit !== "0" ? BigInt(data.tx.gas_limit) : undefined;
            const hash = await sendTransactionAsync({
              to: asAddr(data.tx.to),
              data: data.tx.data as `0x${string}`,
              value: BigInt(data.tx.value || "0"),
              chainId: chainIdOf(hop.from_chain) || undefined,
              ...(swapGasLimit && { gas: swapGasLimit }),
            });
            setTxHash(hash);
            continue;
          }

          const allSteps = data.bridge_params?.steps ?? [];
          if (allSteps.length === 0) throw new Error(`No execution steps returned for hop ${hopIdx + 1}`);
          const hopChainId = chainIdOf(hop.from_chain) || undefined;
          const isCCTP = data.bridge_params?.protocol === "circle_cctp";
          // Holds CCTP message bytes + attestation obtained after depositForBurn confirms.
          let cctpClaimData: { messageBytes: `0x${string}`; attestation: `0x${string}` } | null = null;

          for (const step of allSteps) {
            if (step.step_type === "approve") {
              const info = resolveApprovalInfo(step);
              if (info) {
                const onChain = await publicClient.readContract({
                  address: asAddr(info.token),
                  abi: ERC20_ALLOWANCE_ABI,
                  functionName: "allowance",
                  args: [asAddr(walletAddress), asAddr(info.spender)],
                }) as bigint;

                if (onChain >= info.amount) continue;
                setPhase("approving");
                const approveAmount = approvalMode === "unlimited" ? MAX_UINT256 : info.amount;
                const aHash = await writeContractAsync({
                  address: asAddr(info.token),
                  abi: ERC20_APPROVE_ABI,
                  functionName: "approve",
                  args: [asAddr(info.spender), approveAmount],
                  chainId: info.chainId || hopChainId,
                });
                setApproveTxHash(aHash);
                setPhase("confirming");
                await publicClient.waitForTransactionReceipt({ hash: aHash, retryCount: 60 });
                continue;
              }

              setPhase("approving");
              const aHash = await sendBridgeStep(step, sendTransactionAsync, writeContractAsync);
              setApproveTxHash(aHash);
              setPhase("confirming");
              await publicClient.waitForTransactionReceipt({ hash: aHash, retryCount: 60 });
              continue;
            }

            if (step.step_type === "deposit") {
              setPhase("executing");
              const hash = await sendBridgeStep(step, sendTransactionAsync, writeContractAsync);
              setTxHash(hash);

              // CCTP: wait for depositForBurn receipt, extract MessageSent bytes,
              // then poll Circle Iris until attestation is ready.
              if (isCCTP) {
                setCctpPollCount(0);
                setPhase("cctp_waiting_attestation");
                const receipt = await publicClient.waitForTransactionReceipt({ hash, retryCount: 60 });
                const sentLog = receipt.logs.find(
                  l => l.topics[0]?.toLowerCase() === MESSAGE_SENT_TOPIC,
                );
                if (!sentLog) throw new Error("CCTP: MessageSent event not found in deposit receipt");
                const [messageBytes] = decodeAbiParameters(
                  [{ type: "bytes" }],
                  sentLog.data as `0x${string}`,
                );
                const messageHash = keccak256(messageBytes as `0x${string}`);
                const attestation = await pollCCTPAttestation(messageHash, setCctpPollCount);
                cctpClaimData = {
                  messageBytes: messageBytes as `0x${string}`,
                  attestation: attestation as `0x${string}`,
                };
              }
              continue;
            }

            // CCTP claim step: switch to destination chain, call receiveMessage.
            if (step.step_type === "claim" && isCCTP) {
              if (!cctpClaimData || !step.tx?.contract) {
                throw new Error("CCTP: claim data unavailable — deposit may not have been mined");
              }
              const claimChainId = step.tx.chain_id;
              if (claimChainId && currentChainId !== claimChainId) {
                await switchChainAsync({ chainId: claimChainId });
              }
              setPhase("cctp_claiming");
              const claimHash = await writeContractAsync({
                address: asAddr(step.tx.contract),
                abi: RECEIVE_MESSAGE_ABI,
                functionName: "receiveMessage",
                args: [cctpClaimData.messageBytes, cctpClaimData.attestation],
                chainId: claimChainId || undefined,
              });
              setTxHash(claimHash);
              setCctpClaimDone(true);
              cctpClaimData = null;
              continue;
            }
          }

          // After a bridge deposit: if the next hop runs on a different chain (destination swap),
          // pause here so the user can wait for bridge settlement before executing on dest chain.
          const nextHop = route.hops[hopIdx + 1];
          if (hop.hop_type === "bridge" && nextHop && nextHop.from_chain?.toLowerCase() !== hop.to_chain?.toLowerCase()) {
            setResumeHopIdx(hopIdx + 1);
            setPhase("bridge_submitted");
            return;
          }
        }
        setPhase("done");

      } else {
        setError(`Direct execution not supported for ${bridgeLabel}`);
      }
    } catch (e) {
      const classified = classifyError(e, srcHop.from_chain);
      // For retryable errors: escalate to terminal after 3 attempts.
      if (classified.phase === "error_retryable" && retryCount >= 3) {
        setError("Still failing after 3 attempts. " + classified.message);
        setErrorAction("start_over");
        setPhase("error_terminal");
      } else {
        setError(classified.message);
        setErrorAction(classified.action);
        setPhase(classified.phase);
      }
    }
  }, [walletAddress, publicClient, route, canUseLiFi, useMultiStep, useHopByHop, bridgeLabel, srcChainId, dstChainId, srcHop, dstHop, srcDec, dstDec, sendTransactionAsync, writeContractAsync, approvalMode, srcIsSolana, solanaPublicKey, sendSolanaTransaction, connection, retryCount, currentChainId, switchChainAsync]);

  // Resume execution from resumeHopIdx after bridge has settled on destination.
  const handleResumeSwap = useCallback(async () => {
    if (!walletAddress || !publicClient || resumeHopIdx < 0) return;
    setResumeCountdown(null);
    setError("");
    setPhase("preparing");
    try {
      // Switch to destination chain before executing destination hops.
      const resumeChainId = chainIdOf(route.hops[resumeHopIdx]?.from_chain);
      if (resumeChainId && currentChainId !== resumeChainId) {
        await switchChainAsync({ chainId: resumeChainId });
      }
      for (let hopIdx = resumeHopIdx; hopIdx < route.hops.length; hopIdx += 1) {
        const hop = route.hops[hopIdx];
        const data = await fetchStepTransaction(route, hopIdx, walletAddress, walletAddress);
        if (data.hop_type === "swap") {
          if (!data.tx?.to || !data.tx?.data) {
            throw new Error(`No executable swap transaction returned for hop ${hopIdx + 1}`);
          }
          setPhase("executing");
          const gasLimit = data.tx.gas_limit && data.tx.gas_limit !== "0" ? BigInt(data.tx.gas_limit) : undefined;
          const hash = await sendTransactionAsync({
            to: asAddr(data.tx.to),
            data: data.tx.data as `0x${string}`,
            value: BigInt(data.tx.value || "0"),
            chainId: chainIdOf(hop.from_chain) || undefined,
            ...(gasLimit && { gas: gasLimit }),
          });
          setTxHash(hash);
        }
      }
      setPhase("done");
    } catch (e) {
      const classified = classifyError(e, route.hops[resumeHopIdx]?.from_chain);
      setError(classified.message);
      setErrorAction(classified.action);
      setPhase(classified.phase);
    }
  }, [walletAddress, publicClient, route, resumeHopIdx, sendTransactionAsync, switchChainAsync, currentChainId]);

  // ── Auto-resume after bridge settlement ──────────────────────────────────
  useEffect(() => {
    if (!bridgeSettled || phase !== "bridge_submitted") return;
    setResumeCountdown(3);
    const tick = setInterval(() => setResumeCountdown(n => n !== null && n > 0 ? n - 1 : n), 1000);
    const go = setTimeout(() => handleResumeSwap(), 3000);
    return () => { clearInterval(tick); clearTimeout(go); };
  }, [bridgeSettled, phase, handleResumeSwap]);

  const isErrorPhase = phase === "error_retryable" || phase === "error_action_required" || phase === "error_requote" || phase === "error_terminal";
  const isActive = phase !== "idle" && phase !== "done" && !isErrorPhase
    && phase !== "bridge_submitted"
    && phase !== "cctp_waiting_attestation";
  const needsChainSwitch = srcChainId !== 0 && currentChainId !== srcChainId;

  // Low-value L1 warning
  const isLowValueL1 = (() => {
    if (srcChainId !== 1) return false;
    try { return Number(srcHop.amount_in_base_units ?? "0") / 10 ** srcDec < 5; }
    catch { return false; }
  })();

  useEffect(() => {
    let cancelled = false;
    const loadApprovalHints = async () => {
      if (!walletAddress || !publicClient || !executionSupported || !canExecute) {
        setApprovalHints([]);
        return;
      }
      setApprovalHintsLoading(true);
      try {
        const hints: ApprovalHint[] = [];
        if (canUseLiFi && !useMultiStep) {
          const resp = await fetchBuildTransaction(route, walletAddress);
          const token = (resp.bridge_data?.sendingAssetId ?? "").toLowerCase();
          const required = BigInt(resp.bridge_data?.minAmount ?? "0");
          const spender = (resp.diamond ?? "").toLowerCase();
          if (token && token !== ZERO_ADDRESS && spender && required > 0n) {
            const allowance = await publicClient.readContract({
              address: asAddr(token),
              abi: ERC20_ALLOWANCE_ABI,
              functionName: "allowance",
              args: [asAddr(walletAddress), asAddr(spender)],
            }) as bigint;
            hints.push({ token, spender, required, allowance, chainId: srcChainId });
          }
        } else {
          const bridgeHopIdx = route.hops.findIndex(h => h.hop_type === "bridge");
          if (bridgeHopIdx >= 0) {
            const data = await fetchStepTransaction(route, bridgeHopIdx, walletAddress, walletAddress);
            for (const step of data.bridge_params?.steps ?? []) {
              if (step.step_type !== "approve") continue;
              const info = resolveApprovalInfo(step);
              if (!info) continue;
              const token = info.token.toLowerCase();
              const spender = info.spender.toLowerCase();
              if (!token || !spender || token === ZERO_ADDRESS) continue;
              const allowance = await publicClient.readContract({
                address: asAddr(token),
                abi: ERC20_ALLOWANCE_ABI,
                functionName: "allowance",
                args: [asAddr(walletAddress), asAddr(spender)],
              }) as bigint;
              hints.push({ token, spender, required: info.amount, allowance, chainId: srcChainId });
            }
          }
        }
        if (!cancelled) setApprovalHints(hints);

        // ── Gas estimation ──
        try {
          const gp = await publicClient.getGasPrice();
          if (!cancelled) setGasPrice(gp);
          if (canUseLiFi && !useMultiStep) {
            // LiFi path: try to estimate gas using the encoded calldata from buildTransaction.
            // The resp is already fetched above.
          } else {
            // Hop-by-hop: use gas_limit from first swap hop's step_tx if available.
            const swapHopIdx = route.hops.findIndex(h => h.hop_type === "swap");
            if (swapHopIdx >= 0 && walletAddress) {
              try {
                const sd = await fetchStepTransaction(route, swapHopIdx, walletAddress, walletAddress);
                if (sd.tx?.gas_limit && sd.tx.gas_limit !== "0") {
                  if (!cancelled) setGasEstimate(BigInt(sd.tx.gas_limit));
                }
              } catch { /* ignore */ }
            }
          }
        } catch { /* ignore gas errors */ }
      } catch {
        if (!cancelled) setApprovalHints([]);
      } finally {
        if (!cancelled) setApprovalHintsLoading(false);
      }
    };
    loadApprovalHints();
    return () => { cancelled = true; };
  }, [walletAddress, publicClient, route, canUseLiFi, useMultiStep, executionSupported, canExecute, srcChainId]);


  return (
    <div style={{ border: "1px solid rgba(69,69,85,0.40)", backgroundColor: "#2a2a2a" }}>

      {/* ── Header: protocol label + execution badge ── */}
      <div className="flex items-center justify-between px-6 pt-6 pb-5">
        <div>
          <p className="text-[9px] font-mono uppercase tracking-[0.25em] mb-1" style={{ color: "#908fa1" }}>Via</p>
          <p className="text-xl font-bold" style={{ color: "#e5e2e1" }}>{bridgeLabel}</p>
        </div>
        <span
          className="text-[10px] font-mono px-2.5 py-1.5 uppercase tracking-wider shrink-0"
          style={{
            backgroundColor: route.execution?.supported
              ? route.execution?.intent === "atomic_one_click" ? "rgba(190,194,255,0.15)" : "rgba(198,197,216,0.10)"
              : "rgba(255,180,171,0.12)",
            color: route.execution?.supported
              ? route.execution?.intent === "atomic_one_click" ? "#bec2ff" : "#c6c5d8"
              : "#ffb4ab",
          }}
        >
          {route.execution?.supported ? intentLabel(route.execution?.intent) : "Quote only"}
        </span>
      </div>

      {/* ── YOU SEND ── */}
      <div className="mx-6" style={{ backgroundColor: "#0e0e0e", border: "1px solid rgba(69,69,85,0.40)" }}>
        <p className="px-5 pt-4 pb-3 text-[9px] font-mono uppercase tracking-[0.25em]" style={{ color: "#908fa1" }}>
          You Send
        </p>
        <div className="px-5 pb-5 flex items-center gap-4">
          <TokenIcon chainId={srcChainId} address={tokenAddress(srcChainId, srcHop.from_asset)} symbol={srcHop.from_asset} size={48} />
          <div className="min-w-0">
            <div className="text-4xl font-bold font-mono tabular-nums leading-none" style={{ color: "#e5e2e1" }}>
              {fmtAmt(srcHop.amount_in_base_units ?? "0", srcDec)}
            </div>
            <div className="flex items-center gap-1.5 mt-2.5">
              <span className="text-sm font-mono" style={{ color: "#908fa1" }}>{srcHop.from_asset}</span>
              <span style={{ color: "rgba(69,69,85,0.60)" }}>·</span>
              <ChainIcon chainId={chainIdOf(srcHop.from_chain)} size={12} />
              <span className="text-sm font-mono" style={{ color: "#bec2ff" }}>{chainName(srcHop.from_chain)}</span>
            </div>
          </div>
        </div>
      </div>

      {/* ── Connector ── */}
      <div className="mx-6 py-3 flex items-center justify-center relative">
        <div className="absolute inset-x-0 top-1/2 -translate-y-px" style={{ height: "1px", backgroundColor: "rgba(69,69,85,0.30)" }} />
        <div className="relative flex items-center gap-2.5 px-4 py-1.5 z-10" style={{ backgroundColor: "#2a2a2a" }}>
          <span className="font-mono" style={{ color: "rgba(69,69,85,0.80)", fontSize: "16px" }}>↓</span>
          <span className="text-[11px] font-mono font-semibold" style={{ color: "#c6c5d8" }}>{bridgeLabel}</span>
          {route.estimated_time_seconds > 0 && (
            <>
              <span style={{ color: "rgba(69,69,85,0.60)" }}>·</span>
              <span className="text-[11px] font-mono" style={{ color: "#908fa1" }}>
                ~{route.estimated_time_seconds < 120 ? `${route.estimated_time_seconds}s` : `${Math.round(route.estimated_time_seconds / 60)}m`}
              </span>
            </>
          )}
          {route.total_fee && route.total_fee !== "0" && (
            <>
              <span style={{ color: "rgba(69,69,85,0.60)" }}>·</span>
              <span className="text-[11px] font-mono" style={{ color: "#908fa1" }}>
                ${Number(route.total_fee).toFixed(2)}
              </span>
            </>
          )}
        </div>
      </div>

      {/* ── YOU RECEIVE ── */}
      <div className="mx-6 mb-5" style={{ backgroundColor: "#0e0e0e", border: "1px solid rgba(69,69,85,0.40)" }}>
        <p className="px-5 pt-4 pb-3 text-[9px] font-mono uppercase tracking-[0.25em]" style={{ color: "#908fa1" }}>
          You Receive <span style={{ opacity: 0.5 }}>(Estimated)</span>
        </p>
        <div className="px-5 pb-5 flex items-center gap-4">
          <TokenIcon chainId={dstChainId} address={tokenAddress(dstChainId, dstHop.to_asset)} symbol={dstHop.to_asset} size={48} />
          <div className="min-w-0">
            <div className="text-4xl font-bold font-mono tabular-nums leading-none" style={{ color: "#e5e2e1" }}>
              <span style={{ color: "#454555", fontWeight: 400 }}>~</span>
              {fmtAmt(route.estimated_output_amount, dstDec)}
            </div>
            <div className="flex items-center gap-1.5 mt-2.5">
              <span className="text-sm font-mono" style={{ color: "#908fa1" }}>{dstHop.to_asset}</span>
              <span style={{ color: "rgba(69,69,85,0.60)" }}>·</span>
              <ChainIcon chainId={chainIdOf(dstHop.to_chain)} size={12} />
              <span className="text-sm font-mono" style={{ color: "#bec2ff" }}>{chainName(dstHop.to_chain)}</span>
            </div>
          </div>
        </div>
      </div>

      {/* ── Divider ── */}
      <div style={{ height: "1px", backgroundColor: "rgba(69,69,85,0.20)" }} />

      {/* ── Details + approval (idle state) ── */}
      {phase === "idle" && executionSupported && canExecute && (
        <div className="px-6 py-5 space-y-4">
          <div className="space-y-3">
            {walletAddress && (
              <div className="flex items-center justify-between">
                <span className="text-[10px] font-mono uppercase tracking-widest" style={{ color: "#908fa1" }}>Recipient</span>
                <span className="text-[11px] font-mono" style={{ color: "#c6c5d8" }}>
                  {walletAddress.slice(0, 6)}…{walletAddress.slice(-4)}
                </span>
              </div>
            )}
            {approvalHints[0]?.spender && (
              <div className="flex items-center justify-between">
                <span className="text-[10px] font-mono uppercase tracking-widest" style={{ color: "#908fa1" }}>Spender</span>
                <span className="text-[11px] font-mono" style={{ color: "#c6c5d8" }}>
                  {approvalHints[0].spender.slice(0, 8)}…{approvalHints[0].spender.slice(-6)}
                </span>
              </div>
            )}
            {approvalHintsLoading ? (
              <div className="flex items-center justify-between">
                <span className="text-[10px] font-mono uppercase tracking-widest" style={{ color: "#908fa1" }}>Allowance</span>
                <span className="text-[11px] font-mono animate-pulse" style={{ color: "#908fa1" }}>Checking…</span>
              </div>
            ) : approvalHints.map((h, i) => {
              const ok = (h.allowance ?? 0n) >= h.required;
              return (
                <div key={`${h.spender}-${i}`} className="flex items-center justify-between">
                  <span className="text-[10px] font-mono uppercase tracking-widest" style={{ color: "#908fa1" }}>Allowance</span>
                  <span className="text-[11px] font-mono flex items-center gap-1.5" style={{ color: ok ? "#bec2ff" : "#ffb4ab" }}>
                    {ok ? "✓ Approved" : "✗ Needs approval"}
                  </span>
                </div>
              );
            })}
          </div>

          {/* Approval mode toggle */}
          <div className="flex items-center justify-between pt-1">
            <span className="text-[10px] font-mono uppercase tracking-widest" style={{ color: "#908fa1" }}>Approval</span>
            <div className="flex items-center gap-px" style={{ border: "1px solid rgba(69,69,85,0.40)" }}>
              <button
                type="button"
                onClick={() => { setApprovalMode("exact"); window.localStorage.setItem("approval_mode", "exact"); }}
                className="px-3 py-1.5 text-[10px] font-mono uppercase tracking-wider transition-colors"
                style={{
                  backgroundColor: approvalMode === "exact" ? "#c6c6c7" : "transparent",
                  color: approvalMode === "exact" ? "#131313" : "#908fa1",
                }}
              >
                Exact
              </button>
              <button
                type="button"
                onClick={() => { setApprovalMode("unlimited"); window.localStorage.setItem("approval_mode", "unlimited"); }}
                className="px-3 py-1.5 text-[10px] font-mono uppercase tracking-wider transition-colors"
                style={{
                  backgroundColor: approvalMode === "unlimited" ? "#c6c6c7" : "transparent",
                  color: approvalMode === "unlimited" ? "#131313" : "#908fa1",
                }}
              >
                Unlimited
              </button>
            </div>
          </div>

          {approvalMode === "unlimited" && (
            <p className="text-[10px] leading-relaxed pl-3" style={{ color: "rgba(255,180,171,0.60)", borderLeft: "2px solid rgba(255,180,171,0.20)" }}>
              Unlimited approval grants the spender ongoing access to this token until revoked.
            </p>
          )}
        </div>
      )}

      {/* ── Divider ── */}
      <div style={{ height: "1px", backgroundColor: "rgba(69,69,85,0.20)" }} />

      {/* ── Action area ── */}
      <div className="px-6 py-5 space-y-3">

        {needsChainSwitch && (
          <div className="flex items-center justify-between px-4 py-3" style={{ backgroundColor: "#1c1b1b", border: "1px solid rgba(69,69,85,0.40)" }}>
            <p className="text-xs text-err">Switch to <span className="font-semibold capitalize">{srcHop.from_chain}</span></p>
            <button
              onClick={() => switchChainAsync({ chainId: srcChainId }).catch(() => {})}
              className="text-xs bg-accent text-surface px-3 py-1 font-semibold hover:opacity-90 transition-opacity"
            >
              Switch
            </button>
          </div>
        )}

        {isLowValueL1 && phase === "idle" && (
          <p className="text-[11px] leading-relaxed pl-3" style={{ color: "rgba(255,180,171,0.70)", borderLeft: "2px solid rgba(255,180,171,0.20)" }}>
            Gas on Ethereum L1 can exceed $10–100. Consider a larger amount or L2 origin.
          </p>
        )}

        {!executionSupported && (
          <div className="px-4 py-3" style={{ backgroundColor: "#1c1b1b", border: "1px solid rgba(69,69,85,0.40)" }}>
            <p className="text-xs text-on-surface-variant">
              Quote-only — execution not yet supported.
              {!!route.execution?.reasons?.length && (
                <span className="text-outline ml-1">({route.execution.reasons.join(", ")})</span>
              )}
            </p>
          </div>
        )}

        {isErrorPhase && error && (
          <div className="px-4 py-3 space-y-2" style={{ backgroundColor: "rgba(147,0,10,0.15)", borderLeft: "2px solid #ffb4ab" }}>
            <p className="text-xs leading-relaxed" style={{ color: "#ffb4ab" }}>{error}</p>
            <div className="flex gap-2 flex-wrap">
              {(errorAction === "retry" || phase === "error_retryable") && (
                <button
                  onClick={() => { setRetryCount(n => n + 1); handleExecute(); }}
                  className="text-[11px] font-mono uppercase tracking-wider px-3 py-1.5"
                  style={{ backgroundColor: "rgba(190,194,255,0.15)", border: "1px solid rgba(190,194,255,0.30)", color: "#bec2ff" }}
                >
                  {phase === "error_retryable" ? "Retry" : "Try Again"}
                </button>
              )}
              {errorAction === "switch_chain" && srcChainId !== 0 && (
                <button
                  onClick={() => switchChainAsync({ chainId: srcChainId }).then(handleExecute).catch(() => {})}
                  className="text-[11px] font-mono uppercase tracking-wider px-3 py-1.5"
                  style={{ backgroundColor: "rgba(190,194,255,0.15)", border: "1px solid rgba(190,194,255,0.30)", color: "#bec2ff" }}
                >
                  Switch Chain
                </button>
              )}
              {phase === "error_requote" && (
                <button
                  onClick={() => { setPhase("idle"); setError(""); setErrorAction("none"); setRetryCount(0); }}
                  className="text-[11px] font-mono uppercase tracking-wider px-3 py-1.5"
                  style={{ backgroundColor: "rgba(190,194,255,0.10)", border: "1px solid rgba(190,194,255,0.20)", color: "#bec2ff" }}
                >
                  New Quote
                </button>
              )}
              {(phase === "error_terminal" || phase === "error_action_required") && errorAction === "start_over" && (
                <button
                  onClick={() => { setPhase("idle"); setError(""); setErrorAction("none"); setRetryCount(0); }}
                  className="text-[11px] font-mono uppercase tracking-wider px-3 py-1.5"
                  style={{ backgroundColor: "rgba(190,194,255,0.10)", border: "1px solid rgba(190,194,255,0.20)", color: "#908fa1" }}
                >
                  Start Over
                </button>
              )}
            </div>
          </div>
        )}

        {isActive && (
          <div className="flex items-center gap-3 py-1">
            <span className="relative flex h-3 w-3">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-accent/40" />
              <span className="relative inline-flex rounded-full h-3 w-3 bg-accent/80" />
            </span>
            <AnimatePresence mode="wait">
              <motion.span
                key={phase}
                initial={{ opacity: 0, y: 4 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -4 }}
                transition={{ duration: 0.18, ease: "easeOut" }}
                className="text-sm text-on-surface-variant"
              >
                {PHASE_LABELS[phase]}
              </motion.span>
            </AnimatePresence>
          </div>
        )}

        {approveTxHash && phase !== "idle" && (
          <a href={explorerTx(srcChainId, approveTxHash)} target="_blank" rel="noopener noreferrer"
            className="text-[11px] font-mono text-accent/60 hover:text-accent block">
            Approval tx: {approveTxHash.slice(0, 10)}…{approveTxHash.slice(-6)} ↗
          </a>
        )}

        {phase === "cctp_waiting_attestation" && (
          <div className="px-5 py-4 space-y-2" style={{ backgroundColor: "#1c1b1b", border: "1px solid rgba(190,194,255,0.20)" }}>
            <div className="flex items-center gap-2">
              <span className="animate-spin inline-block w-4 h-4 border-2 rounded-full shrink-0"
                style={{ borderColor: "rgba(190,194,255,0.25)", borderTopColor: "#bec2ff" }} />
              <span className="text-sm font-semibold" style={{ color: "#c6c5d8" }}>Waiting for Circle attestation</span>
            </div>
            <p className="text-[11px] leading-relaxed" style={{ color: "#908fa1" }}>
              USDC burn confirmed on-chain. Polling Circle&apos;s Iris API for attestation
              {cctpPollCount > 0 ? ` (attempt ${cctpPollCount})` : ""}…
            </p>
            <p className="text-[11px]" style={{ color: "rgba(144,143,161,0.60)" }}>
              This takes ~2–5 minutes. Your funds are safe — the burn is irreversible only after attestation confirms.
            </p>
          </div>
        )}

        {phase === "done" && txHash && (
          <div className="px-5 py-4 space-y-2" style={{ backgroundColor: "#1c1b1b", border: "1px solid rgba(190,194,255,0.20)" }}>
            <div className="flex items-center gap-2">
              <span className="w-5 h-5 flex items-center justify-center text-accent text-[11px] font-bold" style={{ border: "1px solid rgba(190,194,255,0.30)" }}>✓</span>
              <span className="text-sm font-semibold text-accent">
                {isCanonicalWithdrawal
                  ? "Withdrawal initiated"
                  : cctpClaimDone
                  ? `USDC claimed on ${chainName(dstHop.to_chain)}`
                  : "Transaction submitted"}
              </span>
            </div>
            <div className="flex items-center gap-4">
              <a href={explorerTx(cctpClaimDone ? dstChainId : srcChainId, txHash)} target="_blank" rel="noopener noreferrer"
                className="text-[11px] font-mono text-accent hover:text-on-surface">
                {txHash.slice(0, 10)}…{txHash.slice(-6)} ↗
              </a>
              {!isAsyncClaim && !cctpClaimDone && (
                <a href={`${ACROSS_FILL_URL}?depositTxHash=${txHash}`} target="_blank" rel="noopener noreferrer"
                  className="text-[11px] text-outline hover:text-on-surface">Track ↗</a>
              )}
            </div>
            {isCanonicalWithdrawal && (
              <p className="text-[11px] text-on-surface-variant leading-relaxed pt-1">
                Funds claimable on {chainName(dstHop.to_chain)} after the ~7 day finality period.
              </p>
            )}
          </div>
        )}

        {/* ── Quote freshness banner ── */}
        {quotedAt && quoteCountdown !== null && (phase === "idle" || isErrorPhase) && (
          <div
            className="px-4 py-2 flex items-center justify-between"
            style={{
              backgroundColor: isQuoteExpired ? "rgba(147,0,10,0.12)" : quoteCountdown < 30 ? "rgba(255,184,77,0.08)" : "transparent",
              border: isQuoteExpired ? "1px solid rgba(255,180,171,0.25)" : quoteCountdown < 30 ? "1px solid rgba(255,184,77,0.20)" : "none",
            }}
          >
            <span className="text-[11px] font-mono" style={{ color: isQuoteExpired ? "#ffb4ab" : quoteCountdown < 30 ? "#ffb84d" : "#908fa1" }}>
              {isQuoteExpired ? "Quote expired — return to re-quote" : `Quote valid for ${quoteCountdown}s`}
            </span>
            {!isQuoteExpired && quoteCountdown < 30 && (
              <span className="text-[10px] font-mono uppercase tracking-widest" style={{ color: "#ffb84d" }}>
                Refresh soon
              </span>
            )}
          </div>
        )}

        {phase === "bridge_submitted" && txHash && (
          <div className="px-5 py-4 space-y-3" style={{ backgroundColor: "#1c1b1b", border: "1px solid rgba(190,194,255,0.20)" }}>
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <span className="w-5 h-5 flex items-center justify-center text-accent text-[11px] font-bold" style={{ border: "1px solid rgba(190,194,255,0.30)" }}>✓</span>
                <span className="text-sm font-semibold text-accent">Bridge submitted</span>
              </div>
              {bridgeSettled ? (
                <span className="text-[11px] font-mono px-2 py-0.5 uppercase tracking-wide" style={{ backgroundColor: "rgba(102,255,102,0.12)", color: "#66ff66", border: "1px solid rgba(102,255,102,0.25)" }}>
                  Funds arrived ✓
                </span>
              ) : (
                <span className="text-[11px] font-mono flex items-center gap-1.5" style={{ color: "#908fa1" }}>
                  <span className="animate-pulse inline-block w-1.5 h-1.5 rounded-full bg-current" />
                  Polling…
                </span>
              )}
            </div>
            <div className="flex items-center gap-4">
              <a href={explorerTx(srcChainId, txHash)} target="_blank" rel="noopener noreferrer"
                className="text-[11px] font-mono text-accent hover:text-on-surface">
                {txHash.slice(0, 10)}…{txHash.slice(-6)} ↗
              </a>
              <a href={`${ACROSS_FILL_URL}?depositTxHash=${txHash}`} target="_blank" rel="noopener noreferrer"
                className="text-[11px] text-outline hover:text-on-surface">Track ↗</a>
            </div>
            <p className="text-[11px] text-on-surface-variant leading-relaxed">
              {bridgeSettled
                ? `Funds confirmed on ${chainName(dstHop.to_chain)}. Switch network and execute the swap below.`
                : `Waiting for funds on ${chainName(dstHop.to_chain)} (~${route.estimated_time_seconds ?? 60}s). Monitoring balance automatically.`}
            </p>
          </div>
        )}

        {executionSupported && canExecute && (
          <>
            {/* Gas estimate row */}
            {gasEstimate !== null && gasPrice !== null && !srcIsSolana && (phase === "idle" || isErrorPhase) && (
              <div className="flex items-center justify-between text-[11px] font-mono" style={{ color: "#908fa1" }}>
                <span>Estimated gas</span>
                <span>~{Number(formatUnits(gasEstimate * gasPrice, 18)).toPrecision(3)} ETH</span>
              </div>
            )}

            {phase === "idle" || isErrorPhase ? (
              isQuoteExpired ? (
                <div className="h-12 w-full flex items-center justify-center text-xs font-mono uppercase tracking-widest" style={{ backgroundColor: "rgba(147,0,10,0.15)", border: "1px solid rgba(255,180,171,0.25)", color: "#ffb4ab" }}>
                  Quote expired — go back and re-quote
                </div>
              ) : (
                <button
                  onClick={handleExecute}
                  disabled={needsChainSwitch || !walletAddress}
                  className="h-12 w-full bg-accent text-surface text-sm font-bold uppercase tracking-[0.15em] transition-all hover:opacity-90 disabled:opacity-30 disabled:cursor-not-allowed"
                >
                  {isCanonicalWithdrawal ? "Initiate Withdrawal" : "Execute"}
                </button>
              )
            ) : isActive ? (
              <button disabled className="h-12 w-full text-on-surface-variant text-sm flex items-center justify-center gap-3" style={{ backgroundColor: "#1c1b1b", border: "1px solid rgba(69,69,85,0.40)" }}>
                <span className="animate-spin inline-block w-4 h-4 border-2 border-outline-variant border-t-on-surface-variant rounded-full" />
                <AnimatePresence mode="wait">
                  <motion.span
                    key={phase}
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    exit={{ opacity: 0 }}
                    transition={{ duration: 0.15 }}
                  >
                    {PHASE_LABELS[phase]}
                  </motion.span>
                </AnimatePresence>
              </button>
            ) : phase === "cctp_waiting_attestation" ? (
              <button disabled className="h-12 w-full text-[11px] font-mono uppercase tracking-wider flex items-center justify-center gap-3" style={{ backgroundColor: "#1c1b1b", border: "1px solid rgba(190,194,255,0.20)", color: "#908fa1" }}>
                <span className="animate-spin inline-block w-4 h-4 border-2 rounded-full shrink-0"
                  style={{ borderColor: "rgba(190,194,255,0.20)", borderTopColor: "rgba(190,194,255,0.60)" }} />
                Waiting for attestation{cctpPollCount > 0 ? ` · ${cctpPollCount}` : ""}…
              </button>
            ) : phase === "bridge_submitted" ? (
              <div className="space-y-2">
                {/* Destination chain switch if needed */}
                {bridgeSettled && currentChainId !== dstChainId && dstChainId !== 0 && (
                  <div className="flex items-center justify-between px-4 py-2.5" style={{ backgroundColor: "#1c1b1b", border: "1px solid rgba(255,184,77,0.25)" }}>
                    <span className="text-xs font-mono" style={{ color: "#ffb84d" }}>Switch to {chainName(dstHop.to_chain)}</span>
                    <button
                      onClick={() => switchChainAsync({ chainId: dstChainId }).catch(() => {})}
                      className="text-xs bg-accent text-surface px-3 py-1 font-semibold hover:opacity-90 transition-opacity"
                    >
                      Switch
                    </button>
                  </div>
                )}
                {resumeCountdown !== null && resumeCountdown > 0 && (
                  <div className="text-center text-[11px] font-mono" style={{ color: "#908fa1" }}>
                    Executing in {resumeCountdown}s…
                  </div>
                )}
                <div className="flex gap-2">
                  <button
                    onClick={handleResumeSwap}
                    disabled={!walletAddress}
                    className="flex-1 h-12 text-surface text-sm font-bold uppercase tracking-[0.15em] transition-all hover:opacity-90 disabled:opacity-30 disabled:cursor-not-allowed"
                    style={{ backgroundColor: bridgeSettled ? "#66ff66" : "#454555" }}
                  >
                    {bridgeSettled
                      ? `Execute Now on ${chainName(dstHop.to_chain)}`
                      : `Execute Swap on ${chainName(dstHop.to_chain)}`}
                  </button>
                </div>
              </div>
            ) : phase === "done" ? (
              <button disabled className="h-14 w-full text-accent text-sm font-semibold" style={{ backgroundColor: "#1c1b1b", border: "1px solid rgba(190,194,255,0.30)" }}>
                Done ✓
              </button>
            ) : null}
          </>
        )}

        {executionSupported && !canExecute && (
          <div className="px-4 py-3" style={{ backgroundColor: "#1c1b1b", border: "1px solid rgba(69,69,85,0.40)" }}>
            <p className="text-xs text-outline">
              {`Direct execution not yet available for ${bridgeLabel}.`}
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
