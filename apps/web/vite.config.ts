import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  define: {
    // @solana/web3.js relies on Node globals; shim them for the browser bundle.
    'process.env': {},
    global: 'globalThis',
  },
  server: {
    proxy: {
      // Testnet traffic is routed to the testnet backend (port 8081).
      // Run the testnet backend with: ./scripts/run-testnet.sh
      '/api/testnet': {
        target: 'http://localhost:8081',
        rewrite: (path: string) => path.replace(/^\/api\/testnet/, '/api'),
        changeOrigin: true,
      },
      // Mainnet traffic goes to the default backend (port 8080).
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
