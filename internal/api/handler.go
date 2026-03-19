package api

import (
	"database/sql"
	"errors"
	"net/http"

	"bridge-aggregator/internal/bridges"
	"bridge-aggregator/internal/dex"
	"bridge-aggregator/internal/models"
	"bridge-aggregator/internal/router"
	"bridge-aggregator/internal/service"
	"bridge-aggregator/internal/store"

	"github.com/gin-gonic/gin"
)

// QuoteHandler returns a Gin handler that uses the quote service to return routes.
func QuoteHandler(adapters []bridges.Adapter, dexAdapters []dex.Adapter) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req models.QuoteRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error(), nil)
			return
		}

		hasSourceSymbol := req.Source.Chain != "" && req.Source.Asset != ""
		hasSourceAddress := req.Source.ChainID != 0 && req.Source.TokenAddress != ""
		hasDestSymbol := req.Destination.Chain != "" && req.Destination.Asset != ""
		hasDestAddress := req.Destination.ChainID != 0 && req.Destination.TokenAddress != ""
		hasAmount := req.Amount != "" || req.AmountBaseUnits != ""

		if (!hasSourceSymbol && !hasSourceAddress) || (!hasDestSymbol && !hasDestAddress) || !hasAmount {
			RespondError(
				c,
				http.StatusBadRequest,
				CodeInvalidRequest,
				"require (source.chain+source.asset OR source.chain_id+source.token_address), (destination.chain+destination.asset OR destination.chain_id+destination.token_address), and (amount OR amount_base_units)",
				nil,
			)
			return
		}

		resp, err := service.Quote(c.Request.Context(), adapters, dexAdapters, req)
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
func ExecuteHandler(s *store.Store, adapters []bridges.Adapter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if s == nil {
			RespondError(c, http.StatusServiceUnavailable, CodeInternal, "database not configured; set DATABASE_URL to enable execute", nil)
			return
		}
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
		if s == nil {
			RespondError(c, http.StatusServiceUnavailable, CodeInternal, "database not configured; set DATABASE_URL to enable operations", nil)
			return
		}
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

// PatchOperationStatusHandler updates the status (and optionally tx_hash) of an operation.
func PatchOperationStatusHandler(s *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if s == nil {
			RespondError(c, http.StatusServiceUnavailable, CodeInternal, "database not configured; set DATABASE_URL to enable operations", nil)
			return
		}
		id := c.Param("id")
		if id == "" {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, "operation id is required", nil)
			return
		}
		var req models.UpdateOperationStatusRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error(), nil)
			return
		}
		if err := service.UpdateOperationStatus(c.Request.Context(), s, id, req); err != nil {
			switch {
			case errors.Is(err, service.ErrInvalidStatus):
				RespondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error(), nil)
			case errors.Is(err, sql.ErrNoRows):
				RespondError(c, http.StatusNotFound, CodeInvalidRequest, "operation not found", nil)
			default:
				RespondError(c, http.StatusInternalServerError, CodeInternal, err.Error(), nil)
			}
			return
		}
		c.JSON(http.StatusOK, gin.H{"operation_id": id, "status": req.Status})
	}
}

// DEXQuoteHandler returns a Gin handler that tries DEX adapters in order and returns the first successful quote.
func DEXQuoteHandler(adapters []dex.Adapter) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req dex.QuoteRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error(), nil)
			return
		}
		if req.TokenInChainID == 0 || req.TokenOutChainID == 0 || req.TokenIn == "" || req.TokenOut == "" || req.Amount == "" {
			RespondError(
				c,
				http.StatusBadRequest,
				CodeInvalidRequest,
				"tokenInChainId, tokenOutChainId, tokenIn, tokenOut, and amount (base units) are required",
				nil,
			)
			return
		}

		for _, adapter := range adapters {
			if adapter == nil {
				continue
			}
			quote, err := adapter.GetQuote(c.Request.Context(), req)
			if err != nil {
				continue
			}
			c.JSON(http.StatusOK, quote)
			return
		}
		RespondError(c, http.StatusBadRequest, CodeInvalidRequest, "no DEX adapter could return a quote for this request", nil)
	}
}

// StepTransactionHandler populates a single hop with tx data when supported (Uniswap or 0x).
func StepTransactionHandler(dexAdapters []dex.Adapter) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req models.StepTransactionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			RespondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error(), nil)
			return
		}
		resp, err := service.PopulateStepTransaction(c.Request.Context(), dexAdapters, req)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrHopIndexOutOfRange), errors.Is(err, service.ErrHopNotSupported), errors.Is(err, service.ErrPermitSignatureReq):
				RespondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error(), nil)
				return
			}
			RespondError(c, http.StatusInternalServerError, CodeInternal, err.Error(), nil)
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}
