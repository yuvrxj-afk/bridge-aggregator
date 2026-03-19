## Bridge Aggregator

### Overview

Bridge Aggregator is a Go service that exposes a single HTTP API for quoting, executing, and tracking cross‑system bridge transfers (e.g. between chains or domains).  
It discovers available routes across multiple bridge providers, scores them, and orchestrates execution of the selected route.


### Project structure

- `cmd/server` – service entrypoint (bootstraps config, HTTP server, routing).
- `internal/api` – HTTP handlers (`/api/v1/quote`, `/api/v1/execute`, `/api/v1/operations/:id`, `/health`).
- `internal/router` – core routing and aggregation logic across bridges.
- `internal/bridges` – bridge abstractions and concrete bridge integrations.
- `internal/models` – shared domain types (requests, routes, operations, hops, etc.).
- `docs` – **ARCHITECTURE.md** (flow, packages, adapters), **ROUTING-EVOLUTION-PLAN.md** (phases), **specdoc.md** (competitor context & gaps).

### Tech stack

- **Go**, **Gin** (HTTP), **PostgreSQL** (operations store). No Redis or sqlc in use yet.
- **Bridge adapters:** Direct (Across, CCTP, Canonical L2 placeholders), optional aggregator (Blockdaemon), optional API-key bridges (Stargate). See `docs/ROUTING-EVOLUTION-PLAN.md` for phased evolution and next steps.

### Running locally

Prerequisites:

- Go 1.21+ installed.

Run the server:

```bash
go run ./cmd/server
```

By default the HTTP server listens on `:8080`.

**Configuration (env or `config.yaml`):** `database_url` is required for execute/status.

**Getting API keys:**
- **Stargate (LayerZero):** Quotes use the [LayerZero Value Transfer API](https://transfer.layerzero-api.com/v1/docs), which requires an API key. Request one here: **[LayerZero API key request form](https://forms.monday.com/forms/c64c278b03d2b40a24e305943a743527?r=use1)**. Then set `STARGATE_API_KEY` or `stargate_api_key`.
- **Blockdaemon:** Get a key at [Blockdaemon](https://app.blockdaemon.com) (DeFi API); set `BLOCKDAEMON_API_KEY` or `blockdaemon_api_key`.
- **Circle CCTP (bridge, USDC-only):** No API key required for quoting. Execution later will require on-chain txs + attestation polling.
- **0x (DEX):** For swap quotes and composed routes (bridge→swap, swap→bridge), set `ZEROEX_API_KEY` or `zeroex_api_key`. Optional: `ZEROEX_API_URL` (default `https://api.0x.org`), `ZEROEX_TAKER` (wallet that holds sell token and sets allowance; defaults to `uniswap_swapper_wallet` if unset).
- **1inch (DEX):** For Classic Swap quotes and swap tx building, set `ONEINCH_API_KEY` or `oneinch_api_key`. Optional: `ONEINCH_API_URL` (default `https://api.1inch.com`), `ONEINCH_API_VERSION` (default `v6.1`), `ONEINCH_SWAPPER` (wallet used as `from` when generating tx; defaults to `uniswap_swapper_wallet` if unset).

Hop is not integrated (v1 API times out; v2 is JS-only). For Hop-backed routes, use Blockdaemon.

Do not commit API keys; use environment variables.

Health check:

```bash
curl http://localhost:8080/health
```

### Key HTTP endpoints

- `GET /health` – service health and version.
- `POST /api/v1/quote` – compute and return one or more bridge routes with quotes.
- `POST /api/v1/execute` – execute a selected route and create an operation.
- `GET /api/v1/operations/:id` – fetch consolidated operation and hop status.
- `POST /api/v1/dex/quote` – get a DEX swap quote (tries Uniswap Trading API, then 0x when configured; requires chain IDs + token addresses + base-unit amount).
- `POST /api/v1/route/stepTransaction` – populate a swap hop with an unsigned tx (Uniswap or 0x; 0x returns the transaction embedded in the quote).

### Address-first bridge quoting (recommended)

For bridge quotes you can provide either the legacy `(chain, asset)` pair **or** the address-first `(chain_id, token_address, token_decimals)` fields.

Notes:
- **Same-chain requests** are treated as swap-only; bridge adapters are not queried.
- Some bridge providers are **direct**, some are **aggregators**, and some are **placeholders** (you’ll see `provider_data.source` on bridge hops).

Example (address-first, amount in base units):

```json
{
  "source": {
    "chain": "arbitrum",
    "chain_id": 42161,
    "token_address": "0xaf88d065e77c8cC2239327C5EDb3A432268e5831",
    "token_decimals": 6,
    "address": "0xYourWallet"
  },
  "destination": {
    "chain": "base",
    "chain_id": 8453,
    "token_address": "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913",
    "token_decimals": 6,
    "address": "0xYourWallet"
  },
  "amount_base_units": "1000000"
}
```


