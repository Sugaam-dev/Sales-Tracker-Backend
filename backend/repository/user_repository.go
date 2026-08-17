// Package repository defines data-access layers.
package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"crm-auth-service/helpers" 
	"crm-auth-service/models"
)

// UserRepository defines the database operations for User entities.
type UserRepository interface {
	// FindByEmail searches for a user by email address.
	FindByEmail(ctx context.Context, email string) (*models.User, error)
	// FindByMobile searches for a user by mobile number.
	FindByMobile(ctx context.Context, mobile string) (*models.User, error)
	// FindByID searches for a user by their UUID primary key.
	FindByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	// UpdatePasswordAndFirstLogin updates password hash and clears is_first_login.
	UpdatePasswordAndFirstLogin(ctx context.Context, id uuid.UUID, newPasswordHash string) error
	// UpdateEmailVerified sets email_verified to TRUE.
	UpdateEmailVerified(ctx context.Context, id uuid.UUID) error
	// UpdateMobileVerified sets mobile_verified to TRUE.
	UpdateMobileVerified(ctx context.Context, id uuid.UUID) error
	// UpdatePassword updates password hash.
	UpdatePassword(ctx context.Context, id uuid.UUID, newPasswordHash string) error
}

// userRepository implements the UserRepository interface using pgxpool.
type userRepository struct {
	db *pgxpool.Pool
}

// NewUserRepository constructs a new instance of UserRepository.
func NewUserRepository(db *pgxpool.Pool) UserRepository {
	return &userRepository{db: db}
}

const selectUserFields = "id, email, mobile, password_hash, role, is_first_login, email_verified, mobile_verified, mfa_enabled, mfa_method, sso_provider, sso_subject_id, created_at, updated_at"

func scanUser(row pgx.Row) (*models.User, error) {
	var user models.User
	err := row.Scan(
		&user.ID,
		&user.Email,
		&user.Mobile,
		&user.PasswordHash,
		&user.Role,
		&user.IsFirstLogin,
		&user.EmailVerified,
		&user.MobileVerified,
		&user.MFAEnabled,
		&user.MFAMethod,
		&user.SSOProvider,
		&user.SSOSubjectID,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, helpers.ErrNotFound
		}
		return nil, err
	}
	return &user, nil
}

// FindByEmail searches for a user by email address.
func (r *userRepository) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	query := "SELECT " + selectUserFields + " FROM users WHERE email = $1"
	row := r.db.QueryRow(ctx, query, email)
	return scanUser(row)
}

// FindByMobile searches for a user by mobile number.
func (r *userRepository) FindByMobile(ctx context.Context, mobile string) (*models.User, error) {
	query := "SELECT " + selectUserFields + " FROM users WHERE mobile = $1"
	row := r.db.QueryRow(ctx, query, mobile)
	return scanUser(row)
}

// FindByID searches for a user by their UUID primary key.
func (r *userRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	query := "SELECT " + selectUserFields + " FROM users WHERE id = $1"
	row := r.db.QueryRow(ctx, query, id)
	return scanUser(row)
}

// UpdatePasswordAndFirstLogin updates password hash and clears is_first_login.
func (r *userRepository) UpdatePasswordAndFirstLogin(ctx context.Context, id uuid.UUID, newPasswordHash string) error {
	query := `UPDATE users SET password_hash = $1, is_first_login = FALSE, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(ctx, query, newPasswordHash, id)
	return err
}

// UpdateEmailVerified sets email_verified to TRUE.
func (r *userRepository) UpdateEmailVerified(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE users SET email_verified = TRUE, updated_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	return err
}

// UpdateMobileVerified sets mobile_verified to TRUE.
func (r *userRepository) UpdateMobileVerified(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE users SET mobile_verified = TRUE, updated_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	return err
}

// UpdatePassword updates password hash.
func (r *userRepository) UpdatePassword(ctx context.Context, id uuid.UUID, newPasswordHash string) error {
	query := `UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(ctx, query, newPasswordHash, id)
	return err
}