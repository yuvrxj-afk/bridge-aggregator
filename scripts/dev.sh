#!/bin/sh
# dev.sh — start all three dev processes: mainnet API, testnet API, and frontend.
# Usage: ./scripts/dev.sh  OR  bun run dev (from repo root)
set -e

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Load env vars for local dev if present.
# This enables API keys (Uniswap/Across/etc) without manual export.
if [ -f "$ROOT/.env" ]; then
  set -a
  # shellcheck disable=SC1091
  . "$ROOT/.env"
  set +a
fi

require_free_port() {
  PORT="$1"
  if lsof -n -P -iTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "error: port $PORT is already in use."
    echo "hint: stop existing dev servers (or kill the process using the port) and retry."
    lsof -n -P -iTCP:"$PORT" -sTCP:LISTEN || true
    exit 1
  fi
}

cleanup() {
  echo "stopping dev servers..."
  kill "$MAINNET_PID" "$TESTNET_PID" "$WEB_PID" 2>/dev/null
  wait "$MAINNET_PID" "$TESTNET_PID" "$WEB_PID" 2>/dev/null
}
trap cleanup INT TERM EXIT

require_free_port 8080
require_free_port 8081
require_free_port 5173

echo "→ starting mainnet API on :8080"
(cd "$ROOT" && go run ./cmd/server) &
MAINNET_PID=$!

echo "→ starting testnet API on :8081"
(cd "$ROOT" && NETWORK=testnet PORT=8081 go run ./cmd/server) &
TESTNET_PID=$!

echo "→ starting frontend on :5173"
(cd "$ROOT/apps/web" && bun dev) &
WEB_PID=$!

wait "$MAINNET_PID" "$TESTNET_PID" "$WEB_PID"
