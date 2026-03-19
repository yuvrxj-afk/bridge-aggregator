package main

import (
	"log"

	"bridge-aggregator/internal/api"
	"bridge-aggregator/internal/bridges"
	"bridge-aggregator/internal/config"
	"bridge-aggregator/internal/dex"
	"bridge-aggregator/internal/store"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	log.Printf("dex config: uniswap_api_key=%t uniswap_swapper_wallet_set=%t zeroex_api_key=%t zeroex_taker_set=%t",
		cfg.UniswapAPIKey != "",
		cfg.UniswapSwapperWallet != "",
		cfg.ZeroExAPIKey != "",
		cfg.ZeroExTaker != "",
	)
	log.Printf("dex config: oneinch_api_key=%t oneinch_swapper_set=%t oneinch_api_version=%s",
		cfg.OneInchAPIKey != "",
		cfg.OneInchSwapper != "",
		cfg.OneInchAPIVersion,
	)

	var dbStore *store.Store
	if cfg.DatabaseURL != "" {
		dbStore, err = store.NewStore(cfg.DatabaseURL)
		if err != nil {
			log.Fatalf("store: %v", err)
		}
	} else {
		log.Printf("warning: DATABASE_URL not set — /execute and /operations endpoints will return 503")
	}

	log.Printf("bridge config: across_depositor_set=%t", cfg.AcrossDepositor != "")
	acrossClient := bridges.NewAcrossClient(cfg.AcrossAPIURL, cfg.AcrossAPIKey, cfg.AcrossDepositor)
	stargateClient := bridges.NewStargateClient(cfg.StargateAPIURL, cfg.StargateAPIKey)
	blockdaemonClient := bridges.NewBlockdaemonClient(cfg.BlockdaemonAPIURL, cfg.BlockdaemonAPIKey)

	adapters := []bridges.Adapter{
		bridges.AcrossAdapter{Client: acrossClient},
		bridges.StargateAdapter{Client: stargateClient},
		bridges.BlockdaemonAdapter{Client: blockdaemonClient},
		bridges.CCTPAdapter{},
		bridges.BaseCanonicalAdapter{},
		bridges.OptimismCanonicalAdapter{},
		bridges.ArbitrumCanonicalAdapter{},
		bridges.NewMayanAdapter(), // Mayan Finance (Wormhole-based, real fees)
	}

	// DEX adapters: Uniswap first, then 0x when configured.
	dexAdapters := []dex.Adapter{
		dex.NewUniswapTradingAdapter(
			cfg.UniswapAPIURL,
			cfg.UniswapAPIKey,
			cfg.UniswapSwapperWallet,
		),
	}
	if cfg.ZeroExAPIKey != "" {
		taker := cfg.ZeroExTaker
		if taker == "" {
			taker = cfg.UniswapSwapperWallet
		}
		dexAdapters = append(dexAdapters, dex.NewZeroExAdapter(cfg.ZeroExAPIURL, cfg.ZeroExAPIKey, taker))
	}
	if cfg.OneInchAPIKey != "" {
		swapper := cfg.OneInchSwapper
		if swapper == "" {
			swapper = cfg.UniswapSwapperWallet
		}
		dexAdapters = append(dexAdapters, dex.NewOneInchAdapter(cfg.OneInchAPIURL, cfg.OneInchAPIKey, cfg.OneInchAPIVersion, swapper))
	}

	r := gin.Default()

	r.GET("/health", api.HealthHandler)

	v1 := r.Group("/api/v1")
	{
		v1.POST("/quote", api.QuoteHandler(adapters, dexAdapters))
		v1.POST("/execute", api.ExecuteHandler(dbStore, adapters))
		v1.GET("/operations/:id", api.GetOperationHandler(dbStore))
		v1.PATCH("/operations/:id/status", api.PatchOperationStatusHandler(dbStore))
		v1.POST("/dex/quote", api.DEXQuoteHandler(dexAdapters))
		v1.POST("/route/stepTransaction", api.StepTransactionHandler(dexAdapters))
	}

	addr := ":" + cfg.Port
	log.Printf("listening on %s", addr)
	r.Run(addr)
}
