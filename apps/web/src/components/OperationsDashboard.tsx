import { useState, useRef, useEffect } from "react";
import { ChainIcon } from "./ChainIcon";
import { fetchAdapterHealth, type AdapterHealth } from "../api";

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

type OpStatus = "active" | "completed" | "failed";

interface TimelineStep {
  label: string;
  timestamp: string;
  status: "done" | "pending" | "failed";
}

interface Operation {
  id: string;
  status: OpStatus;
  fromChain: string;
  fromChainId: number;
  fromToken: string;
  fromAmount: string;
  toChain: string;
  toChainId: number;
  toToken: string;
  toAmount: string;
  bridge: string;
  timeline: TimelineStep[];
  error?: string;
  explorerUrl: string;
}

interface LogEntry {
  timestamp: string;
  level: "SYS" | "INF" | "ERR";
  message: string;
}

// ── Mock data ────────────────────────────────────────────────────────────────

const MOCK_OPS: Operation[] = [
  {
    id: "0xa4f8c91d3e27b056f3a2c8d1e9b74f0621c8ed3a",
    status: "failed",
    fromChain: "Ethereum",
    fromChainId: 1,
    fromToken: "USDC",
    fromAmount: "2,500.00",
    toChain: "Arbitrum",
    toChainId: 42161,
    toToken: "USDC",
    toAmount: "2,497.80",
    bridge: "Across V3",
    timeline: [
      { label: "Quote accepted",   timestamp: "14:32:01", status: "done" },
      { label: "Approval sent",    timestamp: "14:32:18", status: "done" },
      { label: "Deposit tx",       timestamp: "14:32:45", status: "done" },
      { label: "Relay fill",       timestamp: "14:33:12", status: "failed" },
    ],
    error: "RelayerFillTimeout: no relayer picked up the deposit within the SLA window (120 s). Deposit hash 0xa4f8…ed3a is still claimable on-chain.",
    explorerUrl: "https://etherscan.io/tx/0xa4f8c91d",
  },
  {
    id: "0x7b2e0f15d84ca963e1fd08723bc5a19e47d6f0b2",
    status: "active",
    fromChain: "Polygon",
    fromChainId: 137,
    fromToken: "USDT",
    fromAmount: "10,000.00",
    toChain: "Ethereum",
    toChainId: 1,
    toToken: "USDT",
    toAmount: "9,988.50",
    bridge: "Stargate",
    timeline: [
      { label: "Quote accepted",   timestamp: "14:40:05", status: "done" },
      { label: "Approval sent",    timestamp: "14:40:22", status: "done" },
      { label: "Deposit tx",       timestamp: "14:40:58", status: "done" },
      { label: "Awaiting finality", timestamp: "—",       status: "pending" },
    ],
    explorerUrl: "https://polygonscan.com/tx/0x7b2e0f15",
  },
  {
    id: "0x1c9d4a38f62eb071d5fc09284eab7c3f85a01d6e",
    status: "completed",
    fromChain: "Arbitrum",
    fromChainId: 42161,
    fromToken: "ETH",
    fromAmount: "1.5000",
    toChain: "Polygon",
    toChainId: 137,
    toToken: "ETH",
    toAmount: "1.4985",
    bridge: "Across V3",
    timeline: [
      { label: "Quote accepted",   timestamp: "13:15:40", status: "done" },
      { label: "Deposit tx",       timestamp: "13:15:58", status: "done" },
      { label: "Relay fill",       timestamp: "13:16:22", status: "done" },
      { label: "Confirmed",        timestamp: "13:16:44", status: "done" },
    ],
    explorerUrl: "https://arbiscan.io/tx/0x1c9d4a38",
  },
];

const MOCK_LOGS: LogEntry[] = [
  { timestamp: "14:40:58", level: "INF", message: "deposit tx confirmed  hash=0x7b2e…f0b2  chain=polygon  block=62831044" },
  { timestamp: "14:40:55", level: "SYS", message: "waiting for receipt   hash=0x7b2e…f0b2  confirmations=1" },
  { timestamp: "14:40:22", level: "INF", message: "approval tx sent     spender=0x2967…3a1f  token=USDT  chain=polygon" },
  { timestamp: "14:40:06", level: "SYS", message: "quote locked         op=0x7b2e…f0b2  bridge=stargate  ttl=90s" },
  { timestamp: "14:33:12", level: "ERR", message: "relay fill timeout   op=0xa4f8…ed3a  elapsed=120s  bridge=across_v3" },
  { timestamp: "14:32:45", level: "INF", message: "deposit tx confirmed  hash=0xa4f8…ed3a  chain=ethereum  block=19482011" },
  { timestamp: "14:32:18", level: "INF", message: "approval tx sent     spender=0x5c7B…e120  token=USDC  chain=ethereum" },
  { timestamp: "14:32:01", level: "SYS", message: "quote locked         op=0xa4f8…ed3a  bridge=across_v3  ttl=120s" },
  { timestamp: "13:16:44", level: "INF", message: "bridge complete      op=0x1c9d…1d6e  output=1.4985 ETH  dest=polygon" },
  { timestamp: "13:16:22", level: "INF", message: "relay fill confirmed hash=0x1c9d…1d6e  chain=polygon  block=62830188" },
  { timestamp: "13:15:58", level: "INF", message: "deposit tx confirmed  hash=0x1c9d…1d6e  chain=arbitrum  block=198402155" },
  { timestamp: "13:15:40", level: "SYS", message: "quote locked         op=0x1c9d…1d6e  bridge=across_v3  ttl=120s" },
];

// ── Helpers ──────────────────────────────────────────────────────────────────

const TAB_KEYS = ["active", "past", "failed"] as const;
type TabKey = (typeof TAB_KEYS)[number];

const TAB_LABELS: Record<TabKey, string> = {
  active: "Active",
  past:   "Past",
  failed: "Failed",
};

function statusColor(s: OpStatus) {
  if (s === "failed")    return C.err;
  if (s === "active")    return C.amber;
  return C.green;
}

function statusLabel(s: OpStatus) {
  if (s === "failed")    return "Failed";
  if (s === "active")    return "Pending";
  return "Completed";
}

function filterOps(tab: TabKey): Operation[] {
  if (tab === "active") return MOCK_OPS.filter(o => o.status === "active");
  if (tab === "failed") return MOCK_OPS.filter(o => o.status === "failed");
  return MOCK_OPS.filter(o => o.status === "completed");
}

function truncateId(id: string) {
  return id.length > 16 ? `${id.slice(0, 10)}…${id.slice(-6)}` : id;
}

function logLevelColor(lvl: LogEntry["level"]) {
  if (lvl === "ERR") return C.err;
  if (lvl === "INF") return C.accent;
  return C.onSurfaceVariant;
}

function statusDotColor(s: AdapterHealth["status"]) {
  if (s === "healthy")  return C.green;
  if (s === "degraded") return C.amber;
  return C.err;
}

// ── Sub-components ───────────────────────────────────────────────────────────

function StatusBadge({ status }: { status: OpStatus }) {
  const bg = `${statusColor(status)}18`;
  return (
    <span
      className="text-[10px] font-semibold uppercase tracking-wider px-2.5 py-0.5 rounded"
      style={{ color: statusColor(status), backgroundColor: bg }}
    >
      {statusLabel(status)}
    </span>
  );
}

function TimelineStepper({ steps }: { steps: TimelineStep[] }) {
  return (
    <div className="flex flex-col gap-2 mt-3">
      {steps.map((step, i) => {
        const dotColor =
          step.status === "done"    ? C.green :
          step.status === "failed"  ? C.err :
          C.onSurfaceVariant;
        const isLast = i === steps.length - 1;

        return (
          <div key={i} className="flex items-start gap-3">
            {/* vertical track */}
            <div className="flex flex-col items-center">
              <div
                className="w-2 h-2 rounded-full shrink-0 mt-[3px]"
                style={{ backgroundColor: dotColor }}
              />
              {!isLast && (
                <div className="w-px flex-1 min-h-[16px]" style={{ backgroundColor: C.surfaceContainerHigh }} />
              )}
            </div>

            {/* label + timestamp */}
            <div className="flex items-baseline justify-between flex-1 gap-4 pb-1">
              <span
                className="text-xs"
                style={{ color: step.status === "failed" ? C.err : C.onSurface }}
              >
                {step.label}
              </span>
              <span className="text-[10px] font-mono tabular-nums" style={{ color: C.onSurfaceVariant }}>
                {step.timestamp}
              </span>
            </div>
          </div>
        );
      })}
    </div>
  );
}

function OperationCard({ op }: { op: Operation }) {
  return (
    <div className="rounded" style={{ backgroundColor: C.surfaceContainer }}>
      <div className="p-5 flex flex-col gap-3">
        {/* top row: badge + id */}
        <div className="flex items-center justify-between gap-3">
          <StatusBadge status={op.status} />
          <span
            className="text-xs font-mono tabular-nums truncate"
            style={{ color: C.onSurfaceVariant }}
            title={op.id}
          >
            {truncateId(op.id)}
          </span>
        </div>

        {/* route summary */}
        <div className="flex items-center gap-2 flex-wrap">
          <div className="flex items-center gap-1.5">
            <ChainIcon chainId={op.fromChainId} size={14} />
            <span className="text-sm font-medium" style={{ color: C.onSurface }}>
              {op.fromAmount} {op.fromToken}
            </span>
            <span className="text-[10px]" style={{ color: C.onSurfaceVariant }}>{op.fromChain}</span>
          </div>

          <span className="text-xs" style={{ color: C.onSurfaceVariant }}>→</span>

          <div className="flex items-center gap-1.5">
            <ChainIcon chainId={op.toChainId} size={14} />
            <span className="text-sm font-medium" style={{ color: C.onSurface }}>
              {op.toAmount} {op.toToken}
            </span>
            <span className="text-[10px]" style={{ color: C.onSurfaceVariant }}>{op.toChain}</span>
          </div>

          <span
            className="ml-auto text-[10px] uppercase tracking-wider font-medium px-2 py-0.5 rounded"
            style={{ color: C.accent, backgroundColor: `${C.accent}14` }}
          >
            {op.bridge}
          </span>
        </div>

        {/* timeline */}
        <TimelineStepper steps={op.timeline} />

        {/* error callout */}
        {op.error && (
          <div className="rounded px-3.5 py-2.5 text-xs leading-relaxed" style={{ backgroundColor: `${C.err}10`, color: C.err }}>
            {op.error}
          </div>
        )}

        {/* action buttons */}
        <div className="flex items-center gap-2 mt-1">
          {op.status === "failed" && (
            <>
              <ActionButton label="Retry from Step" accent />
              <ActionButton label="Re-quote" />
            </>
          )}
          <ActionButton label="Open Explorer" href={op.explorerUrl} />
        </div>
      </div>
    </div>
  );
}

function ActionButton({ label, accent, href }: { label: string; accent?: boolean; href?: string }) {
  const style: React.CSSProperties = accent
    ? { backgroundColor: C.accent, color: C.surfaceContainerLow }
    : { backgroundColor: C.surfaceContainerHigh, color: C.onSurfaceVariant };

  const className = "text-[11px] font-semibold px-3 py-1.5 rounded cursor-pointer transition-opacity hover:opacity-80";

  if (href) {
    return (
      <a href={href} target="_blank" rel="noopener noreferrer" className={className} style={style}>
        {label}
      </a>
    );
  }
  return (
    <button className={className} style={style}>
      {label}
    </button>
  );
}

function TerminalLogs({ logs }: { logs: LogEntry[] }) {
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [logs.length]);

  return (
    <div
      className="rounded overflow-hidden flex flex-col"
      style={{ backgroundColor: C.surfaceContainerLow }}
    >
      <div className="px-4 py-2.5" style={{ backgroundColor: C.surfaceContainer }}>
        <span className="text-xs font-semibold tracking-wide" style={{ color: C.onSurface }}>
          Live Terminal Logs
        </span>
      </div>

      <div className="h-[280px] overflow-y-auto px-4 py-3 flex flex-col gap-1 font-mono text-[11px] leading-relaxed">
        {logs.map((entry, i) => (
          <div key={i} className="flex gap-2">
            <span className="tabular-nums shrink-0" style={{ color: C.onSurfaceVariant }}>
              {entry.timestamp}
            </span>
            <span className="font-semibold shrink-0" style={{ color: logLevelColor(entry.level) }}>
              [{entry.level}]
            </span>
            <span style={{ color: entry.level === "ERR" ? C.err : C.onSurface }}>
              {entry.message}
            </span>
          </div>
        ))}
        <div ref={bottomRef} />
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

  const renderRows = (rows: AdapterHealth[]) => (
    <div className="flex flex-col">
      {rows.map((a) => (
        <div key={`${a.kind}-${a.service}`} className="px-4 py-2.5 border-t first:border-t-0 border-[#2a2a2a]">
          <div className="flex items-center gap-3">
            <div className="w-2 h-2 rounded-full shrink-0" style={{ backgroundColor: statusDotColor(a.status) }} />
            <span className="text-xs flex-1 truncate" style={{ color: C.onSurface }}>
              {a.service}
            </span>
            <span className="text-[11px] font-mono tabular-nums shrink-0" style={{ color: C.onSurfaceVariant }}>
              {a.latency_ms} ms
            </span>
          </div>
          {!!a.reason && (
            <p className="text-[10px] mt-1 leading-relaxed break-words overflow-hidden" style={{ color: C.onSurfaceVariant, maxHeight: "4.5em" }}>
              {a.reason.length > 200 ? `${a.reason.substring(0, 200)}...` : a.reason}
            </p>
          )}
        </div>
      ))}
    </div>
  );

  return (
    <div className="rounded" style={{ backgroundColor: C.surfaceContainer }}>
      <div className="px-4 py-2.5">
        <span className="text-xs font-semibold tracking-wide" style={{ color: C.onSurface }}>
          Adapter Health
        </span>
      </div>

      {loading ? (
        <div className="px-4 py-4 text-xs" style={{ color: C.onSurfaceVariant }}>Loading adapter checks…</div>
      ) : error ? (
        <div className="px-4 py-4 text-xs" style={{ color: C.err }}>{error}</div>
      ) : adapters.length === 0 ? (
        <div className="px-4 py-4 text-xs" style={{ color: C.onSurfaceVariant }}>No adapters reported</div>
      ) : (
        <div>
          <div className="px-4 py-1 text-[10px] uppercase tracking-wider font-semibold" style={{ color: C.accent }}>
            Bridges ({bridges.length})
          </div>
          {renderRows(bridges)}
          <div className="px-4 py-1 text-[10px] uppercase tracking-wider font-semibold border-t border-[#2a2a2a]" style={{ color: C.accent }}>
            DEXes ({dexes.length})
          </div>
          {renderRows(dexes)}
        </div>
      )}
    </div>
  );
}

// ── Main component ───────────────────────────────────────────────────────────

export function OperationsDashboard() {
  const [tab, setTab] = useState<TabKey>("active");
  const [adapters, setAdapters] = useState<AdapterHealth[]>([]);
  const [healthLoading, setHealthLoading] = useState(true);
  const [healthError, setHealthError] = useState<string | null>(null);
  const ops = filterOps(tab);

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
    const id = window.setInterval(load, 30000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, []);

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
            {ops.length === 0 ? (
              <div className="py-16 text-center">
                <span className="text-sm" style={{ color: C.onSurfaceVariant }}>
                  No {TAB_LABELS[tab].toLowerCase()} operations
                </span>
              </div>
            ) : (
              <div className="flex flex-col gap-3">
                {ops.map((op) => (
                  <OperationCard key={op.id} op={op} />
                ))}
              </div>
            )}
          </div>

          {/* ── Right sidebar ── */}
          <div className="w-full lg:w-[380px] shrink-0 flex flex-col gap-4 lg:sticky lg:top-6">
            <TerminalLogs logs={MOCK_LOGS} />
            <AdapterHealthPanel adapters={adapters} loading={healthLoading} error={healthError} />
          </div>
        </div>
      </div>
    </div>
  );
}

export default OperationsDashboard;
