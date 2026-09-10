package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"crm-auth-service/helpers"
	"crm-auth-service/models"
	"crm-auth-service/repository"
)

// GetAuthUser extracts the authenticated user's ID, role, and email from the Gin context.
func GetAuthUser(c *gin.Context) (uuid.UUID, string, string, error) {
	val, exists := c.Get("user_id")
	if !exists {
		return uuid.Nil, "", "", errors.New("unauthorized: missing user_id in context")
	}

	var userID uuid.UUID
	switch v := val.(type) {
	case uuid.UUID:
		userID = v
	case string:
		parsed, err := uuid.Parse(v)
		if err != nil {
			return uuid.Nil, "", "", errors.New("unauthorized: invalid user_id format")
		}
		userID = parsed
	default:
		return uuid.Nil, "", "", errors.New("unauthorized: unknown user_id type")
	}

	role := c.GetString("role")
	email := c.GetString("email")
	return userID, role, email, nil
}

// RequireRole enforces that the caller has one of the specified primary roles.
func RequireRole(roles ...string) gin.HandlerFunc {
	allowedRoles := make(map[string]bool)
	for _, r := range roles {
		allowedRoles[r] = true
	}

	return func(c *gin.Context) {
		role := c.GetString("role")
		if !allowedRoles[role] {
			helpers.ErrorResponse(c, http.StatusForbidden, "Forbidden: insufficient role permissions")
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequirePermission enforces that the caller is an Admin OR a Leader with the given delegated permission.
func RequirePermission(userRepo repository.UserRepository, permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, role, _, err := GetAuthUser(c)
		if err != nil {
			helpers.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
			c.Abort()
			return
		}

		// Admin has completely unrestricted administrative access
		if role == models.RoleAdmin {
			c.Next()
			return
		}

		// Leaders must have explicit delegated permission
		if role == models.RoleLeader {
			perms, err := userRepo.GetLeaderPermissions(c.Request.Context(), userID)
			if err != nil {
				helpers.ErrorResponse(c, http.StatusInternalServerError, "Failed to verify user permissions")
				c.Abort()
				return
			}
			for _, p := range perms {
				if p == permission {
					c.Next()
					return
				}
			}
		}

		// Sales Manager, Sales Executive, or Leader without permission
		helpers.ErrorResponse(c, http.StatusForbidden, "Forbidden: missing required delegated administrative permission: "+permission)
		c.Abort()
	}
}

// ResolveDataScope dynamically constructs the caller's row-level data scope.
func ResolveDataScope(ctx context.Context, userRepo repository.UserRepository, userID uuid.UUID, role string) (helpers.DataScope, error) {
	scope := helpers.DataScope{
		UserID: userID,
		Role:   role,
	}

	switch role {
	case models.RoleAdmin, models.RoleLeader:
		scope.IsUnrestricted = true
		return scope, nil

	case models.RoleSalesManager:
		scope.IsUnrestricted = false
		execIDs, err := userRepo.GetManagedExecutiveIDs(ctx, userID)
		if err != nil {
			return scope, err
		}
		// Manager's own ID + directly assigned executives' IDs
		allowed := append([]uuid.UUID{userID}, execIDs...)
		scope.AllowedUserIDs = allowed
		return scope, nil

	case models.RoleSalesExecutive:
		scope.IsUnrestricted = false
		scope.AllowedUserIDs = []uuid.UUID{userID}
		return scope, nil

	default:
		// Default to strict individual isolation
		scope.IsUnrestricted = false
		scope.AllowedUserIDs = []uuid.UUID{userID}
		return scope, nil
	}
}
