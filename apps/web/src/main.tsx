import { Buffer } from "buffer";
// @solana/web3.js needs Buffer in the browser bundle
(globalThis as typeof globalThis & { Buffer: typeof Buffer }).Buffer = Buffer;

import { createRoot } from "react-dom/client";
import { Component, type ReactNode } from "react";

class ErrorBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
  state = { error: null };
  static getDerivedStateFromError(error: Error) { return { error }; }
  render() {
    if (this.state.error) {
      return (
        <div style={{ minHeight: "100vh", background: "#131313", color: "#e5e2e1", display: "flex", alignItems: "center", justifyContent: "center", fontFamily: "monospace", padding: "2rem" }}>
          <div style={{ maxWidth: 480, textAlign: "center" }}>
            <p style={{ color: "#ffb4ab", fontSize: 13, marginBottom: 8, textTransform: "uppercase", letterSpacing: "0.1em" }}>Something went wrong</p>
            <p style={{ color: "#908fa1", fontSize: 12 }}>{(this.state.error as Error).message}</p>
            <button onClick={() => window.location.reload()} style={{ marginTop: 24, padding: "8px 20px", background: "#2a2a2a", border: "1px solid #3a3a3a", color: "#e5e2e1", cursor: "pointer", fontSize: 12 }}>
              Reload
            </button>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}
import { BrowserRouter } from "react-router-dom";
import "./index.css";
import App from "./App.tsx";
import "@rainbow-me/rainbowkit/styles.css";
import "@solana/wallet-adapter-react-ui/styles.css";
import { getDefaultConfig, RainbowKitProvider } from "@rainbow-me/rainbowkit";
import { WagmiProvider, http, fallback } from "wagmi";
import { mainnet, polygon, optimism, arbitrum, base, sepolia, baseSepolia, arbitrumSepolia, optimismSepolia } from "wagmi/chains";
import { QueryClientProvider, QueryClient } from "@tanstack/react-query";
import { SolanaProviders } from "./components/SolanaProviders";

const appName = import.meta.env.VITE_APP_NAME || "TERMINAL.AGG";
const walletConnectProjectId = import.meta.env.VITE_WALLETCONNECT_PROJECT_ID || "YOUR_PROJECT_ID";
const solanaRpc = import.meta.env.VITE_RPC_SOLANA || "https://api.mainnet-beta.solana.com";

function chainTransport(chainDefaultRpc: string, customRpc?: string) {
  if (customRpc) {
    return fallback(
      [
        http(customRpc, { batch: false, timeout: 15_000, retryCount: 1 }),
        http(chainDefaultRpc, { batch: false, timeout: 15_000, retryCount: 1 }),
      ],
      { rank: false },
    );
  }
  return http(chainDefaultRpc, { batch: false, timeout: 15_000, retryCount: 1 });
}

// All chains registered regardless of VITE_NETWORK — enables runtime switching
// between mainnet and testnet without a page reload.
const config = getDefaultConfig({
  appName,
  projectId: walletConnectProjectId,
  chains: [mainnet, polygon, optimism, arbitrum, base, sepolia, baseSepolia, arbitrumSepolia, optimismSepolia],
  transports: {
    [mainnet.id]:          chainTransport(mainnet.rpcUrls.default.http[0],          import.meta.env.VITE_RPC_MAINNET),
    [polygon.id]:          chainTransport(polygon.rpcUrls.default.http[0],          import.meta.env.VITE_RPC_POLYGON),
    [optimism.id]:         chainTransport(optimism.rpcUrls.default.http[0],         import.meta.env.VITE_RPC_OPTIMISM),
    [arbitrum.id]:         chainTransport(arbitrum.rpcUrls.default.http[0],         import.meta.env.VITE_RPC_ARBITRUM),
    [base.id]:             chainTransport(base.rpcUrls.default.http[0],             import.meta.env.VITE_RPC_BASE),
    [sepolia.id]:          chainTransport(sepolia.rpcUrls.default.http[0],          import.meta.env.VITE_RPC_SEPOLIA),
    [baseSepolia.id]:      chainTransport(baseSepolia.rpcUrls.default.http[0],      import.meta.env.VITE_RPC_BASE_SEPOLIA),
    [arbitrumSepolia.id]:  chainTransport(arbitrumSepolia.rpcUrls.default.http[0],  import.meta.env.VITE_RPC_ARBITRUM_SEPOLIA),
    [optimismSepolia.id]:  chainTransport(optimismSepolia.rpcUrls.default.http[0],  import.meta.env.VITE_RPC_OP_SEPOLIA),
  },
  ssr: true,
});
const queryClient = new QueryClient();

createRoot(document.getElementById("root")!).render(
  <ErrorBoundary>
    <WagmiProvider config={config}>
      <QueryClientProvider client={queryClient}>
        <RainbowKitProvider>
          <SolanaProviders endpoint={solanaRpc}>
            <BrowserRouter>
              <App />
            </BrowserRouter>
          </SolanaProviders>
        </RainbowKitProvider>
      </QueryClientProvider>
    </WagmiProvider>
  </ErrorBoundary>,
);
