# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# CLAUDE.md — Bridge + Swap Aggregator

> This file is the single source of truth for any AI agent working on this codebase.
> Read it fully before touching any file. Update it when you discover something meaningful.

---

## Commands

**Run backend (dev):**
```sh
go run ./cmd/server
# or via turbo:
bun run dev:api   # equivalent: cd apps/api && bun dev
```

**Run frontend (dev):**
```sh
cd apps/web && bun dev
# or from root:
bun run dev:web
```

**Run all tests (Go):**
```sh
go test ./...
```

**Run a single Go test:**
```sh
go test ./internal/bridges/... -run TestAdapterContract -v
go test ./internal/service/... -run TestStepTx -v
```

**Build backend binary:**
```sh
go build ./cmd/server
```

**Build frontend:**
```sh
bun run build:web
```

**Lint frontend:**
```sh
cd apps/web && bun lint
```

**Smoke test (phase B):**
```sh
bun run smoke:phaseb   # runs ./scripts/smoke_phase_b.sh
```

**Run testnet mode:**
```sh
NETWORK=testnet go run ./cmd/server
# or: ./scripts/run-testnet.sh
```

---

## Tech Stack

**Backend:** Go + Gin HTTP framework, Viper for config (env vars + `internal/config/config.yaml`), optional PostgreSQL (`DATABASE_URL` env var — if unset, `/execute` and `/operations` return 503).

**Frontend:** Vite + React 19, TypeScript, Tailwind CSS, wagmi v2, RainbowKit, viem, TanStack Query.

**Monorepo:** Turborepo with Bun as package manager. `apps/api/package.json` is a thin shim — it just delegates to `go run ./cmd/server`. All real backend code is in Go.

---

## 🎯 Product Goal

Build a **swap + bridge aggregator** that supports **any-to-any** (chain × token) routing.

The product must work in two modes, selectable via UI (dropdown or toggle):
- **Mainnet mode** — live production chains
- **Testnet mode** — test networks (e.g. Sepolia) for pre-production validation

### Success Criteria (in order)
1. Quote flow works — user gets prices from all supported providers
2. Best-price selection works — correct provider is ranked and selected
3. Execution works — transaction is submitted on-chain correctly
4. Funds arrive — destination wallet receives the correct token/amount
5. ✅ Each bridge/swap provider is confirmed working end-to-end on testnet before mainnet

---

## 🔬 Current Testing Status

> Always check `/bridge-aggregator/.claude/memory/project_testing_progress.md` for the latest state before making changes.

**Contract under test (Sepolia):**
`0x4f8bbccc89d443e6998e52d7b57ce2ae09476328`
→ https://sepolia.etherscan.io/address/0x4f8bbccc89d443e6998e52d7b57ce2ae09476328

### Bridge Provider Status (last known)
Source of truth: `/STATUS.md` (keep this table synced when status changes).

| Provider             | Status          | Notes                                                                          |
|----------------------|-----------------|--------------------------------------------------------------------------------|
| Across               | ✅ Confirmed    | Funds received on Base Sepolia (2x fillRelay confirmed 2026-03-30)             |
| CCTP                 | ⚠️ WIRED       | depositForBurn succeeds; receiveMessage (claim) flow is not fully completed end-to-end in UI |
| Canonical ERC20      | ❌ BLOCKED      | USDC rejected for canonical ERC20 testnet path; L1StandardBridge cannot deliver to L2 |
| Canonical ETH        | ⚠️ WIRED       | ETH-only path works in code; end-to-end Sepolia ETH → Base Sepolia still pending |

**Do not move on to the next provider until the current one passes end-to-end.**
Do not mark a provider as done unless funds are confirmed received on-chain.

---

## 🏗️ Architecture Rules

### Repo Structure
This is a **Turborepo** monorepo:
- `apps/web` — frontend (Vite + React, wagmi, RainbowKit, Tailwind)
- `apps/api` — thin npm shim that runs the Go backend via `go run ./cmd/server`
- `cmd/server/` — Go entrypoint; wires adapters, starts Gin HTTP server
- `internal/` — all backend logic:
  - `bridges/` — bridge adapters (Across, CCTP, Canonical, Stargate, Mayan, Blockdaemon) + `bridge.go` interface
  - `dex/` — DEX adapters (1inch, Uniswap, 0x, Blockdaemon DEX) + `dex.go` interface
  - `api/` — Gin HTTP handlers (`handler.go`)
  - `router/` — quote fan-out and ranking engine
  - `service/` — quote and execution orchestration
  - `models/` — shared types
  - `config/` — config loading (env vars + YAML)
  - `store/` — PostgreSQL persistence (optional)
  - `lifi/` — LiFi transaction builder
  

Never mix FE and BE concerns. Never duplicate types across packages.

### Code Quality — Non-Negotiable
- **No god files.** Every file has one clear responsibility.
- **No complex conditional chains.** If you have 3+ nested conditions, it's a signal to refactor.
- **No clueless code.** Every line must have a reason. No dead code, no commented-out blocks left behind.
- **No multiple conditional references** to the same state/variable across unrelated components.
- **Proper abstractions.** Extract hooks, services, and utilities when logic is reused more than once.
- **No `console.log` on the frontend.** Use a proper logger (e.g. `pino`, `winston`, or a thin wrapper). Logging spam is a red flag, not a debug tool.
- **Graceful errors everywhere.** Every async operation has an error state rendered to the user — no silent failures, no white screens.

### Component Rules (Frontend)
- Each bridge/swap provider has its own isolated module — no shared mutation of state between providers.
- UI state (loading, error, success) must be explicit and typed — never inferred.
- Responsiveness is required. No fixed pixel widths. Test at mobile and desktop breakpoints.
- No crashes. If an RPC call fails, show the error. If a quote fails, fall back gracefully.

### Service Rules (Backend)
- Each bridge provider is a separate service/adapter implementing a shared interface.
- Quote aggregation is a pure function — takes inputs, returns ranked quotes, no side effects.
- Execution logic is separate from quoting logic.

---

## 🌉 Bridge / Swap Integration Checklist

For **each provider**, confirm all steps before marking as done:

- [ ] Quote endpoint returns valid response
- [ ] Best price selection includes this provider
- [ ] Transaction is built correctly (correct calldata, value, gas)
- [ ] Transaction submitted on testnet without revert
- [ ] Funds confirmed received at destination wallet on-chain
- [ ] Error states handled (insufficient liquidity, unsupported route, timeout)

---

## 🚀 Deployment Requirements

The goal is to deploy both FE and BE via the Turborepo pipeline.

- CI/CD must build and deploy both `apps/web` and `apps/api` correctly
- Environment variables must be documented in `.env.example` for each app
- No hardcoded RPC URLs, API keys, or chain IDs anywhere in code
- Health check endpoint on the API (`/health`)
- The FE must not crash if the API is slow or temporarily unavailable

---

## 🔮 Future Scope (Do Not Build Yet)

- **Intent-based bot** — this comes **after** all bridge/swap providers are confirmed working on mainnet.

Do not scaffold, stub, or reference this in any current code. Zero premature abstraction for future scope.

---

## 🧠 Memory Index

| File | Purpose |
|------|---------|
| `/bridge-aggregator/.claude/memory/project_testing_progress.md` | Live status of the 3-layer testing strategy |
| `CLAUDE.md` (this file) | Agent instructions, architecture rules, success criteria |

**Update these files when:**
- A provider moves from broken → working (or vice versa)
- A new architectural decision is made
- A significant bug root cause is found
- Testing phase advances

---

## ⚠️ Agent Behavior Rules

1. **Read `project_testing_progress.md` first** on every session before writing code.
2. **Fix broken things before building new things.** CCTP and Canonical are broken. That is the priority.
3. **Do not refactor working code speculatively.** Only refactor when it directly unblocks a task.
4. **Do not introduce new dependencies** without a clear reason stated in the PR/commit.
5. **Do not change the UI** while debugging bridge execution logic — keep concerns isolated.
6. **Always confirm on-chain.** "It looks right in logs" is not done. Etherscan confirmation is done.
7. **When stuck, say so clearly.** Do not silently change approach mid-task. Surface the blocker.
8. **One provider at a time.** Finish CCTP or Canonical before touching anything else.
