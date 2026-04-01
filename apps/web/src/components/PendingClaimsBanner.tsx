import { useEffect, useState, useCallback } from "react";
import { formatUnits } from "viem";
import { useWriteContract, useSwitchChain, useChainId } from "wagmi";
import { loadPendingClaims, removePendingClaim, type PendingClaim } from "../lib/pendingClaims";

const RECEIVE_MESSAGE_ABI = [{
  name: "receiveMessage",
  type: "function",
  stateMutability: "nonpayable",
  inputs: [
    { name: "message", type: "bytes" },
    { name: "attestation", type: "bytes" },
  ],
  outputs: [{ name: "success", type: "bool" }],
}] as const;

function fmtAmt(raw: string, dec: number): string {
  try {
    const n = Number(formatUnits(BigInt(raw), dec));
    if (n === 0) return "0";
    return n >= 1 ? n.toFixed(4).replace(/\.?0+$/, "") : n.toPrecision(4);
  } catch {
    return raw;
  }
}

const CHAIN_NAMES: Record<number, string> = {
  1: "Ethereum", 10: "Optimism", 137: "Polygon", 8453: "Base",
  42161: "Arbitrum", 43114: "Avalanche",
  11155111: "Sepolia", 84532: "Base Sepolia", 421614: "Arb Sepolia", 11155420: "OP Sepolia",
};

interface ClaimState {
  attestation: string | null;
  claiming: boolean;
  claimedTx: string | null;
  error: string | null;
}

interface Props {
  chainScope: "mainnet" | "testnet";
}

export function PendingClaimsBanner({ chainScope }: Props) {
  const [claims, setClaims] = useState<PendingClaim[]>([]);
  const [claimStates, setClaimStates] = useState<Record<string, ClaimState>>({});
  const { writeContractAsync } = useWriteContract();
  const { switchChainAsync } = useSwitchChain();
  const currentChainId = useChainId();

  // Load claims from localStorage whenever window focuses or component mounts.
  const reload = useCallback(() => {
    setClaims(loadPendingClaims());
  }, []);

  useEffect(() => {
    reload();
    window.addEventListener("focus", reload);
    return () => window.removeEventListener("focus", reload);
  }, [reload]);

  // Open an SSE stream for each pending claim that doesn't yet have an attestation.
  // The backend polls Circle Iris internally and pushes an event when ready.
  useEffect(() => {
    if (claims.length === 0) return;
    const sources: EventSource[] = [];

    for (const claim of claims) {
      const state = claimStates[claim.id];
      if (state?.attestation || state?.claimedTx) continue;

      const base = chainScope === "testnet" ? "/api/testnet/v1" : "/api/v1";
      const es = new EventSource(`${base}/cctp/attestation/stream/${claim.messageHash}`);
      const claimId = claim.id;

      es.onmessage = (e) => {
        try {
          const data = JSON.parse(e.data) as { status: string; attestation?: string };
          if (data.status === "complete" && data.attestation) {
            setClaimStates(prev => ({
              ...prev,
              [claimId]: { ...prev[claimId], attestation: data.attestation!, claiming: false, error: null, claimedTx: null },
            }));
            es.close();
          }
        } catch {
          // ignore malformed event
        }
      };

      es.onerror = () => es.close();
      sources.push(es);
    }

    return () => sources.forEach(s => s.close());
  // claimStates intentionally excluded — we only open streams when claims list changes
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [claims, chainScope]);

  const handleClaim = useCallback(async (claim: PendingClaim) => {
    const att = claimStates[claim.id]?.attestation;
    if (!att || !claim.claimContract) return;

    setClaimStates(prev => ({ ...prev, [claim.id]: { ...prev[claim.id] ?? {}, attestation: att, claiming: true, error: null, claimedTx: null } }));

    try {
      if (currentChainId !== claim.claimChainId) {
        await switchChainAsync({ chainId: claim.claimChainId });
      }
      const hash = await writeContractAsync({
        address: claim.claimContract as `0x${string}`,
        abi: RECEIVE_MESSAGE_ABI,
        functionName: "receiveMessage",
        args: [claim.messageBytes as `0x${string}`, att as `0x${string}`],
        chainId: claim.claimChainId,
      });
      setClaimStates(prev => ({ ...prev, [claim.id]: { attestation: att, claiming: false, error: null, claimedTx: hash } }));
      // Remove from pending store after successful claim.
      removePendingClaim(claim.id);
      setClaims(prev => prev.filter(c => c.id !== claim.id));
    } catch (e) {
      const msg = e instanceof Error ? e.message.slice(0, 160) : "Claim failed";
      setClaimStates(prev => ({ ...prev, [claim.id]: { attestation: att, claiming: false, error: msg, claimedTx: null } }));
    }
  }, [claimStates, writeContractAsync, switchChainAsync, currentChainId]);

  const handleDismiss = useCallback((id: string) => {
    removePendingClaim(id);
    setClaims(prev => prev.filter(c => c.id !== id));
  }, []);

  // Filter out claims that have been successfully claimed in this session.
  const visibleClaims = claims.filter(c => !claimStates[c.id]?.claimedTx);
  if (visibleClaims.length === 0) return null;

  return (
    <div className="w-full border-b border-[#2a2a2a] bg-[#1a1a2e]">
      {visibleClaims.map(claim => {
        const state = claimStates[claim.id];
        const ready = !!state?.attestation;
        const claiming = !!state?.claiming;

        return (
          <div key={claim.id} className="max-w-7xl mx-auto px-6 py-3 flex flex-wrap items-center gap-3">
            {/* status dot */}
            <span
              className="w-2 h-2 rounded-full shrink-0"
              style={{ backgroundColor: ready ? "#4ade80" : "#fbbf24" }}
            />

            {/* claim info */}
            <span className="text-[12px] font-mono" style={{ color: "#c6c5d8" }}>
              <strong style={{ color: "#e5e2e1" }}>
                {fmtAmt(claim.amount, claim.decimals)} {claim.fromAsset}
              </strong>
              {" "}burn on {claim.fromChain} →{" "}
              <strong style={{ color: "#e5e2e1" }}>{CHAIN_NAMES[claim.claimChainId] ?? claim.toChain}</strong>
            </span>

            {/* status */}
            <span className="text-[11px] font-mono" style={{ color: ready ? "#4ade80" : "#fbbf24" }}>
              {ready ? "Attestation ready" : "Waiting for attestation…"}
            </span>

            {/* error */}
            {state?.error && (
              <span className="text-[11px]" style={{ color: "#ffb4ab" }}>{state.error}</span>
            )}

            {/* actions */}
            <div className="ml-auto flex items-center gap-2">
              {ready && (
                <button
                  onClick={() => handleClaim(claim)}
                  disabled={claiming}
                  className="px-3 py-1.5 text-[11px] font-mono font-semibold uppercase tracking-wider transition-opacity disabled:opacity-50"
                  style={{ backgroundColor: "#bec2ff", color: "#131313" }}
                >
                  {claiming ? "Claiming…" : `Claim ${claim.toAsset}`}
                </button>
              )}
              <button
                onClick={() => handleDismiss(claim.id)}
                className="px-2 py-1.5 text-[11px] font-mono transition-opacity hover:opacity-70"
                style={{ color: "#908fa1" }}
                title="Dismiss (claim data is saved; you can restore via localStorage)"
              >
                ✕
              </button>
            </div>
          </div>
        );
      })}
    </div>
  );
}
