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

// RespondErrorTyped sends a JSON error envelope with structured error classification.
// errType is one of models.ErrorType* ("retryable", "user_action", "requote", "terminal").
// errCode is a machine-readable sub-type (e.g. "no_routes", "timeout", "invalid_route").
func RespondErrorTyped(c *gin.Context, status int, code, message, errType, errCode string, details map[string]any) {
	var env models.ErrorEnvelope
	env.Error.Code = code
	env.Error.Message = message
	env.Error.ErrorType = errType
	env.Error.ErrorCode = errCode
	if details != nil {
		env.Error.Details = details
	}
	c.JSON(status, env)
}

// RespondNoRoutes sends 422 with NO_ROUTES code, classified as requote.
func RespondNoRoutes(c *gin.Context) {
	RespondErrorTyped(c, http.StatusUnprocessableEntity, CodeNoRoutes,
		"no available routes for the requested pair",
		models.ErrorTypeRequote, "no_routes", nil)
}
