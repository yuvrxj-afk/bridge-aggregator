// ── Quote types ───────────────────────────────────────────────────────────────

export interface Endpoint {
  chain: string;
  chain_id: number;
  asset: string;
  token_address: string;
  token_decimals: number;
  address?: string;
}

export interface QuoteRequest {
  source: Endpoint;
  destination: Endpoint;
  amount_base_units: string;
}

export interface Hop {
  hop_type: "bridge" | "swap";
  bridge_id: string;
  from_chain: string;
  to_chain: string;
  from_asset: string;
  to_asset: string;
  from_token_address: string;
  to_token_address: string;
  amount_in_base_units: string;
  estimated_fee: string;
  provider_data?: Record<string, unknown>;
}

export interface Route {
  route_id: string;
  score: number;
  estimated_output_amount: string;
  estimated_time_seconds: number;
  total_fee: string;
  hops: Hop[];
  execution?: ExecutionProfile;
}

export interface ExecutionProfile {
  supported: boolean;
  intent: "atomic_one_click" | "guided_two_step" | "async_claim" | "unsupported";
  guarantee: "relay_fill_or_refund" | "manual_recovery_required" | "unknown";
  recovery: "automatic" | "resumable_guided" | "manual";
  requirements?: string[];
  reasons?: string[];
  metadata?: Record<string, string>;
}

export interface QuoteResponse {
  routes: Route[];
}

export interface AdapterHealth {
  service: string;
  kind: "bridge" | "dex";
  status: "healthy" | "degraded" | "down";
  reason?: string;
  latency_ms: number;
}

export interface AdapterHealthResponse {
  status: "healthy" | "degraded" | "down";
  adapters: AdapterHealth[];
}

// ── StepTransaction types ─────────────────────────────────────────────────────

export interface TransactionRequest {
  to: string;
  data: string;
  value: string;
  gas_limit: string;
}

export interface TokenApproval {
  chain_id: number;
  token_contract: string;
  spender: string;
  amount: string;
}

export interface BridgeTxCall {
  chain_id: number;
  contract: string;
  function: string;
  params: Record<string, unknown>;
  value: string;
  abi_fragment: string;
}

export interface BridgeStepCall {
  step_type: "approve" | "deposit" | "claim";
  tx?: BridgeTxCall;
  approval?: TokenApproval;
}

export interface BridgeStepParams {
  protocol: string;
  steps: BridgeStepCall[];
  notes: string;
}

export interface SolanaTransactionRequest {
  serialized_tx: string;
  signer_public_key?: string;
}

export interface StepTransactionResponse {
  hop_index: number;
  hop_type: string;
  tx?: TransactionRequest;           // swap hops: ready-to-send tx
  bridge_params?: BridgeStepParams;  // bridge hops: structured steps
  solana_tx?: SolanaTransactionRequest; // Solana source hops (Mayan): sign with Solana wallet
}

// ── LiFi Diamond execution types ─────────────────────────────────────────────

export interface LiFiBridgeData {
  transactionId: string;
  bridge: string;
  integrator: string;
  referrer: string;
  sendingAssetId: string;
  receiver: string;
  minAmount: string;
  destinationChainId: number;
  hasSourceSwaps: boolean;
  hasDestinationCall: boolean;
}

export interface LiFiSwapData {
  callTo: string;
  approveTo: string;
  sendingAssetId: string;
  receivingAssetId: string;
  fromAmount: string;
  callData: string;
  requiresDeposit: boolean;
}

export interface LiFiAcrossV3Data {
  receiverAddress: string;
  refundAddress: string;
  receivingAssetId: string;
  outputAmount: string;
  outputAmountPercent: string;
  exclusiveRelayer: string;
  quoteTimestamp: number;
  fillDeadline: number;
  exclusivityDeadline: number;
  message: string; // "0x" when no dest call; frontend fills from acrossMessage
}

export interface LiFiAcrossMessage {
  transaction_id: string;
  swap_data: LiFiSwapData[];
  receiver: string;
}

export interface LiFiCelerCircleData {
  maxFee: string;
  minFinalityThreshold: number;
}

export interface LiFiBuildResponse {
  diamond: string;
  chain_id: number;
  function: string;
  value: string;
  bridge_data?: LiFiBridgeData;
  swap_data?: LiFiSwapData[];
  across_v3_data?: LiFiAcrossV3Data;
  across_message?: LiFiAcrossMessage; // present when hasDestinationCall=true
  celer_circle_data?: LiFiCelerCircleData;
}

// ── Fetch helpers ─────────────────────────────────────────────────────────────

// In production: VITE_API_URL = mainnet backend, VITE_API_URL_TESTNET = testnet backend.
// In dev: mainnet uses the Vite proxy at /api (→ localhost:8080),
//         testnet uses /api/testnet (→ localhost:8081, rewritten to /api by Vite).
const MAINNET_BASE = (import.meta.env.VITE_API_URL ?? "") + "/api/v1";
const TESTNET_BASE = import.meta.env.VITE_API_URL_TESTNET
  ? import.meta.env.VITE_API_URL_TESTNET + "/api/v1"
  : "/api/testnet/v1";

function getBase(): string {
  if (typeof window === "undefined") return MAINNET_BASE;
  const scope = window.localStorage.getItem("chain_scope") ?? "mainnet";
  return scope === "testnet" ? TESTNET_BASE : MAINNET_BASE;
}

// BridgeError carries the structured error_type and error_code from the backend,
// allowing the frontend to classify errors for appropriate recovery UX.
export class BridgeError extends Error {
  errorType: string; // "retryable" | "user_action" | "requote" | "terminal"
  errorCode: string; // e.g. "no_routes", "timeout", "invalid_route"
  constructor(message: string, errorType = "terminal", errorCode = "internal") {
    super(message);
    this.name = "BridgeError";
    this.errorType = errorType;
    this.errorCode = errorCode;
  }
}

type ApiErrorBody = { error?: { message?: string; error_type?: string; error_code?: string } };

const API_KEY = import.meta.env.VITE_API_KEY as string | undefined;

function authHeaders(): Record<string, string> {
  return API_KEY ? { "X-API-Key": API_KEY } : {};
}

async function apiFetch<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(`${getBase()}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify(body),
  });
  const data = await res.json() as ApiErrorBody;
  if (!res.ok) {
    const err = data.error ?? {};
    throw new BridgeError(
      err.message ?? `HTTP ${res.status}`,
      err.error_type ?? "terminal",
      err.error_code ?? "internal",
    );
  }
  return data as T;
}

async function apiGet<T>(path: string): Promise<T> {
  const res = await fetch(`${getBase()}${path}`, { headers: authHeaders() });
  const data = await res.json() as ApiErrorBody;
  if (!res.ok) {
    const err = data.error ?? {};
    throw new BridgeError(
      err.message ?? `HTTP ${res.status}`,
      err.error_type ?? "terminal",
      err.error_code ?? "internal",
    );
  }
  return data as T;
}

export function fetchQuote(req: QuoteRequest): Promise<QuoteResponse> {
  return apiFetch("/quote", req);
}

// fetchQuoteStream streams routes as they arrive via Server-Sent Events.
// Each yielded Route is ready to display — the generator ends when the server
// sends "event: done" or closes the connection.
export async function* fetchQuoteStream(req: QuoteRequest, signal?: AbortSignal): AsyncGenerator<Route> {
  const res = await fetch(`${getBase()}/quote/stream`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
    signal,
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error((body as { error?: { message?: string } }).error?.message ?? `HTTP ${res.status}`);
  }
  if (!res.body) throw new Error("No response body for streaming");

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buf = "";

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buf += decoder.decode(value, { stream: true });
      // SSE blocks are separated by double newline
      const blocks = buf.split("\n\n");
      buf = blocks.pop() ?? "";
      for (const block of blocks) {
        for (const line of block.split("\n")) {
          if (line.startsWith("data: ")) {
            try {
              const route = JSON.parse(line.slice(6)) as Route;
              if (route && route.route_id) yield route;
            } catch { /* malformed line — skip */ }
          }
        }
      }
    }
  } finally {
    reader.releaseLock();
  }
}

export function fetchStepTransaction(
  route: Route,
  hopIndex: number,
  senderAddress?: string,
  receiverAddress?: string,
): Promise<StepTransactionResponse> {
  return apiFetch("/route/stepTransaction", {
    route,
    hop_index: hopIndex,
    ...(senderAddress && { sender_address: senderAddress }),
    ...(receiverAddress && { receiver_address: receiverAddress }),
  });
}

export function fetchBuildTransaction(route: Route, fromAddress: string): Promise<LiFiBuildResponse> {
  return apiFetch("/route/buildTransaction", { route, from_address: fromAddress });
}

export function fetchAdapterHealth(): Promise<AdapterHealthResponse> {
  return apiGet("/health/adapters");
}

export function fetchTransactionStatus(txHash: string): Promise<{ status: string; progress: number }> {
  return apiGet(`/status/${txHash}`);
}

// ── Operation lifecycle ───────────────────────────────────────────────────────

export interface OperationRecord {
  operation_id: string;
  status: string;
  client_reference_id?: string;
}

export interface OperationEvent {
  id: number;
  event_type: string;
  from_status: string;
  to_status: string;
  tx_hash?: string;
  created_at: string;
}

export interface OperationDetail {
  operation_id: string;
  route: Route;
  status: string;
  tx_hash?: string;
  created_at: string;
  updated_at: string;
  events?: OperationEvent[];
}

// List recent operations for a wallet address (newest-first). wallet is required.
export function fetchOperations(wallet: string, limit = 50, scope?: string): Promise<{ operations: OperationDetail[] }> {
  const params = new URLSearchParams({ wallet, limit: String(limit) });
  if (scope) params.set("scope", scope);
  return apiGet(`/operations?${params}`);
}

// Register an intent to execute a route — called before wallet prompt.
export function createOperation(route: Route, idempotencyKey?: string): Promise<OperationRecord> {
  return apiFetch("/execute", {
    route,
    ...(idempotencyKey && { idempotency_key: idempotencyKey }),
  });
}

// Update operation status after on-chain confirmation.
export function patchOperationStatus(operationId: string, status: string, txHash?: string): Promise<void> {
  return fetch(`${getBase()}/operations/${operationId}/status`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ status, ...(txHash && { tx_hash: txHash }) }),
  }).then(() => undefined);
}

// Fetch a single operation with its events.
export function fetchOperation(operationId: string): Promise<OperationDetail> {
  return apiGet(`/operations/${operationId}`);
}

// ── Intent parsing ─────────────────────────────────────────────────────────────

export interface ParsedIntentResponse {
  amount: string
  src_token: string
  dst_token: string
  src_chain: string
  dst_chain: string
}

// Parse a natural language intent via the backend OpenRouter integration.
export function fetchParseIntent(text: string): Promise<ParsedIntentResponse> {
  return apiFetch<ParsedIntentResponse>("/intent/parse", { text })
}
