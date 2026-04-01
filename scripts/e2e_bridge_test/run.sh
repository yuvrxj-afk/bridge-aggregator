#!/usr/bin/env bash
# E2E bridge test script — Sepolia → Base Sepolia via Across
#
# Prerequisites:
#   - Foundry installed (cast, https://getfoundry.sh)
#   - curl and jq installed
#   - Backend running: NETWORK=testnet go run ./cmd/server
#
# Required env vars:
#   BRIDGE_TEST_PRIVATE_KEY   — private key of the test wallet (never logged)
#   BRIDGE_TEST_WALLET        — public address of the test wallet
#
# Optional env vars (have defaults):
#   BRIDGE_API_URL            — backend base URL (default: http://localhost:8080)
#   BRIDGE_SRC_CHAIN          — source chain name (default: sepolia)
#   BRIDGE_DST_CHAIN          — destination chain name (default: base-sepolia)
#   BRIDGE_TOKEN              — token symbol (default: USDC)
#   BRIDGE_AMOUNT             — human-readable amount (default: 1)
#   BRIDGE_PROVIDER           — bridge provider to use (default: across)
#   BRIDGE_SRC_RPC            — source chain RPC URL (default: https://rpc.sepolia.org)
#   BRIDGE_DST_RPC            — destination chain RPC URL (default: https://sepolia.base.org)
#   BRIDGE_CONFIRM_TIMEOUT    — seconds to wait for destination receipt (default: 300)

set -euo pipefail

# ── Config ────────────────────────────────────────────────────────────────────

: "${BRIDGE_API_URL:=http://localhost:8080}"
: "${BRIDGE_SRC_CHAIN:=sepolia}"
: "${BRIDGE_DST_CHAIN:=base-sepolia}"
: "${BRIDGE_TOKEN:=USDC}"
: "${BRIDGE_AMOUNT:=1}"
: "${BRIDGE_PROVIDER:=across}"
: "${BRIDGE_SRC_RPC:=https://rpc.sepolia.org}"
: "${BRIDGE_DST_RPC:=https://sepolia.base.org}"
: "${BRIDGE_CONFIRM_TIMEOUT:=300}"

# Required vars
if [[ -z "${BRIDGE_TEST_PRIVATE_KEY:-}" ]]; then
  echo "[ERROR] BRIDGE_TEST_PRIVATE_KEY is not set" >&2
  exit 1
fi
if [[ -z "${BRIDGE_TEST_WALLET:-}" ]]; then
  echo "[ERROR] BRIDGE_TEST_WALLET is not set" >&2
  exit 1
fi

WALLET="${BRIDGE_TEST_WALLET}"

# ── Helpers ───────────────────────────────────────────────────────────────────

step() { echo; echo "──────────────────────────────────────────────"; echo "▶ $*"; }
ok()   { echo "  ✅ $*"; }
fail() { echo "  ❌ $*" >&2; exit 1; }
info() { echo "  · $*"; }

# ── Step 1: Health check ──────────────────────────────────────────────────────

step "1/7  Health check"
HEALTH=$(curl -sf "${BRIDGE_API_URL}/health") || fail "Backend not reachable at ${BRIDGE_API_URL}"
info "$(echo "$HEALTH" | jq -r '.status') (version: $(echo "$HEALTH" | jq -r '.version'))"
ok "Backend is up"

# ── Step 2: Resolve token addresses ──────────────────────────────────────────

step "2/7  Fetch capabilities to resolve token addresses"
CAPS=$(curl -sf "${BRIDGE_API_URL}/api/v1/capabilities") || fail "Could not fetch capabilities"

SRC_TOKEN_ADDR=$(echo "$CAPS" | jq -r --arg chain "$BRIDGE_SRC_CHAIN" --arg sym "$BRIDGE_TOKEN" \
  '.chains[] | select(.name == $chain) | .tokens[] | select(.symbol == $sym) | .address' | head -1)
DST_TOKEN_ADDR=$(echo "$CAPS" | jq -r --arg chain "$BRIDGE_DST_CHAIN" --arg sym "$BRIDGE_TOKEN" \
  '.chains[] | select(.name == $chain) | .tokens[] | select(.symbol == $sym) | .address' | head -1)
SRC_DECIMALS=$(echo "$CAPS" | jq -r --arg chain "$BRIDGE_SRC_CHAIN" --arg sym "$BRIDGE_TOKEN" \
  '.chains[] | select(.name == $chain) | .tokens[] | select(.symbol == $sym) | .decimals' | head -1)

if [[ -z "$SRC_TOKEN_ADDR" || "$SRC_TOKEN_ADDR" == "null" ]]; then
  fail "Token ${BRIDGE_TOKEN} not found on chain ${BRIDGE_SRC_CHAIN}. Check NETWORK=testnet is set on the backend."
fi

# Convert amount to base units (6 decimals for USDC)
AMOUNT_BASE=$(python3 -c "import decimal; print(int(decimal.Decimal('${BRIDGE_AMOUNT}') * 10**${SRC_DECIMALS}))" 2>/dev/null || \
  awk "BEGIN { printf \"%d\n\", ${BRIDGE_AMOUNT} * 10^${SRC_DECIMALS} }")

info "Source:      ${BRIDGE_SRC_CHAIN} / ${BRIDGE_TOKEN} @ ${SRC_TOKEN_ADDR}"
info "Destination: ${BRIDGE_DST_CHAIN} / ${BRIDGE_TOKEN} @ ${DST_TOKEN_ADDR}"
info "Amount:      ${BRIDGE_AMOUNT} ${BRIDGE_TOKEN} (${AMOUNT_BASE} base units)"
info "Wallet:      ${WALLET}"
ok "Token addresses resolved"

# ── Step 3: Get quote ─────────────────────────────────────────────────────────

step "3/7  Fetching quote from ${BRIDGE_API_URL}/api/v1/quote"

QUOTE_REQ=$(jq -n \
  --arg sc  "$BRIDGE_SRC_CHAIN" \
  --arg dc  "$BRIDGE_DST_CHAIN" \
  --arg sa  "$BRIDGE_TOKEN" \
  --arg da  "$BRIDGE_TOKEN" \
  --arg sta "$SRC_TOKEN_ADDR" \
  --arg dta "$DST_TOKEN_ADDR" \
  --argjson std "$SRC_DECIMALS" \
  --arg amt "$AMOUNT_BASE" \
  --arg wal "$WALLET" \
  '{
    source: { chain: $sc, asset: $sa, token_address: $sta, token_decimals: $std, address: $wal },
    destination: { chain: $dc, asset: $da, token_address: $dta, token_decimals: $std, address: $wal },
    amount_base_units: $amt
  }')

QUOTE_RESP=$(curl -sf -X POST \
  -H "Content-Type: application/json" \
  -d "$QUOTE_REQ" \
  "${BRIDGE_API_URL}/api/v1/quote") || fail "Quote request failed"

# Find the route for the requested provider
ROUTE=$(echo "$QUOTE_RESP" | jq --arg p "$BRIDGE_PROVIDER" \
  '[.routes[] | select(.hops[].bridge_id == $p)] | sort_by(-.score) | first')

if [[ -z "$ROUTE" || "$ROUTE" == "null" ]]; then
  info "Available routes:"
  echo "$QUOTE_RESP" | jq '[.routes[].hops[].bridge_id]'
  fail "No route found for provider '${BRIDGE_PROVIDER}'"
fi

ROUTE_ID=$(echo "$ROUTE" | jq -r '.route_id')
ESTIMATED_OUT=$(echo "$ROUTE" | jq -r '.estimated_output_amount')
ESTIMATED_TIME=$(echo "$ROUTE" | jq -r '.estimated_time_seconds')

info "Route ID:     ${ROUTE_ID}"
info "Est. output:  ${ESTIMATED_OUT} base units"
info "Est. time:    ${ESTIMATED_TIME}s"
ok "Quote received"

# ── Step 4: Get step transaction params ───────────────────────────────────────

step "4/7  Building step transaction"

STEP_REQ=$(jq -n \
  --argjson route "$ROUTE" \
  --arg wal "$WALLET" \
  '{
    route: $route,
    hop_index: 0,
    sender_address: $wal,
    receiver_address: $wal
  }')

STEP_RESP=$(curl -sf -X POST \
  -H "Content-Type: application/json" \
  -d "$STEP_REQ" \
  "${BRIDGE_API_URL}/api/v1/route/stepTransaction") || fail "StepTransaction request failed"

BRIDGE_PARAMS=$(echo "$STEP_RESP" | jq -r '.bridge_params')
PROTOCOL=$(echo "$BRIDGE_PARAMS" | jq -r '.protocol')
STEPS=$(echo "$BRIDGE_PARAMS" | jq -c '.steps')

info "Protocol:     ${PROTOCOL}"
info "Steps count:  $(echo "$STEPS" | jq 'length')"
ok "Step transaction built"

# ── Step 5: Execute approve step ──────────────────────────────────────────────

step "5/7  Executing on-chain steps"

STEP_COUNT=$(echo "$STEPS" | jq 'length')
APPROVE_TX_HASH=""
DEPOSIT_TX_HASH=""

for i in $(seq 0 $((STEP_COUNT - 1))); do
  STEP=$(echo "$STEPS" | jq -c ".[$i]")
  STEP_TYPE=$(echo "$STEP" | jq -r '.step_type')
  info "Step $((i+1))/${STEP_COUNT}: ${STEP_TYPE}"

  if [[ "$STEP_TYPE" == "approve" ]]; then
    # Structured approval
    APPROVAL=$(echo "$STEP" | jq -r '.approval // empty')
    if [[ -n "$APPROVAL" && "$APPROVAL" != "null" ]]; then
      TOKEN_CONTRACT=$(echo "$APPROVAL" | jq -r '.token_contract')
      SPENDER=$(echo "$APPROVAL" | jq -r '.spender')
      APPROVE_AMOUNT=$(echo "$APPROVAL" | jq -r '.amount')
      info "  approve(${SPENDER}, ${APPROVE_AMOUNT}) on ${TOKEN_CONTRACT}"
      APPROVE_TX_HASH=$(cast send \
        --private-key "$BRIDGE_TEST_PRIVATE_KEY" \
        --rpc-url "$BRIDGE_SRC_RPC" \
        --json \
        "$TOKEN_CONTRACT" \
        "approve(address,uint256)(bool)" \
        "$SPENDER" \
        "$APPROVE_AMOUNT" | jq -r '.transactionHash') || fail "Approve transaction failed"
      info "  tx: ${APPROVE_TX_HASH}"
      ok "  Approval submitted"
    fi

  elif [[ "$STEP_TYPE" == "deposit" ]]; then
    TX=$(echo "$STEP" | jq -r '.tx // empty')
    if [[ -n "$TX" && "$TX" != "null" ]]; then
      CONTRACT=$(echo "$TX" | jq -r '.contract')
      FUNC=$(echo "$TX" | jq -r '.function')
      PARAMS=$(echo "$TX" | jq -r '.params')
      VALUE=$(echo "$TX" | jq -r '.value // "0"')
      ABI_FRAG=$(echo "$TX" | jq -r '.abi_fragment')

      info "  ${FUNC}(...) on ${CONTRACT}"

      # Build the cast call arguments from params JSON
      # Extract individual params in correct ABI order based on function name
      case "$FUNC" in
        depositV3)
          DEPOSITOR=$(echo "$PARAMS"    | jq -r '.depositor')
          RECIPIENT=$(echo "$PARAMS"    | jq -r '.recipient')
          INPUT_TOKEN=$(echo "$PARAMS"  | jq -r '.inputToken')
          OUTPUT_TOKEN=$(echo "$PARAMS" | jq -r '.outputToken')
          INPUT_AMT=$(echo "$PARAMS"    | jq -r '.inputAmount')
          OUTPUT_AMT=$(echo "$PARAMS"   | jq -r '.outputAmount')
          DST_CHAIN_ID=$(echo "$PARAMS" | jq -r '.destinationChainId')
          EX_RELAYER=$(echo "$PARAMS"   | jq -r '.exclusiveRelayer')
          QUOTE_TS=$(echo "$PARAMS"     | jq -r '.quoteTimestamp')
          FILL_DL=$(echo "$PARAMS"      | jq -r '.fillDeadline')
          EX_DL=$(echo "$PARAMS"        | jq -r '.exclusivityDeadline')
          MSG=$(echo "$PARAMS"          | jq -r '.message // "0x"')

          DEPOSIT_TX_HASH=$(cast send \
            --private-key "$BRIDGE_TEST_PRIVATE_KEY" \
            --rpc-url "$BRIDGE_SRC_RPC" \
            --json \
            "$CONTRACT" \
            "depositV3(address,address,address,address,uint256,uint256,uint256,address,uint32,uint32,uint32,bytes)" \
            "$DEPOSITOR" "$RECIPIENT" "$INPUT_TOKEN" "$OUTPUT_TOKEN" \
            "$INPUT_AMT" "$OUTPUT_AMT" "$DST_CHAIN_ID" "$EX_RELAYER" \
            "$QUOTE_TS" "$FILL_DL" "$EX_DL" "$MSG" | jq -r '.transactionHash') || fail "Deposit transaction failed"
          ;;

        depositETH)
          MIN_GAS=$(echo "$PARAMS"    | jq -r '._minGasLimit // 200000')
          EXTRA=$(echo "$PARAMS"      | jq -r '._extraData // "0x"')
          VALUE_WEI=$(echo "$VALUE")
          DEPOSIT_TX_HASH=$(cast send \
            --private-key "$BRIDGE_TEST_PRIVATE_KEY" \
            --rpc-url "$BRIDGE_SRC_RPC" \
            --value "$VALUE_WEI" \
            --json \
            "$CONTRACT" \
            "depositETH(uint32,bytes)" \
            "$MIN_GAS" "$EXTRA" | jq -r '.transactionHash') || fail "Deposit ETH transaction failed"
          ;;

        depositERC20)
          L1_TOKEN=$(echo "$PARAMS"   | jq -r '._l1Token')
          L2_TOKEN=$(echo "$PARAMS"   | jq -r '._l2Token')
          AMOUNT_P=$(echo "$PARAMS"   | jq -r '._amount')
          MIN_GAS=$(echo "$PARAMS"    | jq -r '._minGasLimit // 200000')
          EXTRA=$(echo "$PARAMS"      | jq -r '._extraData // "0x"')
          DEPOSIT_TX_HASH=$(cast send \
            --private-key "$BRIDGE_TEST_PRIVATE_KEY" \
            --rpc-url "$BRIDGE_SRC_RPC" \
            --json \
            "$CONTRACT" \
            "depositERC20(address,address,uint256,uint32,bytes)" \
            "$L1_TOKEN" "$L2_TOKEN" "$AMOUNT_P" "$MIN_GAS" "$EXTRA" | jq -r '.transactionHash') || fail "DepositERC20 transaction failed"
          ;;

        *)
          fail "Unknown function '${FUNC}' — add a case to this script"
          ;;
      esac

      info "  tx: ${DEPOSIT_TX_HASH}"
      ok "  Deposit submitted"
    fi

  elif [[ "$STEP_TYPE" == "claim" ]]; then
    info "  Claim step (CCTP receiveMessage) requires attestation — skipping in this script"
    info "  After source tx is final, poll Circle Iris API then submit receiveMessage manually"
  fi
done

if [[ -z "$DEPOSIT_TX_HASH" ]]; then
  fail "No deposit transaction was submitted"
fi

# ── Step 6: Wait for source confirmation ─────────────────────────────────────

step "6/7  Waiting for source transaction to be confirmed"
info "Tx hash: ${DEPOSIT_TX_HASH}"
info "RPC:     ${BRIDGE_SRC_RPC}"

SOURCE_RECEIPT=$(cast receipt \
  --rpc-url "$BRIDGE_SRC_RPC" \
  --confirmations 1 \
  "$DEPOSIT_TX_HASH" 2>&1) || fail "Source transaction reverted or not found: ${DEPOSIT_TX_HASH}"

STATUS=$(echo "$SOURCE_RECEIPT" | grep -E "^status" | awk '{print $NF}' || echo "")
if [[ "$STATUS" == "0" ]]; then
  fail "Source transaction reverted (status=0): ${DEPOSIT_TX_HASH}"
fi
info "Status:  confirmed (block $(echo "$SOURCE_RECEIPT" | grep -E "^blockNumber" | awk '{print $NF}' || echo "?"))"
ok "Source transaction confirmed"

echo
echo "  Source tx:   https://sepolia.etherscan.io/tx/${DEPOSIT_TX_HASH}"

# ── Step 7: Poll for destination receipt ─────────────────────────────────────

step "7/7  Polling for destination receipt (timeout: ${BRIDGE_CONFIRM_TIMEOUT}s)"
info "Wallet:   ${WALLET}"
info "Dst RPC:  ${BRIDGE_DST_RPC}"

# For Across: the relayer fills on the destination chain automatically.
# We poll for an incoming ERC-20 transfer matching the estimated output amount.
# For simplicity, we wait for any token balance change on the destination.

DST_BALANCE_BEFORE=$(cast call \
  --rpc-url "$BRIDGE_DST_RPC" \
  "$DST_TOKEN_ADDR" \
  "balanceOf(address)(uint256)" \
  "$WALLET" 2>/dev/null || echo "0")

info "Balance before: ${DST_BALANCE_BEFORE} base units (${BRIDGE_DST_CHAIN})"

ELAPSED=0
POLL_INTERVAL=10
RECEIVED=false

while [[ $ELAPSED -lt $BRIDGE_CONFIRM_TIMEOUT ]]; do
  sleep $POLL_INTERVAL
  ELAPSED=$((ELAPSED + POLL_INTERVAL))

  DST_BALANCE_AFTER=$(cast call \
    --rpc-url "$BRIDGE_DST_RPC" \
    "$DST_TOKEN_ADDR" \
    "balanceOf(address)(uint256)" \
    "$WALLET" 2>/dev/null || echo "0")

  if [[ "$DST_BALANCE_AFTER" -gt "$DST_BALANCE_BEFORE" ]]; then
    DELTA=$((DST_BALANCE_AFTER - DST_BALANCE_BEFORE))
    info "Balance after:  ${DST_BALANCE_AFTER} base units (+${DELTA})"
    RECEIVED=true
    break
  fi

  info "  ${ELAPSED}s elapsed — balance unchanged (${DST_BALANCE_AFTER}), still waiting..."
done

if [[ "$RECEIVED" == "true" ]]; then
  ok "Funds received on ${BRIDGE_DST_CHAIN}!"
  echo
  echo "══════════════════════════════════════════════════"
  echo "  E2E PASS ✅"
  echo "  Provider:   ${BRIDGE_PROVIDER}"
  echo "  Source tx:  ${DEPOSIT_TX_HASH}"
  echo "  Amount in:  ${AMOUNT_BASE} base units"
  echo "  Amount out: +${DELTA} base units on ${BRIDGE_DST_CHAIN}"
  echo "══════════════════════════════════════════════════"
else
  echo
  echo "══════════════════════════════════════════════════"
  echo "  TIMEOUT ⏰"
  echo "  Source tx submitted and confirmed: ${DEPOSIT_TX_HASH}"
  echo "  Destination balance unchanged after ${BRIDGE_CONFIRM_TIMEOUT}s."
  echo "  This may still arrive — check ${BRIDGE_DST_CHAIN} manually."
  echo "══════════════════════════════════════════════════"
  echo
  echo "  Destination wallet: ${WALLET}"
  echo "  Destination chain:  ${BRIDGE_DST_CHAIN} (${BRIDGE_DST_RPC})"
  exit 2
fi
