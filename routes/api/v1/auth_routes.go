// Package v1 registers Module A's routes and custom auth features.
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
//	POST /auth/onboarding/set-password
//	POST /auth/onboarding/verify-email
//	POST /auth/onboarding/verify-mobile
//	POST /auth/forgot-password
//	POST /auth/reset-password
//	POST /auth/mfa/verify
//	GET /auth/sso/redirect
//	GET /auth/sso/callback
//	POST /auth/mfa/enable
//	POST /auth/mfa/disable
func RegisterAuthRoutes(rg *gin.RouterGroup, authController *controllers.AuthController, jwtManager *helpers.JWTManager) {
	auth := rg.Group("/auth")
	auth.POST("/login", authController.Login)
	auth.POST("/refresh", authController.Refresh)

	// Module B: Onboarding & Password Reset
	onboarding := auth.Group("/onboarding")
	onboarding.POST("/set-password", authController.SetPassword)
	onboarding.POST("/verify-email", authController.VerifyEmailOTP)
	onboarding.POST("/verify-mobile", authController.VerifyMobileOTP)

	auth.POST("/forgot-password", authController.ForgotPassword)
	auth.POST("/reset-password", authController.ResetPassword)
	auth.POST("/mfa/verify", authController.VerifyMFA)

	// SSO & MFA Management (User Added)
	auth.GET("/sso/redirect", authController.SSORedirect)
	auth.GET("/sso/callback", authController.SSOCallback)
	auth.POST("/mfa/enable", middleware.AuthMiddleware(jwtManager), authController.EnableMFA)
	auth.POST("/mfa/disable", middleware.AuthMiddleware(jwtManager), authController.DisableMFA)
}