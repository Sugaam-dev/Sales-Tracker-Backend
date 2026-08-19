// Package controllers holds HTTP handlers. AuthController only parses
// requests, calls services.AuthService, and writes the response — no
// business logic and no SQL belong here.
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

type AuthController struct {
	authService *services.AuthService
	log         *slog.Logger
}

func NewAuthController(authService *services.AuthService, log *slog.Logger) *AuthController {
	return &AuthController{authService: authService, log: log}
}

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
					Err:     err,
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

