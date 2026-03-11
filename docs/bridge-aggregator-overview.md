## Bridge Aggregator

### What we are building

We are building a Go service that sits in front of multiple “bridges” (providers that move value or messages between chains/systems) and exposes a **single HTTP API** to:

- **Quote**: given a source, destination, and amount, find possible routes and estimate fees, output amount, and ETA.
- **Execute**: run the selected route by calling the underlying bridge(s).
- **Track status**: provide a unified status view for a multi‑hop operation.

This lets clients integrate once with us instead of integrating with every bridge directly.

### High‑level architecture

- **API layer (`internal/api`)**:
  - `gin` HTTP handlers for:
    - `POST /api/v1/quote`
    - `POST /api/v1/execute`
    - `GET /api/v1/operations/:id`
    - `GET /health`
  - Handles JSON binding, validation, and error responses.

- **Router / Aggregator (`internal/router`)**:
  - Knows all registered bridges and their capabilities.
  - Builds candidate routes from source → destination.
  - Calls bridges to get quotes, scores the routes, and returns the best ones.
  - Orchestrates execution of a chosen route and tracks its status.

- **Bridges (`internal/bridges`)**:
  - Common `Bridge` interface (ID, capabilities, quote, execute, get status).
  - Each real integration implements this interface.

- **Models (`internal/models`)**:
  - Shared domain types: `QuoteRequest`, `Route`, `Hop`, `Operation`, `HopStatus`, etc.

- **Store (optional MVP: in‑memory)**:
  - Persists operations and hop status.
  - Supports idempotency for execute calls.

### Core data flow

- **Quote** (`POST /api/v1/quote`):
  1. Client sends source endpoint, destination endpoint, amount, and preferences.
  2. Router finds which bridges can support that pair.
  3. Calls those bridges for hop quotes (in parallel where possible).
  4. Builds one or more routes and scores them (fee, time, reliability, preferences).
  5. Returns a list of routes with `route_id`, score, fees, ETA, and hops.

- **Execute** (`POST /api/v1/execute`):
  1. Client sends `route_id` (or full route), optional `client_reference_id`, and `idempotency_key`.
  2. Router creates an operation record and executes each hop in order.
  3. Each hop delegates to a specific bridge.
  4. Response returns an `operation_id` and initial hop status.

- **Status** (`GET /api/v1/operations/:id`):
  1. Client queries by `operation_id`.
  2. Service loads the operation and hop data (and may refresh from bridges).
  3. Returns unified status (pending, completed, failed) plus per‑hop details.

### Initial scope vs later improvements

- **Initial version**:
  - Production service with configuration via env vars.
  - Support for at least one production bridge integration and single‑hop routes.
  - Basic observability (structured logs, simple metrics).
  - API key–based access control.

- **Later**:
  - Multi‑hop routing and additional bridge integrations.
  - Better scoring (historical reliability, dynamic weights per client).
  - Webhook callbacks or streaming updates instead of polling.
  - Admin controls to enable/disable bridges and tune routing policies.
