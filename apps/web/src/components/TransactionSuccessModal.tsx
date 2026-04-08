import { motion, AnimatePresence } from "framer-motion";

// Bridge-specific status page URLs
function bridgeStatusUrl(bridgeId: string, txHash: string, _srcChainId?: number): string | null {
  if (bridgeId === "across") {
    return `https://app.across.to/transactions/${txHash}`;
  }
  if (bridgeId === "cctp" || bridgeId === "circle_cctp") {
    return `https://cctp.money`;
  }
  return null;
}

function arrivalTimeLabel(estimatedTimeSec?: number): string | null {
  if (!estimatedTimeSec || estimatedTimeSec <= 0) return null;
  const arrival = new Date(Date.now() + estimatedTimeSec * 1000);
  const hh = arrival.getHours().toString().padStart(2, "0");
  const mm = arrival.getMinutes().toString().padStart(2, "0");
  return `${hh}:${mm}`;
}

interface TransactionSuccessModalProps {
  open: boolean;
  txHash: string;
  srcChain: string;
  dstChain: string;
  asset: string;
  amount: string;
  bridgeLabel: string;
  explorerUrl: string;
  // Enhanced fields
  estimatedOutput?: string;  // formatted output amount
  dstAsset?: string;         // destination token symbol
  estimatedTimeSec?: number; // seconds until arrival
  bridgeId?: string;         // for bridge-specific status link
  srcChainId?: number;
  onViewOps: () => void;
  onDone: () => void;
}

export function TransactionSuccessModal({
  open,
  txHash,
  srcChain,
  dstChain,
  asset,
  amount,
  bridgeLabel,
  explorerUrl,
  estimatedOutput,
  dstAsset,
  estimatedTimeSec,
  bridgeId,
  srcChainId,
  onViewOps,
  onDone,
}: TransactionSuccessModalProps) {
  const arrivalTime = arrivalTimeLabel(estimatedTimeSec);
  const statusUrl = bridgeId ? bridgeStatusUrl(bridgeId, txHash, srcChainId) : null;
  return (
    <AnimatePresence>
      {open && (
        <>
          {/* Backdrop */}
          <motion.div
            key="backdrop"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.2 }}
            className="fixed inset-0 z-50"
            style={{ backgroundColor: "rgba(0,0,0,0.72)" }}
            onClick={onDone}
          />

          {/* Modal */}
          <motion.div
            key="modal"
            initial={{ opacity: 0, scale: 0.95, y: 16 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.96, y: 8 }}
            transition={{ duration: 0.22, ease: "easeOut" }}
            className="fixed inset-0 z-50 flex items-center justify-center px-4 pointer-events-none"
          >
            <div
              className="pointer-events-auto w-full max-w-sm"
              style={{
                backgroundColor: "#1c1b1b",
                border: "1px solid rgba(190,194,255,0.20)",
              }}
            >
              {/* Header */}
              <div
                className="flex items-center gap-3 px-6 py-5"
                style={{ borderBottom: "1px solid rgba(190,194,255,0.10)" }}
              >
                <span
                  className="flex items-center justify-center w-8 h-8 text-sm font-bold shrink-0"
                  style={{
                    border: "1.5px solid rgba(190,194,255,0.40)",
                    color: "#bec2ff",
                  }}
                >
                  ✓
                </span>
                <div>
                  <p className="text-sm font-semibold" style={{ color: "#e5e2e1" }}>
                    Transaction Complete
                  </p>
                  <p className="text-[11px] font-mono" style={{ color: "#908fa1" }}>
                    via {bridgeLabel}
                  </p>
                </div>
              </div>

              {/* Route summary */}
              <div className="px-6 py-4 space-y-3">
                <div className="flex items-center justify-between">
                  <div className="space-y-0.5">
                    <p className="text-[11px] font-mono uppercase tracking-widest" style={{ color: "#908fa1" }}>
                      From
                    </p>
                    <p className="text-sm font-semibold" style={{ color: "#e5e2e1" }}>
                      {srcChain}
                    </p>
                  </div>
                  <span className="text-[#908fa1] text-lg">→</span>
                  <div className="space-y-0.5 text-right">
                    <p className="text-[11px] font-mono uppercase tracking-widest" style={{ color: "#908fa1" }}>
                      To
                    </p>
                    <p className="text-sm font-semibold" style={{ color: "#e5e2e1" }}>
                      {dstChain}
                    </p>
                  </div>
                </div>

                <div
                  className="flex items-center justify-between px-4 py-3"
                  style={{ backgroundColor: "rgba(190,194,255,0.06)", border: "1px solid rgba(190,194,255,0.12)" }}
                >
                  <span className="text-[11px] font-mono uppercase tracking-widest" style={{ color: "#908fa1" }}>
                    Sent
                  </span>
                  <span className="text-sm font-bold font-mono" style={{ color: "#bec2ff" }}>
                    {amount} {asset}
                  </span>
                </div>

                {estimatedOutput && dstAsset && (
                  <div
                    className="flex items-center justify-between px-4 py-3"
                    style={{ backgroundColor: "rgba(74,222,128,0.06)", border: "1px solid rgba(74,222,128,0.15)" }}
                  >
                    <span className="text-[11px] font-mono uppercase tracking-widest" style={{ color: "#908fa1" }}>
                      You receive ~
                    </span>
                    <span className="text-sm font-bold font-mono" style={{ color: "#4ade80" }}>
                      {estimatedOutput} {dstAsset}
                    </span>
                  </div>
                )}

                {arrivalTime && (
                  <div className="flex items-center justify-between px-4 py-2" style={{ color: "#908fa1" }}>
                    <span className="text-[11px] font-mono uppercase tracking-widest">Expected by</span>
                    <span className="text-[11px] font-mono font-semibold" style={{ color: "#c6c5d8" }}>
                      {arrivalTime}
                    </span>
                  </div>
                )}

                <a
                  href={explorerUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center justify-between px-4 py-2.5 transition-colors"
                  style={{
                    backgroundColor: "rgba(190,194,255,0.04)",
                    border: "1px solid rgba(190,194,255,0.12)",
                  }}
                  onMouseEnter={e => {
                    e.currentTarget.style.backgroundColor = "rgba(190,194,255,0.08)";
                  }}
                  onMouseLeave={e => {
                    e.currentTarget.style.backgroundColor = "rgba(190,194,255,0.04)";
                  }}
                >
                  <span className="text-[11px] font-mono" style={{ color: "#bec2ff" }}>
                    {txHash.slice(0, 10)}…{txHash.slice(-6)}
                  </span>
                  <span className="text-[11px]" style={{ color: "#908fa1" }}>
                    View ↗
                  </span>
                </a>
                {statusUrl && (
                  <a
                    href={statusUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="flex items-center justify-between px-4 py-2.5 transition-colors"
                    style={{
                      backgroundColor: "rgba(190,194,255,0.04)",
                      border: "1px solid rgba(190,194,255,0.12)",
                    }}
                    onMouseEnter={e => { e.currentTarget.style.backgroundColor = "rgba(190,194,255,0.08)"; }}
                    onMouseLeave={e => { e.currentTarget.style.backgroundColor = "rgba(190,194,255,0.04)"; }}
                  >
                    <span className="text-[11px] font-mono" style={{ color: "#bec2ff" }}>
                      Track on {bridgeLabel}
                    </span>
                    <span className="text-[11px]" style={{ color: "#908fa1" }}>↗</span>
                  </a>
                )}
              </div>

              {/* Actions */}
              <div
                className="flex items-center gap-2 px-6 py-4"
                style={{ borderTop: "1px solid rgba(190,194,255,0.10)" }}
              >
                <button
                  onClick={onViewOps}
                  className="flex-1 h-10 text-xs font-bold uppercase tracking-[0.12em] transition-opacity hover:opacity-80"
                  style={{
                    backgroundColor: "#bec2ff",
                    color: "#1c1b1b",
                  }}
                >
                  View in Operations
                </button>
                <button
                  onClick={onDone}
                  className="h-10 px-4 text-xs font-bold uppercase tracking-[0.12em] transition-opacity hover:opacity-80"
                  style={{
                    backgroundColor: "transparent",
                    border: "1px solid rgba(190,194,255,0.25)",
                    color: "#908fa1",
                  }}
                >
                  Done
                </button>
              </div>
            </div>
          </motion.div>
        </>
      )}
    </AnimatePresence>
  );
}
