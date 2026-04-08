import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { Routes, Route, NavLink, Link, useNavigate } from "react-router-dom";
import { ConnectButton } from "@rainbow-me/rainbowkit";
import { useAccount, useSwitchChain } from "wagmi";
import { motion, AnimatePresence } from "framer-motion";
import { QuoteForm } from "./components/QuoteForm";
import { RouteCard } from "./components/RouteCard";
import { QuoteSummaryCard } from "./components/QuoteSummaryCard";
import { OperationsDashboard } from "./components/OperationsDashboard";
import { PendingClaimsBanner } from "./components/PendingClaimsBanner";
import { IntentPanel } from "./components/IntentPanel";
import { ExecutePage } from "./pages/ExecutePage";
import { fetchOperations, type Route as BridgeRoute, type OperationDetail } from "./api";
import { VISIBLE_PROVIDERS } from "./config/providers";
import { type ParsedIntent } from "./lib/parseIntent";

type ChainScope = "mainnet" | "testnet";
const CHAIN_SCOPE_STORAGE_KEY = "chain_scope";

function readChainScope(): ChainScope {
  if (typeof window === "undefined") return "mainnet";
  const stored = window.localStorage.getItem(CHAIN_SCOPE_STORAGE_KEY);
  if (stored === "mainnet" || stored === "testnet") return stored;
  return "mainnet";
}

function writeChainScope(scope: ChainScope) {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(CHAIN_SCOPE_STORAGE_KEY, scope);
  window.dispatchEvent(new CustomEvent("chain-scope-change", { detail: scope }));
}

function SwapPage() {
  const navigate = useNavigate();
  const [routes, setRoutes] = useState<BridgeRoute[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sortMode, setSortMode] = useState<"best" | "cheapest" | "fastest">("best");
  const [quotedAt, setQuotedAt] = useState<number | null>(null);
  const [pendingIntent, setPendingIntent] = useState<ParsedIntent | null>(null);

  // Listen for intents dispatched by IntentPanel.
  useEffect(() => {
    const onIntent = (e: Event) => {
      const parsed = (e as CustomEvent<ParsedIntent>).detail;
      if (parsed) setPendingIntent(parsed);
    };
    window.addEventListener("intent-execute", onIntent);
    return () => window.removeEventListener("intent-execute", onIntent);
  }, []);

  const handleRoute = useCallback((route: BridgeRoute) => {
    setRoutes(prev => {
      const without = prev.filter(r => r.route_id !== route.route_id);
      return [...without, route].sort((a, b) => b.score - a.score);
    });
    // Record timestamp of the first route arriving.
    setQuotedAt(prev => prev === null ? Date.now() : prev);
  }, []);

  const handleQuoteDirty = useCallback(() => {
    setRoutes((prev) => (prev.length > 0 ? [] : prev));
    setError(null);
    setSortMode("best");
    setQuotedAt(null);
  }, []);

  const handleEdit = useCallback(() => {
    setRoutes([]);
    setError(null);
    setSortMode("best");
    setQuotedAt(null);
  }, []);

  const bestRoute = (() => {
    const supported = routes.filter((r) => r.execution?.supported);
    return supported[0] ?? routes[0] ?? null;
  })();

  const sortedRoutes = (() => {
    let items = [...routes];
    // Only show routes where every hop uses a CONFIRMED provider.
    // See apps/web/src/config/providers.ts — one place to promote a provider.
    items = items.filter(
      (r) => r.hops.every((h) => VISIBLE_PROVIDERS.has(h.bridge_id)),
    );
    if (sortMode === "cheapest") {
      items.sort((a, b) => Number(a.total_fee || "0") - Number(b.total_fee || "0"));
    } else if (sortMode === "fastest") {
      items.sort(
        (a, b) =>
          (a.estimated_time_seconds || Number.MAX_SAFE_INTEGER) -
          (b.estimated_time_seconds || Number.MAX_SAFE_INTEGER),
      );
    }
    return items;
  })();

  // Show route cards as soon as first route arrives (even while still streaming).
  const hasRoutes = routes.length > 0;

  return (
    <motion.div
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      transition={{ duration: 0.3 }}
      className="w-full max-w-7xl mx-auto"
    >
      {/* ── State 1: Entry — centered form, no distractions ── */}
      <div className={hasRoutes ? "hidden" : "max-w-[520px] mx-auto w-full space-y-3"}>
        <div className="rounded bg-[#1c1b1b] border border-[#2a2a2a]">
          <QuoteForm
            onRoute={handleRoute}
            onLoading={setLoading}
            onError={setError}
            onDirty={handleQuoteDirty}
            bestRoute={bestRoute}
            intent={pendingIntent}
          />
        </div>
        {loading && (
          <div className="overflow-hidden rounded bg-[#1c1b1b] border border-[#2a2a2a] h-16 relative">
            <motion.div
              className="absolute inset-0"
              animate={{ backgroundPosition: ["200% 0", "-200% 0"] }}
              transition={{ duration: 1.4, repeat: Infinity, ease: "linear" }}
              style={{
                background: "linear-gradient(90deg, transparent 0%, #2a2a2a 50%, transparent 100%)",
                backgroundSize: "200% 100%",
              }}
            />
          </div>
        )}
        {error && (
          <p className="text-sm text-[#ffb4ab] bg-[#93000a]/10 rounded px-4 py-3 leading-relaxed">
            {error}
          </p>
        )}
      </div>

      {/* ── State 2: Browse — summary strip + route cards ── */}
      <AnimatePresence>
        {hasRoutes && bestRoute && (
          <motion.div
            initial={{ opacity: 0, y: 12 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.25, ease: "easeOut" }}
            className="space-y-6"
          >
            {/* Top strip: summary card (left) + sort controls (right) */}
            <div className="flex flex-col xl:flex-row xl:items-start gap-4">
              <div className="xl:max-w-[400px] w-full">
                <QuoteSummaryCard route={bestRoute} onEdit={handleEdit} />
              </div>
              <div className="xl:ml-auto flex items-center gap-1 bg-[#1c1b1b] border border-[#2a2a2a] p-1 self-start mt-1">
                {(["best", "cheapest", "fastest"] as const).map((id) => (
                  <button
                    key={id}
                    type="button"
                    onClick={() => setSortMode(id)}
                    className={`px-3 py-1.5 text-[11px] font-mono uppercase tracking-wider transition-colors ${
                      sortMode === id
                        ? "bg-[#c6c6c7] text-[#2f3131] font-semibold"
                        : "text-[#c6c5d8] hover:text-[#e5e2e1]"
                    }`}
                  >
                    {id === "best" ? "Best" : id.charAt(0).toUpperCase() + id.slice(1)}
                  </button>
                ))}
              </div>
            </div>

            {/* Route cards — hero content */}
            <div className="space-y-3">
              <div>
                <p className="text-base text-[#e5e2e1] font-semibold">Available Routes</p>
                <p className="text-[11px] text-[#908fa1]">
                  {loading
                    ? `${routes.length} route${routes.length !== 1 ? "s" : ""} found · searching for more…`
                    : `${routes.length} route${routes.length !== 1 ? "s" : ""} aggregated from integrated providers · select one to execute`}
                </p>
              </div>

              <div className="grid grid-cols-1 xl:grid-cols-2 gap-3">
                {sortedRoutes.map((r, i) => (
                  <motion.div
                    key={r.route_id}
                    initial={{ opacity: 0, y: 16 }}
                    animate={{
                      opacity: 1,
                      y: 0,
                      boxShadow: ["0 0 0 1px #bec2ff60", "0 0 0 0px transparent", "0 0 0 0px transparent"],
                    }}
                    transition={{
                      opacity: { delay: i * 0.05, type: "spring", damping: 25, stiffness: 200 },
                      y: { delay: i * 0.05, type: "spring", damping: 25, stiffness: 200 },
                      boxShadow: { duration: 0.6, delay: i * 0.05 + 0.15, times: [0, 0.4, 1] },
                    }}
                  >
                    <RouteCard
                      route={r}
                      isBest={bestRoute?.route_id === r.route_id || (i === 0 && !bestRoute)}
                      selected={false}
                      onSelect={() =>
                        navigate("/execute", { state: { routes: sortedRoutes, selectedRoute: r, quotedAt } })
                      }
                    />
                  </motion.div>
                ))}
              </div>
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </motion.div>
  );
}

function LandingPage() {
  return (
    <div className="min-h-screen bg-[#131313] text-[#e5e2e1]">
      <nav className="sticky top-0 z-50 h-16 backdrop-blur bg-[#131313]/70 border-b border-[#2a2a2a] flex items-center">
        <div className="max-w-7xl w-full mx-auto px-6 flex items-center justify-between">
          <Link to="/app" className="flex items-center gap-2">
            <img src="/tagg-logo.svg" alt="TERMINAL.AGG" className="w-7 h-7" />
            <span className="text-lg font-black tracking-tighter text-[#C6C6C7] uppercase">
              T / AGG
            </span>
          </Link>
          <Link
            to="/app"
            className="bg-[#c6c6c7] text-[#2f3131] px-4 py-2 text-xs font-bold uppercase tracking-wider"
          >
            Launch App
          </Link>
        </div>
      </nav>

      <section className="max-w-7xl mx-auto px-6 min-h-[calc(100vh-64px)] flex items-center">
        <div className="w-full grid grid-cols-1 lg:grid-cols-12 gap-8 items-center py-10">
          <div className="lg:col-span-7">
            <p className="inline-flex items-center gap-2 px-3 py-1 bg-[#2a2a2a] text-[#bec2ff] text-[10px] font-mono uppercase tracking-widest">
              live routing engine
            </p>
            <h1 className="mt-8 text-5xl md:text-7xl font-black leading-[0.95] tracking-tight">
              Swap + Bridge
              <br />
              <span className="text-[#c6c6c7]/60">
                across chains with best-route execution
              </span>
            </h1>
            <p className="mt-6 max-w-2xl text-[#908fa1] text-lg">
              TERMINAL.AGG aggregates bridge and DEX paths, then ranks routes by
              estimated output, fee, and time. You get execution-aware routing
              with spender and approval context before sign.
            </p>
            <div className="mt-10 flex items-center gap-3">
              <Link
                to="/app"
                className="bg-white text-black px-6 py-3 text-xs font-bold uppercase tracking-widest"
              >
                Launch Terminal
              </Link>
              <a
                href="#features"
                className="border border-[#454555] px-6 py-3 text-xs font-bold uppercase tracking-widest"
              >
                Capabilities
              </a>
            </div>
          </div>

          <div className="lg:col-span-5 bg-[#1c1b1b] border border-[#2a2a2a] p-5">
            <div className="flex items-center justify-between mb-4">
              <p className="text-[10px] font-mono uppercase tracking-[0.2em] text-[#908fa1]">
                Route Preview
              </p>
              <span className="text-[10px] font-mono text-[#bec2ff]">
                best yield
              </span>
            </div>
            <div className="space-y-3">
              <div className="bg-[#0e0e0e] p-4">
                <p className="text-[10px] font-mono text-[#908fa1] mb-2">
                  From
                </p>
                <div className="flex items-center justify-between">
                  <span className="text-xl font-mono font-bold">
                    1.5000 ETH
                  </span>
                  <span className="text-xs font-semibold">Ethereum</span>
                </div>
              </div>
              <div className="bg-[#0e0e0e] p-4">
                <p className="text-[10px] font-mono text-[#908fa1] mb-2">
                  To (Estimated)
                </p>
                <div className="flex items-center justify-between">
                  <span className="text-xl font-mono font-bold">
                    3,441.80 USDC
                  </span>
                  <span className="text-xs font-semibold">Arbitrum</span>
                </div>
              </div>
              <div className="bg-[#201f1f] p-4 border border-[#2a2a2a]">
                <div className="flex items-center justify-between text-[11px] font-mono text-[#c6c5d8]">
                  <span>Bridge / DEX</span>
                  <span>Stargate + Uniswap</span>
                </div>
                <div className="flex items-center justify-between text-[11px] font-mono text-[#c6c5d8] mt-2">
                  <span>ETA</span>
                  <span>~2m</span>
                </div>
                <div className="flex items-center justify-between text-[11px] font-mono text-[#c6c5d8] mt-2">
                  <span>Total Fee</span>
                  <span>$8.42</span>
                </div>
                <div className="flex items-center justify-between text-[11px] font-mono text-[#c6c5d8] mt-2">
                  <span>Execution</span>
                  <span>guided two-step</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section
        id="features"
        className="max-w-7xl mx-auto px-6 pb-24 grid grid-cols-1 md:grid-cols-3 gap-6"
      >
        {[
          [
            "Route Aggregation",
            "Compare multiple bridge and DEX-backed paths from a single quote request.",
          ],
          [
            "Execution Readiness",
            "See which routes are executable now versus quote-only before you sign.",
          ],
          [
            "Safety Controls",
            "Allowance checks, spender visibility, and exact/unlimited approval modes.",
          ],
        ].map(([title, body]) => (
          <div key={title} className="bg-[#1c1b1b] border border-[#2a2a2a] p-6">
            <h3 className="text-xl font-semibold">{title}</h3>
            <p className="mt-3 text-sm text-[#908fa1] leading-relaxed">
              {body}
            </p>
          </div>
        ))}
      </section>

      <section className="max-w-7xl mx-auto px-6 pb-24 grid grid-cols-1 lg:grid-cols-2 gap-10">
        <div>
          <p className="text-[10px] font-mono uppercase tracking-[0.2em] text-[#908fa1] mb-3">
            Integration SDK
          </p>
          <h3 className="text-4xl font-black tracking-tight">
            Built for the Protocol Era.
          </h3>
          <p className="text-[#908fa1] mt-4 max-w-xl">
            Embed cross-chain execution directly into your product with one
            integration path.
          </p>
        </div>
        <div className="bg-[#0e0e0e] border border-[#2a2a2a] p-5 font-mono text-xs text-[#c6c5d8]">
          <pre className="whitespace-pre-wrap">{`import { TaggSDK } from "@tagg/sdk"

const sdk = new TaggSDK({ network: "mainnet" })
await sdk.quoteAndExecute({
  from: "ETH",
  to: "USDC",
  fromChain: "ethereum",
  toChain: "arbitrum"
})`}</pre>
        </div>
      </section>

      <footer className="border-t border-[#2a2a2a] bg-[#0e0e0e]">
        <div className="max-w-7xl mx-auto px-6 py-10 flex flex-col md:flex-row items-start md:items-center justify-between gap-4">
          <div>
            <p className="font-black tracking-tighter uppercase text-[#C6C6C7]">
              TERMINAL.AGG
            </p>
            <p className="text-xs text-[#908fa1]">
              Terminal Aggregator for cross-chain execution.
            </p>
          </div>
          <p className="text-[11px] font-mono text-[#908fa1]">
            © 2026 TERMINAL.AGG
          </p>
        </div>
      </footer>
    </div>
  );
}

const navLinkClass = ({ isActive }: { isActive: boolean }) =>
  `text-xs font-medium tracking-wide transition-colors ${
    isActive ? "text-[#e5e2e1]" : "text-[#c6c5d8] hover:text-[#e5e2e1]"
  }`;

const SCOPE_DEFAULT_CHAIN: Record<ChainScope, number> = {
  mainnet: 1,        // Ethereum
  testnet: 11155111, // Sepolia
};

function NetworkToggle({ chainScope, onChange }: { chainScope: ChainScope; onChange: (scope: ChainScope) => void }) {
  const { switchChainAsync } = useSwitchChain();
  const [switching, setSwitching] = useState(false);

  const handleChange = useCallback(async (scope: ChainScope) => {
    onChange(scope);
    setSwitching(true);
    try {
      await switchChainAsync({ chainId: SCOPE_DEFAULT_CHAIN[scope] });
    } catch {
      // Wallet may not be connected — silently ignore
    } finally {
      setSwitching(false);
    }
  }, [onChange, switchChainAsync]);

  return (
    <div className="flex items-center bg-[#1c1b1b] border border-[#2a2a2a] p-0.5">
      {(["mainnet", "testnet"] as const).map((scope) => (
        <button
          key={scope}
          type="button"
          onClick={() => handleChange(scope)}
          disabled={switching}
          className={`px-2.5 py-1 text-[10px] font-mono uppercase tracking-wider transition-colors disabled:opacity-50 ${
            chainScope === scope
              ? "bg-[#c6c6c7] text-[#2f3131] font-semibold"
              : "text-[#c6c5d8] hover:text-[#e5e2e1]"
          }`}
        >
          {switching && chainScope === scope ? "switching..." : scope}
        </button>
      ))}
    </div>
  );
}

function TerminalLayout({ children }: { children: ReactNode }) {
  const { address } = useAccount();
  const [chainScope, setChainScope] = useState<ChainScope>(() => readChainScope());
  const [intentOpen, setIntentOpen] = useState(false);
  const [resumableOps, setResumableOps] = useState<OperationDetail[]>([]);
  const [resumeDismissed, setResumeDismissed] = useState(false);
  const prevAddress = useRef<string | undefined>(undefined);

  // Session-resume: when wallet connects, query for incomplete multi-step operations.
  useEffect(() => {
    if (!address || address === prevAddress.current) return;
    prevAddress.current = address;
    setResumeDismissed(false);
    fetchOperations(address, 20, chainScope)
      .then(({ operations }) => {
        const pending = operations.filter(
          (op) =>
            op.status === "submitted" &&
            ["guided_two_step", "async_claim"].includes(
              (op.route as { execution?: { intent?: string } })?.execution?.intent ?? "",
            ),
        );
        setResumableOps(pending);
      })
      .catch(() => { /* silently ignore — resume is best-effort */ });
  }, [address, chainScope]);

  useEffect(() => {
    const onScope = (e: Event) => {
      const detail = (e as CustomEvent<ChainScope>).detail;
      if (detail === "mainnet" || detail === "testnet") {
        setChainScope(detail);
      }
    };
    window.addEventListener("chain-scope-change", onScope);
    return () => window.removeEventListener("chain-scope-change", onScope);
  }, []);

  const handleChainScopeChange = useCallback((scope: ChainScope) => {
    setChainScope(scope);
    writeChainScope(scope);
  }, []);

  return (
    <div className="min-h-screen bg-[#131313] text-white antialiased">
      {chainScope === "testnet" && (
        <div className="w-full bg-[#2a1f00] border-b border-[#ffb347]/30 px-4 py-1.5 text-center">
          <span className="text-[11px] font-mono uppercase tracking-widest text-[#ffb347]">
            TESTNET MODE — transactions use Sepolia / Base Sepolia / Arbitrum Sepolia / OP Sepolia · funds have no real value
          </span>
        </div>
      )}
      <PendingClaimsBanner chainScope={chainScope} />
      {resumableOps.length > 0 && !resumeDismissed && (
        <div className="w-full bg-[#1a1f2e] border-b border-[#bec2ff]/30 px-4 py-2 flex items-center justify-between gap-4">
          <span className="text-[11px] font-mono text-[#bec2ff]">
            {resumableOps.length === 1
              ? "You have 1 incomplete transfer waiting to be claimed."
              : `You have ${resumableOps.length} incomplete transfers waiting to be claimed.`}
          </span>
          <div className="flex items-center gap-3 shrink-0">
            <NavLink
              to="/operations"
              className="text-[11px] font-mono underline text-[#bec2ff] hover:text-white"
              onClick={() => setResumeDismissed(true)}
            >
              Resume →
            </NavLink>
            <button
              type="button"
              onClick={() => setResumeDismissed(true)}
              className="text-[11px] font-mono text-[#908fa1] hover:text-[#e5e2e1]"
            >
              Dismiss
            </button>
          </div>
        </div>
      )}
      <header className="sticky top-0 z-50 h-16 backdrop-blur bg-[#131313]/70 border-b border-[#2a2a2a] flex items-center">
        <div className="max-w-7xl mx-auto w-full px-6 flex items-center justify-between">
          <Link
            to="/"
            className="flex items-center gap-2"
          >
            <img
              src="/tagg-logo.svg"
              alt="TERMINAL.AGG"
              className="w-7 h-7"
            />
            <span className="text-lg font-black tracking-tighter text-[#C6C6C7] uppercase">
              T / AGG
            </span>
            {chainScope === "testnet" && (
              <span className="ml-1 px-1.5 py-0.5 text-[9px] font-mono font-bold uppercase tracking-widest bg-[#ffb347]/15 text-[#ffb347] border border-[#ffb347]/40">
                TESTNET
              </span>
            )}
          </Link>

          <div className="flex flex-wrap items-center gap-3 sm:gap-6">
            <nav className="flex items-center gap-5">
              <NavLink to="/app" end className={navLinkClass}>
                Swap
              </NavLink>
              <NavLink to="/operations" className={navLinkClass}>
                Operations
              </NavLink>
            </nav>
            {/* Intent Engine toggle */}
            <button
              onClick={() => setIntentOpen(o => !o)}
              className="flex items-center gap-1.5 px-2.5 py-1.5 transition-colors font-mono text-[10px] uppercase tracking-wider"
              style={{
                backgroundColor: intentOpen ? "rgba(190,194,255,0.12)" : "transparent",
                border: `1px solid ${intentOpen ? "rgba(190,194,255,0.35)" : "rgba(69,69,85,0.40)"}`,
                color: intentOpen ? "#bec2ff" : "#908fa1",
              }}
              onMouseEnter={e => { if (!intentOpen) e.currentTarget.style.color = "#e5e2e1"; }}
              onMouseLeave={e => { if (!intentOpen) e.currentTarget.style.color = "#908fa1"; }}
            >
              <span className="material-symbols-outlined" style={{ fontSize: "13px" }}>auto_awesome</span>
              Intent
            </button>
            <NetworkToggle chainScope={chainScope} onChange={handleChainScopeChange} />
            <ConnectButton
              showBalance={false}
              chainStatus="icon"
              accountStatus="avatar"
            />
          </div>
        </div>
      </header>

      <main
        className="w-full px-6 py-10 transition-all duration-300"
        style={{ paddingRight: intentOpen ? "calc(24px + 384px)" : undefined }}
      >
        {children}
      </main>

      <AnimatePresence>
        {intentOpen && (
          <IntentPanel onClose={() => setIntentOpen(false)} />
        )}
      </AnimatePresence>
    </div>
  );
}

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<LandingPage />} />
      <Route
        path="/app"
        element={
          <TerminalLayout>
            <SwapPage />
          </TerminalLayout>
        }
      />
      <Route
        path="/execute"
        element={
          <TerminalLayout>
            <ExecutePage />
          </TerminalLayout>
        }
      />
      <Route
        path="/operations"
        element={
          <TerminalLayout>
            <OperationsDashboard />
          </TerminalLayout>
        }
      />
    </Routes>
  );
}
