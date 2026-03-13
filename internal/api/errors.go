package api

import (
	"net/http"

	"bridge-aggregator/internal/models"

	"github.com/gin-gonic/gin"
)

// ErrorCode constants for the API error envelope.
const (
	CodeInvalidRequest = "INVALID_REQUEST"
	CodeNoRoutes       = "NO_ROUTES"
	CodeInternal       = "INTERNAL_ERROR"
)

// RespondError sends a JSON error envelope and the given status code.
func RespondError(c *gin.Context, status int, code, message string, details map[string]any) {
	var env models.ErrorEnvelope
	env.Error.Code = code
	env.Error.Message = message
	if details != nil {
		env.Error.Details = details
	}
	c.JSON(status, env)
}

// RespondNoRoutes sends 422 with NO_ROUTES code.
func RespondNoRoutes(c *gin.Context) {
	RespondError(c, http.StatusUnprocessableEntity, CodeNoRoutes, "no available routes for the requested pair", nil)
}
