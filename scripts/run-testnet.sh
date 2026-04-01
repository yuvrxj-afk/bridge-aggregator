#!/usr/bin/env bash
# Run the bridge aggregator server in testnet mode.
# Usage: ./scripts/run-testnet.sh
set -euo pipefail

export NETWORK=testnet
export ACROSS_API_URL=https://testnet.across.to/api
export CCTP_ATTESTATION_URL=https://iris-api-sandbox.circle.com

# Override with local overrides if .env.testnet exists
if [ -f .env.testnet ]; then
  set -a
  source .env.testnet
  set +a
fi

exec go run ./cmd/server/main.go
