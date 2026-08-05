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
	userRepo    repository.UserRepository
	sessionRepo repository.SessionRepository
	otpRepo     repository.UserEmailOTPRepository
	jwtManager  *helpers.JWTManager
	emailSvc    helpers.EmailService
	smsSvc      helpers.SMSService
	rateLimiter *middleware.RateLimiter
	log         *slog.Logger
}

// NewAuthService constructs a new instance of AuthService.
func NewAuthService(
	userRepo repository.UserRepository,
	sessionRepo repository.SessionRepository,
	otpRepo repository.UserEmailOTPRepository,
	jwtManager *helpers.JWTManager,
	emailSvc helpers.EmailService,
	smsSvc helpers.SMSService,
	rateLimiter *middleware.RateLimiter,
	log *slog.Logger,
) *AuthService {
	return &AuthService{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		otpRepo:     otpRepo,
		jwtManager:  jwtManager,
		emailSvc:    emailSvc,
		smsSvc:      smsSvc,
		rateLimiter: rateLimiter,
		log:         log,
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