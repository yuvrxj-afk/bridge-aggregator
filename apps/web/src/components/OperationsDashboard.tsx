import { useState, useRef, useEffect, useCallback } from "react";
import { ChainIcon } from "./ChainIcon";
import { fetchAdapterHealth, fetchOperations, type AdapterHealth, type OperationDetail } from "../api";
import { formatUnits } from "viem";

// ── Design tokens ────────────────────────────────────────────────────────────

const C = {
  surface:               "#131313",
  surfaceContainer:      "#201f1f",
  surfaceContainerHigh:  "#2a2a2a",
  surfaceContainerLow:   "#0e0e0e",
  onSurface:             "#e5e2e1",
  onSurfaceVariant:      "#c6c5d8",
  accent:                "#bec2ff",
  err:                   "#ffb4ab",
  amber:                 "#fbbf24",
  green:                 "#4ade80",
} as const;

// ── Types ────────────────────────────────────────────────────────────────────

type TabKey = "active" | "past" | "failed";

const TAB_KEYS: TabKey[] = ["active", "past", "failed"];

const TAB_LABELS: Record<TabKey, string> = {
  active: "Active",
  past:   "Past",
  failed: "Failed",
};

// ── Helpers ──────────────────────────────────────────────────────────────────

function dbStatusToTab(status: string): TabKey {
  if (status === "pending" || status === "submitted") return "active";
  if (status === "failed") return "failed";
  return "past";
}

function statusLabel(status: string): string {
  if (status === "pending")   return "Pending";
  if (status === "submitted") return "Submitted";
  if (status === "completed") return "Completed";
  if (status === "failed")    return "Failed";
  return status;
}

function statusColor(status: string): string {
  if (status === "failed")    return C.err;
  if (status === "completed") return C.green;
  return C.amber;
}

function truncateId(id: string) {
  return id.length > 18 ? `${id.slice(0, 10)}…${id.slice(-6)}` : id;
}

function formatAmount(amountBaseUnits: string, decimals: number): string {
  try {
    const n = Number(formatUnits(BigInt(amountBaseUnits), decimals));
    if (n === 0) return "0";
    if (n >= 1000) return n.toLocaleString(undefined, { maximumFractionDigits: 2 });
    if (n >= 1)    return n.toFixed(4).replace(/\.?0+$/, "");
    return n.toPrecision(4);
  } catch {
    return amountBaseUnits;
  }
}

function explorerUrl(chainId: number, txHash: string): string {
  const explorers: Record<number, string> = {
    1:         "https://etherscan.io/tx/",
    10:        "https://optimistic.etherscan.io/tx/",
    137:       "https://polygonscan.com/tx/",
    42161:     "https://arbiscan.io/tx/",
    8453:      "https://basescan.org/tx/",
    11155111:  "https://sepolia.etherscan.io/tx/",
    84532:     "https://sepolia.basescan.org/tx/",
    421614:    "https://sepolia.arbiscan.io/tx/",
    11155420:  "https://sepolia-optimism.etherscan.io/tx/",
  };
  const base = explorers[chainId] ?? `https://blockscan.com/tx/`;
  return base + txHash;
}

function relativeTime(dateStr: string): string {
  const diff = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
  if (diff < 60)   return `${diff}s ago`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

function statusDotColor(s: AdapterHealth["status"]) {
  if (s === "healthy")  return C.green;
  if (s === "degraded") return C.amber;
  return C.err;
}

// ── Sub-components ───────────────────────────────────────────────────────────

function StatusBadge({ status }: { status: string }) {
  const color = statusColor(status);
  const bg = `${color}18`;
  return (
    <span
      className="text-[10px] font-semibold uppercase tracking-wider px-2.5 py-0.5 rounded"
      style={{ color, backgroundColor: bg }}
    >
      {statusLabel(status)}
    </span>
  );
}

function OperationCard({ op }: { op: OperationDetail }) {
  const firstHop = op.route.hops[0];
  const lastHop  = op.route.hops[op.route.hops.length - 1];
  if (!firstHop || !lastHop) return null;

  const fromAmount = formatAmount(firstHop.amount_in_base_units, 6);
  const toAmount   = formatAmount(op.route.estimated_output_amount, 6);
  const bridgeNames = [...new Set(op.route.hops.map(h => h.bridge_id))].join(" + ");

  const events = op.events ?? [];
  const timeline = events.slice().reverse().map(ev => ({
    label: ev.event_type === "created" ? "Operation created" :
           ev.event_type === "status_transition" ? `${ev.from_status} → ${ev.to_status}` :
           ev.event_type,
    timestamp: relativeTime(ev.created_at),
    status: (ev.to_status === "failed" ? "failed" : "done") as "done" | "failed" | "pending",
  }));

  const txUrl = op.tx_hash
    ? explorerUrl(firstHop.from_chain === "ethereum" ? 1 : firstHop.from_chain === "sepolia" ? 11155111 : 0, op.tx_hash)
    : null;

  return (
    <div className="rounded" style={{ backgroundColor: C.surfaceContainer }}>
      <div className="p-5 flex flex-col gap-3">
        {/* top row: badge + id + time */}
        <div className="flex items-center justify-between gap-3">
          <StatusBadge status={op.status} />
          <div className="flex items-center gap-3 min-w-0">
            <span
              className="text-xs font-mono tabular-nums truncate"
              style={{ color: C.onSurfaceVariant }}
              title={op.operation_id}
            >
              {truncateId(op.operation_id)}
            </span>
            <span className="text-[10px] font-mono shrink-0" style={{ color: C.onSurfaceVariant }}>
              {relativeTime(op.created_at)}
            </span>
          </div>
        </div>

        {/* route summary */}
        <div className="flex items-center gap-2 flex-wrap">
          <div className="flex items-center gap-1.5">
            <ChainIcon chainId={firstHop.from_chain === "ethereum" ? 1 : firstHop.from_chain === "sepolia" ? 11155111 : 0} size={14} />
            <span className="text-sm font-medium" style={{ color: C.onSurface }}>
              {fromAmount} {firstHop.from_asset}
            </span>
            <span className="text-[10px]" style={{ color: C.onSurfaceVariant }}>{firstHop.from_chain}</span>
          </div>

          <span className="text-xs" style={{ color: C.onSurfaceVariant }}>→</span>

          <div className="flex items-center gap-1.5">
            <ChainIcon chainId={lastHop.to_chain === "ethereum" ? 1 : lastHop.to_chain === "base" ? 8453 : lastHop.to_chain === "base_sepolia" ? 84532 : 0} size={14} />
            <span className="text-sm font-medium" style={{ color: C.onSurface }}>
              {toAmount} {lastHop.to_asset}
            </span>
            <span className="text-[10px]" style={{ color: C.onSurfaceVariant }}>{lastHop.to_chain}</span>
          </div>

          <span
            className="ml-auto text-[10px] uppercase tracking-wider font-medium px-2 py-0.5 rounded"
            style={{ color: C.accent, backgroundColor: `${C.accent}14` }}
          >
            {bridgeNames}
          </span>
        </div>

        {/* timeline from DB events */}
        {timeline.length > 0 && (
          <div className="flex flex-col gap-2 mt-1">
            {timeline.map((step, i) => {
              const dotColor =
                step.status === "done"   ? C.green :
                step.status === "failed" ? C.err :
                C.onSurfaceVariant;
              const isLast = i === timeline.length - 1;
              return (
                <div key={i} className="flex items-start gap-3">
                  <div className="flex flex-col items-center">
                    <div className="w-2 h-2 rounded-full shrink-0 mt-[3px]" style={{ backgroundColor: dotColor }} />
                    {!isLast && <div className="w-px flex-1 min-h-[16px]" style={{ backgroundColor: C.surfaceContainerHigh }} />}
                  </div>
                  <div className="flex items-baseline justify-between flex-1 gap-4 pb-1">
                    <span className="text-xs" style={{ color: step.status === "failed" ? C.err : C.onSurface }}>{step.label}</span>
                    <span className="text-[10px] font-mono tabular-nums" style={{ color: C.onSurfaceVariant }}>{step.timestamp}</span>
                  </div>
                </div>
              );
            })}
          </div>
        )}

        {/* action buttons */}
        {txUrl && (
          <div className="flex items-center gap-2 mt-1">
            <a
              href={txUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="text-[11px] font-semibold px-3 py-1.5 rounded transition-opacity hover:opacity-80"
              style={{ backgroundColor: C.surfaceContainerHigh, color: C.onSurfaceVariant }}
            >
              Open Explorer
            </a>
          </div>
        )}
      </div>
    </div>
  );
}

function AdapterHealthPanel({
  adapters,
  loading,
  error,
}: {
  adapters: AdapterHealth[];
  loading: boolean;
  error: string | null;
}) {
  const bridges = adapters.filter(a => a.kind === "bridge");
  const dexes = adapters.filter(a => a.kind === "dex");

  const statusBadge = (a: AdapterHealth) => {
    const color = statusDotColor(a.status);
    const label = a.status === "healthy" ? "OK" : a.status === "degraded" ? "DEG" : "DOWN";
    return (
      <span
        className="text-[9px] font-mono font-bold px-1.5 py-0.5 rounded-sm shrink-0"
        style={{ color, backgroundColor: `${color}18`, border: `1px solid ${color}30` }}
      >
        {label}
      </span>
    );
  };

  const latencyBadge = (ms: number) => {
    const color = ms === 0 ? C.onSurfaceVariant : ms < 300 ? C.green : ms < 800 ? C.amber : C.err;
    return (
      <span className="text-[9px] font-mono tabular-nums" style={{ color }}>
        {ms === 0 ? "—" : `${ms}ms`}
      </span>
    );
  };

  const adapterName = (service: string) => {
    const names: Record<string, string> = {
      across: "Across", cctp: "Circle CCTP", stargate: "Stargate",
      canonical_base: "Base Bridge", canonical_optimism: "OP Bridge",
      canonical_arbitrum: "Arb Bridge", mayan: "Mayan", blockdaemon: "Blockdaemon",
      uniswap_trading_api: "Uniswap", oneinch: "1inch", zeroex: "0x",
      blockdaemon_dex: "Blockdaemon DEX",
    };
    return names[service] ?? service.replace(/_/g, " ").replace(/\b\w/g, c => c.toUpperCase());
  };

  const renderGroup = (rows: AdapterHealth[]) => (
    <div className="grid grid-cols-1 gap-1.5 px-4 pb-3">
      {rows.map((a) => (
        <div
          key={`${a.kind}-${a.service}`}
          className="flex items-start gap-2.5 px-3 py-2.5 rounded"
          style={{ backgroundColor: C.surfaceContainerLow }}
        >
          <div className="mt-0.5">{statusBadge(a)}</div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center justify-between gap-2">
              <span className="text-[12px] font-medium truncate" style={{ color: C.onSurface }}>
                {adapterName(a.service)}
              </span>
              {latencyBadge(a.latency_ms)}
            </div>
            {!!a.reason && a.status !== "healthy" && (
              <p className="text-[10px] mt-1 leading-snug" style={{ color: C.onSurfaceVariant, opacity: 0.75 }}>
                {a.reason.length > 120 ? `${a.reason.slice(0, 120)}…` : a.reason}
              </p>
            )}
          </div>
        </div>
      ))}
    </div>
  );

  const healthyCount = adapters.filter(a => a.status === "healthy").length;

  return (
    <div className="rounded overflow-hidden" style={{ backgroundColor: C.surfaceContainer }}>
      {/* Header */}
      <div className="px-4 py-3 flex items-center justify-between border-b border-[#2a2a2a]">
        <span className="text-xs font-semibold tracking-wide" style={{ color: C.onSurface }}>
          Adapter Health
        </span>
        {!loading && !error && adapters.length > 0 && (
          <span className="text-[10px] font-mono" style={{ color: healthyCount === adapters.length ? C.green : C.amber }}>
            {healthyCount}/{adapters.length} healthy
          </span>
        )}
      </div>

      {loading ? (
        <div className="px-4 py-5 flex items-center gap-2">
          <span className="w-3 h-3 rounded-full animate-pulse" style={{ backgroundColor: C.accent }} />
          <span className="text-xs" style={{ color: C.onSurfaceVariant }}>Checking adapters…</span>
        </div>
      ) : error ? (
        <div className="px-4 py-4 text-xs" style={{ color: C.err }}>{error}</div>
      ) : adapters.length === 0 ? (
        <div className="px-4 py-4 text-xs" style={{ color: C.onSurfaceVariant }}>No adapters reported</div>
      ) : (
        <>
          {bridges.length > 0 && (
            <>
              <div className="px-4 pt-3 pb-1.5 flex items-center gap-2">
                <span className="text-[9px] font-mono uppercase tracking-widest font-bold" style={{ color: C.accent }}>
                  Bridges
                </span>
                <span className="text-[9px] font-mono" style={{ color: C.onSurfaceVariant }}>({bridges.length})</span>
              </div>
              {renderGroup(bridges)}
            </>
          )}
          {dexes.length > 0 && (
            <>
              <div className="px-4 pt-1.5 pb-1.5 flex items-center gap-2 border-t border-[#2a2a2a]">
                <span className="text-[9px] font-mono uppercase tracking-widest font-bold" style={{ color: C.accent }}>
                  DEXes
                </span>
                <span className="text-[9px] font-mono" style={{ color: C.onSurfaceVariant }}>({dexes.length})</span>
              </div>
              {renderGroup(dexes)}
            </>
          )}
        </>
      )}
    </div>
  );
}

function RecentActivity({ ops }: { ops: OperationDetail[] }) {
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [ops.length]);

  return (
    <div
      className="rounded overflow-hidden flex flex-col"
      style={{ backgroundColor: C.surfaceContainerLow }}
    >
      <div className="px-4 py-2.5" style={{ backgroundColor: C.surfaceContainer }}>
        <span className="text-xs font-semibold tracking-wide" style={{ color: C.onSurface }}>
          Recent Activity
        </span>
      </div>

      <div className="h-[280px] overflow-y-auto px-4 py-3 flex flex-col gap-1 font-mono text-[11px] leading-relaxed">
        {ops.length === 0 ? (
          <span style={{ color: C.onSurfaceVariant }}>No operations yet</span>
        ) : (
          ops.flatMap(op =>
            (op.events ?? []).map((ev, i) => (
              <div key={`${op.operation_id}-${i}`} className="flex gap-2">
                <span className="tabular-nums shrink-0" style={{ color: C.onSurfaceVariant }}>
                  {relativeTime(ev.created_at)}
                </span>
                <span
                  className="font-semibold shrink-0"
                  style={{ color: ev.event_type === "status_transition" && ev.to_status === "failed" ? C.err : C.accent }}
                >
                  [{ev.event_type.toUpperCase().slice(0, 3)}]
                </span>
                <span style={{ color: C.onSurface }}>
                  op={truncateId(op.operation_id)} {ev.to_status ? `→ ${ev.to_status}` : ""} {ev.tx_hash ? `hash=${ev.tx_hash.slice(0, 10)}…` : ""}
                </span>
              </div>
            ))
          )
        )}
        <div ref={bottomRef} />
      </div>
    </div>
  );
}

// ── Main component ───────────────────────────────────────────────────────────

export function OperationsDashboard() {
  const [tab, setTab] = useState<TabKey>("active");
  const [ops, setOps] = useState<OperationDetail[]>([]);
  const [opsLoading, setOpsLoading] = useState(true);
  const [opsError, setOpsError] = useState<string | null>(null);
  const [adapters, setAdapters] = useState<AdapterHealth[]>([]);
  const [healthLoading, setHealthLoading] = useState(true);
  const [healthError, setHealthError] = useState<string | null>(null);
  const [chainScope, setChainScope] = useState<string>(() => {
    const s = window.localStorage.getItem("chain_scope");
    return s === "testnet" ? "testnet" : "mainnet";
  });

  useEffect(() => {
    const handler = (e: Event) => {
      const detail = (e as CustomEvent<string>).detail;
      if (detail === "mainnet" || detail === "testnet") setChainScope(detail);
    };
    window.addEventListener("chain-scope-change", handler);
    return () => window.removeEventListener("chain-scope-change", handler);
  }, []);

  const loadOps = useCallback(async (cancelled: { v: boolean }, scope: string) => {
    try {
      const data = await fetchOperations(50, scope);
      if (cancelled.v) return;
      setOps(data.operations ?? []);
      setOpsError(null);
    } catch (e) {
      if (cancelled.v) return;
      setOpsError(e instanceof Error ? e.message : "Failed to load operations");
    } finally {
      if (!cancelled.v) setOpsLoading(false);
    }
  }, []);

  useEffect(() => {
    const cancelled = { v: false };
    loadOps(cancelled, chainScope);
    const id = window.setInterval(() => loadOps(cancelled, chainScope), 10_000);
    return () => {
      cancelled.v = true;
      window.clearInterval(id);
    };
  }, [loadOps, chainScope]);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const data = await fetchAdapterHealth();
        if (cancelled) return;
        setAdapters(data.adapters ?? []);
        setHealthError(null);
      } catch (e) {
        if (cancelled) return;
        setHealthError(e instanceof Error ? e.message : "Failed to load adapter health");
      } finally {
        if (!cancelled) setHealthLoading(false);
      }
    };
    load();
    const id = window.setInterval(load, 30_000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, []);

  const filteredOps = ops.filter(op => dbStatusToTab(op.status) === tab);

  return (
    <div className="max-w-7xl mx-auto w-full">
      <div>
        {/* Page header */}
        <div className="mb-8">
          <h1 className="text-2xl font-semibold tracking-tight" style={{ color: C.onSurface }}>
            Operations Dashboard
          </h1>
          <p className="text-sm mt-1" style={{ color: C.onSurfaceVariant }}>
            Global Transaction Monitoring &amp; Error Recovery
          </p>
        </div>

        <div className="flex gap-6 items-start flex-col lg:flex-row">
          {/* ── Left: operations ── */}
          <div className="flex-1 min-w-0 flex flex-col gap-5">
            {/* Tab bar */}
            <div className="flex gap-1 rounded p-1" style={{ backgroundColor: C.surfaceContainer }}>
              {TAB_KEYS.map((key) => {
                const active = tab === key;
                return (
                  <button
                    key={key}
                    onClick={() => setTab(key)}
                    className="flex-1 text-xs font-semibold py-2 rounded transition-colors cursor-pointer"
                    style={{
                      backgroundColor: active ? C.surfaceContainerHigh : "transparent",
                      color: active ? C.accent : C.onSurfaceVariant,
                    }}
                  >
                    {TAB_LABELS[key]}
                  </button>
                );
              })}
            </div>

            {/* Operation cards */}
            {opsLoading ? (
              <div className="py-16 text-center">
                <span className="text-sm" style={{ color: C.onSurfaceVariant }}>Loading operations…</span>
              </div>
            ) : opsError ? (
              <div className="py-16 text-center">
                <span className="text-sm" style={{ color: C.err }}>{opsError}</span>
              </div>
            ) : filteredOps.length === 0 ? (
              <div className="py-16 text-center">
                <span className="text-sm" style={{ color: C.onSurfaceVariant }}>
                  No {TAB_LABELS[tab].toLowerCase()} operations
                </span>
              </div>
            ) : (
              <div className="flex flex-col gap-3">
                {filteredOps.map((op) => (
                  <OperationCard key={op.operation_id} op={op} />
                ))}
              </div>
            )}
          </div>

          {/* ── Right sidebar ── */}
          <div className="w-full lg:w-[380px] shrink-0 flex flex-col gap-4 lg:sticky lg:top-6">
            <RecentActivity ops={ops} />
            <AdapterHealthPanel adapters={adapters} loading={healthLoading} error={healthError} />
          </div>
        </div>
      </div>
    </div>
  );
}

export default OperationsDashboard;
