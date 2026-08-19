package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"crm-auth-service/models"
)

// UserEmailOTPRepository defines the database operations for storing MFA OTP codes.
type UserEmailOTPRepository interface {
	// Create persists a new MFA OTP record.
	Create(ctx context.Context, otp *models.MFAOtp) error
	// FindValid finds a valid unused MFA OTP.
	FindValid(ctx context.Context, userID uuid.UUID, otpHash string) (*models.MFAOtp, error)
	// MarkUsed marks an MFA OTP as used.
	MarkUsed(ctx context.Context, id uuid.UUID) error
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
	query := `INSERT INTO mfa_otps (user_id, otp_hash, used, expires_at, created_at)
			  VALUES ($1, $2, $3, $4, NOW())
			  RETURNING id, created_at`
	return r.db.QueryRow(ctx, query, otp.UserID, otp.OTPHash, otp.Used, otp.ExpiresAt).
		Scan(&otp.ID, &otp.CreatedAt)
}

// FindValid finds a valid unused MFA OTP.
func (r *userEmailOTPRepository) FindValid(ctx context.Context, userID uuid.UUID, otpHash string) (*models.MFAOtp, error) {
	query := `SELECT id, user_id, otp_hash, used, expires_at, created_at
			  FROM mfa_otps
			  WHERE user_id = $1 AND otp_hash = $2 AND used = FALSE AND expires_at > NOW()`
	
	var otp models.MFAOtp
	err := r.db.QueryRow(ctx, query, userID, otpHash).
		Scan(&otp.ID, &otp.UserID, &otp.OTPHash, &otp.Used, &otp.ExpiresAt, &otp.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &otp, nil
}

// MarkUsed marks an MFA OTP as used.
func (r *userEmailOTPRepository) MarkUsed(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE mfa_otps SET used = TRUE WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	return err
}