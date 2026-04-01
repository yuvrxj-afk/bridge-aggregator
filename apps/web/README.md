# Web App Environment

Create a local env file before running the app:

```bash
cp .env.example .env.local
```

Then set values in `.env.local`:

- `VITE_APP_NAME`
- `VITE_WALLETCONNECT_PROJECT_ID`
- `VITE_RPC_MAINNET`
- `VITE_RPC_POLYGON`
- `VITE_RPC_OPTIMISM`
- `VITE_RPC_ARBITRUM`
- `VITE_RPC_BASE`

Custom RPCs are optional but recommended for production reliability.
