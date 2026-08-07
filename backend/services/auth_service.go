// Package services holds business logic.
package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"crm-auth-service/helpers"
	"crm-auth-service/middleware"
	"crm-auth-service/models"
	"crm-auth-service/repository"
)

const (
	refreshTokenTTL = 7 * 24 * time.Hour
	mfaOTPTTL       = 5 * time.Minute
)

// AuthService owns the Login and Refresh flows end to end.
type AuthService struct {
	userRepo           repository.UserRepository
	sessionRepo        repository.SessionRepository
	otpRepo            repository.UserEmailOTPRepository
	emailOTPRepo       repository.EmailOTPRepository
	mobileOTPRepo      repository.MobileOTPRepository
	forgotPasswordRepo repository.ForgotPasswordRepository
	jwtManager         *helpers.JWTManager
	emailSvc           helpers.EmailService
	smsSvc             helpers.SMSService
	rateLimiter        *middleware.RateLimiter
	log                *slog.Logger
}

// NewAuthService constructs a new instance of AuthService.
func NewAuthService(
	userRepo repository.UserRepository,
	sessionRepo repository.SessionRepository,
	otpRepo repository.UserEmailOTPRepository,
	emailOTPRepo repository.EmailOTPRepository,
	mobileOTPRepo repository.MobileOTPRepository,
	forgotPasswordRepo repository.ForgotPasswordRepository,
	jwtManager *helpers.JWTManager,
	emailSvc helpers.EmailService,
	smsSvc helpers.SMSService,
	rateLimiter *middleware.RateLimiter,
	log *slog.Logger,
) *AuthService {
	return &AuthService{
		userRepo:           userRepo,
		sessionRepo:        sessionRepo,
		otpRepo:            otpRepo,
		emailOTPRepo:       emailOTPRepo,
		mobileOTPRepo:      mobileOTPRepo,
		forgotPasswordRepo: forgotPasswordRepo,
		jwtManager:         jwtManager,
		emailSvc:           emailSvc,
		smsSvc:             smsSvc,
		rateLimiter:        rateLimiter,
		log:                log,
	}
}

// Login validates user credentials, checks rate limits, and routes to the appropriate login path.
func (s *AuthService) Login(ctx context.Context, req models.LoginRequest, clientIP string) (any, error) {
	rateLimitKey := req.Identifier + "|" + clientIP

	// We check the rate limit before performing any database read operation
	// to protect resource consumption and mitigate brute-force attempts.
	if blocked, retryAfter := s.rateLimiter.IsBlocked(rateLimitKey); blocked {
		return nil, helpers.ErrRateLimited(retryAfter)
	}

	var user *models.User
	var err error
	if helpers.IsEmailIdentifier(req.Identifier) {
		user, err = s.userRepo.FindByEmail(ctx, req.Identifier)
	} else {
		user, err = s.userRepo.FindByMobile(ctx, req.Identifier)
	}
	if err != nil {
		if errors.Is(err, helpers.ErrNotFound) {
			// Record the failure to update rate limits. We use a generic error
			// to prevent user-enumeration side channels.
			s.rateLimiter.RecordFailure(rateLimitKey)
			return nil, helpers.ErrInvalidCredentials()
		}
		return nil, fmt.Errorf("services: find user: %w", err)
	}

	// Verify the password hash matches the plaintext password.
	if !helpers.ComparePassword(req.Password, user.PasswordHash) {
		s.rateLimiter.RecordFailure(rateLimitKey)
		return nil, helpers.ErrInvalidCredentials()
	}

	// Reset failure count upon a correct credential submission.
	s.rateLimiter.Reset(rateLimitKey)

	// If it is the user's first login, route them to onboarding.
	if user.IsFirstLogin {
		tempToken, err := s.jwtManager.GenerateTempToken(user.ID)
		if err != nil {
			return nil, fmt.Errorf("services: generate temp token: %w", err)
		}
		return &models.LoginFirstTimeResponse{
			RequiresOnboarding: true,
			TempToken:          tempToken,
		}, nil
	}

	// Trigger MFA verification if the user has enabled it.
	if user.MFAEnabled {
		return s.triggerMFA(ctx, user)
	}

	// Establish session and issue authentication tokens.
	return s.issueSession(ctx, user)
}

// Refresh handles token rotation by validating the old token and generating a new token pair.
func (s *AuthService) Refresh(ctx context.Context, req models.RefreshRequest) (*models.RefreshResponse, error) {
	tokenHash := helpers.HashRefreshToken(req.RefreshToken)

	tokenRecord, err := s.sessionRepo.FindActiveByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, helpers.ErrNotFound) {
			return nil, helpers.ErrTokenInvalid("Invalid or expired token")
		}
		return nil, fmt.Errorf("services: find refresh token: %w", err)
	}

	// Revoke the old token before generating a new pair to prevent reuse attacks.
	if err := s.sessionRepo.Revoke(ctx, tokenRecord.ID); err != nil {
		return nil, fmt.Errorf("services: revoke refresh token: %w", err)
	}

	user, err := s.userRepo.FindByID(ctx, tokenRecord.UserID)
	if err != nil {
		if errors.Is(err, helpers.ErrNotFound) {
			return nil, helpers.ErrTokenInvalid("Invalid or expired token")
		}
		return nil, fmt.Errorf("services: find user for refresh: %w", err)
	}

	newAccessToken, err := s.jwtManager.GenerateAccessToken(user.ID, user.Role, user.Email)
	if err != nil {
		return nil, fmt.Errorf("services: generate access token: %w", err)
	}

	newRawRefreshToken, err := helpers.GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("services: generate refresh token: %w", err)
	}

	newRecord := &models.RefreshToken{
		UserID:    user.ID,
		TokenHash: helpers.HashRefreshToken(newRawRefreshToken),
		ExpiresAt: time.Now().Add(refreshTokenTTL),
	}
	if err := s.sessionRepo.Create(ctx, newRecord); err != nil {
		return nil, fmt.Errorf("services: store rotated refresh token: %w", err)
	}

	return &models.RefreshResponse{
		AccessToken:  newAccessToken,
		RefreshToken: newRawRefreshToken,
	}, nil
}

// triggerMFA initiates the multi-factor authentication flow.
func (s *AuthService) triggerMFA(ctx context.Context, user *models.User) (*models.LoginMFARequiredResponse, error) {
	otp, err := helpers.GenerateOTP()
	if err != nil {
		return nil, fmt.Errorf("services: generate otp: %w", err)
	}

	otpRecord := &models.MFAOtp{
		UserID:    user.ID,
		OTPHash:   helpers.HashOTP(otp),
		ExpiresAt: time.Now().Add(mfaOTPTTL),
	}
	if err := s.otpRepo.Create(ctx, otpRecord); err != nil {
		return nil, fmt.Errorf("services: store otp: %w", err)
	}

	if err := s.deliverOTP(user, otp); err != nil {
		s.log.Error("mfa otp delivery failed", "user_id", user.ID, "error", err)
		return nil, fmt.Errorf("services: deliver otp: %w", err)
	}

	mfaPendingToken, err := s.jwtManager.GenerateMFAPendingToken(user.ID)
	if err != nil {
		return nil, fmt.Errorf("services: generate mfa pending token: %w", err)
	}

	return &models.LoginMFARequiredResponse{
		MFARequired:     true,
		MFAPendingToken: mfaPendingToken,
	}, nil
}

// deliverOTP delivers the generated MFA OTP based on the user's selected delivery channel.
func (s *AuthService) deliverOTP(user *models.User, otp string) error {
	method := "email"
	if user.MFAMethod != nil {
		method = *user.MFAMethod
	}

	switch method {
	case "sms":
		if user.Mobile == nil {
			return fmt.Errorf("mfa_method is sms but user has no mobile number on file")
		}
		return s.smsSvc.SendOTP(*user.Mobile, otp)
	default:
		return s.emailSvc.SendOTP(user.Email, otp)
	}
}

// issueSession creates and stores access and refresh tokens for a successful authentication event.
func (s *AuthService) issueSession(ctx context.Context, user *models.User) (*models.LoginSuccessResponse, error) {
	accessToken, err := s.jwtManager.GenerateAccessToken(user.ID, user.Role, user.Email)
	if err != nil {
		return nil, fmt.Errorf("services: generate access token: %w", err)
	}

	rawRefreshToken, err := helpers.GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("services: generate refresh token: %w", err)
	}

	refreshRecord := &models.RefreshToken{
		UserID:    user.ID,
		TokenHash: helpers.HashRefreshToken(rawRefreshToken),
		ExpiresAt: time.Now().Add(refreshTokenTTL),
	}
	if err := s.sessionRepo.Create(ctx, refreshRecord); err != nil {
		return nil, fmt.Errorf("services: store refresh token: %w", err)
	}

	return &models.LoginSuccessResponse{
		AccessToken:  accessToken,
		RefreshToken: rawRefreshToken,
		User: models.UserSummary{
			ID:             user.ID,
			Email:          user.Email,
			Mobile:         user.Mobile,
			Role:           user.Role,
			EmailVerified:  user.EmailVerified,
			MobileVerified: user.MobileVerified,
		},
	}, nil
}
// --- Module B: Samruddhi ---

// SetPassword handles the first-time login password setup.
func (s *AuthService) SetPassword(ctx context.Context, tempToken, newPassword string) error {
	claims, err := s.jwtManager.ValidateTempToken(tempToken)
	if err != nil {
		return helpers.ErrTokenInvalid("temp_token is invalid or expired")
	}

	if err := helpers.ValidatePasswordStrength(newPassword); err != nil {
		return err
	}

	hash, err := helpers.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("services: hash new password: %w", err)
	}

	if err := s.userRepo.UpdatePasswordAndFirstLogin(ctx, claims.UserID, hash); err != nil {
		return fmt.Errorf("services: update password and first login: %w", err)
	}

	return nil
}

// SendEmailOTP generates and sends an email OTP for onboarding.
func (s *AuthService) SendEmailOTP(ctx context.Context, tempToken string) error {
	claims, err := s.jwtManager.ValidateTempToken(tempToken)
	if err != nil {
		return helpers.ErrTokenInvalid("temp_token is invalid or expired")
	}

	user, err := s.userRepo.FindByID(ctx, claims.UserID)
	if err != nil {
		return fmt.Errorf("services: find user: %w", err)
	}

	otp, err := helpers.GenerateOTP()
	if err != nil {
		return fmt.Errorf("services: generate email otp: %w", err)
	}

	if err := s.emailOTPRepo.DeleteUnused(ctx, claims.UserID); err != nil {
		return fmt.Errorf("services: delete old email otps: %w", err)
	}

	otpRecord := &models.UserEmailOTP{
		UserID:    claims.UserID,
		OTPHash:   helpers.HashOTP(otp),
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	if err := s.emailOTPRepo.Create(ctx, otpRecord); err != nil {
		return fmt.Errorf("services: store email otp: %w", err)
	}

	return s.emailSvc.SendOTP(user.Email, otp)
}

// VerifyEmailOTP verifies the email OTP.
func (s *AuthService) VerifyEmailOTP(ctx context.Context, tempToken, otp string) error {
	claims, err := s.jwtManager.ValidateTempToken(tempToken)
	if err != nil {
		return helpers.ErrTokenInvalid("temp_token is invalid or expired")
	}

	otpHash := helpers.HashOTP(otp)
	otpRecord, err := s.emailOTPRepo.FindValid(ctx, claims.UserID, otpHash)
	if err != nil {
		return fmt.Errorf("services: find email otp: %w", err)
	}
	if otpRecord == nil {
		return helpers.ErrBadRequest("Invalid or expired OTP")
	}

	if err := s.emailOTPRepo.MarkUsed(ctx, otpRecord.ID); err != nil {
		return fmt.Errorf("services: mark email otp used: %w", err)
	}

	if err := s.userRepo.UpdateEmailVerified(ctx, claims.UserID); err != nil {
		return fmt.Errorf("services: update email verified: %w", err)
	}

	return nil
}

// SendMobileOTP generates and sends a mobile OTP for onboarding.
func (s *AuthService) SendMobileOTP(ctx context.Context, tempToken string) error {
	claims, err := s.jwtManager.ValidateTempToken(tempToken)
	if err != nil {
		return helpers.ErrTokenInvalid("temp_token is invalid or expired")
	}

	user, err := s.userRepo.FindByID(ctx, claims.UserID)
	if err != nil {
		return fmt.Errorf("services: find user: %w", err)
	}

	if user.Mobile == nil {
		return helpers.ErrBadRequest("User does not have a mobile number")
	}

	otp, err := helpers.GenerateOTP()
	if err != nil {
		return fmt.Errorf("services: generate mobile otp: %w", err)
	}

	if err := s.mobileOTPRepo.DeleteUnused(ctx, claims.UserID); err != nil {
		return fmt.Errorf("services: delete old mobile otps: %w", err)
	}

	otpRecord := &models.MobileOTP{
		UserID:    claims.UserID,
		OTPHash:   helpers.HashOTP(otp),
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	if err := s.mobileOTPRepo.Create(ctx, otpRecord); err != nil {
		return fmt.Errorf("services: store mobile otp: %w", err)
	}

	return s.smsSvc.SendOTP(*user.Mobile, otp)
}

// VerifyMobileOTP verifies the mobile OTP and optionally issues the final JWT.
func (s *AuthService) VerifyMobileOTP(ctx context.Context, tempToken, otp string) (any, error) {
	claims, err := s.jwtManager.ValidateTempToken(tempToken)
	if err != nil {
		return nil, helpers.ErrTokenInvalid("temp_token is invalid or expired")
	}

	otpHash := helpers.HashOTP(otp)
	otpRecord, err := s.mobileOTPRepo.FindValid(ctx, claims.UserID, otpHash)
	if err != nil {
		return nil, fmt.Errorf("services: find mobile otp: %w", err)
	}
	if otpRecord == nil {
		return nil, helpers.ErrBadRequest("Invalid or expired OTP")
	}

	if err := s.mobileOTPRepo.MarkUsed(ctx, otpRecord.ID); err != nil {
		return nil, fmt.Errorf("services: mark mobile otp used: %w", err)
	}

	if err := s.userRepo.UpdateMobileVerified(ctx, claims.UserID); err != nil {
		return nil, fmt.Errorf("services: update mobile verified: %w", err)
	}

	user, err := s.userRepo.FindByID(ctx, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("services: fetch user after mobile verify: %w", err)
	}

	if user.EmailVerified && user.MobileVerified {
		// Both verified, issue session
		return s.issueSession(ctx, user)
	}

	// Return simple message if both are not verified yet
	return map[string]string{
		"message": "Mobile verified. Please also verify your email.",
	}, nil
}

// ForgotPassword generates a reset token and sends it.
func (s *AuthService) ForgotPassword(ctx context.Context, email string) error {
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, helpers.ErrNotFound) {
			// Do not leak existence, pretend success
			return nil
		}
		return fmt.Errorf("services: find user for forgot password: %w", err)
	}

	rawToken, err := helpers.GenerateRefreshToken() // 64 bytes hex
	if err != nil {
		return fmt.Errorf("services: generate reset token: %w", err)
	}
	tokenHash := helpers.HashRefreshToken(rawToken) // SHA256

	if err := s.forgotPasswordRepo.DeleteUnused(ctx, user.ID); err != nil {
		return fmt.Errorf("services: delete old reset tokens: %w", err)
	}

	record := &models.PasswordResetToken{
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}
	if err := s.forgotPasswordRepo.Create(ctx, record); err != nil {
		return fmt.Errorf("services: store reset token: %w", err)
	}

	resetLink := fmt.Sprintf("https://crm.pmrgsolution.com/reset-password?token=%s", rawToken)
	return s.emailSvc.SendPasswordReset(user.Email, resetLink)
}

// ResetPassword validates the reset token and updates the user's password.
func (s *AuthService) ResetPassword(ctx context.Context, token, newPassword string) error {
	tokenHash := helpers.HashRefreshToken(token)
	record, err := s.forgotPasswordRepo.FindValid(ctx, tokenHash)
	if err != nil {
		return fmt.Errorf("services: find reset token: %w", err)
	}
	if record == nil {
		return helpers.ErrBadRequest("This reset link is invalid or has expired.")
	}

	if err := helpers.ValidatePasswordStrength(newPassword); err != nil {
		return err
	}

	hash, err := helpers.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("services: hash reset password: %w", err)
	}

	if err := s.userRepo.UpdatePassword(ctx, record.UserID, hash); err != nil {
		return fmt.Errorf("services: update reset password: %w", err)
	}

	if err := s.forgotPasswordRepo.MarkUsed(ctx, record.ID); err != nil {
		return fmt.Errorf("services: mark reset token used: %w", err)
	}

	if err := s.sessionRepo.RevokeAllForUser(ctx, record.UserID); err != nil {
		return fmt.Errorf("services: revoke all sessions on password reset: %w", err)
	}

	return nil
}

// VerifyMFA validates the MFA OTP.
func (s *AuthService) VerifyMFA(ctx context.Context, mfaPendingToken, otp string) (*models.LoginSuccessResponse, error) {
	rateLimitKey := mfaPendingToken

	if blocked, retryAfter := s.rateLimiter.IsBlocked(rateLimitKey); blocked {
		return nil, helpers.ErrRateLimited(retryAfter)
	}

	claims, err := s.jwtManager.ValidateMFAPendingToken(mfaPendingToken)
	if err != nil {
		s.rateLimiter.RecordFailure(rateLimitKey)
		return nil, helpers.ErrTokenInvalid("mfa_pending_token is missing, invalid, or expired")
	}

	otpHash := helpers.HashOTP(otp)
	
	// Ensure to use FindValid from the repo handling mfa_otps.
	otpRecord, err := s.otpRepo.FindValid(ctx, claims.UserID, otpHash)
	if err != nil {
		s.rateLimiter.RecordFailure(rateLimitKey)
		return nil, fmt.Errorf("services: find mfa otp: %w", err)
	}
	if otpRecord == nil {
		s.rateLimiter.RecordFailure(rateLimitKey)
		return nil, helpers.ErrBadRequest("Invalid or expired OTP")
	}

	s.rateLimiter.Reset(rateLimitKey)

	if err := s.otpRepo.MarkUsed(ctx, otpRecord.ID); err != nil {
		return nil, fmt.Errorf("services: mark mfa otp used: %w", err)
	}

	user, err := s.userRepo.FindByID(ctx, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("services: fetch user after mfa verify: %w", err)
	}

	return s.issueSession(ctx, user)
}

