package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"crm-auth-service/helpers"
	"crm-auth-service/models"
)

type OAuthStateRepository interface {
	SaveOAuthState(ctx context.Context, state *models.OAuthState) error
	VerifyOAuthState(ctx context.Context, stateToken string) (*models.OAuthState, error)
}

type oauthStateRepository struct {
	db *pgxpool.Pool
}

func NewOAuthStateRepository(db *pgxpool.Pool) OAuthStateRepository {
	return &oauthStateRepository{db: db}
}

func (r *oauthStateRepository) SaveOAuthState(ctx context.Context, state *models.OAuthState) error {
	query := `INSERT INTO oauth_states (state, provider, expires_at)
			  VALUES ($1, $2, $3)
			  RETURNING id, created_at`
	return r.db.QueryRow(ctx, query, state.State, state.Provider, state.ExpiresAt).
		Scan(&state.ID, &state.CreatedAt)
}

func (r *oauthStateRepository) VerifyOAuthState(ctx context.Context, stateToken string) (*models.OAuthState, error) {
	query := `SELECT id, state, provider, expires_at, created_at
			  FROM oauth_states
			  WHERE state = $1`

	var state models.OAuthState
	err := r.db.QueryRow(ctx, query, stateToken).
		Scan(&state.ID, &state.State, &state.Provider, &state.ExpiresAt, &state.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, helpers.ErrNotFound
		}
		return nil, err
	}

	// Delete state token immediately so it's a one-time verify
	deleteQuery := `DELETE FROM oauth_states WHERE id = $1`
	_, _ = r.db.Exec(ctx, deleteQuery, state.ID)

	if time.Now().After(state.ExpiresAt) {
		return nil, errors.New("state expired")
	}

	return &state, nil
}
