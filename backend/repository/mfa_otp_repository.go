package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"crm-auth-service/models"
)

// UserEmailOTPRepository defines the database operations for storing MFA OTP codes.
type UserEmailOTPRepository interface {
	// Create persists a new MFA OTP record.
	Create(ctx context.Context, otp *models.MFAOtp) error
}

// userEmailOTPRepository implements the UserEmailOTPRepository interface using pgxpool.
type userEmailOTPRepository struct {
	db *pgxpool.Pool
}

// NewUserEmailOTPRepository constructs a new instance of UserEmailOTPRepository.
func NewUserEmailOTPRepository(db *pgxpool.Pool) UserEmailOTPRepository {
	return &userEmailOTPRepository{db: db}
}

// Create persists a new MFA OTP record.
func (r *userEmailOTPRepository) Create(ctx context.Context, otp *models.MFAOtp) error {
	query := `INSERT INTO mfa_otps (user_id, otp_hash, used, expires_at)
			  VALUES ($1, $2, $3, $4)
			  RETURNING id, created_at`
	return r.db.QueryRow(ctx, query, otp.UserID, otp.OTPHash, otp.Used, otp.ExpiresAt).
		Scan(&otp.ID, &otp.CreatedAt)
}