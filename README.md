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
- `docs` – product spec, architecture overview, and **integration plan** (`docs/integration-plan.md`).

### Building and integrations

- **Tech stack:** Go, Gin (HTTP), go-ethereum (blockchain), PostgreSQL, sqlc (DB), Redis (cache / rate limits).
- **Step-by-step plan:** See **`docs/integration-plan.md`** for the phased integration plan (config → models & quote API → DB → Redis → real bridge adapters → execute/status → blockchain if needed → observability). Follow phases in order so the service stays runnable at each step.

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


