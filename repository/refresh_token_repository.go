package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"crm-auth-service/helpers"
	"crm-auth-service/models"
)

// SessionRepository defines the database operations for Managing Active Sessions (Refresh Tokens).
type SessionRepository interface {
	// Create persists a new refresh token session record in the database.
	Create(ctx context.Context, token *models.RefreshToken) error
	// FindActiveByHash searches for an unexpired, unrevoked token session record by SHA-256 hash.
	FindActiveByHash(ctx context.Context, hash string) (*models.RefreshToken, error)
	// Revoke invalidates an active session refresh token by marking it revoked.
	Revoke(ctx context.Context, id uuid.UUID) error
	// RevokeAllForUser invalidates all active session refresh tokens for a user.
	RevokeAllForUser(ctx context.Context, userID uuid.UUID) error
}

// sessionRepository implements the SessionRepository interface using pgxpool.
type sessionRepository struct {
	db *pgxpool.Pool
}

// NewSessionRepository constructs a new instance of SessionRepository.
func NewSessionRepository(db *pgxpool.Pool) SessionRepository {
	return &sessionRepository{db: db}
}

// Create persists a new refresh token session record in the database.
func (r *sessionRepository) Create(ctx context.Context, token *models.RefreshToken) error {
	query := `
		INSERT INTO refresh_tokens
		(user_id, token_hash, revoked, expires_at, created_at)
		VALUES ($1, $2, $3, $4, NOW())
		RETURNING id, created_at
	`

	return r.db.QueryRow(
		ctx,
		query,
		token.UserID,
		token.TokenHash,
		token.Revoked,
		token.ExpiresAt,
	).Scan(&token.ID, &token.CreatedAt)
}

// FindActiveByHash searches for an unexpired, unrevoked token session record by SHA-256 hash.
func (r *sessionRepository) FindActiveByHash(ctx context.Context, hash string) (*models.RefreshToken, error) {
	query := `SELECT id, user_id, token_hash, revoked, expires_at, created_at
			  FROM refresh_tokens
			  WHERE token_hash = $1 AND revoked = $2 AND expires_at > $3`
	var token models.RefreshToken
	err := r.db.QueryRow(ctx, query, hash, false, time.Now()).
		Scan(&token.ID, &token.UserID, &token.TokenHash, &token.Revoked, &token.ExpiresAt, &token.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, helpers.ErrNotFound
		}
		return nil, err
	}
	return &token, nil
}

// Revoke invalidates an active session refresh token by marking it revoked.
func (r *sessionRepository) Revoke(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE refresh_tokens SET revoked = $1 WHERE id = $2`
	_, err := r.db.Exec(ctx, query, true, id)
	return err
}

// RevokeAllForUser invalidates all active session refresh tokens for a user.
func (r *sessionRepository) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE refresh_tokens SET revoked = TRUE WHERE user_id = $1`
	_, err := r.db.Exec(ctx, query, userID)
	return err
}
