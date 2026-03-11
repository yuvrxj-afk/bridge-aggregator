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
- `docs` – product spec and high‑level architecture docs.

### Running locally

Prerequisites:

- Go 1.21+ installed.

Run the server:

```bash
go run ./cmd/server
```

By default the HTTP server listens on `:8080`.

Health check:

```bash
curl http://localhost:8080/health
```

### Key HTTP endpoints

- `GET /health` – service health and version.
- `POST /api/v1/quote` – compute and return one or more bridge routes with quotes.
- `POST /api/v1/execute` – execute a selected route and create an operation.
- `GET /api/v1/operations/:id` – fetch consolidated operation and hop status.


