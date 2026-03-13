package main

import (
	"log"

	"bridge-aggregator/internal/api"
	"bridge-aggregator/internal/bridges"
	"bridge-aggregator/internal/config"
	"bridge-aggregator/internal/store"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if cfg.DatabaseURL == "" {
		log.Fatalf("database_url must be set in config for execute/status")
	}

	dbStore, err := store.NewStore(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("store: %v", err)
	}

	acrossClient := bridges.NewAcrossClient(cfg.AcrossAPIURL, cfg.AcrossAPIKey)
	stargateClient := bridges.NewStargateClient(cfg.StargateAPIURL, cfg.StargateAPIKey)
	blockdaemonClient := bridges.NewBlockdaemonClient(cfg.BlockdaemonAPIURL, cfg.BlockdaemonAPIKey)

	adapters := []bridges.Adapter{
		bridges.AcrossAdapter{Client: acrossClient},
		bridges.StargateAdapter{Client: stargateClient},
		bridges.BlockdaemonAdapter{Client: blockdaemonClient},
	}

	r := gin.Default()

	r.GET("/health", api.HealthHandler)

	v1 := r.Group("/api/v1")
	{
		v1.POST("/quote", api.QuoteHandler(adapters))
		v1.POST("/execute", api.ExecuteHandler(dbStore, adapters))
		v1.GET("/operations/:id", api.GetOperationHandler(dbStore))
	}

	addr := ":" + cfg.Port
	log.Printf("listening on %s", addr)
	r.Run(addr)
}
