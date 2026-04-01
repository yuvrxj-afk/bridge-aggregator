# E2E Bridge Test Script

Executes a real on-chain bridge transfer end-to-end: quote → approve → deposit → destination receipt.

## Prerequisites

- [Foundry](https://getfoundry.sh) installed (`cast` must be on PATH)
- `curl` and `jq` installed
- Backend running in testnet mode: `NETWORK=testnet go run ./cmd/server`
- Test wallet funded with testnet USDC and ETH for gas

## Usage

```sh
export BRIDGE_TEST_PRIVATE_KEY="0x..."    # private key — never logged
export BRIDGE_TEST_WALLET="0x..."         # public address

# Run with defaults (1 USDC, Sepolia → Base Sepolia via Across)
bash scripts/e2e_bridge_test/run.sh

# Override any parameter
BRIDGE_AMOUNT=0.5 BRIDGE_PROVIDER=across bash scripts/e2e_bridge_test/run.sh
```

## Configuration

| Variable                | Default                        | Description                                      |
|-------------------------|--------------------------------|--------------------------------------------------|
| `BRIDGE_TEST_PRIVATE_KEY` | *required*                   | Wallet private key (never logged)                |
| `BRIDGE_TEST_WALLET`    | *required*                     | Wallet public address                            |
| `BRIDGE_API_URL`        | `http://localhost:8080`        | Backend base URL                                 |
| `BRIDGE_SRC_CHAIN`      | `sepolia`                      | Source chain name                                |
| `BRIDGE_DST_CHAIN`      | `base-sepolia`                 | Destination chain name                           |
| `BRIDGE_TOKEN`          | `USDC`                         | Token symbol                                     |
| `BRIDGE_AMOUNT`         | `1`                            | Human-readable amount                            |
| `BRIDGE_PROVIDER`       | `across`                       | Bridge provider ID                               |
| `BRIDGE_SRC_RPC`        | `https://rpc.sepolia.org`      | Source chain RPC URL                             |
| `BRIDGE_DST_RPC`        | `https://sepolia.base.org`     | Destination chain RPC URL                        |
| `BRIDGE_CONFIRM_TIMEOUT`| `300`                          | Seconds to wait for destination receipt          |

## Supported providers

| Provider | Status           |
|----------|------------------|
| `across` | ✅ Confirmed working end-to-end |
| `cctp`   | ⏳ Source step only — claim step not yet scripted |

## Exit codes

- `0` — funds confirmed received on destination chain
- `1` — script error (tx reverted, API unreachable, bad config)
- `2` — timeout: source tx confirmed but destination balance unchanged within timeout window
