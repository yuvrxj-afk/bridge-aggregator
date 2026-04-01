/**
 * Provider visibility registry — mirrors STATUS.md.
 *
 * All providers in this set are shown in the UI. Routes that cannot execute
 * (missing credentials, known bugs) are gated at the execution layer via
 * execution.supported=false, not here.
 *
 * To add a provider: uncomment its entry and update STATUS.md.
 * To remove a provider from the UI: comment it out here.
 */

export const VISIBLE_PROVIDERS = new Set([
  "across",              // CONFIRMED 2026-03-31: multiple fillRelay receipts on Base Sepolia
  "cctp",                // CONFIRMED 2026-03-31: burn 0x7d6840ff → claim 0x8c287c9b (Sepolia→Base Sepolia)
  "canonical_base",      // WIRED: ETH path unconfirmed mainnet; ERC20 blocked testnet
  "canonical_optimism",  // WIRED: same as canonical_base
  "canonical_arbitrum",  // WIRED: same as canonical_base
  "uniswap_trading_api", // WIRED: swap legs in composite routes unconfirmed end-to-end
  "blockdaemon",         // WIRED: missing tx data on some quotes; not confirmed end-to-end
  "mayan",               // WIRED: Solana path unconfirmed; mainnet-only
  "stargate",            // BLOCKED: waiting on STARGATE_API_KEY — quotes will not return
  "zeroex",              // BLOCKED: needs ZEROEX_TAKER configured
  "oneinch",             // BLOCKED: waiting on ONEINCH_API_KEY
]);
