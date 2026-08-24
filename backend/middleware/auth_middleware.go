package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"crm-auth-service/helpers"
)

// AuthMiddleware is a Gin middleware that validates the access token from the Authorization header.
func AuthMiddleware(jwtManager *helpers.JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			helpers.ErrorResponse(c, http.StatusUnauthorized, "Authorization header is required")
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			helpers.ErrorResponse(c, http.StatusUnauthorized, "Authorization header must be Bearer token")
			c.Abort()
			return
		}

		claims, err := jwtManager.ValidateAccessToken(parts[1])
		if err != nil {
			helpers.ErrorResponse(c, http.StatusUnauthorized, "Invalid or expired token")
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("role", claims.Role)
		c.Set("email", claims.Email)
		c.Next()
	}
}
