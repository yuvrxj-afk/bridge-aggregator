package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"bridge-aggregator/internal/api"
	"bridge-aggregator/internal/bridges"
	"bridge-aggregator/internal/config"
	"bridge-aggregator/internal/dex"
	"bridge-aggregator/internal/middleware"
	"bridge-aggregator/internal/service"
	"bridge-aggregator/internal/store"

	"github.com/gin-contrib/cors"
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

	if cfg.Network == "testnet" {
		bridges.RegisterTestnetChains()
		// Switch CCTP attestation URL to sandbox unless the operator has explicitly overridden it.
		if cfg.CCTPAttestationURL == "https://iris-api.circle.com" {
			cfg.CCTPAttestationURL = "https://iris-api-sandbox.circle.com"
		}
		log.Printf("NETWORK=testnet: testnet chain IDs and contract addresses activated; cctp_attestation_url=%s", cfg.CCTPAttestationURL)
	}

	log.Printf("bridge config: across_depositor_set=%t", cfg.AcrossDepositor != "")
	acrossClient := bridges.NewAcrossClient(cfg.AcrossAPIURL, cfg.AcrossAPIKey, cfg.AcrossDepositor)
	stargateClient := bridges.NewStargateClient(cfg.StargateAPIURL, cfg.StargateAPIKey)
	blockdaemonClient := bridges.NewBlockdaemonClient(cfg.BlockdaemonAPIURL, cfg.BlockdaemonAPIKey)

	mayanAdapter := bridges.NewMayanAdapter()
	adapters := []bridges.Adapter{
		bridges.AcrossAdapter{Client: acrossClient},
		bridges.StargateAdapter{Client: stargateClient},
		bridges.CCTPAdapter{},
		bridges.BaseCanonicalAdapter{},
		bridges.OptimismCanonicalAdapter{},
		bridges.ArbitrumCanonicalAdapter{},
		mayanAdapter,
	}

	bridgeClients := &service.BridgeClients{
		Stargate: stargateClient,
		Mayan:    &mayanAdapter,
		Across:   acrossClient,
	}

	// DEX adapters: Uniswap first, then 0x when configured, then Blockdaemon DEX aggregator.
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
	// Add Blockdaemon DEX aggregator if API key is configured
	if cfg.BlockdaemonAPIKey != "" {
		bdDexClient := dex.NewBlockdaemonDEXClient(cfg.BlockdaemonAPIURL, cfg.BlockdaemonAPIKey)
		dexAdapters = append(dexAdapters, dex.NewBlockdaemonDEXAdapter(bdDexClient))
	}

	r := gin.Default()

	// Body size limit — reject requests larger than 1MB before parsing.
	r.Use(func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
		c.Next()
	})

	// CORS — require explicit ALLOWED_ORIGIN in production.
	// Defaults to localhost in dev; never falls back to wildcard.
	allowedOrigin := os.Getenv("ALLOWED_ORIGIN")
	var allowOrigins []string
	if allowedOrigin != "" {
		allowOrigins = strings.Split(allowedOrigin, ",")
	} else {
		if os.Getenv("ENV") == "production" {
			log.Fatal("ALLOWED_ORIGIN must be set in production (ENV=production is set)")
		}
		log.Printf("warning: ALLOWED_ORIGIN not set — defaulting to http://localhost:5173 (dev only)")
		allowOrigins = []string{"http://localhost:5173"}
	}
	r.Use(cors.New(cors.Config{
		AllowOrigins: allowOrigins,
		AllowMethods: []string{"GET", "POST", "PATCH", "OPTIONS"},
		AllowHeaders: []string{"Content-Type", "Accept", "X-API-Key"},
	}))

	// Per-IP rate limiters.
	quoteRL  := middleware.NewRateLimiter(10.0/60, 3)  // 10/min, burst 3
	intentRL := middleware.NewRateLimiter(5.0/60, 2)   // 5/min,  burst 2
	executeRL := middleware.NewRateLimiter(20.0/60, 5) // 20/min, burst 5

	// API key middleware for mutating endpoints.
	apiKeyMW := middleware.RequireAPIKey(cfg.APIKey)
	if cfg.APIKey == "" {
		log.Printf("warning: API_KEY not set — /execute and PATCH /operations/:id/status are unprotected")
	}

	r.GET("/health", api.HealthHandler)

	v1 := r.Group("/api/v1")
	{
		v1.GET("/health/adapters", api.AdapterHealthHandler(adapters, dexAdapters, cfg.Network))
		v1.GET("/capabilities", api.CapabilitiesHandler(adapters, dexAdapters))
		v1.POST("/quote", quoteRL.Limit(), api.QuoteHandler(adapters, dexAdapters))
		v1.POST("/quote/stream", quoteRL.Limit(), api.StreamQuoteHandler(adapters, dexAdapters))
		v1.POST("/execute", executeRL.Limit(), apiKeyMW, api.ExecuteHandler(dbStore, adapters))
		v1.GET("/operations", api.ListOperationsHandler(dbStore))
		v1.GET("/operations/:id", api.GetOperationHandler(dbStore))
		v1.GET("/status/:txHash", api.TransactionStatusHandler(blockdaemonClient))
		v1.GET("/operations/:id/events", api.GetOperationEventsHandler(dbStore))
		v1.PATCH("/operations/:id/status", apiKeyMW, api.PatchOperationStatusHandler(dbStore))
		v1.POST("/dex/quote", quoteRL.Limit(), api.DEXQuoteHandler(dexAdapters))
		v1.POST("/route/stepTransaction", api.StepTransactionHandler(dexAdapters, bridgeClients))
		v1.POST("/route/buildTransaction", api.BuildTransactionHandler(adapters))
		v1.GET("/cctp/attestation/:messageHash", api.CCTPAttestationHandler(cfg.CCTPAttestationURL))
		v1.GET("/cctp/attestation/stream/:messageHash", api.CCTPAttestationStreamHandler(cfg.CCTPAttestationURL))
		v1.POST("/intent/parse", intentRL.Limit(), api.IntentParseHandler(cfg.OpenRouterKey))
	}

	addr := ":" + cfg.Port
	srv := &http.Server{Addr: addr, Handler: r}

	go func() {
		log.Printf("listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down…")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}
	log.Println("server stopped")
}
