package models

import (
	"github.com/google/uuid"
	"time"
)

// MFAOtp stores the SHA-256 hash of a 6-digit OTP generated during
// login when the user has MFA enabled. Module A only writes rows here
// (on login); verifying and marking them used belongs to Module B's
// /auth/mfa/verify endpoint. Kept as its own table (mfa_otps) rather
// than merged into a generic "user_email_otp" table since it covers
// both email and SMS delivery, per user.mfa_method.
type MFAOtp struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index"`
	OTPHash   string    `gorm:"type:varchar;not null"`
	Used      bool      `gorm:"not null;default:false"`
	ExpiresAt time.Time `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null;autoCreateTime"`
}

func (MFAOtp) TableName() string { return "mfa_otps" }
