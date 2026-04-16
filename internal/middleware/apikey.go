package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireAPIKey returns a Gin middleware that enforces a static API key check.
// The key must be supplied in the X-API-Key request header.
// If key is empty (not configured), the middleware is a no-op — do not deploy
// with an empty key in production.
func RequireAPIKey(key string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if key == "" {
			// No key configured — pass through (dev mode only).
			c.Next()
			return
		}
		if c.GetHeader("X-API-Key") != key {
			// Match the API's error envelope shape so the frontend can display a useful message.
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message":    "unauthorized — X-API-Key header required",
					"error_type": "terminal",
					"error_code": "unauthorized",
				},
			})
			return
		}
		c.Next()
	}
}
