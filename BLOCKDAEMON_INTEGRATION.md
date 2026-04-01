# Blockdaemon DeFi API Integration

## Overview
This document describes the integration of Blockdaemon's DeFi API into the bridge aggregator platform.

## What Was Integrated

### 1. DEX Aggregation (`blockdaemon_dex`)
**Purpose**: Aggregates same-chain swap quotes from multiple DEXs

**Endpoint**: `GET /defi/v1/dex/quotes`

**Supported DEXs**:
- UniswapV2, UniswapV3
- SushiswapV2
- BalancerV2
- CurveV2
- Kyberswap
- 0x

**Files Created**:
- `internal/dex/blockdaemon_dex.go` - Adapter implementation
- `internal/dex/blockdaemon_dex_client.go` - HTTP client

**How It Works**:
1. Receives same-chain swap request (tokenIn → tokenOut)
2. Calls Blockdaemon API with `path` parameter (comma-separated addresses)
3. Returns best quote from aggregated DEX results
4. Competes alongside Uniswap Trading API, 0x, and 1inch adapters

### 2. Transaction Status Tracking
**Purpose**: Track cross-chain transaction progress for user experience

**Endpoint**: `GET /api/v1/status/:txHash`

**Response**:
```json
{
  "txHash": "0x...",
  "status": "pending|completed|failed",
  "bridgeUsed": "Stargate",
  "progress": 45,
  "timestamp": 1234567890
}
```

**Use Case**: Frontend can poll this endpoint to show "Your bridge is 45% complete"

**Files Modified**:
- `internal/bridges/blockdaemon_client.go` - Added `GetTransactionStatus` method
- `internal/api/handler.go` - Added `TransactionStatusHandler`
- `cmd/server/main.go` - Registered route

## Configuration

The integration reuses existing Blockdaemon configuration:

```yaml
# internal/config/config.yaml
blockdaemon_api_url: https://svc.blockdaemon.com
blockdaemon_api_key: "REPLACE_WITH_BLOCKDAEMON_API_KEY"
```

## Health Check Results

```json
{
  "service": "blockdaemon",
  "kind": "bridge",
  "status": "healthy",
  "latency_ms": 4342
}
{
  "service": "blockdaemon_dex",
  "kind": "dex",
  "status": "healthy",
  "latency_ms": 968
}
```

## API Examples

### DEX Quote (Same-Chain Swap)
```bash
curl 'https://svc.blockdaemon.com/defi/v1/dex/quotes?chainId=1&path=0xdac17f958d2ee523a2206206994597c13d831ec7,0x6B175474E89094C44Da98b954EedeAC495271d0F&amountIn=1000000' \
  -H 'X-API-Key: YOUR_KEY'
```

**Response**: Returns quotes from BalancerV2, CurveV2, UniswapV3, etc. sorted by best output

### Bridge Quote (Cross-Chain)
```bash
curl 'https://svc.blockdaemon.com/defi/v1/bridge/quotes?srcChainId=1&dstChainId=10&srcTokenSymbol=USDC&dstTokenSymbol=USDC&amountIn=10000' \
  -H 'X-API-Key: YOUR_KEY'
```

### Transaction Status
```bash
curl 'http://localhost:8080/api/v1/status/0x1234567890abcdef'
```

## Product Positioning

### Why Keep Multiple Integrations?

We are a **Meta-Aggregator** that:

1. **Competes Aggregators Against Each Other**
   - Blockdaemon's best quote vs direct Uniswap API vs 0x vs 1inch
   - Example: Blockdaemon quotes UniswapV3 at 100 USDC, but direct Uniswap API gives 102 USDC (fresher data, different routing)

2. **Unique Route Compositions**
   - **Swap→Bridge→Swap** (3-hop routes) for arbitrary token pairs
   - Example: PEPE (Ethereum) → USDC → Bridge → ARB (Arbitrum)
   - This unlocks ANY token to ANY token across chains

3. **Specialized Bridge Features**
   - **Across**: Fastest bridge with intent-based architecture
   - **Mayan**: Solana integration
   - **CCTP**: Official Circle USDC bridge (lowest slippage)

4. **DEX Feature Differentiation**
   - **0x**: Allowance-holder quotes (gas-efficient)
   - **Uniswap Trading API**: Permit2, advanced routing, private pools
   - **1inch**: Best prices for small trades, limit orders
   - **Blockdaemon DEX**: Aggregates 7+ DEXs in one call

5. **Fallback & Reliability**
   - If Blockdaemon API is down → 7+ other options still work
   - Rate limits on one provider → Others continue serving

6. **Cost Optimization**
   - Direct APIs might be cheaper than aggregator fees
   - We can A/B test different providers

### Value Proposition

**We are**: A smart routing engine that aggregates aggregators + direct integrations

**We offer**: Best prices through competition, unique route compositions, specialized features

**We differentiate**: Meta-aggregation with advanced multi-hop routing logic

## Future Enhancements

### 3. Lending/Borrowing Aggregation (Not Yet Integrated)

Blockdaemon also provides a lending/borrowing aggregator:

**Endpoint**: `GET /defi/v1/lendborrow/getpools`

**Example**:
```bash
curl 'https://svc.blockdaemon.com/defi/v1/lendborrow/getpools?lendborrowId=1200&assets=USDC&useCustomVaults=true' \
  -H 'Authorization: Bearer YOUR_KEY'
```

**Use Case**: Compare lending rates across Aave, Compound, etc.

**Potential Integration**:
- New endpoint: `GET /api/v1/lending/rates`
- Compare APYs across multiple protocols
- Help users optimize yield

## Testing

### Manual Testing

1. **Start server**:
   ```bash
   bun run dev
   # or
   go run ./cmd/server
   ```

2. **Check health**:
   ```bash
   curl http://localhost:8080/api/v1/health/adapters
   ```

3. **Test DEX quote** (same-chain):
   ```bash
   curl -X POST http://localhost:8080/api/v1/dex/quote \
     -H 'Content-Type: application/json' \
     -d '{
       "tokenInChainId": 1,
       "tokenOutChainId": 1,
       "tokenIn": "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
       "tokenOut": "0xdac17f958d2ee523a2206206994597c13d831ec7",
       "amount": "1000000"
     }'
   ```

4. **Test cross-chain bridge quote**:
   ```bash
   curl -X POST http://localhost:8080/api/v1/quote \
     -H 'Content-Type: application/json' \
     -d '{
       "source": {
         "chainId": 1,
         "chain": "ethereum",
         "asset": "USDC",
         "tokenAddress": "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
         "tokenDecimals": 6
       },
       "destination": {
         "chainId": 10,
         "chain": "optimism",
         "asset": "USDC",
         "tokenAddress": "0x7F5c764cBc14f9669B88837ca1490cCa17c31607",
         "tokenDecimals": 6
       },
       "amountBaseUnits": "1000000"
     }'
   ```

## Notes

- Blockdaemon DEX endpoint sometimes returns bridge quotes mixed in (e.g., SquidRouterV2)
- Health check uses Ethereum mainnet (chain 1) for best compatibility
- The `/dex/quotes` endpoint uses `path` parameter (comma-separated addresses), not separate `tokenIn`/`tokenOut`
- Status tracking requires a valid transaction hash from a Blockdaemon-executed bridge

## References

- [Blockdaemon DeFi API Overview](https://docs.blockdaemon.com/docs/defi-api-overview)
- [Local Swap Documentation](https://docs.blockdaemon.com/docs/defi-api-execute-a-local-swap)
- [Cross-Chain Swap Documentation](https://docs.blockdaemon.com/docs/defi-api-execute-a-cross-chain-swap)
