import { Fragment, useState } from "react";
import { formatUnits } from "viem";
import { motion } from "framer-motion";
import { TOKENS } from "../tokens";
import { TokenIcon } from "./TokenIcon";
import { type Route, type Hop } from "../api";
import { CONFIRMED_PROVIDERS } from "../config/providers";

// ── Chain helpers ────────────────────────────────────────────────────────────

const CHAIN_ID: Record<string, number> = {
  ethereum: 1, base: 8453, arbitrum: 42161, optimism: 10, polygon: 137,
  sepolia: 11155111, "base-sepolia": 84532,
  "arbitrum-sepolia": 421614, "op-sepolia": 11155420,
};
const CHAIN_SHORT: Record<string, string> = {
  ethereum: "Ethereum", base: "Base", arbitrum: "Arbitrum",
  optimism: "Optimism", polygon: "Polygon",
  sepolia: "Sepolia", "base-sepolia": "Base Sepolia",
  "arbitrum-sepolia": "Arb Sepolia", "op-sepolia": "OP Sepolia",
};

function cid(name: string) { return CHAIN_ID[name?.toLowerCase()] ?? 0; }
function cshort(name: string) { return CHAIN_SHORT[name?.toLowerCase()] ?? name; }

function tokenAddress(chain: string, symbol: string) {
  return TOKENS[cid(chain)]?.find(t => t.symbol === symbol)?.address ?? "";
}
function tokenDecimals(chain: string, symbol: string) {
  return TOKENS[cid(chain)]?.find(t => t.symbol === symbol)?.decimals ?? 18;
}
function fmtAmt(raw: string, decimals: number) {
  if (!raw || raw === "0") return "0";
  try {
    const n = Number(formatUnits(BigInt(raw), decimals));
    if (!n) return "0";
    return n >= 1000 ? n.toLocaleString(undefined, { maximumFractionDigits: 2 })
         : n >= 1    ? n.toFixed(4)
                     : n.toPrecision(4);
  } catch { return raw; }
}

function hopLabel(id: string) {
  const M: Record<string, string> = {
    across: "Across Protocol", circle_cctp: "Circle CCTP",
    uniswap_trading_api: "Uniswap V3", zerox: "0x", zeroex: "0x",
    oneinch: "1inch", mayan: "Mayan Finance",
    stargate: "Stargate V2", canonical_base: "Base Bridge",
    canonical_optimism: "OP Bridge", canonical_arbitrum: "Arb Bridge",
  };
  return M[id] ?? id;
}

function hopDescription(id: string) {
  const D: Record<string, string> = {
    across: "Optimistic relay bridge", circle_cctp: "Native USDC burn & mint",
    uniswap_trading_api: "Concentrated liquidity AMM", zerox: "DEX aggregator",
    zeroex: "DEX aggregator", oneinch: "Multi-path DEX routing",
    mayan: "Solana-bridged cross-chain", stargate: "LayerZero omnichain",
    canonical_base: "L1↔L2 native bridge", canonical_optimism: "L1↔L2 native bridge",
    canonical_arbitrum: "L1↔L2 native bridge",
  };
  return D[id] ?? (id.includes("bridge") ? "Cross-chain transfer" : "Token exchange");
}

function intentLabel(intent?: string) {
  switch (intent) {
    case "atomic_one_click": return "One-click";
    case "guided_two_step":  return "2-step";
    case "async_claim":      return "Claim required";
    default:                 return "Unsupported";
  }
}

// ── Guarantee Badge ───────────────────────────────────────────────────────────

function GuaranteeBadge({ guarantee, reasons }: { guarantee?: string; reasons?: string[] }) {
  if (!guarantee || guarantee === "unknown") return null;

  const isProtected = guarantee === "relay_fill_or_refund";
  const label  = isProtected ? "Protected" : "Manual Recovery";
  const icon   = isProtected ? "verified_user" : "warning";
  const color  = isProtected ? "#4ade80" : "#fbbf24";
  const bg     = isProtected ? "rgba(74,222,128,0.10)" : "rgba(251,191,36,0.10)";
  const border = isProtected ? "rgba(74,222,128,0.25)" : "rgba(251,191,36,0.25)";
  const tip    = isProtected
    ? "Funds guaranteed to arrive or be automatically refunded."
    : (reasons?.[0] ?? "Manual action may be required if the bridge fails.");

  return (
    <div className="relative group/badge">
      <span
        className="flex items-center gap-1 text-[10px] font-mono uppercase tracking-wider px-2 py-0.5 cursor-default"
        style={{ color, backgroundColor: bg, border: `1px solid ${border}` }}
      >
        <span className="material-symbols-outlined" style={{ fontSize: "11px", fontVariationSettings: "'FILL' 1" }}>
          {icon}
        </span>
        {label}
      </span>
      {/* Tooltip */}
      <div
        className="absolute bottom-full left-0 mb-1.5 w-56 px-3 py-2 text-[11px] leading-relaxed pointer-events-none opacity-0 group-hover/badge:opacity-100 transition-opacity z-20"
        style={{ backgroundColor: "#1c1b1b", border: "1px solid rgba(69,69,85,0.6)", color: "#c6c5d8" }}
      >
        {tip}
      </div>
    </div>
  );
}

// ── Intent Chip with tooltip ──────────────────────────────────────────────────

function IntentChip({ exec }: { exec?: { intent?: string; supported?: boolean; reasons?: string[] } }) {
  if (!exec) return null;
  const label = intentLabel(exec.intent);
  const isOneClick = exec.intent === "atomic_one_click";
  const tip = exec.reasons?.[0];

  return (
    <div className="relative group/intent">
      <span
        className="text-xs font-mono flex items-center gap-1.5 px-2.5 py-1 uppercase cursor-default"
        style={{
          backgroundColor: exec.supported
            ? isOneClick ? "rgba(190,194,255,0.15)" : "rgba(198,197,216,0.10)"
            : "rgba(255,180,171,0.12)",
          color: exec.supported
            ? isOneClick ? "#bec2ff" : "#c6c5d8"
            : "#ffb4ab",
        }}
      >
        <span
          className="material-symbols-outlined"
          style={{ fontSize: "16px", fontVariationSettings: "'FILL' 1", verticalAlign: "middle" }}
        >
          {exec.supported ? (isOneClick ? "bolt" : "sync") : "description"}
        </span>
        {exec.supported ? label : "Quote only"}
      </span>
      {tip && (
        <div
          className="absolute bottom-full right-0 mb-1.5 w-60 px-3 py-2 text-[11px] leading-relaxed pointer-events-none opacity-0 group-hover/intent:opacity-100 transition-opacity z-20"
          style={{ backgroundColor: "#1c1b1b", border: "1px solid rgba(69,69,85,0.6)", color: "#c6c5d8" }}
        >
          {tip}
        </div>
      )}
    </div>
  );
}

// ── Verification badge (demo UX) ──────────────────────────────────────────────

function VerificationBadge({ route }: { route: Route }) {
  // Only consider BRIDGE hops for "receipt-confirmed" status; swap hops are DEXes.
  const bridgeHops = route.hops.filter((h) => h.hop_type === "bridge");
  if (bridgeHops.length === 0) return null;

  const unverified = Array.from(
    new Set(bridgeHops.map((h) => h.bridge_id).filter((id) => !CONFIRMED_PROVIDERS.has(id))),
  );
  const verified = unverified.length === 0;

  const label = verified ? "Verified" : "Unverified";
  const icon = verified ? "verified" : "science";
  const color = verified ? "#4ade80" : "#fbbf24";
  const bg = verified ? "rgba(74,222,128,0.10)" : "rgba(251,191,36,0.10)";
  const border = verified ? "rgba(74,222,128,0.25)" : "rgba(251,191,36,0.25)";
  const tip = verified
    ? "Receipt-confirmed provider(s) only."
    : `Not receipt-confirmed yet: ${unverified.map(hopLabel).join(", ")}. Quote is shown for battle-testing.`;

  return (
    <div className="relative group/vbadge">
      <span
        className="flex items-center gap-1 text-[10px] font-mono uppercase tracking-wider px-2 py-0.5 cursor-default"
        style={{ color, backgroundColor: bg, border: `1px solid ${border}` }}
      >
        <span className="material-symbols-outlined" style={{ fontSize: "11px", fontVariationSettings: "'FILL' 1" }}>
          {icon}
        </span>
        {label}
      </span>
      <div
        className="absolute bottom-full left-0 mb-1.5 w-64 px-3 py-2 text-[11px] leading-relaxed pointer-events-none opacity-0 group-hover/vbadge:opacity-100 transition-opacity z-20"
        style={{ backgroundColor: "#1c1b1b", border: "1px solid rgba(69,69,85,0.6)", color: "#c6c5d8" }}
      >
        {tip}
      </div>
    </div>
  );
}

// ── Material Icon Component ──────────────────────────────────────────────────

function MaterialIcon({
  icon,
  filled = false,
  className = "",
  size = 20,
}: {
  icon: string;
  filled?: boolean;
  className?: string;
  size?: number;
}) {
  return (
    <span
      className={`material-symbols-outlined ${className}`}
      style={{
        fontVariationSettings: filled ? "'FILL' 1" : "'FILL' 0",
        fontSize: `${size}px`,
        verticalAlign: "middle",
      }}
    >
      {icon}
    </span>
  );
}

// ── Flow path components ─────────────────────────────────────────────────────

const CHAIN_ABBREV: Record<string, string> = {
  ethereum: "ETH", base: "BASE", arbitrum: "ARB", optimism: "OP", polygon: "POL",
};
function chainAbbrev(name: string) {
  return CHAIN_ABBREV[name?.toLowerCase()] ?? name?.toUpperCase().slice(0, 4);
}

function FlowNode({ symbol, chain }: { symbol: string; chain: string }) {
  const chainId = cid(chain);
  return (
    <span className="px-2 py-1 bg-[#0e0e0e] border border-[#454555]/40 flex items-center gap-1.5 shrink-0 whitespace-nowrap">
      <TokenIcon chainId={chainId} address={tokenAddress(chain, symbol)} symbol={symbol} size={12} />
      <span className="font-mono text-[10px] tracking-tight uppercase" style={{ color: "#c6c5d8" }}>
        {symbol}
      </span>
      <span className="text-[10px] opacity-30">•</span>
      <span className="text-[10px] uppercase" style={{ color: "#c6c5d8" }}>{chainAbbrev(chain)}</span>
    </span>
  );
}

function FlowArrow() {
  return <MaterialIcon icon="arrow_forward" className="text-[14px] opacity-30" />;
}

function FlowProtocol({ label }: { label: string }) {
  return (
    <span className="px-2 py-1 bg-[#0e0e0e] border border-[#454555]/15 text-[10px] font-mono uppercase shrink-0 whitespace-nowrap" style={{ color: "#c6c5d8" }}>
      {label}
    </span>
  );
}

// ── Routing Flow (Collapsible) ──────────────────────────────────────────────

function RoutingFlow({ hops, collapsed }: { hops: Hop[]; collapsed: boolean }) {
  if (!hops.length) return null;

  const first = hops[0];
  const last = hops[hops.length - 1];

  if (collapsed && hops.length > 1) {
    return (
      <div className="flex items-center gap-2 font-mono text-[10px] uppercase">
        <FlowNode symbol={first.from_asset} chain={first.from_chain} />
        <FlowArrow />
        <span className="px-2 py-1 bg-[#0e0e0e] border border-[#454555]/15 text-[#c6c5d8]">
          {hops.length} hops
        </span>
        <FlowArrow />
        <FlowNode symbol={last.to_asset} chain={last.to_chain} />
      </div>
    );
  }

  return (
    <div className="flex items-center gap-2 font-mono text-[10px] uppercase overflow-x-auto no-scrollbar">
      <FlowNode symbol={first.from_asset} chain={first.from_chain} />
      {hops.map((hop, i) => (
        <Fragment key={i}>
          <FlowArrow />
          <FlowProtocol label={hopLabel(hop.bridge_id)} />
          <FlowArrow />
          <FlowNode symbol={hop.to_asset} chain={hop.to_chain} />
        </Fragment>
      ))}
    </div>
  );
}

// ── RouteCard ────────────────────────────────────────────────────────────────

interface Props {
  route: Route;
  isBest: boolean;
  selected: boolean;
  onSelect: () => void;
}

const STABLES = new Set(["USDC", "USDT", "DAI", "BUSD", "USDC.E", "USDCE", "FRAX", "LUSD", "PYUSD"]);
function isStable(sym: string) { return STABLES.has(sym?.toUpperCase()); }

export function RouteCard({ route, isBest, selected, onSelect }: Props) {
  const [expanded, setExpanded] = useState(false);
  const last  = route.hops[route.hops.length - 1];
  const first = route.hops[0];
  const srcDec = tokenDecimals(first?.from_chain, first?.from_asset);
  const dec   = tokenDecimals(last?.to_chain, last?.to_asset);
  const out   = fmtAmt(route.estimated_output_amount, dec);
  const fee   = route.total_fee && route.total_fee !== "0"
    ? `$${Number(route.total_fee).toFixed(2)}`
    : "$0.00";
  const mins  = Math.ceil(route.estimated_time_seconds / 60);
  const exec  = route.execution;
  const primaryBridge = first?.bridge_id ?? route.route_id;

  // ── Price impact / effective rate ──────────────────────────────────────────
  const srcSym = first?.from_asset?.toUpperCase() ?? "";
  const dstSym = last?.to_asset?.toUpperCase() ?? "";
  const inputRaw  = first?.amount_in_base_units ?? "0";
  const outputRaw = route.estimated_output_amount ?? "0";
  const inputNum  = (() => { try { return Number(formatUnits(BigInt(inputRaw), srcDec)); } catch { return 0; } })();
  const outputNum = (() => { try { return Number(formatUnits(BigInt(outputRaw), dec)); } catch { return 0; } })();

  // For same-symbol or stable↔stable pairs: show cost % (lower = better)
  const isSameCategory = srcSym === dstSym || (isStable(srcSym) && isStable(dstSym));
  const priceImpact = isSameCategory && inputNum > 0
    ? ((1 - outputNum / inputNum) * 100)
    : null;

  // For cross-asset: show exchange rate
  const effectiveRate = !isSameCategory && inputNum > 0
    ? outputNum / inputNum
    : null;

  return (
    <motion.div
      onClick={onSelect}
      className="relative p-6 cursor-pointer min-h-[180px] flex flex-col group"
      animate={{
        scale: selected ? 1.01 : 1,
        backgroundColor: selected ? "#2a2a2a" : "#2a2a2a",
      }}
      whileHover={{
        scale: 1.005,
        y: -2,
        backgroundColor: "#2f2f2f",
      }}
      transition={{ duration: 0.2, ease: "easeOut" }}
      style={{
        border: selected ? "1px solid #c6c6c7" : "1px solid rgba(69,69,85,0.40)",
        boxShadow: selected
          ? "0 4px 12px rgba(0,0,0,0.3)"
          : "0 2px 4px rgba(0,0,0,0.2)",
      }}
    >
      {/* ── Top Row: Badges ── */}
      <div className="flex justify-between items-start mb-5">
        <div className="flex items-center gap-2 flex-wrap">
          {isBest && (
            <span
              className="text-[10px] font-bold px-2 py-0.5 uppercase tracking-wider"
              style={{ backgroundColor: "#c6c6c7", color: "#131313" }}
            >
              Best Route
            </span>
          )}
          <VerificationBadge route={route} />
          <GuaranteeBadge guarantee={exec?.guarantee} reasons={exec?.reasons} />
        </div>
        <IntentChip exec={exec} />
      </div>

      {/* ── Protocol Name + Output Amount (Side by Side) ── */}
      <div className="flex justify-between items-start mb-5 gap-6">
        <div className="flex-1">
          <div className="text-xl font-bold leading-tight mb-1.5" style={{ color: "#e5e2e1" }}>
            {hopLabel(primaryBridge)}
          </div>
          <div className="text-xs leading-relaxed" style={{ color: "#908fa1" }}>
            {hopDescription(primaryBridge)}
          </div>
        </div>
        <div className="text-right shrink-0">
          <div className="text-xl sm:text-3xl font-bold font-mono leading-none tabular-nums" style={{ color: "#e5e2e1" }}>
            {out}
          </div>
          <div className="text-sm font-medium font-mono mt-1" style={{ color: "#908fa1" }}>
            {last?.to_asset}
          </div>
          <div className="text-[10px] font-mono uppercase tracking-widest mt-1.5" style={{ color: "#bec2ff" }}>
            on {cshort(last?.to_chain)}
          </div>
        </div>
      </div>

      {/* ── Stats Row ── */}
      <div
        className="flex items-center gap-4 text-xs font-mono mb-4 flex-wrap"
        style={{ color: selected ? "#c6c6c7" : "#908fa1" }}
      >
        <span className="flex items-center gap-1">
          <MaterialIcon icon="schedule" size={20} />
          ~{mins}m
        </span>
        <span className="flex items-center gap-1">
          <MaterialIcon icon="payments" size={20} />
          {fee}
        </span>
        <span className="flex items-center gap-1">
          <MaterialIcon icon="hub" size={20} />
          {route.hops.length} {route.hops.length === 1 ? "hop" : "hops"}
        </span>
        {priceImpact !== null && (
          <span
            className="flex items-center gap-1"
            style={{ color: priceImpact > 2 ? "#ffb4ab" : priceImpact > 0.5 ? "#ffb84d" : "#4caf50" }}
          >
            <MaterialIcon icon="trending_down" size={16} />
            {priceImpact < 0 ? "+" : ""}{(-priceImpact).toFixed(2)}%
          </span>
        )}
        {effectiveRate !== null && (
          <span className="flex items-center gap-1">
            <MaterialIcon icon="swap_horiz" size={16} />
            1 {srcSym} = {effectiveRate >= 1000
              ? effectiveRate.toLocaleString(undefined, { maximumFractionDigits: 0 })
              : effectiveRate.toPrecision(4)} {dstSym}
          </span>
        )}
      </div>

      {/* ── Routing Path (Bottom) ── */}
      <div className="border-t pt-4 mt-auto" style={{ borderColor: "rgba(69,69,85,0.15)" }}>
        <div
          className="cursor-pointer"
          onClick={(e) => {
            e.stopPropagation();
            if (route.hops.length > 1) setExpanded(!expanded);
          }}
        >
          <RoutingFlow hops={route.hops} collapsed={!expanded} />
        </div>

        {/* Per-hop fee breakdown — shown when expanded or single-hop */}
        {(expanded || route.hops.length === 1) && (
          <div className="mt-3 space-y-1">
            {route.hops.map((hop, i) => {
              const hopFee = hop.estimated_fee && hop.estimated_fee !== "0"
                ? `$${Number(hop.estimated_fee).toFixed(2)}`
                : null;
              if (!hopFee) return null;
              return (
                <div key={i} className="flex items-center justify-between text-[10px] font-mono" style={{ color: "#908fa1" }}>
                  <span className="uppercase tracking-wide">{hopLabel(hop.bridge_id)}</span>
                  <span>{hopFee}</span>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* ── Selection Indicator ── */}
      {selected && (
        <div
          className="absolute top-0 right-0 w-8 h-8 flex items-center justify-center"
          style={{ backgroundColor: "#c6c6c7", color: "#131313" }}
        >
          <MaterialIcon icon="check" size={24} />
        </div>
      )}
    </motion.div>
  );
}
