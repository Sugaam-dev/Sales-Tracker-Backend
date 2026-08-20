package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"crm-auth-service/models"
)

// ForgotPasswordRepository defines the database operations for storing and verifying password reset tokens.
type ForgotPasswordRepository interface {
	DeleteUnused(ctx context.Context, userID uuid.UUID) error
	Create(ctx context.Context, token *models.PasswordResetToken) error
	FindValid(ctx context.Context, tokenHash string) (*models.PasswordResetToken, error)
	MarkUsed(ctx context.Context, id uuid.UUID) error
}

type forgotPasswordRepository struct {
	db *pgxpool.Pool
}

func NewForgotPasswordRepository(db *pgxpool.Pool) ForgotPasswordRepository {
	return &forgotPasswordRepository{db: db}
}

func (r *forgotPasswordRepository) DeleteUnused(ctx context.Context, userID uuid.UUID) error {
	query := `DELETE FROM password_reset_tokens WHERE user_id = $1 AND used = FALSE`
	_, err := r.db.Exec(ctx, query, userID)
	return err
}

func (r *forgotPasswordRepository) Create(ctx context.Context, token *models.PasswordResetToken) error {
	query := `INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
			  VALUES ($1, $2, $3)
			  RETURNING id, created_at, used`
	return r.db.QueryRow(ctx, query, token.UserID, token.TokenHash, token.ExpiresAt).
		Scan(&token.ID, &token.CreatedAt, &token.Used)
}

func (r *forgotPasswordRepository) FindValid(ctx context.Context, tokenHash string) (*models.PasswordResetToken, error) {
	query := `SELECT id, user_id, token_hash, used, expires_at, created_at
			  FROM password_reset_tokens
			  WHERE token_hash = $1 AND used = FALSE AND expires_at > NOW()`
	
	var token models.PasswordResetToken
	err := r.db.QueryRow(ctx, query, tokenHash).
		Scan(&token.ID, &token.UserID, &token.TokenHash, &token.Used, &token.ExpiresAt, &token.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Return nil, nil when not found to easily check
		}
		return nil, err
	}
	return &token, nil
}

func (r *forgotPasswordRepository) MarkUsed(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE password_reset_tokens SET used = TRUE WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	return err
}
