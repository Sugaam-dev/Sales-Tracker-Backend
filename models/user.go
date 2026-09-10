package models

import (
	"time"

	"github.com/google/uuid"
)

// User is the shared users table. Module A only reads/writes a subset
// of these columns (password_hash, is_first_login, mfa_enabled,
// mfa_method), but the full schema is defined here because Module A
// owns this table's migration — other modules (onboarding, SSO, MFA
// management) read and write the remaining columns without needing a
// second migration.
const (
	RoleAdmin          = "admin"
	RoleSalesManager   = "sales_manager"
	RoleSalesExecutive = "sales_executive"
	RoleLeader         = "leader"

	PermissionUserView             = "user.view"
	PermissionUserCreate           = "user.create"
	PermissionUserUpdate           = "user.update"
	PermissionUserDelete           = "user.delete"
	PermissionManagerManage        = "manager.manage"
	PermissionSystemSettingsManage = "system.settings.manage"
)

// IsValidPermission checks if the permission is a known valid delegated permission.
func IsValidPermission(p string) bool {
	switch p {
	case PermissionUserView, PermissionUserCreate, PermissionUserUpdate,
		PermissionUserDelete, PermissionManagerManage, PermissionSystemSettingsManage:
		return true
	}
	return false
}

// IsValidRole returns true if the input role matches one of the four valid roles.
func IsValidRole(role string) bool {
	switch role {
	case RoleAdmin, RoleSalesManager, RoleSalesExecutive, RoleLeader:
		return true
	}
	return false
}

type User struct {
	ID                     uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name                   string     `gorm:"type:varchar;not null"`
	Email                  string     `gorm:"type:varchar;uniqueIndex;not null"`
	Mobile                 *string    `gorm:"type:varchar;uniqueIndex"`
	PasswordHash           string     `gorm:"type:varchar;not null"`
	Role                   string     `gorm:"type:varchar;not null"` // admin | sales_manager | sales_executive | leader
	ManagerID              *uuid.UUID `gorm:"type:uuid;index"`
	IsFirstLogin           bool       `gorm:"not null;default:true"`
	PasswordChangeRequired bool       `gorm:"not null;default:true"`
	EmailVerified          bool       `gorm:"not null;default:false"`
	MobileVerified         bool       `gorm:"not null;default:false"`
	MFAEnabled             bool       `gorm:"not null;default:false"`
	MFAMethod              *string    `gorm:"type:varchar"` // "email" | "sms"
	SSOProvider            *string    `gorm:"type:varchar"`
	SSOSubjectID           *string    `gorm:"type:varchar"`
	IsActive               bool       `gorm:"not null;default:true"`
	CreatedAt              time.Time  `gorm:"not null;autoCreateTime"`
	UpdatedAt              time.Time  `gorm:"not null;autoUpdateTime"`
}

func (User) TableName() string { return "users" }

type LeaderDelegation struct {
	ID         uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID     uuid.UUID  `json:"userId" gorm:"type:uuid;not null;index"`
	Permission string     `json:"permission" gorm:"type:varchar(100);not null"`
	GrantedBy  *uuid.UUID `json:"grantedBy,omitempty" gorm:"type:uuid"`
	CreatedAt  time.Time  `json:"createdAt" gorm:"not null;autoCreateTime"`
}

func (LeaderDelegation) TableName() string { return "leader_delegations" }

// --- Login payloads (POST /auth/login) ---------------------------------

// LoginRequest is the request body. Identifier is either an email or a
// mobile number — the service layer detects which by checking for "@".
type LoginRequest struct {
	Identifier string `json:"identifier" binding:"required"`
	Password   string `json:"password" binding:"required"`
}

// UserSummary is the trimmed-down user object returned on login. Never
// includes PasswordHash or any other sensitive field.
type UserSummary struct {
	ID             uuid.UUID  `json:"id"`
	Name           string     `json:"name"`
	Email          string     `json:"email"`
	Mobile         *string    `json:"mobile,omitempty"`
	Role           string     `json:"role"`
	ManagerID      *uuid.UUID `json:"manager_id,omitempty"`
	ManagerName    *string    `json:"manager_name,omitempty"`
	Permissions    []string   `json:"permissions,omitempty"`
	EmailVerified  bool       `json:"email_verified"`
	MobileVerified bool       `json:"mobile_verified"`
	IsActive       bool       `json:"is_active"`
}

// LoginSuccessResponse — normal login: no pending onboarding, MFA not enabled.
type LoginSuccessResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	User         UserSummary `json:"user"`
}

// LoginFirstTimeResponse — users.is_first_login or password_change_required was true.
type LoginFirstTimeResponse struct {
	AccessToken            string `json:"access_token"`
	TokenType              string `json:"token_type"`
	ExpiresIn              int    `json:"expires_in"`
	FirstTimeLogin         bool   `json:"first_time_login"`
	PasswordChangeRequired bool   `json:"password_change_required"`
	Message                string `json:"message"`
}

// LoginMFARequiredResponse — user has MFA enabled. Not logged in yet;
// mfa_pending_token only authorizes POST /auth/mfa/verify (Module B).
type LoginMFARequiredResponse struct {
	MFARequired     bool   `json:"mfa_required"`
	MFAPendingToken string `json:"mfa_pending_token"`
}

// AdminCreatedUser represents the dedicated user payload returned in CreateUserResponse.
type AdminCreatedUser struct {
	ID                     uuid.UUID `json:"id"`
	Name                   string    `json:"name"`
	Email                  string    `json:"email"`
	Role                   string    `json:"role"`
	IsFirstLogin           bool      `json:"is_first_login"`
	PasswordChangeRequired bool      `json:"password_change_required"`
}

// CreateUserResponse is the response returned after an admin creates a new user.
type CreateUserResponse struct {
	Success   bool              `json:"success"`
	Message   string            `json:"message"`
	EmailSent bool              `json:"email_sent"`
	User      *AdminCreatedUser `json:"user,omitempty"`
}

// ChangePasswordRequest is the payload for changing password.
type ChangePasswordRequest struct {
	NewPassword string `json:"new_password" binding:"required"`
}
