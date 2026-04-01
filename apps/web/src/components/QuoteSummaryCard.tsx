import { formatUnits } from "viem";
import { type Route } from "../api";
import { TOKENS } from "../tokens";
import { TokenIcon } from "./TokenIcon";
import { ChainIcon } from "./ChainIcon";

// ── Helpers (same pattern as ExecutePanel / RouteCard) ────────────────────────

const CHAIN_ID: Record<string, number> = {
  ethereum: 1, base: 8453, arbitrum: 42161, optimism: 10, polygon: 137,
  sepolia: 11155111, "base-sepolia": 84532,
  "arbitrum-sepolia": 421614, "op-sepolia": 11155420,
};
const CHAIN_NAME: Record<string, string> = {
  ethereum: "Ethereum", base: "Base", arbitrum: "Arbitrum",
  optimism: "Optimism", polygon: "Polygon",
  sepolia: "Sepolia", "base-sepolia": "Base Sepolia",
  "arbitrum-sepolia": "Arb Sepolia", "op-sepolia": "OP Sepolia",
};

function cidOf(name: string) { return CHAIN_ID[name?.toLowerCase()] ?? 0; }
function chainName(name: string) { return CHAIN_NAME[name?.toLowerCase()] ?? name; }
function tokenDecimals(chainId: number, symbol: string) {
  return TOKENS[chainId]?.find(t => t.symbol === symbol)?.decimals ?? 18;
}
function tokenAddress(chainId: number, symbol: string) {
  return TOKENS[chainId]?.find(t => t.symbol === symbol)?.address ?? "";
}
function fmtAmt(raw: string, dec: number) {
  if (!raw || raw === "0") return "0";
  try {
    const n = Number(formatUnits(BigInt(raw), dec));
    if (!n) return "0";
    return n >= 1000
      ? n.toLocaleString(undefined, { maximumFractionDigits: 2 })
      : n >= 1 ? n.toFixed(4) : n.toPrecision(4);
  } catch { return raw; }
}
function hopLabel(id: string) {
  const M: Record<string, string> = {
    across: "Across", circle_cctp: "CCTP", uniswap_trading_api: "Uniswap",
    zerox: "0x", zeroex: "0x", oneinch: "1inch", mayan: "Mayan",
    stargate: "Stargate", canonical_base: "Base Bridge",
    canonical_optimism: "OP Bridge", canonical_arbitrum: "Arb Bridge",
  };
  return M[id] ?? id.replace(/_/g, " ").replace(/\b\w/g, c => c.toUpperCase());
}

// ── Component ─────────────────────────────────────────────────────────────────

interface Props {
  route: Route;
  onEdit: () => void;
}

export function QuoteSummaryCard({ route, onEdit }: Props) {
  const srcHop = route.hops[0];
  const dstHop = route.hops[route.hops.length - 1];

  const srcChainId = cidOf(srcHop.from_chain);
  const dstChainId = cidOf(dstHop.to_chain);
  const srcDec = tokenDecimals(srcChainId, srcHop.from_asset);
  const dstDec = tokenDecimals(dstChainId, dstHop.to_asset);

  const srcAmt = fmtAmt(srcHop.amount_in_base_units ?? "0", srcDec);
  const dstAmt = fmtAmt(route.estimated_output_amount, dstDec);

  const fee = route.total_fee && route.total_fee !== "0"
    ? `$${Number(route.total_fee).toFixed(2)}`
    : null;
  const mins = route.estimated_time_seconds
    ? Math.ceil(route.estimated_time_seconds / 60)
    : null;

  const bridgeNames = route.hops
    .filter(h => h.hop_type === "bridge")
    .map(h => hopLabel(h.bridge_id))
    .join(" + ") || hopLabel(route.hops[0]?.bridge_id ?? "");

  return (
    <div className="rounded bg-[#1c1b1b] border border-[#2a2a2a] overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between px-5 py-3 border-b border-[#2a2a2a]">
        <span className="text-[10px] font-mono uppercase tracking-[0.18em] text-[#908fa1]">
          Quote Summary
        </span>
        <button
          type="button"
          onClick={onEdit}
          className="flex items-center gap-1.5 text-[11px] text-[#c6c5d8] hover:text-[#e5e2e1] transition-colors"
        >
          <svg className="w-3 h-3" viewBox="0 0 12 12" fill="none">
            <path d="M9 1L3 7M3 7H7M3 7V3" stroke="currentColor" strokeWidth="1.5"
              strokeLinecap="round" strokeLinejoin="round" />
          </svg>
          Edit
        </button>
      </div>

      {/* From / To */}
      <div className="px-5 py-5 space-y-1">
        {/* From */}
        <div className="flex items-center gap-3">
          <TokenIcon
            chainId={srcChainId}
            address={tokenAddress(srcChainId, srcHop.from_asset)}
            symbol={srcHop.from_asset}
            size={36}
          />
          <div>
            <div className="text-2xl font-semibold font-mono tabular-nums tracking-tight text-[#e5e2e1]">
              {srcAmt}
              <span className="text-lg ml-1.5 text-[#c6c5d8]">{srcHop.from_asset}</span>
            </div>
            <div className="flex items-center gap-1 mt-0.5">
              <ChainIcon chainId={srcChainId} size={11} />
              <span className="text-[11px] text-[#908fa1]">{chainName(srcHop.from_chain)}</span>
            </div>
          </div>
        </div>

        {/* Arrow */}
        <div className="flex items-center gap-3 py-1">
          <div className="w-9 flex justify-center">
            <svg className="w-4 h-4 text-[#454555]" viewBox="0 0 16 16" fill="none">
              <path d="M8 3v10M4 9l4 4 4-4" stroke="currentColor" strokeWidth="1.5"
                strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </div>
          <span className="text-[10px] font-mono text-[#454555] uppercase tracking-wider">
            {bridgeNames}
          </span>
        </div>

        {/* To */}
        <div className="flex items-center gap-3">
          <TokenIcon
            chainId={dstChainId}
            address={tokenAddress(dstChainId, dstHop.to_asset)}
            symbol={dstHop.to_asset}
            size={36}
          />
          <div>
            <div className="text-2xl font-semibold font-mono tabular-nums tracking-tight text-[#e5e2e1]">
              <span className="text-[#908fa1] text-lg mr-0.5">~</span>
              {dstAmt}
              <span className="text-lg ml-1.5 text-[#c6c5d8]">{dstHop.to_asset}</span>
            </div>
            <div className="flex items-center gap-1 mt-0.5">
              <ChainIcon chainId={dstChainId} size={11} />
              <span className="text-[11px] text-[#908fa1]">{chainName(dstHop.to_chain)}</span>
            </div>
          </div>
        </div>
      </div>

      {/* Footer stats */}
      <div className="flex items-center gap-4 px-5 py-3 border-t border-[#2a2a2a] text-[11px] font-mono text-[#908fa1]">
        {mins !== null && (
          <span className="flex items-center gap-1">
            <svg className="w-3.5 h-3.5" viewBox="0 0 16 16" fill="none">
              <circle cx="8" cy="8" r="6" stroke="currentColor" strokeWidth="1.5"/>
              <path d="M8 5v3.5l2 1.5" stroke="currentColor" strokeWidth="1.5"
                strokeLinecap="round"/>
            </svg>
            ~{mins}m
          </span>
        )}
        {fee && (
          <span className="flex items-center gap-1">
            <svg className="w-3.5 h-3.5" viewBox="0 0 16 16" fill="none">
              <path d="M3 8h10M8 3v10" stroke="currentColor" strokeWidth="1.5"
                strokeLinecap="round"/>
            </svg>
            {fee} fee
          </span>
        )}
        <span className="flex items-center gap-1">
          <svg className="w-3.5 h-3.5" viewBox="0 0 16 16" fill="none">
            <path d="M2 8h3l2-4 3 8 2-4h2" stroke="currentColor" strokeWidth="1.5"
              strokeLinecap="round" strokeLinejoin="round"/>
          </svg>
          {route.hops.length} hop{route.hops.length !== 1 ? "s" : ""}
        </span>
        <span className="ml-auto text-[10px] uppercase tracking-wider text-[#bec2ff]">
          Best route
        </span>
      </div>
    </div>
  );
}
