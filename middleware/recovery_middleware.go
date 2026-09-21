package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"crm-auth-service/helpers"
)

// RecoveryMiddleware gracefully catches unhandled panics in HTTP handlers,
// logs the stack trace server-side, and returns a structured 500 JSON response.
func RecoveryMiddleware(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				stack := string(debug.Stack())
				if log != nil {
					log.Error("Unhandled panic recovered in HTTP handler",
						"panic", r,
						"path", c.Request.URL.Path,
						"method", c.Request.Method,
						"stack", stack,
					)
				}
				helpers.ErrorResponse(c, http.StatusInternalServerError, "An unexpected internal server error occurred.")
				c.Abort()
			}
		}()
		c.Next()
	}
}
