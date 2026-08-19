// Package controllers holds HTTP handlers.
package controllers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"crm-auth-service/helpers"
	"crm-auth-service/models"
	"crm-auth-service/services"
)

// AuthController delegates incoming client HTTP operations to the business services layer.
type AuthController struct {
	authService *services.AuthService
	log         *slog.Logger
}

// NewAuthController constructs a new instance of AuthController.
func NewAuthController(authService *services.AuthService, log *slog.Logger) *AuthController {
	return &AuthController{
		authService: authService,
		log:         log,
	}
}

// Login handles authentication requests (POST /auth/login).
func (ac *AuthController) Login(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "identifier and password are required")
		return
	}

	result, err := ac.authService.Login(c.Request.Context(), req, c.ClientIP())
	if err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, result)
}

// Refresh handles token rotation requests (POST /auth/refresh).
func (ac *AuthController) Refresh(c *gin.Context) {
	var req models.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "refresh_token is required")
		return
	}

	result, err := ac.authService.Refresh(c.Request.Context(), req)
	if err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, result)
}

// --- Module B endpoints ---

// SetPassword handles POST /auth/onboarding/set-password.
func (ac *AuthController) SetPassword(c *gin.Context) {
	var req struct {
		TempToken   string `json:"temp_token" binding:"required"`
		NewPassword string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "temp_token and new_password are required")
		return
	}

	if err := ac.authService.SetPassword(c.Request.Context(), req.TempToken, req.NewPassword); err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, map[string]string{
		"message": "Password updated. Please verify your email and mobile.",
	})
}

// VerifyEmailOTP handles POST /auth/onboarding/verify-email.
func (ac *AuthController) VerifyEmailOTP(c *gin.Context) {
	var req struct {
		TempToken string  `json:"temp_token" binding:"required"`
		OTP       *string `json:"otp"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "temp_token is required")
		return
	}

	if req.OTP == nil {
		// Send OTP
		if err := ac.authService.SendEmailOTP(c.Request.Context(), req.TempToken); err != nil {
			helpers.RespondError(c, err, ac.log)
			return
		}
		helpers.SuccessResponse(c, http.StatusOK, map[string]string{
			"message": "OTP sent to your email.",
		})
		return
	}

	// Verify OTP
	if err := ac.authService.VerifyEmailOTP(c.Request.Context(), req.TempToken, *req.OTP); err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}
	helpers.SuccessResponse(c, http.StatusOK, map[string]string{
		"message": "Email verified successfully.",
	})
}

// VerifyMobileOTP handles POST /auth/onboarding/verify-mobile.
func (ac *AuthController) VerifyMobileOTP(c *gin.Context) {
	var req struct {
		TempToken string  `json:"temp_token" binding:"required"`
		OTP       *string `json:"otp"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "temp_token is required")
		return
	}

	if req.OTP == nil {
		if err := ac.authService.SendMobileOTP(c.Request.Context(), req.TempToken); err != nil {
			helpers.RespondError(c, err, ac.log)
			return
		}
		helpers.SuccessResponse(c, http.StatusOK, map[string]string{
			"message": "OTP sent to your mobile.",
		})
		return
	}

	res, err := ac.authService.VerifyMobileOTP(c.Request.Context(), req.TempToken, *req.OTP)
	if err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}
	helpers.SuccessResponse(c, http.StatusOK, res)
}

// ForgotPassword handles POST /auth/forgot-password.
func (ac *AuthController) ForgotPassword(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "valid email is required")
		return
	}

	if err := ac.authService.ForgotPassword(c.Request.Context(), req.Email); err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, map[string]string{
		"message": "If an account exists with this email, a reset link has been sent.",
	})
}

// ResetPassword handles POST /auth/reset-password.
func (ac *AuthController) ResetPassword(c *gin.Context) {
	var req struct {
		Token       string `json:"token" binding:"required"`
		NewPassword string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "token and new_password are required")
		return
	}

	if err := ac.authService.ResetPassword(c.Request.Context(), req.Token, req.NewPassword); err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, map[string]string{
		"message": "Password updated successfully. Please log in again.",
	})
}

// VerifyMFA handles POST /auth/mfa/verify.
func (ac *AuthController) VerifyMFA(c *gin.Context) {
	var req struct {
		MFAPendingToken string `json:"mfa_pending_token" binding:"required"`
		OTP             string `json:"otp" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "mfa_pending_token and otp are required")
		return
	}

	res, err := ac.authService.VerifyMFA(c.Request.Context(), req.MFAPendingToken, req.OTP)
	if err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}
	helpers.SuccessResponse(c, http.StatusOK, res)
}

// --- SSO & MFA Management Custom Methods (User Added) ---

func (ac *AuthController) SSORedirect(c *gin.Context) {
	provider := c.DefaultQuery("provider", "microsoft")
	url, err := ac.authService.BuildSSORedirectURL(c.Request.Context(), provider)
	if err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}

	c.Redirect(http.StatusFound, url)
}

func (ac *AuthController) SSOCallback(c *gin.Context) {
	provider := c.DefaultQuery("provider", "microsoft")
	code := c.Query("code")
	state := c.Query("state")

	if code == "" || state == "" {
		helpers.RespondError(c, helpers.ErrBadRequest("Code and state are required"), ac.log)
		return
	}

	result, err := ac.authService.SSOCallback(c.Request.Context(), provider, code, state)
	if err != nil {
		if os.Getenv("APP_ENV") != "production" {
			var appErr *helpers.AppError
			if !errors.As(err, &appErr) {
				clientSecret := os.Getenv("AZURE_CLIENT_SECRET")
				sanitizedErrMsg := helpers.SanitizeString(err.Error(), clientSecret, code)
				err = &helpers.AppError{
					Status:  http.StatusInternalServerError,
					Message: fmt.Sprintf("SSO callback failed: %s", sanitizedErrMsg),
				}
			}
		}
		helpers.RespondError(c, err, ac.log)
		return
	}

	frontendURL := os.Getenv("FRONTEND_SSO_REDIRECT_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:3000/sso-success"
	}

	targetURL := fmt.Sprintf("%s?access_token=%s&refresh_token=%s", frontendURL, result.AccessToken, result.RefreshToken)
	c.Redirect(http.StatusFound, targetURL)
}

type MFAEnableRequest struct {
	Method string `json:"method" binding:"required"`
}

func (ac *AuthController) EnableMFA(c *gin.Context) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		helpers.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	userID, ok := userIDVal.(uuid.UUID)
	if !ok {
		helpers.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req MFAEnableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "method is required")
		return
	}

	err := ac.authService.EnableMFA(c.Request.Context(), userID, req.Method)
	if err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, gin.H{"message": "MFA enabled successfully"})
}

func (ac *AuthController) DisableMFA(c *gin.Context) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		helpers.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	userID, ok := userIDVal.(uuid.UUID)
	if !ok {
		helpers.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	err := ac.authService.DisableMFA(c.Request.Context(), userID)
	if err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, gin.H{"message": "MFA disabled successfully"})
}
