// Module A has no separate "active sessions" table — a valid,
// non-revoked refresh token IS the session, per the documentation.
// This file is named session_repository.go to match the requested
// convention, but operates on models.RefreshToken; a true
// active_sessions table (if one gets added later) belongs to whichever
// module introduces it.
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"crm-auth-service/helpers"
	"crm-auth-service/models"
)

type SessionRepository interface {
	Create(ctx context.Context, token *models.RefreshToken) error
	// FindActiveByHash returns the token row only if not revoked and
	// not expired — matches the doc's step 2 query exactly.
	FindActiveByHash(ctx context.Context, hash string) (*models.RefreshToken, error)
	Revoke(ctx context.Context, id uuid.UUID) error
}

type sessionRepository struct {
	db *gorm.DB
}

func NewSessionRepository(db *gorm.DB) SessionRepository {
	return &sessionRepository{db: db}
}

func (r *sessionRepository) Create(ctx context.Context, token *models.RefreshToken) error {
	return r.db.WithContext(ctx).Create(token).Error
}

func (r *sessionRepository) FindActiveByHash(ctx context.Context, hash string) (*models.RefreshToken, error) {
	var token models.RefreshToken
	err := r.db.WithContext(ctx).
		Where("token_hash = ? AND revoked = ? AND expires_at > ?", hash, false, time.Now()).
		First(&token).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, helpers.ErrNotFound
		}
		return nil, err
	}
	return &token, nil
}

// Revoke marks a token as used. The service layer MUST call this
// before issuing a replacement (see the "revoke first, then issue"
// rule in the auth documentation) — reversing the order would let a
// crash between the two steps leave two simultaneously-valid tokens.
func (r *sessionRepository) Revoke(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&models.RefreshToken{}).
		Where("id = ?", id).
		Update("revoked", true).Error
}