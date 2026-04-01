import type { ComponentType } from "react";
import TokenETH from "@web3icons/react/icons/tokens/TokenETH";
import TokenUSDC from "@web3icons/react/icons/tokens/TokenUSDC";
import TokenUSDT from "@web3icons/react/icons/tokens/TokenUSDT";
import TokenDAI from "@web3icons/react/icons/tokens/TokenDAI";
import TokenMATIC from "@web3icons/react/icons/tokens/TokenMATIC";
import TokenPOL from "@web3icons/react/icons/tokens/TokenPOL";
import TokenAVAX from "@web3icons/react/icons/tokens/TokenAVAX";
import TokenBNB from "@web3icons/react/icons/tokens/TokenBNB";
import { TOKEN_COLOR } from "../tokens";

interface Props {
  chainId: number;
  address: string;
  symbol: string;
  size?: number;
}

export function TokenIcon({ chainId, address, symbol, size = 28 }: Props) {
  const color = TOKEN_COLOR[symbol.toUpperCase()] ?? "#3f3f46";
  const initials = symbol.slice(0, 2).toUpperCase();
  const key = symbol.toUpperCase();
  const iconMap: Record<string, ComponentType<{ size?: number }>> = {
    ETH: TokenETH,
    WETH: TokenETH,
    USDC: TokenUSDC,
    USDT: TokenUSDT,
    DAI: TokenDAI,
    MATIC: TokenMATIC,
    POL: TokenPOL,
    AVAX: TokenAVAX,
    BNB: TokenBNB,
  };
  const Icon = iconMap[key];

  return (
    <div
      className="rounded-full flex items-center justify-center shrink-0 overflow-hidden bg-zinc-900"
      style={{ width: size, height: size }}
      title={`${symbol} (${chainId}:${address})`}
    >
      {Icon ? (
        <Icon size={size} />
      ) : (
          <div
            className="rounded-full flex items-center justify-center text-white font-bold shrink-0"
            style={{
              width: size, height: size, fontSize: size * 0.35,
              background: color,
            }}
          >
            {initials}
          </div>
      )}
    </div>
  );
}
