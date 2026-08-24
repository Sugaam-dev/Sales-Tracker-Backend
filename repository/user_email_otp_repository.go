package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"crm-auth-service/models"
)

// EmailOTPRepository defines the database operations for storing and verifying Email OTP codes.
type EmailOTPRepository interface {
	DeleteUnused(ctx context.Context, userID uuid.UUID) error
	Create(ctx context.Context, otp *models.UserEmailOTP) error
	FindValid(ctx context.Context, userID uuid.UUID, otpHash string) (*models.UserEmailOTP, error)
	MarkUsed(ctx context.Context, id uuid.UUID) error
}

type emailOTPRepository struct {
	db *pgxpool.Pool
}

func NewEmailOTPRepository(db *pgxpool.Pool) EmailOTPRepository {
	return &emailOTPRepository{db: db}
}

func (r *emailOTPRepository) DeleteUnused(ctx context.Context, userID uuid.UUID) error {
	query := `DELETE FROM email_otps WHERE user_id = $1 AND used = FALSE`
	_, err := r.db.Exec(ctx, query, userID)
	return err
}

func (r *emailOTPRepository) Create(ctx context.Context, otp *models.UserEmailOTP) error {
	query := `INSERT INTO email_otps (user_id, otp_hash, expires_at, created_at)
			  VALUES ($1, $2, $3, NOW())
			  RETURNING id, created_at, used`
	return r.db.QueryRow(ctx, query, otp.UserID, otp.OTPHash, otp.ExpiresAt).
		Scan(&otp.ID, &otp.CreatedAt, &otp.Used)
}

func (r *emailOTPRepository) FindValid(ctx context.Context, userID uuid.UUID, otpHash string) (*models.UserEmailOTP, error) {
	query := `SELECT id, user_id, otp_hash, used, expires_at, created_at
			  FROM email_otps
			  WHERE user_id = $1 AND otp_hash = $2 AND used = FALSE AND expires_at > NOW()`

	var otp models.UserEmailOTP
	err := r.db.QueryRow(ctx, query, userID, otpHash).
		Scan(&otp.ID, &otp.UserID, &otp.OTPHash, &otp.Used, &otp.ExpiresAt, &otp.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Return nil, nil when not found to easily check
		}
		return nil, err
	}
	return &otp, nil
}

func (r *emailOTPRepository) MarkUsed(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE email_otps SET used = TRUE WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	return err
}

// MobileOTPRepository defines the database operations for storing and verifying Mobile OTP codes.
type MobileOTPRepository interface {
	DeleteUnused(ctx context.Context, userID uuid.UUID) error
	Create(ctx context.Context, otp *models.MobileOTP) error
	FindValid(ctx context.Context, userID uuid.UUID, otpHash string) (*models.MobileOTP, error)
	MarkUsed(ctx context.Context, id uuid.UUID) error
}

type mobileOTPRepository struct {
	db *pgxpool.Pool
}

func NewMobileOTPRepository(db *pgxpool.Pool) MobileOTPRepository {
	return &mobileOTPRepository{db: db}
}

func (r *mobileOTPRepository) DeleteUnused(ctx context.Context, userID uuid.UUID) error {
	query := `DELETE FROM mobile_otps WHERE user_id = $1 AND used = FALSE`
	_, err := r.db.Exec(ctx, query, userID)
	return err
}

func (r *mobileOTPRepository) Create(ctx context.Context, otp *models.MobileOTP) error {
	query := `INSERT INTO mobile_otps (user_id, otp_hash, expires_at)
			  VALUES ($1, $2, $3)
			  RETURNING id, created_at, used`
	return r.db.QueryRow(ctx, query, otp.UserID, otp.OTPHash, otp.ExpiresAt).
		Scan(&otp.ID, &otp.CreatedAt, &otp.Used)
}

func (r *mobileOTPRepository) FindValid(ctx context.Context, userID uuid.UUID, otpHash string) (*models.MobileOTP, error) {
	query := `SELECT id, user_id, otp_hash, used, expires_at, created_at
			  FROM mobile_otps
			  WHERE user_id = $1 AND otp_hash = $2 AND used = FALSE AND expires_at > NOW()`

	var otp models.MobileOTP
	err := r.db.QueryRow(ctx, query, userID, otpHash).
		Scan(&otp.ID, &otp.UserID, &otp.OTPHash, &otp.Used, &otp.ExpiresAt, &otp.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Return nil, nil when not found to easily check
		}
		return nil, err
	}
	return &otp, nil
}

func (r *mobileOTPRepository) MarkUsed(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE mobile_otps SET used = TRUE WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	return err
}
