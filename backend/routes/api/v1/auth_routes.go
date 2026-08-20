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
//	POST /auth/onboarding/set-password
//	POST /auth/onboarding/verify-email
//	POST /auth/onboarding/verify-mobile
//	POST /auth/forgot-password
//	POST /auth/reset-password
//	POST /auth/mfa/verify
func RegisterAuthRoutes(rg *gin.RouterGroup, authController *controllers.AuthController) {
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
} 