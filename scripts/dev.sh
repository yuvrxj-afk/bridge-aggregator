#!/bin/sh
# dev.sh — start all three dev processes: mainnet API, testnet API, and frontend.
# Usage: ./scripts/dev.sh  OR  bun run dev (from repo root)
set -e

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

cleanup() {
  echo "stopping dev servers..."
  kill "$MAINNET_PID" "$TESTNET_PID" "$WEB_PID" 2>/dev/null
  wait "$MAINNET_PID" "$TESTNET_PID" "$WEB_PID" 2>/dev/null
}
trap cleanup INT TERM EXIT

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
