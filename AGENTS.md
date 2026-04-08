# AGENTS.md — Bridge Aggregator: Full Project State

> This file is the single source of truth for any AI agent (Claude, Cursor, Codex, or other) picking up this codebase mid-stream.
> Read this before writing any code. It tells you what works, what's broken, what's dangerous, and what to build next.
> Keep it updated when anything meaningful changes.

---

## Product Vision — What We Are Building

This is not a bridge aggregator wrapper. It is a cross-chain intent execution layer.

The difference:
- Wrappers accept transaction descriptions (bridge X from A to B) and forward them.
- An intent execution layer accepts outcome descriptions (I want X on chain B) and resolves
  the full path — reading wallet balances across chains, computing all routes including
  yield at destination, executing atomically where possible, recovering automatically on failure.

**V1 ships:** confirmed route execution with execution guarantee transparency — every route
labeled by what happens to funds if it fails (auto-refund, guided claim, or manual recovery).

**Roadmap unlocks:** wallet-aware routing (read balances across chains), yield-aware scoring
(factor in APY at destination), server-side execution engine (complete even if tab closes),
multi-asset consolidation as single intents.

The moat is not the number of bridges. It is execution reliability and outcome ownership.

**Real consumer:** `payflip-bridge` (Rust + Solidity, `/Users/catalyst/Downloads/payflip-bridge`)
is a proprietary stablecoin tunnel that uses this aggregator as its routing and fallback layer.
Three integration seams identified — see project memory for details.

---

## What This System Does

A cross-chain bridge + DEX aggregator. Given a source (chain + token + amount) and a destination (chain + token), it:

1. Fan-outs quotes to all configured bridge and DEX providers in parallel
2. Scores and ranks routes (fee + time + execution safety)
3. Builds and returns transaction calldata for each route hop
4. Tracks operations in PostgreSQL with an immutable event log
5. Supports one-click atomic execution via LiFi Diamond, and guided multi-step execution for CCTP

**Stack:** Go + Gin (backend), Vite + React 19 + wagmi v2 (frontend), PostgreSQL (optional, required for `/execute` and `/operations`)

---

## Current Provider Status

| Provider | Type | Status | Notes |
|---|---|---|---|
| Across | Bridge | ✅ CONFIRMED WORKING | Funds received on Base Sepolia. In `CONFIRMED_PROVIDERS`. One-click LiFi Diamond path works. |
| CCTP (Circle) | Bridge | ⚠️ WIRED, UNCONFIRMED | `depositForBurn` works. `receiveMessage` claim is fully implemented in `ExecutePanel.tsx` (`cctp_claiming` phase). Needs E2E confirmation on testnet. |
| Canonical L2 (Base/OP/Arb) | Bridge | ⚠️ PARTIAL | Quote works. `StepTransaction()` not implemented for `depositETH()`. USDC blocked on testnet (not registered). ETH-only path possible. |
| Stargate/LayerZero | Bridge | ⚠️ QUOTE ONLY | Quote works via Stargate API. `StepTransaction()` not implemented. |
| Mayan | Bridge | ⚠️ WIRED, UNCONFIRMED | Solana↔EVM. Code complete. Needs Solana devnet wallet + E2E confirmation. |
| Uniswap Trading API | DEX | ✅ WORKING | Tier 1. Full quote + step transaction. |
| 1inch Classic Swap | DEX | ✅ WORKING | Tier 1. Full quote + step transaction. |
| 0x Allowance Holder | DEX | ⚠️ DEGRADED | Tier 2. Requires valid taker address. Drops to ConfigBroken without it. |
| Blockdaemon DEX | DEX | ⚠️ DEGRADED | Tier 2. Same-chain only. May return incomplete tx data. |

**Rule:** Do NOT add a provider to `apps/web/src/config/providers.ts` (`CONFIRMED_PROVIDERS`) until funds are confirmed received on-chain via Etherscan/Solscan.

---

## Architecture — Key Facts

```
POST /api/v1/quote
  → service.Quote() enriches request (ChainID, TokenAddress from registry)
  → router.QuoteUnified() fans out to bridge + DEX adapters in parallel
  → filterSaneRoutes() → filterSaneRoutesWithReference() (CoinGecko USD check) → filterMinInputValue()
  → scoreRoute() ranks: fee + time + execution intent penalty + route tier penalty
  → returns sorted routes with full ExecutionProfile per route

POST /api/v1/route/stepTransaction  (per hop, called at execution time)
  → service.PopulateStepTransaction()
  → returns TransactionRequest (EVM) or SolanaTransactionRequest or BridgeTxCall

POST /api/v1/route/buildTransaction  (one-click atomic, Across + CCTP only)
  → internal/lifi/builder.go translates route to LiFi Diamond parameters

POST /api/v1/execute
  → creates Operation in PostgreSQL (status: pending)
  → frontend signs + broadcasts each step
  → PATCH /api/v1/operations/:id/status to update
```

**ExecutionProfile** (computed per route by `deriveExecutionProfile()` in `internal/router/engine.go`):
- `intent`: `atomic_one_click` | `guided_two_step` | `async_claim` | `unsupported`
- `guarantee`: `relay_fill_or_refund` | `manual_recovery_required` | `unknown`
- `recovery`: `automatic` | `resumable_guided` | `manual`

This is richer safety metadata than most aggregators expose. It is fully computed but **not yet rendered in the UI**.

**Scoring** (`scoreRoute()` in `internal/router/engine.go`) is multi-dimensional:
- Base: `1000 / (1 + fee)` for cheapest mode, `1000 / timeNorm` for fastest
- Execution penalty: `async_claim` +1.5, `guided_two_step` +0.1, `atomic_one_click` +0
- Route tier penalty: `aggregator` +0.25, `placeholder` +2.0

**Token + Chain Registry:** `internal/bridges/chainmap.go` — single source of truth for chain IDs, token addresses, decimals. Both mainnet and testnet. Must be updated when adding new chains or tokens.

**Provider Visibility:** `apps/web/src/config/providers.ts` — `CONFIRMED_PROVIDERS` set. Only providers in this set appear in the UI. Single place to enable/disable providers.

---

## Security Issues — Must Fix Before Production

These were identified 2026-04-06 from actual code reads. All are real, not theoretical.

### CRITICAL (block production deployment)

**1. CORS Wildcard Default** — `cmd/server/main.go` lines 115–126
When `ALLOWED_ORIGIN` env var is not set, server allows all origins (`["*"]`).
Any malicious webpage can call `/execute` cross-origin in a user's browser.
**Fix:** Remove wildcard fallback. Require explicit `ALLOWED_ORIGIN`.

**2. Zero Authentication** — `cmd/server/main.go`, `internal/api/handler.go`
No API key, JWT, or wallet signature check on any endpoint.
`PATCH /api/v1/operations/:id/status` accepts `"completed"` from anyone.
`GET /api/v1/operations` exposes all users' route + wallet data.
**Fix:** Static `X-API-Key` middleware on mutating endpoints minimum. Wallet signature on `/execute` for public product.

### HIGH (fix in first sprint)

**3. No Rate Limiting**
`/quote` fans out to multiple external APIs. No per-IP throttle.
Attacker can exhaust Across/1inch API quotas, getting the server banned for all users.
**Fix:** `golang.org/x/time/rate` middleware. `/quote`: 10 QPM, `/intent/parse`: 5 QPM, `/execute`: 20 QPM.

**4. No Request Body Size Limit**
`c.ShouldBindJSON()` used everywhere without size cap. 100MB body is parsed in full.
**Fix:** Global `http.MaxBytesReader` middleware, 1MB limit.

**5. messageHash Concatenated Raw into URL** — `internal/api/handler.go` ~line 789
`url := attestationURL + "/v1/attestations/" + messageHash`
`messageHash` is user-supplied with no validation.
**Fix:** Validate `^0x[0-9a-fA-F]{64}$` before use.

### MEDIUM

**6. Prompt Injection in Intent Parser** — `internal/intent/parser.go`
Raw user text sent to OpenRouter without sanitization.
**Fix:** 500 char limit. Strip `ignore`, `system:`, `{{`, `<|` patterns.

**7. Raw External API Error Body Returned to Client** — `internal/intent/parser.go` ~line 80
OpenRouter error body truncated and returned in API response.
**Fix:** Return generic error message. Log raw body server-side only.

### LOW

**8. Operations List Has No Ownership Scoping**
`GET /api/v1/operations` with no wallet filter returns all users' operations.
**Fix:** Require wallet address param, filter in SQL query.

**9. Wallet Address Not Validated Before External Calls**
`walletAddress` from request body passed to Across/LiFi without `ethutil.IsAddress()` check.

### Confirmed Safe (verified)
- SQL injection: parameterized queries throughout `internal/store/store.go` ✅
- Amount overflow: `math/big.Int` everywhere ✅
- Hardcoded secrets: none ✅
- eval / dangerouslySetInnerHTML: none ✅

---

## Real Product Gaps

These are confirmed gaps from code reads (not assumptions):

| Gap | Where | Impact |
|---|---|---|
| Session-resume: no on-connect query for pending ops | `apps/web/src/App.tsx` | Pending CCTP/Canonical ops invisible on new session |
| No fallback routing on execution failure | `apps/web/src/components/ExecutePanel.tsx`, `internal/service/execute.go` | Failure = full restart, no automatic retry |
| No proactive quote expiry timer | `apps/web/src/components/ExecutePanel.tsx` | Stale quotes cause silent reverts (reactive error detection exists, no countdown) |
| SSE progressive rendering not used | `apps/web/src/App.tsx` | Frontend likely uses blocking `/quote` (7s) not streaming `/quote/stream` (first result ~1s) |
| ExecutionProfile not rendered | Route card components | Rich guarantee/intent data computed, never shown to user |
| CoinGecko no fallback | `internal/service/quote.go` | USD sanity filter silently disabled on CoinGecko outage (fail-open, safe but degraded) |

---

## Build Plan

Work phases in this order. Do not skip Phase 0.

### Phase 0 — Security (Do First)
| Task | File(s) |
|---|---|
| Remove CORS wildcard; require explicit `ALLOWED_ORIGIN` | `cmd/server/main.go` |
| Per-IP rate limiting middleware | `cmd/server/main.go`, new `internal/middleware/ratelimit.go` |
| Request body size limit (1MB global) | `cmd/server/main.go` |
| Validate `messageHash` format before URL concat | `internal/api/handler.go` |
| Static `X-API-Key` check on mutating endpoints | `cmd/server/main.go` |
| Sanitize intent parser input | `internal/intent/parser.go` |
| Replace raw error body with generic message | `internal/intent/parser.go` |
| Wallet address validation before external calls | `internal/api/handler.go` |
| Scope `GET /api/v1/operations` to wallet address | `internal/api/handler.go`, `internal/store/store.go` |

**Done when:** Server rejects missing `ALLOWED_ORIGIN`, `/execute` returns 401 without key, 100MB body returns 413, invalid messageHash returns 400.

### Phase 1 — Reliability
| Task | File(s) |
|---|---|
| Session-resume on wallet connect | `apps/web/src/App.tsx`, new hook |
| Quote expiry countdown timer | `apps/web/src/components/ExecutePanel.tsx` |
| Switch to SSE for progressive quote rendering | `apps/web/src/App.tsx`, `apps/web/src/api.ts` |
| Fallback routing on execution failure | `apps/web/src/components/ExecutePanel.tsx` |
| Secondary price source (stablecoin=$1 fallback) | `internal/service/quote.go` |

### Phase 2 — Product Quality
| Task | File(s) |
|---|---|
| Render ExecutionProfile guarantee + intent badges | Route card components |
| "Safe Routes Only" filter toggle | `apps/web/src/App.tsx` |
| Per-hop fee breakdown | Route card components |
| Intent receipt post-submission | `apps/web/src/components/TransactionSuccessModal.tsx` |
| Bridge status deep-links from tx_hash | Execute panel |

### Phase 3 — Provider Completion (testnet E2E)
| Task |
|---|
| CCTP: confirm USDC received on Base Sepolia |
| Canonical ETH: implement `depositETH()` step tx, confirm E2E |
| Stargate: implement `StepTransaction()`, confirm E2E |
| Mayan: Solana devnet E2E confirmation |

---

## Commands Reference

```sh
# Backend dev
go run ./cmd/server

# Frontend dev
cd apps/web && bun dev

# All Go tests
go test ./...

# Testnet mode
NETWORK=testnet go run ./cmd/server

# Build frontend
bun run build:web
```

**Key env vars:**
- `ALLOWED_ORIGIN` — required in production (comma-separated origins)
- `DATABASE_URL` — PostgreSQL; if unset, `/execute` and `/operations` return 503
- `NETWORK` — `mainnet` (default) or `testnet`
- `ACROSS_API_KEY`, `ACROSS_DEPOSITOR` — required for Across
- `UNISWAP_API_KEY`, `UNISWAP_SWAPPER_WALLET` — required for Uniswap
- `ONEINCH_API_KEY` — required for 1inch
- `OPENROUTER_KEY` — required for intent parsing
