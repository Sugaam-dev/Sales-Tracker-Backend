package repository

import (
	"context"

	"gorm.io/gorm"

	"crm-auth-service/models"
)

// UserEmailOTPRepository only writes rows during login (Module A's
// scope). Reading/verifying/marking-used belongs to Module B's
// repository. Named to match the requested convention; underlying
// model (models.MFAOtp) covers SMS OTPs too, gated by user.mfa_method.
type UserEmailOTPRepository interface {
	Create(ctx context.Context, otp *models.MFAOtp) error
}

type userEmailOTPRepository struct {
	db *gorm.DB
}

func NewUserEmailOTPRepository(db *gorm.DB) UserEmailOTPRepository {
	return &userEmailOTPRepository{db: db}
}

func (r *userEmailOTPRepository) Create(ctx context.Context, otp *models.MFAOtp) error {
	return r.db.WithContext(ctx).Create(otp).Error
}