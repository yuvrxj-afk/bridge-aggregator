import { motion } from "framer-motion";
import { formatUnits } from "viem";
import { TOKENS } from "../tokens";
import { type Route } from "../api";

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
  return M[id] ?? id.replace(/_/g, " ").replace(/\b\w/g, c => c.toUpperCase());
}
function intentLabel(intent?: string) {
  switch (intent) {
    case "atomic_one_click": return "One-click";
    case "guided_two_step":  return "2-step";
    case "async_claim":      return "Async";
    default:                 return "Quote only";
  }
}

interface Props {
  route: Route;
  selected: boolean;
  onSelect: () => void;
}

export function MiniRouteCard({ route, selected, onSelect }: Props) {
  const last  = route.hops[route.hops.length - 1];
  const first = route.hops[0];
  const dec   = tokenDecimals(last?.to_chain, last?.to_asset);
  const out   = fmtAmt(route.estimated_output_amount, dec);
  const mins  = Math.ceil(route.estimated_time_seconds / 60);
  const exec  = route.execution;
  const intent = intentLabel(exec?.intent);
  const primaryBridge = first?.bridge_id ?? route.route_id;
  const fee = route.total_fee && route.total_fee !== "0"
    ? `$${Number(route.total_fee).toFixed(2)}`
    : "$0.00";

  return (
    <motion.div
      onClick={onSelect}
      className="relative p-4 cursor-pointer flex flex-col gap-2"
      animate={{ backgroundColor: selected ? "#2a2a2a" : "#1c1b1b" }}
      whileHover={{ backgroundColor: "#2f2f2f" }}
      transition={{ duration: 0.15, ease: "easeOut" }}
      style={{
        border: selected ? "1px solid #c6c6c7" : "1px solid rgba(69,69,85,0.40)",
        boxShadow: selected ? "0 4px 12px rgba(0,0,0,0.3)" : "none",
      }}
    >
      {/* Badge row */}
      <div className="flex items-center gap-1.5 flex-wrap">
        <span
          className="text-[9px] font-bold px-1.5 py-0.5 uppercase tracking-wider font-mono"
          style={{ color: "#908fa1", border: "1px solid rgba(69,69,85,0.40)" }}
        >
          {hopLabel(primaryBridge)}
        </span>
        <span
          className="text-[9px] font-mono px-1.5 py-0.5 uppercase"
          style={{
            backgroundColor: exec?.supported
              ? exec?.intent === "atomic_one_click" ? "rgba(190,194,255,0.15)" : "rgba(198,197,216,0.10)"
              : "rgba(255,180,171,0.12)",
            color: exec?.supported
              ? exec?.intent === "atomic_one_click" ? "#bec2ff" : "#c6c5d8"
              : "#ffb4ab",
          }}
        >
          {exec?.supported ? intent : "Quote only"}
        </span>
      </div>

      {/* Output amount */}
      <div>
        <div className="text-xl font-bold font-mono tabular-nums leading-none" style={{ color: "#e5e2e1" }}>
          {out}
        </div>
        <div className="text-[10px] font-mono mt-0.5" style={{ color: "#908fa1" }}>
          {last?.to_asset}
          <span className="mx-1 opacity-30">·</span>
          <span style={{ color: "#bec2ff" }}>{cshort(last?.to_chain)}</span>
        </div>
      </div>

      {/* Stats row */}
      <div className="flex items-center gap-2 text-[10px] font-mono" style={{ color: "#908fa1" }}>
        <span>~{mins}m</span>
        <span className="opacity-30">·</span>
        <span>{fee}</span>
        <span className="opacity-30">·</span>
        <span>{route.hops.length} hop{route.hops.length !== 1 ? "s" : ""}</span>
      </div>

      {/* Selected indicator */}
      {selected && (
        <div
          className="absolute top-0 right-0 w-6 h-6 flex items-center justify-center"
          style={{ backgroundColor: "#c6c6c7", color: "#131313" }}
        >
          <span className="material-symbols-outlined" style={{ fontSize: "14px" }}>check</span>
        </div>
      )}
    </motion.div>
  );
}
