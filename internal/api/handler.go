package api

import (
	"errors"
	"net/http"

	"bridge-aggregator/internal/bridges"
	"bridge-aggregator/internal/models"
	"bridge-aggregator/internal/router"
	"bridge-aggregator/internal/service"
	"bridge-aggregator/internal/store"

	"github.com/gin-gonic/gin"
)

// QuoteHandler returns a Gin handler that uses the quote service to return routes.
func QuoteHandler(adapters []bridges.Adapter) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req models.QuoteRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error(), nil)
			return
		}

		if req.Source.Chain == "" || req.Source.Asset == "" || req.Destination.Chain == "" || req.Destination.Asset == "" || req.Amount == "" {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, "source.chain, source.asset, destination.chain, destination.asset, and amount are required", nil)
			return
		}

		resp, err := service.Quote(c.Request.Context(), adapters, req)
		if err != nil {
			if errors.Is(err, router.ErrNoRoutes) {
				RespondNoRoutes(c)
				return
			}
			RespondError(c, http.StatusInternalServerError, CodeInternal, err.Error(), nil)
			return
		}

		c.JSON(http.StatusOK, resp)
	}
}

// HealthHandler responds with service status and version.
func HealthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "version": "v1"})
}

// ExecuteHandler returns a Gin handler that creates an operation for a selected route.
// This POC version does NOT send on-chain transactions; it records the operation in Postgres.
func ExecuteHandler(s *store.Store, adapters []bridges.Adapter) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req models.ExecuteRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error(), nil)
			return
		}

		resp, err := service.Execute(c.Request.Context(), s, adapters, req)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrRouteRequired), errors.Is(err, service.ErrRouteHopsEmpty), errors.Is(err, service.ErrUnknownBridgeID):
				RespondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error(), nil)
				return
			}
			RespondError(c, http.StatusInternalServerError, CodeInternal, err.Error(), nil)
			return
		}

		c.JSON(http.StatusOK, resp)
	}
}

// GetOperationHandler returns a Gin handler that fetches an operation by ID.
func GetOperationHandler(s *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, "operation id is required", nil)
			return
		}

		resp, err := service.GetOperation(c.Request.Context(), s, id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, CodeInternal, err.Error(), nil)
			return
		}
		if resp == nil {
			RespondError(c, http.StatusNotFound, CodeInvalidRequest, "operation not found", nil)
			return
		}

		c.JSON(http.StatusOK, resp)
	}
}
