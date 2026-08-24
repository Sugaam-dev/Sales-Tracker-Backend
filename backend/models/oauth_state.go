package models

import (
	"time"

	"github.com/google/uuid"
)

type OAuthState struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	State     string    `gorm:"type:varchar;uniqueIndex;not null"`
	Provider  string    `gorm:"type:varchar;not null"`
	ExpiresAt time.Time `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null;autoCreateTime"`
}

func (OAuthState) TableName() string { return "oauth_states" }
