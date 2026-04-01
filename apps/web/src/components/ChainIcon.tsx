import type { ComponentType } from "react";
import NetworkEthereum from "@web3icons/react/icons/networks/NetworkEthereum";
import NetworkBase from "@web3icons/react/icons/networks/NetworkBase";
import NetworkArbitrumOne from "@web3icons/react/icons/networks/NetworkArbitrumOne";
import NetworkOptimism from "@web3icons/react/icons/networks/NetworkOptimism";
import NetworkPolygon from "@web3icons/react/icons/networks/NetworkPolygon";
import NetworkAvalanche from "@web3icons/react/icons/networks/NetworkAvalanche";
import NetworkBinanceSmartChain from "@web3icons/react/icons/networks/NetworkBinanceSmartChain";

const byChainID: Record<number, ComponentType<{ size?: number }>> = {
  1: NetworkEthereum,
  8453: NetworkBase,
  42161: NetworkArbitrumOne,
  10: NetworkOptimism,
  137: NetworkPolygon,
  43114: NetworkAvalanche,
  56: NetworkBinanceSmartChain,
  // Testnet chains reuse mainnet icons
  11155111: NetworkEthereum,   // Sepolia → Ethereum
  84532: NetworkBase,          // Base Sepolia → Base
  421614: NetworkArbitrumOne,  // Arbitrum Sepolia → Arbitrum
  11155420: NetworkOptimism,   // OP Sepolia → Optimism
};

const fallbackByChain: Record<number, string> = {
  1: "#627EEA",
  8453: "#0052FF",
  42161: "#28A0F0",
  10: "#FF0420",
  137: "#8247E5",
  43114: "#E84142",
  56: "#F3BA2F",
  11155111: "#627EEA",
  84532: "#0052FF",
  421614: "#28A0F0",
  11155420: "#FF0420",
};

export function ChainIcon({ chainId, size = 16 }: { chainId: number; size?: number }) {
  const Icon = byChainID[chainId];
  if (Icon) return <Icon size={size} />;
  return (
    <div
      className="rounded-full"
      style={{ width: size, height: size, backgroundColor: fallbackByChain[chainId] ?? "#52525b" }}
    />
  );
}

