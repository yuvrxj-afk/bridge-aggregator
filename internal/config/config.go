package config

import (
	"errors"

	"github.com/spf13/viper"
)

type Config struct {
	Port        string
	DatabaseURL string
	RPCURL      string

	AcrossAPIURL string
	AcrossAPIKey string

	StargateAPIURL string
	StargateAPIKey string

	BlockdaemonAPIURL string
	BlockdaemonAPIKey string
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./internal/config")
	viper.AddConfigPath("./config")
	viper.AddConfigPath(".")

	viper.SetDefault("port", "8080")
	viper.SetDefault("database_url", "")
	viper.SetDefault("rpc_url", "")
	viper.SetDefault("across_api_url", "https://app.across.to/api")
	viper.SetDefault("across_api_key", "")
	viper.SetDefault("stargate_api_url", "https://transfer.layerzero-api.com/v1")
	viper.SetDefault("stargate_api_key", "")
	viper.SetDefault("blockdaemon_api_url", "https://svc.blockdaemon.com")
	viper.SetDefault("blockdaemon_api_key", "")
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

		AcrossAPIURL: viper.GetString("across_api_url"),
		AcrossAPIKey: viper.GetString("across_api_key"),
		StargateAPIURL: viper.GetString("stargate_api_url"),
		StargateAPIKey:    viper.GetString("stargate_api_key"),
		BlockdaemonAPIURL: viper.GetString("blockdaemon_api_url"),
		BlockdaemonAPIKey: viper.GetString("blockdaemon_api_key"),
	}
	return cfg, nil
}
