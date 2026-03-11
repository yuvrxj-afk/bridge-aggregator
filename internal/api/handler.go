package api

import (
	"net/http"

	"bridge-aggregator/internal/bridges"
	"bridge-aggregator/internal/models"
	"bridge-aggregator/internal/router"

	"github.com/gin-gonic/gin"
)

func QuoteHandler(c *gin.Context) {

	var req models.QuoteRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	adapters := []bridges.BridgeAdapter{
		bridges.AcrossAdapter{},
		bridges.HopAdapter{},
	}

	best, err := router.FindBestRoute(adapters, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, models.RouteResponse{
		BestRoute: best,
	})
}