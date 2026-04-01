#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required for smoke checks"
  exit 1
fi

post_json() {
  local path="$1"
  local body="$2"
  curl -sS -X POST "${BASE_URL}${path}" \
    -H "Content-Type: application/json" \
    --data "${body}"
}

check_routes_non_empty() {
  local name="$1"
  local body="$2"
  local resp
  resp="$(post_json "/api/v1/quote" "${body}")"
  local count
  count="$(jq '.routes | length' <<<"${resp}")"
  if [[ "${count}" == "0" ]]; then
    echo "FAIL: ${name} returned no routes"
    echo "${resp}" | jq .
    exit 1
  fi
  echo "PASS: ${name} routes=${count}"
}

echo "Running Phase B smoke checks against ${BASE_URL}"

check_routes_non_empty "same-chain swap" '{
  "source": {"chain_id": 8453, "chain": "base", "asset": "USDC", "token_address": "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913", "token_decimals": 6},
  "destination": {"chain_id": 8453, "chain": "base", "asset": "WETH", "token_address": "0x4200000000000000000000000000000000000006", "token_decimals": 18},
  "amount_base_units": "1000000"
}'

check_routes_non_empty "swap->bridge composition" '{
  "source": {"chain_id": 8453, "chain": "base", "asset": "USDC", "token_address": "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913", "token_decimals": 6},
  "destination": {"chain_id": 42161, "chain": "arbitrum", "asset": "USDC", "token_address": "0xaf88d065e77c8cC2239327C5EDb3A432268e5831", "token_decimals": 6},
  "amount_base_units": "1000000",
  "metadata": {"source_swap_token_out_address": "0x4200000000000000000000000000000000000006", "source_swap_token_out_decimals": 18}
}'

check_routes_non_empty "bridge->swap composition" '{
  "source": {"chain_id": 42161, "chain": "arbitrum", "asset": "USDC", "token_address": "0xaf88d065e77c8cC2239327C5EDb3A432268e5831", "token_decimals": 6},
  "destination": {"chain_id": 8453, "chain": "base", "asset": "WETH", "token_address": "0x4200000000000000000000000000000000000006", "token_decimals": 18},
  "amount_base_units": "1000000"
}'

check_routes_non_empty "swap->bridge->swap composition" '{
  "source": {"chain_id": 8453, "chain": "base", "asset": "USDC", "token_address": "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913", "token_decimals": 6},
  "destination": {"chain_id": 42161, "chain": "arbitrum", "asset": "WETH", "token_address": "0x82aF49447D8a07e3bd95BD0d56f35241523fBab1", "token_decimals": 18},
  "amount_base_units": "1000000"
}'

check_routes_non_empty "direct across bridge" '{
  "source": {"chain_id": 42161, "chain": "arbitrum", "asset": "USDC", "token_address": "0xaf88d065e77c8cC2239327C5EDb3A432268e5831", "token_decimals": 6},
  "destination": {"chain_id": 8453, "chain": "base", "asset": "USDC", "token_address": "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913", "token_decimals": 6},
  "amount_base_units": "1000000"
}'

check_routes_non_empty "CCTP async path" '{
  "source": {"chain_id": 42161, "chain": "arbitrum", "asset": "USDC", "token_address": "0xaf88d065e77c8cC2239327C5EDb3A432268e5831", "token_decimals": 6},
  "destination": {"chain_id": 8453, "chain": "base", "asset": "USDC", "token_address": "0x833589fCD6eDb6E08f4c7C32D4f71b54bDa02913", "token_decimals": 6},
  "amount_base_units": "5000000"
}'

health="$(curl -sS "${BASE_URL}/api/v1/health/adapters")"
echo "${health}" | jq .
if [[ "$(jq -r '.adapters | length' <<<"${health}")" == "0" ]]; then
  echo "FAIL: /api/v1/health/adapters returned no adapters"
  exit 1
fi
echo "PASS: adapter health endpoint"

echo "All Phase B smoke checks completed"
