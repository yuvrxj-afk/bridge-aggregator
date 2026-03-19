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
	}
	return cfg, nil
}
