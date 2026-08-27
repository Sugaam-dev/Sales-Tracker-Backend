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
)

// IsValidRole returns true if the input role matches one of the four valid roles.
func IsValidRole(role string) bool {
	switch role {
	case RoleAdmin, RoleSalesManager, RoleSalesExecutive, RoleLeader:
		return true
	}
	return false
}

type User struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name           string    `gorm:"type:varchar;not null"`
	Email          string    `gorm:"type:varchar;uniqueIndex;not null"`
	Mobile         *string   `gorm:"type:varchar;uniqueIndex"`
	PasswordHash   string    `gorm:"type:varchar;not null"`
	Role           string    `gorm:"type:varchar;not null"` // admin | sales_manager | sales_executive | leader
	IsFirstLogin   bool      `gorm:"not null;default:true"`
	EmailVerified  bool      `gorm:"not null;default:false"`
	MobileVerified bool      `gorm:"not null;default:false"`
	MFAEnabled     bool      `gorm:"not null;default:false"`
	MFAMethod      *string   `gorm:"type:varchar"` // "email" | "sms"
	SSOProvider    *string   `gorm:"type:varchar"`
	SSOSubjectID   *string   `gorm:"type:varchar"`
	IsActive       bool      `gorm:"not null;default:true"`
	CreatedAt      time.Time `gorm:"not null;autoCreateTime"`
	UpdatedAt      time.Time `gorm:"not null;autoUpdateTime"`
}

func (User) TableName() string { return "users" }

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
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	Email          string    `json:"email"`
	Mobile         *string   `json:"mobile,omitempty"`
	Role           string    `json:"role"`
	EmailVerified  bool      `json:"email_verified"`
	MobileVerified bool      `json:"mobile_verified"`
	IsActive       bool      `json:"is_active"`
}

// LoginSuccessResponse — normal login: no pending onboarding, MFA not enabled.
type LoginSuccessResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	User         UserSummary `json:"user"`
}

// LoginFirstTimeResponse — users.is_first_login was true.
type LoginFirstTimeResponse struct {
	RequiresOnboarding bool   `json:"requires_onboarding"`
	TempToken          string `json:"temp_token"`
}

// LoginMFARequiredResponse — user has MFA enabled. Not logged in yet;
// mfa_pending_token only authorizes POST /auth/mfa/verify (Module B).
type LoginMFARequiredResponse struct {
	MFARequired     bool   `json:"mfa_required"`
	MFAPendingToken string `json:"mfa_pending_token"`
}