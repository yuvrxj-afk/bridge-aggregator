# Repository Alignment Plan

## The Problem

The codebase has adapters wired for 6 bridges, 4 DEXes, and 8 chains.
"Wired" and "working" are not the same thing.

Testnet testing revealed this for Canonical. It will reveal it for others too.
The goal of this plan is to establish ground truth before we go further — not to rewrite anything, but to know exactly what we have, what we can trust, and what still needs confirmation.

---

## Step 1 — Create a Confidence Registry

Add a single file at the repo root:

**`/STATUS.md`**

Every bridge adapter, DEX adapter, and chain pairing gets a row. Three possible states only — no in-betweens:

| State | Meaning |
|-------|---------|
| ✅ `CONFIRMED` | End-to-end tested. Funds received on-chain (testnet or mainnet). |
| ⚠️ `WIRED` | Code exists and compiles. Not end-to-end tested. Do not present to user as working. |
| ❌ `BLOCKED` | Missing credential, known bug, or protocol limitation. Must not be callable. |

Fill this in honestly based on what we know right now. Do not mark anything `CONFIRMED` that hasn't had on-chain verification.

**Current point-in-time status (synced 2026-03-31):**

| Component | Status | Notes |
|-----------|--------|-------|
| Across (EVM→EVM) | ✅ CONFIRMED | fillRelay receipts on Base Sepolia |
| CCTP | ✅ CONFIRMED | depositForBurn + receiveMessage confirmed testnet (Sepolia→Base Sepolia, 2026-03-31; burn 0x7d6840ff, claim 0x8c287c9b) |
| Canonical ERC20 (Base/Optimism/Arbitrum) | ❌ BLOCKED | Testnet USDC cannot be delivered via L1StandardBridge minting path |
| Canonical ETH | ⚠️ WIRED | ETH path exists in code; end-to-end confirmation pending |
| Blockdaemon Bridge | ⚠️ WIRED | Missing tx data on some quotes; not confirmed end-to-end |
| Mayan (Solana↔EVM) | ⚠️ WIRED | Intermittent 500s; documented testnet support is unavailable, so testnet E2E remains blocked |
| Stargate | ❌ BLOCKED | Waiting on STARGATE_API_KEY |
| Uniswap Trading API | ⚠️ WIRED | Code exists; swap legs in composite routes unconfirmed |
| Blockdaemon DEX | ⚠️ WIRED | Same tx data caveat as bridge adapter |
| 0x v2 | ❌ BLOCKED | Needs zeroex_taker in config to execute |
| 1inch | ❌ BLOCKED | Waiting on ONEINCH_API_KEY |

---

## Step 2 — Gate the UI Strictly to CONFIRMED

Anything not `CONFIRMED` must not be selectable or executable in the UI.

This is not about hiding features — it's about not letting a user submit a transaction we haven't confirmed works. A `WIRED` route appearing in the UI is a liability.

Implementation: a single allow-list in the frontend config (not scattered conditionals) that maps directly to the `STATUS.md` entries. When a provider moves to `CONFIRMED`, it gets added to the allow-list. One place, one change.

---

## Step 3 — Confirm Providers One at a Time, In Priority Order

Work through the `WIRED` list in this order based on value and effort:

1. **CCTP** — High value (native USDC), clear gap (FE claim step missing). Fix the claim flow, confirm on testnet.
2. **Uniswap swap legs** — Needed for Swap→Bridge and Bridge→Swap routes. Confirm source and dest swap in a composite route.
3. **Canonical (mainnet ETH)** — ETH bridging was never broken. Confirm ETH-only on mainnet before any ERC20 work.
4. **Blockdaemon** — Validate the quote completeness issue, confirm a route end-to-end.
5. **Mayan** — Solana path. Confirm once EVM routes are solid.
6. **Stargate / 1inch / 0x** — Unblock credentials, then confirm. Last priority.

Do not work on more than one provider at a time. A provider is done when `STATUS.md` moves to `CONFIRMED`.

---

## Step 4 — One Folder Per Adapter, No Shared Mutable State

While working through the confirmation pass, enforce this structural rule:

Each adapter lives in its own package. It implements the shared interface. It does not reach into another adapter's state, config, or types. Mainnet config and testnet config are separate structs — a testnet limitation cannot affect the mainnet implementation.

If you find adapters sharing state or config in ways that violate this, fix it as you touch that adapter — not speculatively across the whole codebase.

---

## Step 5 — Deployment Only After Core Routes Are CONFIRMED

Deployment is the final step. The deployment checklist (Dockerfiles, env vars, health checks, Turborepo pipeline) does not get touched until at minimum these are `CONFIRMED`:

- Across ✅ (already done)
- CCTP ✅
- Uniswap swap legs ✅ (needed for composite routes)

Deploying before these are confirmed means deploying a product where the core value proposition (Swap→Bridge→Swap) is unverified. That's not a v1 — that's a demo.

---

## What This Is Not

- Not a rewrite
- Not a new architecture
- Not adding features

This is an audit pass that produces ground truth, gates the UI honestly, and creates a clear path to confirmed coverage. Every session from here runs against `STATUS.md` — if it's not `CONFIRMED`, it's not done.
