// Package controllers holds HTTP handlers.
package controllers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"crm-auth-service/helpers"
	"crm-auth-service/middleware"
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

func (ac *AuthController) Logout(c *gin.Context) {
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

	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "refresh_token is required")
		return
	}

	err := ac.authService.Logout(c.Request.Context(), req.RefreshToken, userID)
	if err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, gin.H{"message": "Logged out successfully"})
}

func (ac *AuthController) CreateUser(c *gin.Context) {
	var req struct {
		Name      string `json:"name" binding:"required"`
		Email     string `json:"email" binding:"required,email"`
		Mobile    string `json:"mobile"`
		Password  string `json:"password" binding:"required"`
		Role      string `json:"role" binding:"required"`
		ManagerID any    `json:"manager_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "Missing or invalid payload fields")
		return
	}

	if !models.IsValidRole(req.Role) {
		helpers.ErrorResponse(c, http.StatusBadRequest, "Invalid role value")
		return
	}

	var managerUUID *uuid.UUID
	if req.Role == models.RoleSalesExecutive {
		if req.ManagerID == nil {
			helpers.ErrorResponse(c, http.StatusBadRequest, "manager_id is required for sales_executive")
			return
		}
		switch v := req.ManagerID.(type) {
		case string:
			trimmed := strings.TrimSpace(v)
			if trimmed == "" {
				helpers.ErrorResponse(c, http.StatusBadRequest, "manager_id is required for sales_executive")
				return
			}
			parsed, err := uuid.Parse(trimmed)
			if err != nil {
				helpers.ErrorResponse(c, http.StatusBadRequest, "invalid manager_id: must be a valid UUID")
				return
			}
			managerUUID = &parsed
		default:
			helpers.ErrorResponse(c, http.StatusBadRequest, "invalid manager_id: must be a valid UUID")
			return
		}
	}

	result, err := ac.authService.CreateUser(c.Request.Context(), req.Name, req.Email, req.Mobile, req.Password, req.Role, managerUUID)
	if err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, result)
}

// ChangePassword handles POST /api/v1/auth/change-password.
func (ac *AuthController) ChangePassword(c *gin.Context) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		helpers.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var userID uuid.UUID
	switch v := userIDVal.(type) {
	case uuid.UUID:
		userID = v
	case string:
		parsed, err := uuid.Parse(v)
		if err != nil {
			helpers.ErrorResponse(c, http.StatusUnauthorized, "invalid user id in context")
			return
		}
		userID = parsed
	default:
		helpers.ErrorResponse(c, http.StatusUnauthorized, "unknown user id type in context")
		return
	}

	var req models.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "new_password is required")
		return
	}

	if err := ac.authService.ChangePassword(c.Request.Context(), userID, req.NewPassword); err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, gin.H{
		"success": true,
		"message": "Password changed successfully",
	})
}

// ListUsers handles GET /api/v1/users.
func (ac *AuthController) ListUsers(c *gin.Context) {
	users, err := ac.authService.ListUsers(c.Request.Context())
	if err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}
	helpers.SuccessResponse(c, http.StatusOK, users)
}

// UpdateUser handles PATCH /api/v1/users/:id and PUT /api/v1/users/:id.
func (ac *AuthController) UpdateUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "invalid user id")
		return
	}

	var req struct {
		Name      string     `json:"name" binding:"required"`
		Email     string     `json:"email" binding:"required,email"`
		Role      string     `json:"role" binding:"required"`
		IsActive  *bool      `json:"is_active"`
		ManagerID *uuid.UUID `json:"manager_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "invalid payload")
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	updated, err := ac.authService.UpdateUser(c.Request.Context(), id, req.Name, req.Email, req.Role, isActive, req.ManagerID)
	if err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, updated)
}

// DeleteUser handles DELETE /api/v1/users/:id.
func (ac *AuthController) DeleteUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "invalid user id")
		return
	}

	if err := ac.authService.DeleteUser(c.Request.Context(), id); err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, gin.H{"message": "user deleted successfully"})
}

// AssignManager handles PATCH /api/v1/users/:id/manager.
func (ac *AuthController) AssignManager(c *gin.Context) {
	idStr := c.Param("id")
	execID, err := uuid.Parse(idStr)
	if err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "invalid executive id")
		return
	}

	var req struct {
		ManagerID *uuid.UUID `json:"manager_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "invalid payload")
		return
	}

	if err := ac.authService.AssignManager(c.Request.Context(), execID, req.ManagerID); err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, gin.H{"message": "manager assigned successfully"})
}

// GetLeaderPermissions handles GET /api/v1/users/:id/delegations.
func (ac *AuthController) GetLeaderPermissions(c *gin.Context) {
	idStr := c.Param("id")
	leaderID, err := uuid.Parse(idStr)
	if err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "invalid leader id")
		return
	}

	perms, err := ac.authService.GetLeaderPermissions(c.Request.Context(), leaderID)
	if err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, perms)
}

// GrantLeaderPermission handles POST /api/v1/users/:id/delegations.
func (ac *AuthController) GrantLeaderPermission(c *gin.Context) {
	idStr := c.Param("id")
	leaderID, err := uuid.Parse(idStr)
	if err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "invalid leader id")
		return
	}

	callerID, _, _, err := middleware.GetAuthUser(c)
	if err != nil {
		helpers.ErrorResponse(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Permission string `json:"permission" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "permission is required")
		return
	}

	if err := ac.authService.GrantLeaderPermission(c.Request.Context(), leaderID, req.Permission, callerID); err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, gin.H{"message": "permission granted successfully"})
}

// RevokeLeaderPermission handles DELETE /api/v1/users/:id/delegations/:permission and DELETE /api/v1/users/:id/delegations.
func (ac *AuthController) RevokeLeaderPermission(c *gin.Context) {
	idStr := c.Param("id")
	leaderID, err := uuid.Parse(idStr)
	if err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "invalid leader id")
		return
	}

	permission := c.Param("permission")
	if permission == "" {
		var req struct {
			Permission string `json:"permission"`
		}
		if err := c.ShouldBindJSON(&req); err == nil && req.Permission != "" {
			permission = req.Permission
		}
	}
	if permission == "" {
		helpers.ErrorResponse(c, http.StatusBadRequest, "permission is required")
		return
	}

	if err := ac.authService.RevokeLeaderPermission(c.Request.Context(), leaderID, permission); err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, gin.H{"message": "permission revoked successfully"})
}

// GetMyPermissions handles GET /api/v1/auth/me/permissions.
func (ac *AuthController) GetMyPermissions(c *gin.Context) {
	callerID, role, _, err := middleware.GetAuthUser(c)
	if err != nil {
		helpers.ErrorResponse(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	perms, err := ac.authService.GetMyPermissions(c.Request.Context(), callerID, role)
	if err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, gin.H{
		"role":        role,
		"permissions": perms,
	})
}

