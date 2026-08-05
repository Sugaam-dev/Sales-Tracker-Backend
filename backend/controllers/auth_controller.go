// Package controllers holds HTTP handlers.
package controllers

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

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
