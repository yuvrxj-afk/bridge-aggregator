package config

import (
	"errors"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Port        string
	DatabaseURL string
	RPCURL      string

	// Network selects mainnet vs testnet mode. "mainnet" (default) or "testnet".
	// Set via NETWORK env var. Testnet mode activates testnet chain IDs, contract
	// addresses, and adapter API URLs without touching any mainnet config.
	Network string

	// CCTPAttestationURL is the Circle Iris API for CCTP attestations.
	// Defaults to mainnet; override to sandbox URL in testnet mode.
	CCTPAttestationURL string

	// Testnet RPC endpoints — only used when NETWORK=testnet.
	// Defaults are public free-tier endpoints; override with Alchemy/Infura for reliability.
	SepoliaRPCURL        string
	BaseSepoliaRPCURL    string
	ArbitrumSepoliaRPCURL string
	OPSepoliaRPCURL      string

	AcrossAPIURL      string
	AcrossAPIKey      string
	AcrossDepositor   string // wallet address used as depositor in Across quotes

	StargateAPIURL string
	StargateAPIKey string

	BlockdaemonAPIURL string
	BlockdaemonAPIKey string

	UniswapAPIURL        string
	UniswapAPIKey        string
	UniswapSwapperWallet string

	ZeroExAPIURL string
	ZeroExAPIKey string
	ZeroExTaker  string // taker address for 0x allowance-holder quote; falls back to UniswapSwapperWallet if empty

	OneInchAPIURL     string
	OneInchAPIKey     string
	OneInchAPIVersion string // e.g. "v6.1"
	OneInchSwapper    string // wallet used for swap tx building (from=)

	OpenRouterKey string // OpenRouter API key for intent parsing (optional)

	// APIKey is a static shared secret for mutating API endpoints (/execute, PATCH /operations/:id/status).
	// Set via API_KEY env var. If empty, mutating endpoints are unprotected — do not run in production without this.
	APIKey string
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./internal/config")
	viper.AddConfigPath("./config")
	viper.AddConfigPath(".")

	// Allow env vars like ZEROEX_API_KEY to override config keys like zeroex_api_key.
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	viper.SetDefault("port", "8080")
	viper.SetDefault("database_url", "")
	viper.SetDefault("rpc_url", "")
	viper.SetDefault("network", "mainnet")
	viper.SetDefault("cctp_attestation_url", "https://iris-api.circle.com")
	viper.SetDefault("sepolia_rpc_url", "https://rpc.sepolia.org")
	viper.SetDefault("base_sepolia_rpc_url", "https://sepolia.base.org")
	viper.SetDefault("arbitrum_sepolia_rpc_url", "https://sepolia-rollup.arbitrum.io/rpc")
	viper.SetDefault("op_sepolia_rpc_url", "https://sepolia.optimism.io")
	viper.SetDefault("across_api_url", "https://app.across.to/api")
	viper.SetDefault("across_api_key", "")
	viper.SetDefault("across_depositor", "")
	viper.SetDefault("stargate_api_url", "https://transfer.layerzero-api.com/v1")
	viper.SetDefault("stargate_api_key", "")
	viper.SetDefault("blockdaemon_api_url", "https://svc.blockdaemon.com")
	viper.SetDefault("blockdaemon_api_key", "")
	viper.SetDefault("uniswap_api_url", "https://trade-api.gateway.uniswap.org/v1")
	viper.SetDefault("uniswap_api_key", "")
	viper.SetDefault("uniswap_swapper_wallet", "")
	viper.SetDefault("zeroex_api_url", "https://api.0x.org")
	viper.SetDefault("zeroex_api_key", "")
	viper.SetDefault("zeroex_taker", "")
	viper.SetDefault("oneinch_api_url", "https://api.1inch.com")
	viper.SetDefault("oneinch_api_key", "")
	viper.SetDefault("oneinch_api_version", "v6.1")
	viper.SetDefault("oneinch_swapper", "")
	viper.SetDefault("openrouter_key", "")
	viper.SetDefault("api_key", "")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, err
		}
	}

	cfg := &Config{
		Port:        viper.GetString("port"),
		DatabaseURL: viper.GetString("database_url"),
		RPCURL:      viper.GetString("rpc_url"),
		Network:             viper.GetString("network"),
		CCTPAttestationURL:  viper.GetString("cctp_attestation_url"),
		SepoliaRPCURL:         viper.GetString("sepolia_rpc_url"),
		BaseSepoliaRPCURL:     viper.GetString("base_sepolia_rpc_url"),
		ArbitrumSepoliaRPCURL: viper.GetString("arbitrum_sepolia_rpc_url"),
		OPSepoliaRPCURL:       viper.GetString("op_sepolia_rpc_url"),

		AcrossAPIURL:    viper.GetString("across_api_url"),
		AcrossAPIKey:    viper.GetString("across_api_key"),
		AcrossDepositor: viper.GetString("across_depositor"),
		StargateAPIURL:    viper.GetString("stargate_api_url"),
		StargateAPIKey:     viper.GetString("stargate_api_key"),
		BlockdaemonAPIURL:  viper.GetString("blockdaemon_api_url"),
		BlockdaemonAPIKey:  viper.GetString("blockdaemon_api_key"),
		UniswapAPIURL:        viper.GetString("uniswap_api_url"),
		UniswapAPIKey:        viper.GetString("uniswap_api_key"),
		UniswapSwapperWallet: viper.GetString("uniswap_swapper_wallet"),
		ZeroExAPIURL:         viper.GetString("zeroex_api_url"),
		ZeroExAPIKey:         viper.GetString("zeroex_api_key"),
		ZeroExTaker:          viper.GetString("zeroex_taker"),
		OneInchAPIURL:        viper.GetString("oneinch_api_url"),
		OneInchAPIKey:        viper.GetString("oneinch_api_key"),
		OneInchAPIVersion:    viper.GetString("oneinch_api_version"),
		OneInchSwapper:       viper.GetString("oneinch_swapper"),
		OpenRouterKey:        viper.GetString("openrouter_key"),
		APIKey:               viper.GetString("api_key"),
	}
	return cfg, nil
}
