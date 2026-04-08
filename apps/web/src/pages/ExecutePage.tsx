import { useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { motion, AnimatePresence } from "framer-motion";
import { ExecutePanel } from "../components/ExecutePanel";
import { MiniRouteCard } from "../components/MiniRouteCard";
import { type Route } from "../api";

interface LocationState {
  routes: Route[];
  selectedRoute: Route;
  quotedAt?: number;
}

export function ExecutePage() {
  const location = useLocation();
  const navigate = useNavigate();
  const state = location.state as LocationState | null;

  const routes = state?.routes ?? [];
  const quotedAt = state?.quotedAt;
  const [selected, setSelected] = useState<Route>(
    state?.selectedRoute ?? routes[0],
  );

  // Guard: if someone navigates here directly with no state
  if (!state || !selected) {
    return (
      <div className="max-w-7xl mx-auto w-full py-20 flex flex-col items-center gap-4">
        <p className="text-[#908fa1] text-sm font-mono">No route selected.</p>
        <button
          onClick={() => navigate("/app")}
          className="text-xs font-mono uppercase tracking-widest text-[#bec2ff] hover:text-[#e5e2e1] transition-colors"
        >
          ← Back to swap
        </button>
      </div>
    );
  }

  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.2, ease: "easeOut" }}
      className="max-w-7xl mx-auto w-full"
    >
      {/* ── Back link ── */}
      <button
        onClick={() => navigate("/app")}
        className="flex items-center gap-1.5 text-[11px] font-mono uppercase tracking-widest mb-6 transition-colors"
        style={{ color: "#908fa1" }}
        onMouseEnter={e => (e.currentTarget.style.color = "#e5e2e1")}
        onMouseLeave={e => (e.currentTarget.style.color = "#908fa1")}
      >
        <span className="material-symbols-outlined" style={{ fontSize: "14px" }}>arrow_back</span>
        Back to routes
      </button>

      {/* ── Main grid: sidebar + execute panel ── */}
      <div className="grid grid-cols-1 xl:grid-cols-[280px_minmax(0,680px)] gap-6 items-start">

        {/* ── Left: route switcher sidebar (scrollable) ── */}
        <div className="flex flex-col gap-2 xl:sticky xl:top-24 xl:max-h-[calc(100vh-120px)] xl:overflow-y-auto">
          <p className="text-[10px] font-mono uppercase tracking-[0.18em] mb-2 shrink-0" style={{ color: "#908fa1" }}>
            {routes.length} Route{routes.length !== 1 ? "s" : ""}
          </p>
          {routes.map((r, i) => (
            <motion.div
              key={r.route_id}
              initial={{ opacity: 0, x: -8 }}
              animate={{ opacity: 1, x: 0 }}
              transition={{ delay: i * 0.04, duration: 0.2 }}
            >
              <MiniRouteCard
                route={r}
                selected={selected.route_id === r.route_id}
                onSelect={() => setSelected(r)}
              />
            </motion.div>
          ))}
        </div>

        {/* ── Right: execute panel ── */}
        <AnimatePresence mode="wait">
          <motion.div
            key={selected.route_id}
            initial={{ opacity: 0, y: -6 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.18, ease: "easeOut" }}
          >
            <ExecutePanel
              route={selected}
              quotedAt={quotedAt}
              onTryNextRoute={(() => {
                const idx = routes.findIndex(r => r.route_id === selected.route_id);
                const next = routes[idx + 1];
                return next ? () => setSelected(next) : undefined;
              })()}
            />
          </motion.div>
        </AnimatePresence>
      </div>
    </motion.div>
  );
}
