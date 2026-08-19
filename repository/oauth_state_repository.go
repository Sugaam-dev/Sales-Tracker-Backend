package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"crm-auth-service/helpers"
	"crm-auth-service/models"
)

type OAuthStateRepository interface {
	SaveOAuthState(ctx context.Context, state *models.OAuthState) error
	VerifyOAuthState(ctx context.Context, stateToken string) (*models.OAuthState, error)
}

type oauthStateRepository struct {
	db *gorm.DB
}

func NewOAuthStateRepository(db *gorm.DB) OAuthStateRepository {
	return &oauthStateRepository{db: db}
}

func (r *oauthStateRepository) SaveOAuthState(ctx context.Context, state *models.OAuthState) error {
	return r.db.WithContext(ctx).Create(state).Error
}

func (r *oauthStateRepository) VerifyOAuthState(ctx context.Context, stateToken string) (*models.OAuthState, error) {
	var state models.OAuthState
	err := r.db.WithContext(ctx).Where("state = ?", stateToken).First(&state).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, helpers.ErrNotFound
		}
		return nil, err
	}

	// Delete state token immediately so it's a one-time verify
	r.db.WithContext(ctx).Delete(&state)

	if time.Now().After(state.ExpiresAt) {
		return nil, errors.New("state expired")
	}

	return &state, nil
}
