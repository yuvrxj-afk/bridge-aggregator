// Persistent store for CCTP pending claims (localStorage).
// After a depositForBurn tx mines, the claim info is saved here so the user
// can navigate away and complete the receiveMessage call later.

export interface PendingClaim {
  id: string;             // random UUID — used to remove specific claims
  messageHash: string;    // keccak256(messageBytes) — for attestation polling
  messageBytes: string;   // raw bytes from MessageSent log
  claimContract: string;  // MessageTransmitter address on destination chain
  claimChainId: number;
  srcTxHash: string;      // depositForBurn tx hash (for explorer link)
  srcChainId: number;
  amount: string;         // base units (for display)
  fromAsset: string;
  toAsset: string;
  fromChain: string;
  toChain: string;
  decimals: number;
  savedAt: number;        // Date.now() — for sorting / expiry
}

const KEY = "cctp_pending_claims";

export function loadPendingClaims(): PendingClaim[] {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return [];
    return JSON.parse(raw) as PendingClaim[];
  } catch {
    return [];
  }
}

export function savePendingClaim(claim: PendingClaim): void {
  const claims = loadPendingClaims().filter(c => c.id !== claim.id);
  claims.push(claim);
  localStorage.setItem(KEY, JSON.stringify(claims));
}

export function removePendingClaim(id: string): void {
  const claims = loadPendingClaims().filter(c => c.id !== id);
  localStorage.setItem(KEY, JSON.stringify(claims));
}
