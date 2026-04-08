import { useState, useCallback } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { parseIntent, type ParsedIntent } from "../lib/parseIntent";

interface IntentInputProps {
  onIntent: (parsed: ParsedIntent) => void;
}

const EXAMPLES = [
  "Bridge 10 USDC from Ethereum to Base",
  "Send 0.5 ETH from Arbitrum to Optimism",
  "Swap 100 USDC from Sepolia to Base Sepolia",
];

export function IntentInput({ onIntent }: IntentInputProps) {
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState("");
  const [error, setError] = useState("");

  const submit = useCallback(() => {
    const trimmed = value.trim();
    if (!trimmed) return;

    const parsed = parseIntent(trimmed);
    if (!parsed) {
      setError(
        `Couldn't parse that. Try: "${EXAMPLES[0]}"`
      );
      return;
    }
    setError("");
    setValue("");
    setOpen(false);
    onIntent(parsed);
  }, [value, onIntent]);

  const handleKey = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (e.key === "Enter") submit();
      if (e.key === "Escape") { setOpen(false); setError(""); }
    },
    [submit],
  );

  return (
    <div>
      {/* Toggle button */}
      <button
        onClick={() => { setOpen(o => !o); setError(""); }}
        className="flex items-center gap-1.5 text-[11px] font-mono uppercase tracking-widest transition-colors"
        style={{ color: open ? "#bec2ff" : "#908fa1" }}
        onMouseEnter={e => { if (!open) e.currentTarget.style.color = "#e5e2e1"; }}
        onMouseLeave={e => { if (!open) e.currentTarget.style.color = "#908fa1"; }}
      >
        <span
          className="material-symbols-outlined"
          style={{ fontSize: "14px", fontVariationSettings: "'wght' 300" }}
        >
          {open ? "expand_less" : "auto_awesome"}
        </span>
        {open ? "Close" : "Use natural language"}
      </button>

      {/* Expandable input */}
      <AnimatePresence>
        {open && (
          <motion.div
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: "auto" }}
            exit={{ opacity: 0, height: 0 }}
            transition={{ duration: 0.18, ease: "easeOut" }}
            className="overflow-hidden mt-2"
          >
            <div
              className="space-y-2 p-3"
              style={{
                backgroundColor: "#1c1b1b",
                border: "1px solid rgba(190,194,255,0.15)",
              }}
            >
              <div className="flex gap-2">
                <input
                  type="text"
                  autoFocus
                  value={value}
                  onChange={e => { setValue(e.target.value); setError(""); }}
                  onKeyDown={handleKey}
                  placeholder={EXAMPLES[0]}
                  className="flex-1 bg-transparent text-sm outline-none placeholder:text-[#908fa1] font-mono"
                  style={{ color: "#e5e2e1" }}
                />
                <button
                  onClick={submit}
                  disabled={!value.trim()}
                  className="text-[11px] font-bold uppercase tracking-widest px-3 transition-opacity hover:opacity-80 disabled:opacity-30 disabled:cursor-not-allowed shrink-0"
                  style={{
                    backgroundColor: "rgba(190,194,255,0.12)",
                    border: "1px solid rgba(190,194,255,0.25)",
                    color: "#bec2ff",
                  }}
                >
                  Fill
                </button>
              </div>

              {error && (
                <p className="text-[11px] font-mono" style={{ color: "#ffb4ab" }}>
                  {error}
                </p>
              )}

              {/* Example chips */}
              <div className="flex flex-wrap gap-1.5 pt-0.5">
                {EXAMPLES.map(ex => (
                  <button
                    key={ex}
                    onClick={() => setValue(ex)}
                    className="text-[10px] font-mono px-2 py-0.5 transition-colors"
                    style={{
                      backgroundColor: "rgba(190,194,255,0.06)",
                      border: "1px solid rgba(190,194,255,0.12)",
                      color: "#908fa1",
                    }}
                    onMouseEnter={e => { e.currentTarget.style.color = "#bec2ff"; }}
                    onMouseLeave={e => { e.currentTarget.style.color = "#908fa1"; }}
                  >
                    {ex}
                  </button>
                ))}
              </div>
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}
