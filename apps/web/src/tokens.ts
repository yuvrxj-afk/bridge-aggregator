export interface Token {
  symbol: string;
  address: string;
  decimals: number;
}

export interface Chain {
  id: number;
  name: string;
  label: string;
  logoColor: string;
}

export const MAINNET_CHAIN_IDS = [1, 8453, 42161, 10, 137, 43114, 56, 900] as const;
export const TESTNET_CHAIN_IDS = [11155111, 84532, 421614, 11155420] as const;

export const MAINNET_CHAINS: Chain[] = [
  { id: 1,     name: "ethereum", label: "Ethereum", logoColor: "#627EEA" },
  { id: 8453,  name: "base",     label: "Base",     logoColor: "#0052FF" },
  { id: 42161, name: "arbitrum", label: "Arbitrum", logoColor: "#28A0F0" },
  { id: 10,    name: "optimism", label: "Optimism", logoColor: "#FF0420" },
  { id: 137,   name: "polygon",  label: "Polygon",  logoColor: "#8247E5" },
  { id: 43114, name: "avalanche", label: "Avalanche", logoColor: "#E84142" },
  { id: 56,    name: "bsc",       label: "BNB Chain", logoColor: "#F3BA2F" },
  { id: 900,   name: "solana",    label: "Solana",    logoColor: "#9945FF" },
];

export const CHAINS: Chain[] = [...MAINNET_CHAINS];

// Sentinel chain ID for Solana (not a real EVM chain ID).
export const SOLANA_CHAIN_ID = 900;

const NATIVE = "0x0000000000000000000000000000000000000000";

export const MAINNET_TOKENS: Record<number, Token[]> = {
  1: [
    { symbol: "ETH",  address: NATIVE,                                       decimals: 18 },
    { symbol: "USDC", address: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", decimals: 6  },
    { symbol: "USDT", address: "0xdAC17F958D2ee523a2206206994597C13D831ec7", decimals: 6  },
    { symbol: "WETH", address: "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", decimals: 18 },
  ],
  8453: [
    { symbol: "ETH",  address: NATIVE,                                       decimals: 18 },
    { symbol: "USDC", address: "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913", decimals: 6  },
    { symbol: "WETH", address: "0x4200000000000000000000000000000000000006", decimals: 18 },
    { symbol: "DAI",  address: "0x50c5725949A6F0c72E6C4a641F24049A917DB0Cb", decimals: 18 },
  ],
  42161: [
    { symbol: "ETH",  address: NATIVE,                                       decimals: 18 },
    { symbol: "USDC", address: "0xaf88d065e77c8cC2239327C5EDb3A432268e5831", decimals: 6  },
    { symbol: "USDT", address: "0xFd086bC7CD5C481DCC9C85ebE478A1C0b69FCbb9", decimals: 6  },
    { symbol: "WETH", address: "0x82aF49447D8a07e3bd95BD0d56f35241523fBab1", decimals: 18 },
  ],
  10: [
    { symbol: "ETH",  address: NATIVE,                                       decimals: 18 },
    { symbol: "USDC", address: "0x0b2C639c533813f4Aa9D7837CAf62653d097Ff85", decimals: 6  },
    { symbol: "USDT", address: "0x94b008aA00579c1307B0EF2c499aD98a8ce58e58", decimals: 6  },
    { symbol: "WETH", address: "0x4200000000000000000000000000000000000006", decimals: 18 },
  ],
  137: [
    { symbol: "MATIC", address: NATIVE,                                       decimals: 18 },
    { symbol: "USDC",  address: "0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359", decimals: 6  },
    { symbol: "USDT",  address: "0xc2132D05D31c914a87C6611C10748AEb04B58e8F", decimals: 6  },
    { symbol: "WETH",  address: "0x7ceB23fD6bC0adD59E62ac25578270cFf1b9f619", decimals: 18 },
  ],
  43114: [
    { symbol: "AVAX",  address: NATIVE,                                       decimals: 18 },
    { symbol: "USDC",  address: "0xB97EF9Ef8734C71904D8002F8b6Bc66Dd9c48a6E", decimals: 6  },
    { symbol: "USDT",  address: "0x9702230A8Ea53601f5cD2dc00fDBc13d4dF4A8c7", decimals: 6  },
    { symbol: "WETH",  address: "0x49D5c2BdFfac6CE2BFdB6640F4F80f226bc10bAB", decimals: 18 },
  ],
  56: [
    { symbol: "BNB",   address: NATIVE,                                       decimals: 18 },
    { symbol: "USDC",  address: "0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d", decimals: 18 },
    { symbol: "USDT",  address: "0x55d398326f99059fF775485246999027B3197955", decimals: 18 },
    { symbol: "WETH",  address: "0x2170Ed0880ac9A755fd29B2688956BD959F933F8", decimals: 18 },
  ],
  // Solana — SPL token mint addresses (base58), not EVM hex addresses.
  900: [
    { symbol: "SOL",  address: "So11111111111111111111111111111111111111112", decimals: 9  },
    { symbol: "USDC", address: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", decimals: 6  },
    { symbol: "USDT", address: "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB", decimals: 6  },
  ],
};

// ── Testnet chains and tokens ─────────────────────────────────────────────────
// Only activated when VITE_NETWORK=testnet. Additive — does NOT replace mainnet entries.

export const TESTNET_CHAINS: Chain[] = [
  { id: 11155111, name: "sepolia",          label: "Sepolia",          logoColor: "#627EEA" },
  { id: 84532,    name: "base-sepolia",     label: "Base Sepolia",     logoColor: "#0052FF" },
  { id: 421614,   name: "arbitrum-sepolia", label: "Arbitrum Sepolia", logoColor: "#28A0F0" },
  { id: 11155420, name: "op-sepolia",       label: "OP Sepolia",       logoColor: "#FF0420" },
];

// Testnet USDC addresses — sourced from Circle's sandbox deployment.
export const TESTNET_TOKENS: Record<number, Token[]> = {
  11155111: [
    { symbol: "ETH",  address: NATIVE,                                       decimals: 18 },
    { symbol: "WETH", address: "0xfFf9976782d46CC05630D1f6eBAb18b2324d6B14", decimals: 18 },
    { symbol: "USDC", address: "0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238", decimals: 6  },
  ],
  84532: [
    { symbol: "ETH",  address: NATIVE,                                       decimals: 18 },
    { symbol: "WETH", address: "0x4200000000000000000000000000000000000006", decimals: 18 },
    { symbol: "USDC", address: "0x036CbD53842c5426634e7929541eC2318f3dCF7e", decimals: 6  },
  ],
  421614: [
    { symbol: "ETH",  address: NATIVE,                                       decimals: 18 },
    { symbol: "WETH", address: "0x980B62Da83eFf3D4576C647993b0c1D7faf17c73", decimals: 18 },
    { symbol: "USDC", address: "0x75faf114eafb1BDbe2F0316DF893fd58CE46AA4d", decimals: 6  },
  ],
  11155420: [
    { symbol: "ETH",  address: NATIVE,                                       decimals: 18 },
    { symbol: "WETH", address: "0x4200000000000000000000000000000000000006", decimals: 18 },
    { symbol: "USDC", address: "0x5fd84259d66Cd46123540766Be93DFE6D43130D7", decimals: 6  },
  ],
};

// Always register testnet chains — the runtime toggle needs them regardless of build-time VITE_NETWORK.
// VITE_NETWORK only controls the default scope, not which chains are available.
CHAINS.push(...TESTNET_CHAINS);
export const TOKENS: Record<number, Token[]> = { ...MAINNET_TOKENS, ...TESTNET_TOKENS };

export function getChain(id: number): Chain | undefined {
  return CHAINS.find((c) => c.id === id);
}

export function getTokens(chainId: number, scope?: "mainnet" | "testnet"): Token[] {
  if (scope === "mainnet" && isTestnetChain(chainId)) return [];
  if (scope === "testnet" && !isTestnetChain(chainId)) return [];
  return TOKENS[chainId] ?? [];
}

export function isTestnetChain(chainId: number): boolean {
  return (TESTNET_CHAIN_IDS as readonly number[]).includes(chainId);
}

// ── Token logo URLs (Trust Wallet Assets CDN) ─────────────────────────────────
// https://github.com/trustwallet/assets

const TW = "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains";

const TW_CHAIN: Record<number, string> = {
  1:     "ethereum",
  8453:  "base",
  42161: "arbitrum",
  10:    "optimism",
  137:   "polygon",
  43114: "avalanche",
  56:    "smartchain",
};

// EIP-55 checksum — lightweight, no external dep
function toChecksumAddress(addr: string): string {
  const a = addr.toLowerCase().replace("0x", "");
  return "0x" + a; // TrustWallet accepts lowercase in practice
}

export function tokenLogoUrl(chainId: number, address: string): string {
  const chain = TW_CHAIN[chainId];
  if (!chain) return "";

  // Native coins
  if (!address || address === NATIVE) {
    // ETH is the native token for Ethereum and all OP-stack / Arbitrum chains
    if ([1, 8453, 42161, 10].includes(chainId)) {
      return `${TW}/ethereum/info/logo.png`;
    }
    // MATIC on Polygon
    if (chainId === 137) return `${TW}/polygon/info/logo.png`;
    if (chainId === 56) return `${TW}/smartchain/info/logo.png`;
    if (chainId === 43114) return `${TW}/avalanche/info/logo.png`;
    return `${TW}/${chain}/info/logo.png`;
  }

  return `${TW}/${chain}/assets/${toChecksumAddress(address)}/logo.png`;
}

// Symbol → background color fallback when logo fails to load
export const TOKEN_COLOR: Record<string, string> = {
  ETH:   "#627EEA",
  WETH:  "#627EEA",
  USDC:  "#2775CA",
  USDT:  "#26A17B",
  DAI:   "#F5AC37",
  MATIC: "#8247E5",
  BNB:   "#F3BA2F",
  AVAX:  "#E84142",
};
