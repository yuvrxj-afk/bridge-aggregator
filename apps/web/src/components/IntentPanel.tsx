import { useState, useCallback, useRef, useEffect } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { fetchParseIntent, BridgeError } from "../api";
import {
  mergeParsedIntent,
  normalizeBackendIntent,
  parseIntent,
  validateIntent,
  type IntentExecuteEventDetail,
  type ParsedIntent,
} from "../lib/parseIntent";

interface IntentPanelProps {
  onClose: () => void;
}

// ── Log entry types ────────────────────────────────────────────────────────────

type LogEntry =
  | { id: number; type: "system"; timestamp: string; text: string }
  | { id: number; type: "user"; text: string }
  | { id: number; type: "parsing" }
  | { id: number; type: "result"; parsed: ParsedIntent }
  | { id: number; type: "error"; text: string };

let _id = 0;
function nextId() { return ++_id; }

function nowTimestamp() {
  return new Date().toLocaleTimeString("en-GB", { hour12: false });
}

const INIT_LOGS: LogEntry[] = [
  { id: nextId(), type: "system", timestamp: "00:00:01", text: "Initializing sub-graph orchestration..." },
  { id: nextId(), type: "system", timestamp: "00:00:01", text: "Loading bridge adapter registry..." },
  { id: nextId(), type: "system", timestamp: "00:00:02", text: "System-01 Core Handshake: SUCCESS" },
  { id: nextId(), type: "system", timestamp: "00:00:02", text: "Ready. Type an intent to begin routing." },
];

function chainLabel(name: string) {
  const M: Record<string, string> = {
    ethereum: "Ethereum", base: "Base", arbitrum: "Arbitrum",
    optimism: "Optimism", polygon: "Polygon", avalanche: "Avalanche",
    bsc: "BNB Chain", solana: "Solana", sepolia: "Sepolia",
    "base-sepolia": "Base Sepolia", "arbitrum-sepolia": "Arb Sepolia",
    "op-sepolia": "OP Sepolia",
  };
  return M[name] ?? name;
}

// ── Component ─────────────────────────────────────────────────────────────────

export function IntentPanel({ onClose }: IntentPanelProps) {
  const [logs, setLogs] = useState<LogEntry[]>(INIT_LOGS);
  const [input, setValue] = useState("");
  const [parsing, setParsing] = useState(false);
  const [lastResult, setLastResult] = useState<ParsedIntent | null>(null);
  const [draftIntent, setDraftIntent] = useState<ParsedIntent | null>(null);
  const logEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // Auto-scroll to bottom when logs change.
  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [logs]);

  // Focus input when panel opens.
  useEffect(() => {
    const t = setTimeout(() => inputRef.current?.focus(), 100);
    return () => clearTimeout(t);
  }, []);

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const addLog = useCallback((entry: any) => {
    setLogs(prev => [...prev, { ...entry, id: nextId() } as LogEntry]);
  }, []);

  const submit = useCallback(() => {
    const text = input.trim();
    if (!text || parsing) return;

    setValue("");
    setLastResult(null);
    addLog({ type: "user", text });
    setParsing(true);
    addLog({ type: "parsing" });

    (async () => {
      try {
        const raw = await fetchParseIntent(text);
        const parsed = normalizeBackendIntent(raw);
        const merged = mergeParsedIntent(draftIntent, parsed);
        const issues = validateIntent(merged);

        setLogs(prev => {
          const idx = [...prev].reverse().findIndex(e => e.type === "parsing");
          if (idx < 0) return prev;
          const realIdx = prev.length - 1 - idx;
          const next = [...prev];
          if (issues.length > 0) {
            next[realIdx] = {
              id: nextId(),
              type: "error",
              text: `${issues.join(" · ")}. Clarify the missing parts, for example: "to USDT on Polygon".`,
            };
            return next;
          }
          next[realIdx] = { id: nextId(), type: "result", parsed: merged };
          return next;
        });
        setDraftIntent(merged);
        if (issues.length === 0) {
          setLastResult(merged);
        } else {
          setLastResult(null);
        }
      } catch (err) {
        // Defensive local fallback: keep intent UX working even if backend parsing
        // fails (auth/network/provider outage).
        const local = parseIntent(text);
        if (local) {
          const merged = mergeParsedIntent(draftIntent, local);
          const issues = validateIntent(merged);
          setLogs(prev => {
            const idx = [...prev].reverse().findIndex(e => e.type === "parsing");
            if (idx < 0) return prev;
            const realIdx = prev.length - 1 - idx;
            const next = [...prev];
            if (issues.length > 0) {
              next[realIdx] = {
                id: nextId(),
                type: "error",
                text: `${issues.join(" · ")}. Clarify the missing parts, for example: "to USDT on Polygon".`,
              };
              return next;
            }
            next[realIdx] = { id: nextId(), type: "result", parsed: merged };
            return next;
          });
          setDraftIntent(merged);
          if (issues.length === 0) setLastResult(merged);
        } else {
          const msg =
            err instanceof BridgeError
              ? err.message
              : "Intent parse failed. Please try again.";
          setLogs(prev => {
            const idx = [...prev].reverse().findIndex(e => e.type === "parsing");
            if (idx < 0) return prev;
            const realIdx = prev.length - 1 - idx;
            const next = [...prev];
            next[realIdx] = {
              id: nextId(),
              type: "error",
              text: msg,
            };
            return next;
          });
        }
      } finally {
        setParsing(false);
      }
    })();
  }, [input, parsing, addLog, draftIntent]);

  const handleKey = useCallback((e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") submit();
  }, [submit]);

  const executeIntent = useCallback(() => {
    if (!lastResult) return;
    const payload: IntentExecuteEventDetail = {
      parsed: lastResult,
      autoQuote: true,
      requestId: Date.now(),
    };
    window.dispatchEvent(new CustomEvent("intent-execute", { detail: payload }));
    addLog({ type: "system", timestamp: nowTimestamp(), text: "Intent dispatched → pre-filling form..." });
    setLastResult(null);
    setDraftIntent(null);
  }, [lastResult, addLog]);

  return (
    <motion.aside
      initial={{ x: "100%", opacity: 0 }}
      animate={{ x: 0, opacity: 1 }}
      exit={{ x: "100%", opacity: 0 }}
      transition={{ type: "spring", stiffness: 320, damping: 32 }}
      className="fixed right-0 top-16 h-[calc(100vh-64px)] w-full max-w-sm flex flex-col z-40 overflow-hidden"
      style={{
        backgroundColor: "#0e0e0e",
        borderLeft: "1px solid rgba(69,69,85,0.30)",
        boxShadow: "-8px 0 32px rgba(0,0,0,0.6)",
      }}
    >
      {/* Dot-grid texture overlay */}
      <div
        className="absolute inset-0 pointer-events-none opacity-[0.03]"
        style={{
          backgroundImage: "radial-gradient(#BEC2FF 0.5px, transparent 0.5px)",
          backgroundSize: "16px 16px",
        }}
      />

      {/* ── Header ── */}
      <header
        className="relative z-10 h-14 flex items-center justify-between px-4 shrink-0"
        style={{ borderBottom: "1px solid rgba(69,69,85,0.20)", backgroundColor: "#0e0e0e" }}
      >
        <div className="flex items-center gap-3">
          <span className="font-mono text-[10px] font-bold tracking-widest" style={{ color: "#c6c6c7" }}>
            INTENT_ENGINE
          </span>
          <div
            className="flex items-center gap-1.5 px-2 py-0.5"
            style={{
              backgroundColor: "rgba(33,42,202,0.20)",
              border: "1px solid rgba(190,194,255,0.20)",
            }}
          >
            <span
              className="w-1.5 h-1.5 rounded-full"
              style={{ backgroundColor: "#bec2ff", boxShadow: "0 0 6px #bec2ff" }}
            />
            <span className="font-mono text-[9px] uppercase tracking-tighter" style={{ color: "#bec2ff" }}>
              CLI_READY
            </span>
          </div>
        </div>
        <button
          onClick={onClose}
          className="transition-colors"
          style={{ color: "rgba(198,197,216,0.40)" }}
          onMouseEnter={e => { e.currentTarget.style.color = "#e5e2e1"; }}
          onMouseLeave={e => { e.currentTarget.style.color = "rgba(198,197,216,0.40)"; }}
        >
          <span className="material-symbols-outlined" style={{ fontSize: "18px" }}>close</span>
        </button>
      </header>

      {/* ── Log body ── */}
      <main
        className="relative z-10 flex-1 overflow-y-auto p-4 space-y-5"
        style={{ scrollbarWidth: "thin", scrollbarColor: "#454555 #0e0e0e" }}
      >
        {logs.map(entry => (
          <LogItem key={entry.id} entry={entry} onExecute={executeIntent} hasResult={!!lastResult && entry.type === "result"} />
        ))}
        <div ref={logEndRef} />
      </main>

      {/* ── Footer CLI input ── */}
      <footer
        className="relative z-10 p-4 shrink-0"
        style={{ borderTop: "1px solid rgba(69,69,85,0.30)", backgroundColor: "#0e0e0e" }}
      >
        <div
          className="flex items-center gap-3 h-10 px-3 transition-colors"
          style={{
            backgroundColor: "#131313",
            borderBottom: "1px solid rgba(190,194,255,0.25)",
          }}
          onFocus={e => { (e.currentTarget as HTMLDivElement).style.borderBottomColor = "#bec2ff"; }}
          onBlur={e => { (e.currentTarget as HTMLDivElement).style.borderBottomColor = "rgba(190,194,255,0.25)"; }}
        >
          <span className="font-mono font-bold text-sm shrink-0" style={{ color: "#bec2ff" }}>›</span>
          <input
            ref={inputRef}
            type="text"
            spellCheck={false}
            value={input}
            onChange={e => setValue(e.target.value)}
            onKeyDown={handleKey}
            placeholder="bridge 10 USDC from Ethereum to Base…"
            className="flex-1 bg-transparent border-none outline-none font-mono text-sm"
            style={{ color: "#e5e2e1" }}
          />
          {parsing && (
            <span
              className="animate-spin inline-block w-3 h-3 rounded-full border border-t-transparent shrink-0"
              style={{ borderColor: "rgba(190,194,255,0.40)", borderTopColor: "transparent" }}
            />
          )}
        </div>
        <div className="mt-3 flex justify-between items-center">
          <div className="flex gap-4">
            <span className="font-mono text-[9px] opacity-40" style={{ color: "#908fa1" }}>
              MEM: 128MB
            </span>
            <span className="font-mono text-[9px] opacity-40" style={{ color: "#908fa1" }}>
              LAT: &lt;12ms
            </span>
          </div>
          <span className="material-symbols-outlined text-[14px]" style={{ color: "rgba(144,143,161,0.40)" }}>terminal</span>
        </div>
      </footer>
    </motion.aside>
  );
}

// ── Log item renderer ──────────────────────────────────────────────────────────

function LogItem({
  entry,
  onExecute,
  hasResult,
}: {
  entry: LogEntry;
  onExecute: () => void;
  hasResult: boolean;
}) {
  if (entry.type === "system") {
    return (
      <div className="flex gap-3 font-mono text-[11px]">
        <span className="shrink-0" style={{ color: "#5d5f5f" }}>[{entry.timestamp}]</span>
        <span style={{ color: "#908fa1" }}>{entry.text}</span>
      </div>
    );
  }

  if (entry.type === "user") {
    return (
      <div
        className="pl-3"
        style={{ borderLeft: "2px solid rgba(198,198,199,0.20)" }}
      >
        <div className="flex gap-2">
          <span className="font-mono font-bold" style={{ color: "#bec2ff" }}>›</span>
          <p className="font-mono text-sm tracking-tight" style={{ color: "#bec2ff" }}>
            {entry.text}
          </p>
        </div>
      </div>
    );
  }

  if (entry.type === "parsing") {
    return (
      <div className="flex items-center gap-3">
        <span
          className="inline-block w-1 h-4 animate-pulse"
          style={{ backgroundColor: "#bec2ff" }}
        />
        <span
          className="font-mono text-xs font-bold tracking-[0.2em]"
          style={{ color: "#bec2ff" }}
        >
          PARSING INTENT...
        </span>
      </div>
    );
  }

  if (entry.type === "error") {
    return (
      <div className="font-mono text-[11px]" style={{ color: "#ffb4ab" }}>
        ✕ {entry.text}
      </div>
    );
  }

  if (entry.type === "result") {
    const { parsed } = entry;
    const isMultiChain = parsed.srcChain && parsed.dstChain && parsed.srcChain !== parsed.dstChain;
    const action = isMultiChain ? "BRIDGE" : "SWAP";

    return (
      <div className="space-y-3">
        {/* Analysis table */}
        <div style={{ backgroundColor: "#131313", border: "1px solid rgba(69,69,85,0.30)" }}>
          <div
            className="px-3 py-2"
            style={{ borderBottom: "1px solid rgba(69,69,85,0.20)" }}
          >
            <span
              className="font-mono text-[10px] font-bold uppercase tracking-widest"
              style={{ color: "#bec2ff" }}
            >
              ANALYSIS RESULTS
            </span>
          </div>
          <div className="p-3 space-y-2">
            <div className="grid grid-cols-2 gap-y-2 font-mono text-[11px]">
              <span className="uppercase" style={{ color: "#5d5f5f" }}>ACTION</span>
              <span className="text-right" style={{ color: "#e5e2e1" }}>{action}</span>

              <span className="uppercase" style={{ color: "#5d5f5f" }}>AMOUNT</span>
              <span className="text-right" style={{ color: "#e5e2e1" }}>{parsed.amount} {parsed.srcToken}</span>

              {parsed.srcChain && (
                <>
                  <span className="uppercase" style={{ color: "#5d5f5f" }}>FROM</span>
                  <span className="text-right" style={{ color: "#e5e2e1" }}>{chainLabel(parsed.srcChain)}</span>
                </>
              )}

              <span className="uppercase" style={{ color: "#5d5f5f" }}>TO</span>
              <span className="text-right" style={{ color: "#e5e2e1" }}>
                {parsed.dstToken || "—"}{parsed.dstChain ? ` · ${chainLabel(parsed.dstChain)}` : ""}
              </span>
            </div>
          </div>
        </div>

        {/* Execute button */}
        {hasResult && (
          <AnimatePresence>
            <motion.div
              initial={{ opacity: 0, y: 4 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.2 }}
            >
              <button
                onClick={onExecute}
                className="w-full h-11 font-mono text-xs font-bold uppercase tracking-widest flex items-center justify-center gap-2 group transition-opacity hover:opacity-80 active:scale-[0.98] transition-transform"
                style={{ backgroundColor: "#c6c6c7", color: "#1c1b1b" }}
              >
                <span
                  className="material-symbols-outlined text-sm"
                  style={{ fontVariationSettings: "'FILL' 1" }}
                >
                  bolt
                </span>
                EXECUTE INTENT
                <span className="material-symbols-outlined text-sm transition-transform group-hover:translate-x-0.5">
                  arrow_forward
                </span>
              </button>
              <p
                className="mt-2 text-[9px] font-mono text-center italic"
                style={{ color: "#5d5f5f" }}
              >
                Pre-fills swap form. Wallet signature required for execution.
              </p>
            </motion.div>
          </AnimatePresence>
        )}
      </div>
    );
  }

  return null;
}
