// Package v1 registers Module A's routes.
package v1

import (
	"github.com/gin-gonic/gin"

	"crm-auth-service/controllers"
)

// RegisterAuthRoutes mounts:
//
//	POST /auth/login
//	POST /auth/refresh
func RegisterAuthRoutes(rg *gin.RouterGroup, authController *controllers.AuthController) {
	auth := rg.Group("/auth")
	auth.POST("/login", authController.Login)
	auth.POST("/refresh", authController.Refresh)
} 