import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import { parseUnits, formatUnits } from "viem";
import { useAccount, useBalance } from "wagmi";
import { useWallet } from "@solana/wallet-adapter-react";
import { WalletMultiButton } from "@solana/wallet-adapter-react-ui";
import { fetchQuoteStream, type Route } from "../api";
import { CHAINS, TOKENS, getTokens, SOLANA_CHAIN_ID, isTestnetChain, type Chain, type Token } from "../tokens";
import { type ParsedIntent } from "../lib/parseIntent";
import { TokenIcon } from "./TokenIcon";
import { ChainIcon } from "./ChainIcon";

const ZERO_ADDRESS = "0x0000000000000000000000000000000000000000";

interface Props {
  onRoute: (route: Route) => void;
  onLoading: (v: boolean) => void;
  onError: (msg: string | null) => void;
  onDirty?: () => void;
  bestRoute: Route | null;
  intent?: ParsedIntent | null;
  intentAutoQuote?: boolean;
  intentRequestId?: number;
}

type ChainScope = "mainnet" | "testnet";
const CHAIN_SCOPE_STORAGE_KEY = "chain_scope";

/* ── Click-outside hook ──────────────────────────────────────────────────── */

function useClickOutside(
  ref: React.RefObject<HTMLElement | null>,
  handler: () => void,
) {
  useEffect(() => {
    const onPointer = (e: PointerEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) handler();
    };
    document.addEventListener("pointerdown", onPointer);
    return () => document.removeEventListener("pointerdown", onPointer);
  }, [ref, handler]);
}

/* ── Shared chevron ──────────────────────────────────────────────────────── */

function Chevron() {
  return (
    <svg className="w-3 h-3 text-on-surface-variant shrink-0" viewBox="0 0 12 12" fill="none">
      <path
        d="M3 4.5L6 7.5L9 4.5"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

/* ── Chain selector pill ─────────────────────────────────────────────────── */

function ChainDropdown({
  value,
  onChange,
  options,
}: {
  value: Chain;
  onChange: (id: number) => void;
  options: Chain[];
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useClickOutside(ref, () => setOpen(false));

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open]);

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className="flex items-center gap-1.5 rounded bg-surface-container-high
                   hover:bg-surface-container-highest px-2.5 py-1.5 transition-colors"
      >
        <ChainIcon chainId={value.id} size={14} />
        <span className="text-[13px] text-on-surface font-medium whitespace-nowrap leading-none">
          {value.label}
        </span>
        <Chevron />
      </button>

      {open && (
        <div className="absolute top-full left-0 mt-1 z-50 min-w-[168px] rounded bg-surface-container-high shadow-2xl py-1">
          {options.map((c) => (
            <button
              key={c.id}
              type="button"
              onClick={() => {
                onChange(c.id);
                setOpen(false);
              }}
              className={`flex items-center gap-2 w-full px-3 py-2 text-[13px] transition-colors ${
                c.id === value.id
                  ? "text-accent bg-surface-container-highest"
                  : "text-on-surface hover:bg-surface-container-highest"
              }`}
            >
              <ChainIcon chainId={c.id} size={14} />
              {c.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

/* ── Token selector pill ─────────────────────────────────────────────────── */

function TokenDropdown({
  chain,
  value,
  onChange,
  scope,
}: {
  chain: Chain;
  value: Token;
  onChange: (address: string) => void;
  scope: "mainnet" | "testnet";
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useClickOutside(ref, () => setOpen(false));

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open]);

  const tokens = getTokens(chain.id, scope);

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className="flex items-center gap-1.5 rounded bg-surface-container-high
                   hover:bg-surface-container-highest px-2.5 py-1.5 transition-colors"
      >
        <TokenIcon
          chainId={chain.id}
          address={value.address}
          symbol={value.symbol}
          size={16}
        />
        <span className="text-[13px] text-on-surface font-medium leading-none">
          {value.symbol}
        </span>
        <Chevron />
      </button>

      {open && (
        <div className="absolute top-full left-0 mt-1 z-50 min-w-[148px] rounded bg-surface-container-high shadow-2xl py-1">
          {tokens.map((t) => (
            <button
              key={t.address}
              type="button"
              onClick={() => {
                onChange(t.address);
                setOpen(false);
              }}
              className={`flex items-center gap-2 w-full px-3 py-2 text-[13px] transition-colors ${
                t.address === value.address
                  ? "text-accent bg-surface-container-highest"
                  : "text-on-surface hover:bg-surface-container-highest"
              }`}
            >
              <TokenIcon
                chainId={chain.id}
                address={t.address}
                symbol={t.symbol}
                size={16}
              />
              {t.symbol}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

/* ── Panel ────────────────────────────────────────────────────────────────── */

function Panel({
  label,
  amount,
  onAmountChange,
  editable,
  chain,
  token,
  onChainChange,
  onTokenChange,
  onMaxClick,
  maxDisabled,
  balanceHint,
  availableChains,
  tokenScope,
}: {
  label: string;
  amount: string;
  onAmountChange?: (v: string) => void;
  editable: boolean;
  chain: Chain;
  token: Token;
  onChainChange: (id: number) => void;
  onTokenChange: (addr: string) => void;
  onMaxClick?: () => void;
  maxDisabled?: boolean;
  balanceHint?: string;
  availableChains: Chain[];
  tokenScope: "mainnet" | "testnet";
}) {
  return (
    <div className="flex flex-col gap-5 bg-surface-container rounded px-6 py-6 min-h-[168px]">
      <div className="flex items-center justify-between">
        <span
          className="text-[11px] text-on-surface-variant font-medium uppercase"
          style={{ letterSpacing: "0.08em" }}
        >
          {label}
        </span>
        {editable && balanceHint && (
          <span className="text-[11px] text-on-surface-variant">
            Balance: {balanceHint}
          </span>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-4 min-h-[78px]">
        <div className="flex items-center gap-1.5 shrink-0">
          <ChainDropdown value={chain} onChange={onChainChange} options={availableChains} />
          <TokenDropdown chain={chain} value={token} onChange={onTokenChange} scope={tokenScope} />
        </div>

        {editable ? (
          <div className="min-w-0 flex-1 flex items-center justify-end gap-2">
            <input
              className="min-w-0 w-full text-right text-2xl sm:text-4xl font-light text-on-surface
                         bg-transparent outline-none placeholder:text-outline-variant"
              style={{ letterSpacing: "-0.02em" }}
              type="text"
              inputMode="decimal"
              placeholder="0.00"
              value={amount}
              onChange={(e) => onAmountChange?.(e.target.value)}
            />
            <button
              type="button"
              onClick={onMaxClick}
              disabled={maxDisabled}
              className="h-9 px-3 rounded bg-surface-container-high text-[11px] font-semibold uppercase tracking-wide shrink-0
                         text-on-surface-variant hover:text-on-surface hover:bg-surface-container-highest transition-colors
                         disabled:opacity-40 disabled:cursor-not-allowed"
            >
              Max
            </button>
          </div>
        ) : (
          <div
            className="min-w-0 flex-1 text-right text-2xl sm:text-4xl font-light truncate pr-[2px]"
            style={{ letterSpacing: "-0.02em" }}
          >
            {amount ? (
              <span className="text-on-surface">{amount}</span>
            ) : (
              <span className="text-outline-variant">0.00</span>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

/* ── QuoteForm ────────────────────────────────────────────────────────────── */

export function QuoteForm({
  onRoute,
  onLoading,
  onError,
  onDirty,
  bestRoute,
  intent,
  intentAutoQuote,
  intentRequestId,
}: Props) {
  const { address } = useAccount();
  const { publicKey: solanaPublicKey } = useWallet();

  const [srcChain, setSrcChain] = useState<Chain>(CHAINS[0]);
  const [dstChain, setDstChain] = useState<Chain>(CHAINS[1]);
  const [srcToken, setSrcToken] = useState<Token>(TOKENS[CHAINS[0].id][0]);
  const [dstToken, setDstToken] = useState<Token>(TOKENS[CHAINS[1].id][0]);
  const [amount, setAmount] = useState("");
  const [loading, setLoading] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [slippageBps, setSlippageBps] = useState(50);
  const [customSlippage, setCustomSlippage] = useState("");
  const [chainScope, setChainScope] = useState<ChainScope>(() => {
    if (typeof window === "undefined") return "mainnet";
    const stored = window.localStorage.getItem(CHAIN_SCOPE_STORAGE_KEY);
    if (stored === "mainnet" || stored === "testnet") return stored;
    return "mainnet";
  });
  const streamAbort = useRef<AbortController | null>(null);
  const pendingAutoQuoteRef = useRef<number | null>(null);
  const lastAutoQuoteRunRef = useRef<number | null>(null);
  const lastAppliedIntentIdRef = useRef<number | null>(null);
  const availableChains = useMemo(
    () =>
      CHAINS.filter((c) =>
        chainScope === "testnet" ? isTestnetChain(c.id) : !isTestnetChain(c.id),
      ),
    [chainScope],
  );

  const srcIsNative = srcToken.address.toLowerCase() === ZERO_ADDRESS;
  const srcIsSolana = srcChain.id === SOLANA_CHAIN_ID;
  const dstIsSolana = dstChain.id === SOLANA_CHAIN_ID;
  // EVM balance — disabled when source chain is Solana (no wagmi support for Solana).
  const { data: srcBalance, isFetching: balanceFetching } = useBalance({
    address,
    chainId: srcChain.id,
    token: srcIsNative ? undefined : (srcToken.address as `0x${string}`),
    query: {
      enabled: !!address && !srcIsSolana,
      refetchInterval: 10_000,
    },
  });

  // Apply parsed intent from IntentPanel, preferring chains within current scope.
  useEffect(() => {
    if (!intent) return;
    if (intent.network && intent.network !== chainScope) {
      // The App should auto-switch scope before sending the intent here, but keep
      // this guard to prevent cross-network prefill if timing ever races.
      const msg = `Intent targets ${intent.network}, but your UI is set to ${chainScope}. Switching scope will apply it.`;
      setFormError((prev) => (prev === msg ? prev : msg));
      onError(msg);
      return;
    }
    // Prevent "dancing": only apply a dispatched intent once (QuoteForm can re-render
    // frequently due to balances, scope toggles, and streaming routes).
    if (intentRequestId && lastAppliedIntentIdRef.current === intentRequestId) return;
    if (intentRequestId) lastAppliedIntentIdRef.current = intentRequestId;

    // Clear any previous intent error before applying a new one.
    setFormError(null);
    if (intent.amount) setAmount(intent.amount);

    const preferredChains = availableChains.length > 0 ? availableChains : CHAINS;
    const pickChain = (name: string) =>
      preferredChains.find(ch => ch.name === name) ?? CHAINS.find(ch => ch.name === name);
    const pickToken = (chainId: number, symbol: string) => {
      const tokens = getTokens(chainId, chainScope);
      if (!tokens || tokens.length === 0) return null;
      // IMPORTANT: do not silently fall back to the first token. If the intent asks
      // for a token we don't support on that chain (e.g. USDT on Base Sepolia),
      // falling back produces an incorrect prefill and a confusing execution.
      return tokens.find(tk => tk.symbol === symbol) ?? null;
    };

    if (intent.srcChain) {
      const c = pickChain(intent.srcChain);
      if (c) {
        setSrcChain(c);
        const t = pickToken(c.id, intent.srcToken);
        if (t) {
          setSrcToken(t);
        } else if (intent.srcToken) {
          const msg = `Intent requested ${intent.srcToken} on ${c.label}, but that token isn't available in this app's registry.`;
          setFormError(prev => (prev === msg ? prev : msg));
          onError(msg);
        }
      }
    }
    if (intent.dstChain) {
      const c = pickChain(intent.dstChain);
      if (c) {
        setDstChain(c);
        const t = pickToken(c.id, intent.dstToken);
        if (t) {
          setDstToken(t);
        } else if (intent.dstToken) {
          const msg = `Intent requested ${intent.dstToken} on ${c.label}, but that token isn't available in this app's registry.`;
          setFormError(prev => (prev === msg ? prev : msg));
          onError(msg);
        }
      }
    }
    if (intentAutoQuote && intentRequestId) {
      pendingAutoQuoteRef.current = intentRequestId;
    }
  }, [intent, intentAutoQuote, intentRequestId, availableChains, onError]);

  useEffect(() => {
    onDirty?.();
  }, [srcChain.id, dstChain.id, srcToken.address, dstToken.address, amount, onDirty]);

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

  useEffect(() => {
    if (availableChains.length === 0) return;
    if (!availableChains.some((c) => c.id === srcChain.id)) {
      const fallback = availableChains[0];
      setSrcChain(fallback);
      const nextTokens = getTokens(fallback.id, chainScope);
      if (nextTokens.length) {
        const preferred = nextTokens.find(t => t.symbol === srcToken.symbol) ?? nextTokens[0];
        setSrcToken(preferred);
      }
    }
    if (!availableChains.some((c) => c.id === dstChain.id)) {
      const fallback = availableChains[Math.min(1, availableChains.length - 1)];
      setDstChain(fallback);
      const nextTokens = getTokens(fallback.id, chainScope);
      if (nextTokens.length) {
        const preferred = nextTokens.find(t => t.symbol === dstToken.symbol) ?? nextTokens[0];
        setDstToken(preferred);
      }
    }
  }, [availableChains, srcChain.id, dstChain.id, srcToken.symbol, dstToken.symbol]);

  const setSrc = (chainId: number) => {
    const nextChain = CHAINS.find((c) => c.id === chainId);
    if (!nextChain) return;
    setSrcChain(nextChain);
    const tokens = getTokens(chainId, chainScope);
    if (tokens.length) setSrcToken(tokens[0]);
  };

  const setDst = (chainId: number) => {
    const nextChain = CHAINS.find((c) => c.id === chainId);
    if (!nextChain) return;
    setDstChain(nextChain);
    const tokens = getTokens(chainId, chainScope);
    if (tokens.length) setDstToken(tokens[0]);
  };

  const flip = () => {
    const [c1, t1, c2, t2] = [srcChain, srcToken, dstChain, dstToken];
    setSrcChain(c2);
    setSrcToken(t2);
    setDstChain(c1);
    setDstToken(t1);
  };

  // Resolve the actual output token decimals from the route's last hop.
  // Falls back to dstToken.decimals when the route output matches the selected token.
  const bestRouteOutputDecimals = (() => {
    if (!bestRoute) return dstToken.decimals;
    const last = bestRoute.hops[bestRoute.hops.length - 1];
    if (!last) return dstToken.decimals;
    const chainTokens = getTokens(
      CHAINS.find(c => c.name === last.to_chain)?.id ?? 0
    );
    const match = chainTokens.find(
      t => t.symbol.toUpperCase() === last.to_asset?.toUpperCase()
    );
    return match?.decimals ?? dstToken.decimals;
  })();

  const bestOutput = (() => {
    if (!bestRoute) return "";
    const last = bestRoute.hops[bestRoute.hops.length - 1];
    if (!last) return "";
    try {
      const n = Number(
        formatUnits(BigInt(bestRoute.estimated_output_amount), bestRouteOutputDecimals),
      );
      if (n === 0) return "";
      if (n >= 1000) return n.toLocaleString(undefined, { maximumFractionDigits: 2 });
      if (n >= 1)    return n.toFixed(4).replace(/\.?0+$/, "");
      return n.toPrecision(4);
    } catch {
      return "";
    }
  })();

  const exchangeRate = (() => {
    if (!bestRoute || !amount || Number(amount) <= 0) return null;
    try {
      const inputNum = Number(amount);
      const outputNum = Number(
        formatUnits(BigInt(bestRoute.estimated_output_amount), bestRouteOutputDecimals),
      );
      if (outputNum === 0 || inputNum === 0) return null;
      const rate = outputNum / inputNum;
      return {
        from: srcToken.symbol,
        to: dstToken.symbol,
        value: rate >= 1 ? rate.toFixed(4) : rate.toPrecision(4),
      };
    } catch {
      return null;
    }
  })();

  const submit = useCallback(async () => {
    // Determine which wallet address to use per chain type.
    const srcAddr = srcIsSolana ? solanaPublicKey?.toString() : address;
    const dstAddr = dstIsSolana ? solanaPublicKey?.toString() : address;

    if (!srcAddr) {
      const msg = srcIsSolana
        ? "Connect a Solana wallet (Phantom/Solflare) to get routes."
        : "Connect wallet to request executable routes.";
      setFormError(msg);
      return onError(msg);
    }
    if (dstIsSolana && !dstAddr) {
      const msg = "Connect a Solana wallet to set your destination address.";
      setFormError(msg);
      return onError(msg);
    }
    if (
      srcChain.id === dstChain.id &&
      srcToken.address.toLowerCase() === dstToken.address.toLowerCase()
    ) {
      const msg = "Select a different destination token or chain.";
      setFormError(msg);
      return onError(msg);
    }
    const trimmed = amount.trim();
    if (!trimmed || isNaN(Number(trimmed)) || Number(trimmed) <= 0) {
      const msg = "Enter a valid amount.";
      setFormError(msg);
      return onError(msg);
    }
    let amountBaseUnits: string;
    try {
      amountBaseUnits = parseUnits(trimmed, srcToken.decimals).toString();
      if (amountBaseUnits === "0") {
        const msg = "Amount is too small for token decimals.";
        setFormError(msg);
        return onError(msg);
      }
    } catch {
      const msg = "Too many decimal places.";
      setFormError(msg);
      return onError(msg);
    }

    setFormError(null);
    onError(null);
    // Cancel any in-flight stream before starting a new one.
    streamAbort.current?.abort();
    const ctrl = new AbortController();
    streamAbort.current = ctrl;

    setLoading(true);
    onLoading(true);
    let count = 0;
    try {
      const metadata: Record<string, unknown> = {};
      // Enable swap→bridge composition by telling the backend the *source-chain* address
      // for the destination token symbol (e.g. dst=Base USDC, src-side USDC on Ethereum).
      if (!srcIsSolana && !dstIsSolana && srcChain.id !== dstChain.id && srcToken.symbol !== dstToken.symbol) {
        const dstOnSource = getTokens(srcChain.id, chainScope).find((t) => t.symbol === dstToken.symbol);
        if (dstOnSource?.address) {
          metadata.source_swap_token_out_address = dstOnSource.address;
          metadata.source_swap_token_out_decimals = dstOnSource.decimals;
        }
      }
      const quoteReq = {
        source: {
          chain: srcChain.name,
          chain_id: srcChain.id,
          asset: srcToken.symbol,
          token_address: srcToken.address,
          token_decimals: srcToken.decimals,
          address: srcAddr,
        },
        destination: {
          chain: dstChain.name,
          chain_id: dstChain.id,
          asset: dstToken.symbol,
          token_address: dstToken.address,
          token_decimals: dstToken.decimals,
          address: dstAddr ?? srcAddr,
        },
        amount_base_units: amountBaseUnits,
        preferences: { max_slippage_bps: slippageBps },
        ...(Object.keys(metadata).length > 0 ? { metadata } : {}),
      };
      for await (const route of fetchQuoteStream(quoteReq, ctrl.signal)) {
        if (ctrl.signal.aborted) break;
        count++;
        onRoute(route);
      }
      if (count === 0) onError("No routes found for this pair.");
    } catch (e) {
      if (!(e instanceof DOMException && e.name === "AbortError")) {
        onError(e instanceof Error ? e.message : "Request failed.");
      }
    } finally {
      setLoading(false);
      onLoading(false);
    }
  }, [
    amount,
    srcChain,
    srcToken,
    dstChain,
    dstToken,
    srcIsSolana,
    dstIsSolana,
    address,
    solanaPublicKey,
    onRoute,
    onLoading,
    onError,
    slippageBps,
  ]);

  useEffect(() => {
    const reqId = pendingAutoQuoteRef.current;
    if (!reqId) return;
    if (lastAutoQuoteRunRef.current === reqId) return;
    if (!intentAutoQuote) return;
    if (loading) return;
    if (!intent || !intent.amount || !intent.srcToken || !intent.dstToken || !intent.dstChain) return;
    const intentApplied =
      amount.trim() === intent.amount &&
      srcToken.symbol === intent.srcToken &&
      dstToken.symbol === intent.dstToken &&
      (!intent.srcChain || srcChain.name === intent.srcChain) &&
      dstChain.name === intent.dstChain;
    if (!intentApplied) return;
    lastAutoQuoteRunRef.current = reqId;
    pendingAutoQuoteRef.current = null;
    void submit();
  }, [intentAutoQuote, intent, loading, submit, amount, srcToken.symbol, dstToken.symbol, srcChain.name, dstChain.name]);

  const walletReady = srcIsSolana ? !!solanaPublicKey : !!address;
  const canSubmit = walletReady && !loading && Number(amount || "0") > 0;

  const trimAmount = (v: string) => {
    if (!v.includes(".")) return v;
    // Cap at 8 decimal places then strip trailing zeros.
    const [int, dec] = v.split(".");
    const capped = (int ?? "") + "." + (dec ?? "").slice(0, 8);
    return capped.replace(/(\.\d*?[1-9])0+$/u, "$1").replace(/\.0+$/u, "").replace(/\.$/u, "");
  };

  const handleSetMax = useCallback(() => {
    if (!srcBalance) return;
    const maxRaw = formatUnits(srcBalance.value, srcBalance.decimals);
    setAmount(trimAmount(maxRaw));
  }, [srcBalance]);

  const balanceHint = (() => {
    if (!address) return "";
    if (!srcBalance) return balanceFetching ? "Loading..." : "—";
    return `${trimAmount(formatUnits(srcBalance.value, srcBalance.decimals))} ${srcToken.symbol}`;
  })();

  return (
    <div className="flex flex-col">
      <div className="flex items-center justify-between mb-3 px-6 pt-5">
        <h2 className="text-xl font-semibold tracking-tight text-on-surface">Swap &amp; Bridge</h2>
        <button
          type="button"
          onClick={() => window.location.reload()}
          className="text-xs text-on-surface-variant hover:text-on-surface transition-colors"
        >
          refresh
        </button>
      </div>
      <div className="mb-3 px-6 flex items-center justify-between gap-3">
        <span className="text-[11px] text-on-surface-variant">
          {address
            ? `EVM: ${address.slice(0, 6)}…${address.slice(-4)}`
            : "Connect EVM wallet for EVM chains"}
        </span>
        {(srcIsSolana || dstIsSolana) && (
          <WalletMultiButton
            style={{
              height: 28,
              fontSize: 11,
              padding: "0 10px",
              background: solanaPublicKey ? "#1e1e1e" : "#9945FF",
              borderRadius: 6,
            }}
          />
        )}
      </div>
      {/* Source panel */}
      <Panel
        label="Source Chain & Token"
        amount={amount}
        onAmountChange={setAmount}
        editable
        chain={srcChain}
        token={srcToken}
        onChainChange={setSrc}
        onTokenChange={(a) =>
          setSrcToken(getTokens(srcChain.id, chainScope).find((t) => t.address === a)!)
        }
        onMaxClick={handleSetMax}
        maxDisabled={!address || !srcBalance || srcBalance.value === 0n}
        balanceHint={balanceHint}
        availableChains={availableChains}
        tokenScope={chainScope}
      />

      {/* Flip button — overlaps both panels */}
      <div className="flex justify-center -my-[14px] relative z-10">
        <button
          type="button"
          onClick={flip}
          className="w-8 h-8 rounded bg-surface-container-high hover:bg-surface-container-highest
                     flex items-center justify-center text-on-surface-variant hover:text-on-surface
                     transition-colors"
        >
          <svg
            className="w-4 h-4"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <path d="M7 10l5-5 5 5" />
            <path d="M7 14l5 5 5-5" />
          </svg>
        </button>
      </div>

      {/* Destination panel */}
      <Panel
        label="Destination Chain & Token"
        amount={bestOutput}
        editable={false}
        chain={dstChain}
        token={dstToken}
        onChainChange={setDst}
        onTokenChange={(a) =>
          setDstToken(getTokens(dstChain.id, chainScope).find((t) => t.address === a)!)
        }
        availableChains={availableChains}
        tokenScope={chainScope}
      />

      {/* Slippage + info row */}
      <div className="px-6 pt-4 pb-2 space-y-2">
        <div className="flex items-center gap-2 flex-wrap">
          <span className="text-xs text-on-surface-variant shrink-0">Slippage</span>
          {([10, 50, 100, 200] as const).map((bps) => (
            <button
              key={bps}
              type="button"
              onClick={() => { setSlippageBps(bps); setCustomSlippage(""); }}
              className={`px-2 py-0.5 text-[11px] font-mono rounded transition-colors ${
                slippageBps === bps && !customSlippage
                  ? "bg-accent text-[#2f3131] font-semibold"
                  : "bg-surface-container-high text-on-surface-variant hover:text-on-surface"
              }`}
            >
              {bps / 100}%
            </button>
          ))}
          <input
            type="text"
            inputMode="decimal"
            placeholder="custom"
            value={customSlippage}
            onChange={(e) => {
              const v = e.target.value;
              setCustomSlippage(v);
              const n = parseFloat(v);
              if (!isNaN(n) && n > 0 && n <= 50) setSlippageBps(Math.round(n * 100));
            }}
            className="w-16 px-2 py-0.5 text-[11px] font-mono rounded bg-surface-container-high text-on-surface outline-none border border-transparent focus:border-accent/40"
          />
          <span className="text-[11px] text-on-surface-variant ml-auto">
            {(slippageBps / 100).toFixed(2)}%
          </span>
        </div>
        <div className="flex items-center justify-between text-xs text-on-surface-variant">
          <span />
          {exchangeRate ? (
            <span>
              1 {exchangeRate.from} ={" "}
              <span className="text-on-surface font-medium font-mono">
                {exchangeRate.value}
              </span>{" "}
              {exchangeRate.to}
            </span>
          ) : (
            <span className="text-outline-variant">—</span>
          )}
        </div>
      </div>

      {/* Inline error */}
      {formError && (
        <div className="rounded bg-err-container mx-6 px-3 py-2 mt-1 text-xs text-err">
          {formError}
        </div>
      )}

      {/* CTA */}
      <button
        type="button"
        onClick={submit}
        disabled={!canSubmit}
        className="mt-4 mx-6 mb-5 h-14 w-[calc(100%-3rem)] rounded font-semibold text-[16px] transition-colors
                   bg-accent text-[#2f3131] hover:brightness-110 active:brightness-95
                   disabled:opacity-35 disabled:cursor-not-allowed"
        style={{ letterSpacing: "-0.01em" }}
      >
        {loading ? (
          <span className="flex items-center justify-center gap-2">
            <span className="animate-spin inline-block">◌</span>
            Searching…
          </span>
        ) : walletReady ? (
          "Get Best Route"
        ) : srcIsSolana ? (
          "Connect Solana Wallet"
        ) : (
          "Connect Wallet"
        )}
      </button>
    </div>
  );
}
