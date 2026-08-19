// Package v1 registers Module A's routes.
package v1

import (
	"github.com/gin-gonic/gin"

	"crm-auth-service/controllers"
	"crm-auth-service/helpers"
	"crm-auth-service/middleware"
)

// RegisterAuthRoutes mounts:
//
//	POST /auth/login
//	POST /auth/refresh
//	GET /auth/sso/redirect
//	GET /auth/sso/callback
//	POST /auth/mfa/enable
//	POST /auth/mfa/disable
func RegisterAuthRoutes(rg *gin.RouterGroup, authController *controllers.AuthController, jwtManager *helpers.JWTManager) {
	auth := rg.Group("/auth")
	auth.POST("/login", authController.Login)
	auth.POST("/refresh", authController.Refresh)

	auth.GET("/sso/redirect", authController.SSORedirect)
	auth.GET("/sso/callback", authController.SSOCallback)

	auth.POST("/mfa/enable", middleware.AuthMiddleware(jwtManager), authController.EnableMFA)
	auth.POST("/mfa/disable", middleware.AuthMiddleware(jwtManager), authController.DisableMFA)
} 