package models

import (
	"time"
	"github.com/google/uuid"
)

// RefreshToken stores the SHA-256 hash of an issued refresh token. The
// raw token is never persisted — only returned once to the client.
type RefreshToken struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index"`
	TokenHash string    `gorm:"type:varchar;not null;uniqueIndex"`
	Revoked   bool      `gorm:"not null;default:false"`
	ExpiresAt time.Time `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null;autoCreateTime"`
}

func (RefreshToken) TableName() string { return "refresh_tokens" }

// --- Refresh payloads (POST /auth/refresh) ------------------------------

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type RefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}