import { CHAINS, TOKENS } from "../tokens";

export interface ParsedIntent {
  amount: string;
  srcToken: string;
  dstToken: string;
  srcChain: string;  // canonical chain name, e.g. "ethereum", "base-sepolia"
  dstChain: string;
}

// Build chain alias map from the CHAINS array + manual aliases
const CHAIN_ALIASES: Record<string, string> = {
  "eth":                 "ethereum",
  "ethereum":            "ethereum",
  "base":                "base",
  "arb":                 "arbitrum",
  "arbitrum":            "arbitrum",
  "op":                  "optimism",
  "optimism":            "optimism",
  "poly":                "polygon",
  "polygon":             "polygon",
  "avax":                "avalanche",
  "avalanche":           "avalanche",
  "bnb":                 "bsc",
  "bsc":                 "bsc",
  "bnb chain":           "bsc",
  "sol":                 "solana",
  "solana":              "solana",
  "sepolia":             "sepolia",
  "base sepolia":        "base-sepolia",
  "base-sepolia":        "base-sepolia",
  "arb sepolia":         "arbitrum-sepolia",
  "arbitrum sepolia":    "arbitrum-sepolia",
  "arbitrum-sepolia":    "arbitrum-sepolia",
  "op sepolia":          "op-sepolia",
  "optimism sepolia":    "op-sepolia",
  "op-sepolia":          "op-sepolia",
};
// Add canonical chain names from CHAINS (redundant for most, but ensures coverage).
for (const c of CHAINS) {
  CHAIN_ALIASES[c.name.toLowerCase()] = c.name;
  CHAIN_ALIASES[c.label.toLowerCase()] = c.name;
}

// Collect all token symbols that exist in the TOKENS registry.
const KNOWN_TOKENS = new Set<string>();
for (const tokens of Object.values(TOKENS)) {
  for (const t of tokens) {
    KNOWN_TOKENS.add(t.symbol.toLowerCase());
  }
}

function resolveChain(text: string): string | null {
  const t = text.toLowerCase().trim();
  // Try longest match first to handle "base sepolia" before "base".
  const aliases = Object.keys(CHAIN_ALIASES).sort((a, b) => b.length - a.length);
  for (const alias of aliases) {
    if (t === alias) return CHAIN_ALIASES[alias];
  }
  // Partial prefix match: "base sep" → "base sepolia"
  for (const alias of aliases) {
    if (alias.startsWith(t) || t.startsWith(alias)) return CHAIN_ALIASES[alias];
  }
  return null;
}

function resolveToken(text: string): string | null {
  const t = text.toLowerCase().trim();
  if (KNOWN_TOKENS.has(t)) return t.toUpperCase();
  return null;
}

/**
 * Parses a natural language bridge/swap intent into structured form.
 * Returns null when the intent cannot be parsed with sufficient confidence.
 *
 * Supported patterns:
 *   "Bridge 10 USDC from Ethereum to Base"
 *   "Swap 0.1 ETH to USDC on Base"
 *   "Send 5 USDC from Sepolia to Base Sepolia"
 *   "10 USDC from Ethereum to Base"
 */
export function parseIntent(text: string): ParsedIntent | null {
  const lower = text.toLowerCase().trim();

  // 1. Extract amount (first decimal number in the string).
  const amountMatch = lower.match(/\b(\d+(?:\.\d+)?)\b/);
  if (!amountMatch) return null;
  const amount = amountMatch[1];

  // 2. Strip the action verb prefix ("bridge", "swap", "send", "transfer", "move").
  const stripped = lower.replace(/^(bridge|swap|send|transfer|move)\s+/, "");

  // 3. Identify the token immediately after the amount.
  const tokenAfterAmount = stripped.match(/\d+(?:\.\d+)?\s+([a-z]+)/);
  let srcToken = tokenAfterAmount ? resolveToken(tokenAfterAmount[1]) : null;

  // 4. Extract "from <src>" and "to <dst>" segments.
  //    Handle multi-word chain names like "Base Sepolia".
  let srcChain: string | null = null;
  let dstChain: string | null = null;
  let dstToken: string | null = null;

  // Pattern: "from X to Y"
  const fromToMatch = stripped.match(/\bfrom\s+([a-z0-9][a-z0-9\s-]*?)\s+to\s+([a-z0-9][a-z0-9\s-]*)(?:\s|$)/);
  if (fromToMatch) {
    srcChain = resolveChain(fromToMatch[1].trim());
    // "to Y" — Y could be a chain name or a token (for swaps like "swap ETH to USDC on Base")
    const dstText = fromToMatch[2].trim();
    dstChain = resolveChain(dstText);
    if (!dstChain) dstToken = resolveToken(dstText);
  }

  if (!fromToMatch) {
    // Pattern: "to X" (destination only)
    const toMatch = stripped.match(/\bto\s+([a-z0-9][a-z0-9\s-]*)(?:\s|$)/);
    if (toMatch) {
      const dstText = toMatch[1].trim();
      dstChain = resolveChain(dstText);
      if (!dstChain) dstToken = resolveToken(dstText);
    }

    // Pattern: "on X" (for swaps: "swap ETH to USDC on Base")
    const onMatch = stripped.match(/\bon\s+([a-z0-9][a-z0-9\s-]*)(?:\s|$)/);
    if (onMatch && !srcChain) {
      srcChain = resolveChain(onMatch[1].trim());
      if (!dstChain) dstChain = srcChain; // same chain swap
    }
  }

  // 5. Need at least a destination chain or a destination token.
  if (!dstChain && !dstToken) return null;

  // 6. Resolve default destination token (same as source for bridges).
  if (!dstToken) dstToken = srcToken;
  if (!srcToken) srcToken = dstToken;
  if (!srcToken) return null;

  return {
    amount,
    srcToken,
    dstToken,
    srcChain: srcChain ?? "",
    dstChain: dstChain ?? srcChain ?? "",
  };
}
