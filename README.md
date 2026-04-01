## Bridge Aggregator

Bridge Aggregator is a Go service for cross-chain route discovery and transaction preparation.  
It aggregates bridge and DEX providers, returns ranked routes, and provides execution material (LiFi build payloads and step transactions) for supported flows.

---

## Documentation Index

Only core docs are kept:

- `docs/WALKTHROUGH.md` - code walkthrough (how requests move through the codebase, where DEX/bridge adapters are used).
- `docs/V1-PRODUCT.md` - current product scope and v1 boundaries.
- `docs/V2-PRODUCT.md` - v2 product plan, architecture flow, milestones, and acceptance criteria.

---

## Current Capabilities (at a glance)

- **Bridge adapters:** Across, Stargate, Blockdaemon, CCTP, Canonical Base/Optimism/Arbitrum (plus Wormhole placeholder path if configured).
- **DEX adapters:** Uniswap Trading API, 0x, 1inch.
- **Core APIs:** quote, build transaction, step transaction, execute, operation status, capabilities.
- **Execution model:** non-custodial (wallet signs transactions), with mixed execution depth by provider/route shape.

---

## Project Structure

- `apps/api` - monorepo app package for backend build/dev/test tasks.
- `apps/web` - Vite frontend app package (build/dev/test tasks + source).
- `cmd/server` - entrypoint and dependency wiring.
- `internal/api` - thin HTTP handlers.
- `internal/service` - quote/execute/step orchestration.
- `internal/router` - route composition, scoring, execution profiling.
- `internal/bridges` - bridge adapter interface and implementations.
- `internal/dex` - DEX adapter interface and implementations.
- `internal/lifi` - LiFi Diamond transaction builder for supported paths.
- `internal/store` - operation persistence.
- `internal/models` - shared request/response and route models.

---

## Run Locally

Requirements:
- Go 1.21+

Monorepo tasks:

```bash
npm run build
npm run test
npm run dev:web
```

Start server:

```bash
go run ./cmd/server
```

Health check:

```bash
curl http://localhost:8080/health
```

---

## Configuration

`database_url` is required for execute/operation persistence.

Common API-key environment variables:
- `STARGATE_API_KEY`
- `BLOCKDAEMON_API_KEY`
- `UNISWAP_API_KEY`
- `UNISWAP_SWAPPER_WALLET`
- `ZEROEX_API_KEY`
- `ONEINCH_API_KEY`

Do not commit secrets; use environment variables or local config files.

---

## API Endpoints

- `GET /health`
- `GET /api/v1/capabilities`
- `POST /api/v1/quote`
- `POST /api/v1/route/buildTransaction`
- `POST /api/v1/route/stepTransaction`
- `POST /api/v1/execute`
- `GET /api/v1/operations/:id`


