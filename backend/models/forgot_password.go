package models

import (
	"time"

	"github.com/google/uuid"
)

// PasswordResetToken stores the SHA-256 hash of a reset token generated when
// the user requests to reset their password.
type PasswordResetToken struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index"`
	TokenHash string    `gorm:"type:varchar;not null"`
	Used      bool      `gorm:"not null;default:false"`
	ExpiresAt time.Time `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null;autoCreateTime"`
}

func (PasswordResetToken) TableName() string { return "password_reset_tokens" }
