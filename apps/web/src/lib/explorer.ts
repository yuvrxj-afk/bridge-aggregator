/** Block explorer base URLs by chain id (EVM). */

const EXPLORER_BASE: Record<number, string> = {
  1: "https://etherscan.io",
  8453: "https://basescan.org",
  42161: "https://arbiscan.io",
  10: "https://optimistic.etherscan.io",
  137: "https://polygonscan.com",
  43114: "https://snowtrace.io",
  56: "https://bscscan.com",
  900: "https://explorer.solana.com",
  11155111: "https://sepolia.etherscan.io",
  84532: "https://sepolia.basescan.org",
  421614: "https://sepolia.arbiscan.io",
  11155420: "https://sepolia-optimism.etherscan.io",
};

function base(chainId: number): string {
  return EXPLORER_BASE[chainId] ?? "https://etherscan.io";
}

export function explorerTxUrl(chainId: number, hash: string): string {
  return `${base(chainId)}/tx/${hash}`;
}

/** Wallet / contract page on the chain explorer (token transfers appear here). */
export function explorerAddressUrl(chainId: number, address: string): string {
  const a = address.trim();
  if (!a.startsWith("0x") || a.length < 42) return `${base(chainId)}/address/${address}`;
  return `${base(chainId)}/address/${a}`;
}
