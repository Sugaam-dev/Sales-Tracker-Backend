package models

import (
	"time"

	"github.com/google/uuid"
)

// UserEmailOTP stores the SHA-256 hash of a 6-digit OTP generated during
// onboarding when the user verifies their email address.
type UserEmailOTP struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index"`
	OTPHash   string    `gorm:"type:varchar;not null"`
	Used      bool      `gorm:"not null;default:false"`
	ExpiresAt time.Time `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null;autoCreateTime"`
}

func (UserEmailOTP) TableName() string { return "email_otps" }

// MobileOTP stores the SHA-256 hash of a 6-digit OTP generated during
// onboarding when the user verifies their mobile number via SMS.
type MobileOTP struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index"`
	OTPHash   string    `gorm:"type:varchar;not null"`
	Used      bool      `gorm:"not null;default:false"`
	ExpiresAt time.Time `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null;autoCreateTime"`
}

func (MobileOTP) TableName() string { return "mobile_otps" }
