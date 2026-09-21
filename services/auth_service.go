// Package services holds business logic.
package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

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
	oauthStateRepo     repository.OAuthStateRepository
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
	oauthStateRepo repository.OAuthStateRepository,
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
		oauthStateRepo:     oauthStateRepo,
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

	// If it is the user's first login or password change is required, route them to onboarding.
	if user.IsFirstLogin || user.PasswordChangeRequired {
		accessToken, err := s.jwtManager.GenerateAccessToken(user.ID, user.Role, user.Email)
		if err != nil {
			return nil, fmt.Errorf("services: generate access token: %w", err)
		}
		return &models.LoginFirstTimeResponse{
			AccessToken:            accessToken,
			TokenType:              "Bearer",
			ExpiresIn:              900,
			FirstTimeLogin:         true,
			PasswordChangeRequired: true,
			Message:                "Password change required before proceeding",
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

	var perms []string
	if user.Role == models.RoleLeader {
		perms, _ = s.userRepo.GetLeaderPermissions(ctx, user.ID)
	}

	var managerName *string
	if user.ManagerID != nil {
		if m, err := s.userRepo.FindByID(ctx, *user.ManagerID); err == nil && m != nil {
			managerName = &m.Name
		}
	}

	return &models.LoginSuccessResponse{
		AccessToken:  accessToken,
		RefreshToken: rawRefreshToken,
		User: models.UserSummary{
			ID:             user.ID,
			Name:           user.Name,
			Email:          user.Email,
			Mobile:         user.Mobile,
			Role:           user.Role,
			ManagerID:      user.ManagerID,
			ManagerName:    managerName,
			Permissions:    perms,
			EmailVerified:  user.EmailVerified,
			MobileVerified: user.MobileVerified,
			IsActive:       user.IsActive,
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

// --- SSO & MFA Management Custom Methods (User Added) ---

func (s *AuthService) BuildSSORedirectURL(ctx context.Context, provider string) (string, error) {
	if provider == "" {
		provider = "microsoft"
	}
	if provider != "azure" && provider != "microsoft" {
		return "", helpers.ErrBadRequest("Unsupported SSO provider")
	}

	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", err
	}
	stateToken := hex.EncodeToString(stateBytes)

	err := s.oauthStateRepo.SaveOAuthState(ctx, &models.OAuthState{
		State:     stateToken,
		Provider:  provider,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	})
	if err != nil {
		return "", err
	}

	clientID := os.Getenv("AZURE_CLIENT_ID")
	redirectURI := os.Getenv("SSO_CALLBACK_URL")
	if redirectURI == "" {
		redirectURI = "http://localhost:8080/api/v1/auth/sso/callback"
	}
	tenantID := os.Getenv("AZURE_TENANT_ID")
	if tenantID == "" {
		tenantID = "a58ed82f-bd29-465a-85aa-8352e1f17715"
	}
	authURL := fmt.Sprintf(
		"https://login.microsoftonline.com/%s/oauth2/v2.0/authorize?client_id=%s&redirect_uri=%s&response_type=code&scope=openid%%20profile%%20email&state=%s&prompt=select_account",
		url.QueryEscape(tenantID),
		url.QueryEscape(clientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(stateToken),
	)

	return authURL, nil
}

func (s *AuthService) SSOCallback(ctx context.Context, provider, code, state string) (*models.LoginSuccessResponse, error) {
	if provider == "" {
		provider = "microsoft"
	}
	stateRecord, err := s.oauthStateRepo.VerifyOAuthState(ctx, state)
	if err != nil {
		return nil, helpers.ErrTokenInvalid("Invalid or expired OAuth state")
	}

	provider = stateRecord.Provider

	if provider != "azure" && provider != "microsoft" {
		return nil, helpers.ErrBadRequest("Unsupported SSO provider")
	}

	form := url.Values{}
	form.Set("code", code)
	form.Set("grant_type", "authorization_code")

	redirectURI := os.Getenv("SSO_CALLBACK_URL")
	if redirectURI == "" {
		redirectURI = "http://localhost:8080/api/v1/auth/sso/callback"
	}

	tenantID := os.Getenv("AZURE_TENANT_ID")
	if tenantID == "" {
		tenantID = "a58ed82f-bd29-465a-85aa-8352e1f17715"
	}
	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenantID)
	form.Set("client_id", os.Getenv("AZURE_CLIENT_ID"))
	form.Set("client_secret", os.Getenv("AZURE_CLIENT_SECRET"))
	form.Set("redirect_uri", redirectURI)

	resp, err := http.PostForm(tokenURL, form)
	if err != nil {
		return nil, fmt.Errorf("SSO token exchange error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		var oauthErr struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		_ = json.Unmarshal(bodyBytes, &oauthErr)

		clientSecret := os.Getenv("AZURE_CLIENT_SECRET")
		sanitizedBody := helpers.SanitizeString(string(bodyBytes), clientSecret, code)
		sanitizedOAuthErr := helpers.SanitizeString(oauthErr.Error, clientSecret, code)
		sanitizedOAuthDesc := helpers.SanitizeString(oauthErr.ErrorDescription, clientSecret, code)

		s.log.Error("Microsoft SSO token exchange failed",
			"status_code", resp.StatusCode,
			"response_body", sanitizedBody,
			"oauth_error", sanitizedOAuthErr,
			"oauth_error_description", sanitizedOAuthDesc,
		)

		return nil, fmt.Errorf("SSO token exchange failed: status=%d, error=%s, description=%s", resp.StatusCode, sanitizedOAuthErr, sanitizedOAuthDesc)
	}

	var tokenResp struct {
		IDToken string `json:"id_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode SSO tokens: %w", err)
	}

	claims := jwt.MapClaims{}
	parser := jwt.NewParser()
	_, _, err = parser.ParseUnverified(tokenResp.IDToken, &claims)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ID token: %w", err)
	}

	email, _ := claims["email"].(string)
	sub, _ := claims["sub"].(string)

	if email == "" {
		if pref, ok := claims["preferred_username"].(string); ok {
			email = pref
		} else if uniq, ok := claims["unique_name"].(string); ok {
			email = uniq
		}
	}

	if email == "" || sub == "" {
		return nil, helpers.ErrBadRequest("SSO response missing email or subject ID")
	}

	user, err := s.userRepo.FindBySSOID(ctx, provider, sub)
	if err != nil {
		if errors.Is(err, helpers.ErrNotFound) {
			user, err = s.userRepo.FindByEmail(ctx, email)
			if err != nil {
				if errors.Is(err, helpers.ErrNotFound) {
					user = &models.User{
						Email:         email,
						Role:          "agent",
						IsFirstLogin:  false,
						EmailVerified: true,
						SSOProvider:   &provider,
						SSOSubjectID:  &sub,
						PasswordHash:  "sso-placeholder",
					}
					if err := s.userRepo.Create(ctx, user); err != nil {
						return nil, fmt.Errorf("failed to create SSO user: %w", err)
					}
				} else {
					return nil, err
				}
			} else {
				user.SSOProvider = &provider
				user.SSOSubjectID = &sub
				if err := s.userRepo.Update(ctx, user); err != nil {
					return nil, fmt.Errorf("failed to update user SSO info: %w", err)
				}
			}
		} else {
			return nil, err
		}
	}

	return s.issueSession(ctx, user)
}

func (s *AuthService) EnableMFA(ctx context.Context, userID uuid.UUID, method string) error {
	if method != "email" && method != "sms" {
		return helpers.ErrBadRequest("Invalid MFA method. Allowed methods: email, sms")
	}

	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}

	if user.SSOProvider != nil && *user.SSOProvider != "" {
		return helpers.ErrBadRequest("SSO users cannot enable MFA")
	}

	user.MFAEnabled = true
	user.MFAMethod = &method
	return s.userRepo.Update(ctx, user)
}

func (s *AuthService) DisableMFA(ctx context.Context, userID uuid.UUID) error {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}

	user.MFAEnabled = false
	user.MFAMethod = nil
	return s.userRepo.Update(ctx, user)
}

func (s *AuthService) CreateUser(ctx context.Context, name, email, mobile, password, role string, managerID *uuid.UUID) (*models.CreateUserResponse, error) {
	if !models.IsValidRole(role) {
		return nil, helpers.ErrBadRequest("Invalid role value")
	}

	// Validate manager assignment based on role
	var managerName string
	if role == models.RoleSalesExecutive {
		if managerID == nil {
			return nil, helpers.ErrBadRequest("manager_id is required for sales_executive")
		}
		mgr, err := s.userRepo.FindByID(ctx, *managerID)
		if err != nil || mgr == nil {
			return nil, helpers.ErrBadRequest("assigned manager does not exist")
		}
		if !mgr.IsActive {
			return nil, helpers.ErrBadRequest("assigned manager is inactive")
		}
		if mgr.Role != models.RoleSalesManager {
			return nil, helpers.ErrBadRequest("assigned manager must have sales_manager role")
		}
		managerName = mgr.Name
	} else {
		// Manager assignment is not applicable for Admin, Leader, or Sales Manager
		managerID = nil
		managerName = "Not applicable"
	}

	// Check if email already exists
	existingUser, err := s.userRepo.FindByEmail(ctx, email)
	if err == nil && existingUser != nil {
		return nil, helpers.ErrConflict("Email address is already registered")
	}

	// Check if mobile already exists
	if mobile != "" {
		existingMobile, err := s.userRepo.FindByMobile(ctx, mobile)
		if err == nil && existingMobile != nil {
			return nil, helpers.ErrConflict("Mobile number is already registered")
		}
	}

	// Hash initial temporary password
	passwordHash, err := helpers.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("services: hash password: %w", err)
	}

	var mobilePtr *string
	if mobile != "" {
		mobilePtr = &mobile
	}

	user := &models.User{
		Name:                   name,
		Email:                  email,
		Mobile:                 mobilePtr,
		PasswordHash:           passwordHash,
		Role:                   role,
		ManagerID:              managerID,
		IsFirstLogin:           true,
		PasswordChangeRequired: true,
		IsActive:               true,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("services: create user: %w", err)
	}

	// Send welcome email immediately with temporary credentials
	displayRole := formatRoleDisplay(role)
	emailErr := s.emailSvc.SendWelcomeEmail(email, name, password, displayRole, managerName)
	emailSent := true
	message := "User created successfully and welcome email sent."
	if emailErr != nil {
		// Log technical email error without exposing plaintext password
		s.log.Error("welcome email delivery failed", "user_id", user.ID, "email", email, "error", emailErr)
		emailSent = false
		message = "User created, but welcome email failed."
	}

	return &models.CreateUserResponse{
		Success:   true,
		Message:   message,
		EmailSent: emailSent,
		User: &models.AdminCreatedUser{
			ID:                     user.ID,
			Name:                   user.Name,
			Email:                  user.Email,
			Role:                   user.Role,
			IsFirstLogin:           user.IsFirstLogin,
			PasswordChangeRequired: user.PasswordChangeRequired,
		},
	}, nil
}

// ChangePassword changes the authenticated user's password and clears temporary flags.
func (s *AuthService) ChangePassword(ctx context.Context, userID uuid.UUID, newPassword string) error {
	if err := helpers.ValidatePasswordStrength(newPassword); err != nil {
		return err
	}

	hash, err := helpers.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("services: hash new password: %w", err)
	}

	if err := s.userRepo.UpdatePassword(ctx, userID, hash); err != nil {
		return fmt.Errorf("services: update password: %w", err)
	}

	// Revoke existing sessions to prevent reuse of old tokens
	if err := s.sessionRepo.RevokeAllForUser(ctx, userID); err != nil {
		s.log.Warn("failed to revoke old sessions on password change", "user_id", userID, "error", err)
	}

	return nil
}

func formatRoleDisplay(role string) string {
	switch role {
	case models.RoleAdmin:
		return "Admin"
	case models.RoleSalesManager:
		return "Sales Manager"
	case models.RoleSalesExecutive:
		return "Sales Executive"
	case models.RoleLeader:
		return "Leader"
	default:
		return role
	}
}

func (s *AuthService) Logout(ctx context.Context, rawRefreshToken string, currentUserID uuid.UUID) error {
	tokenHash := helpers.HashRefreshToken(rawRefreshToken)
	tokenRecord, err := s.sessionRepo.FindActiveByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, helpers.ErrNotFound) {
			return helpers.ErrTokenInvalid("Invalid or expired refresh token")
		}
		return fmt.Errorf("services: find refresh token: %w", err)
	}

	if tokenRecord.UserID != currentUserID {
		return helpers.ErrForbidden("You do not have permission to revoke this session")
	}

	if err := s.sessionRepo.Revoke(ctx, tokenRecord.ID); err != nil {
		return fmt.Errorf("services: revoke session: %w", err)
	}
	return nil
}

// ListUsers returns all users in the system with their manager info and permissions.
func (s *AuthService) ListUsers(ctx context.Context) ([]*models.UserSummary, error) {
	users, err := s.userRepo.FindAllUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("services: list users: %w", err)
	}

	userMap := make(map[uuid.UUID]*models.User, len(users))
	for _, u := range users {
		userMap[u.ID] = u
	}

	leaderPermsMap, err := s.userRepo.GetAllLeaderPermissions(ctx)
	if err != nil {
		s.log.Warn("failed to bulk fetch leader permissions, proceeding with empty permissions", "error", err)
		leaderPermsMap = make(map[uuid.UUID][]string)
	}

	summaries := make([]*models.UserSummary, 0, len(users))
	for _, u := range users {
		summary := &models.UserSummary{
			ID:             u.ID,
			Name:           u.Name,
			Email:          u.Email,
			Mobile:         u.Mobile,
			Role:           u.Role,
			EmailVerified:  u.EmailVerified,
			MobileVerified: u.MobileVerified,
			ManagerID:      u.ManagerID,
			IsActive:       u.IsActive,
			Permissions:    make([]string, 0),
		}

		if u.ManagerID != nil {
			if mgr, ok := userMap[*u.ManagerID]; ok && mgr != nil {
				summary.ManagerName = &mgr.Name
			}
		}

		if u.Role == models.RoleLeader {
			if perms, ok := leaderPermsMap[u.ID]; ok && perms != nil {
				summary.Permissions = perms
			}
		}

		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// UpdateUser updates user details such as name, email, role, is_active, manager_id.
func (s *AuthService) UpdateUser(ctx context.Context, id uuid.UUID, name, email, role string, isActive bool, managerID *uuid.UUID) (*models.UserSummary, error) {
	if !models.IsValidRole(role) {
		return nil, helpers.ErrBadRequest("invalid role specified")
	}

	// If managerID provided, verify manager exists and is a sales_manager
	if managerID != nil {
		mgr, err := s.userRepo.FindByID(ctx, *managerID)
		if err != nil || mgr == nil {
			return nil, helpers.ErrBadRequest("assigned manager does not exist")
		}
		if mgr.Role != models.RoleSalesManager {
			return nil, helpers.ErrBadRequest("assigned manager must have role sales_manager")
		}
	}

	err := s.userRepo.UpdateUserDetails(ctx, id, name, email, role, isActive, managerID)
	if err != nil {
		return nil, fmt.Errorf("services: update user: %w", err)
	}

	updated, err := s.userRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("services: find updated user: %w", err)
	}

	summary := &models.UserSummary{
		ID:             updated.ID,
		Name:           updated.Name,
		Email:          updated.Email,
		Mobile:         updated.Mobile,
		Role:           updated.Role,
		EmailVerified:  updated.EmailVerified,
		MobileVerified: updated.MobileVerified,
		ManagerID:      updated.ManagerID,
		IsActive:       updated.IsActive,
		Permissions:    make([]string, 0),
	}

	if updated.ManagerID != nil {
		if mgr, err := s.userRepo.FindByID(ctx, *updated.ManagerID); err == nil && mgr != nil {
			summary.ManagerName = &mgr.Name
		}
	}

	if updated.Role == models.RoleLeader {
		if perms, err := s.userRepo.GetLeaderPermissions(ctx, updated.ID); err == nil {
			summary.Permissions = perms
		}
	}

	return summary, nil
}

// DeleteUser deletes or deactivates a user.
func (s *AuthService) DeleteUser(ctx context.Context, id uuid.UUID) error {
	return s.userRepo.DeleteUser(ctx, id)
}

// AssignManager links an executive to a manager.
func (s *AuthService) AssignManager(ctx context.Context, executiveID uuid.UUID, managerID *uuid.UUID) error {
	exec, err := s.userRepo.FindByID(ctx, executiveID)
	if err != nil || exec == nil {
		return helpers.ErrNotFound
	}
	if exec.Role != models.RoleSalesExecutive {
		return helpers.ErrBadRequest("can only assign managers to sales_executives")
	}

	if managerID != nil {
		mgr, err := s.userRepo.FindByID(ctx, *managerID)
		if err != nil || mgr == nil {
			return helpers.ErrBadRequest("manager does not exist")
		}
		if mgr.Role != models.RoleSalesManager {
			return helpers.ErrBadRequest("assigned manager must have sales_manager role")
		}
	}

	return s.userRepo.AssignExecutiveToManager(ctx, executiveID, managerID)
}

// GetLeaderPermissions returns the active permissions granted to a leader.
func (s *AuthService) GetLeaderPermissions(ctx context.Context, leaderID uuid.UUID) ([]string, error) {
	leader, err := s.userRepo.FindByID(ctx, leaderID)
	if err != nil || leader == nil {
		return nil, helpers.ErrNotFound
	}
	if leader.Role != models.RoleLeader {
		return nil, helpers.ErrBadRequest("user is not a leader")
	}

	return s.userRepo.GetLeaderPermissions(ctx, leaderID)
}

// GrantLeaderPermission grants an administrative permission to a leader.
func (s *AuthService) GrantLeaderPermission(ctx context.Context, leaderID uuid.UUID, permission string, grantedBy uuid.UUID) error {
	if !models.IsValidPermission(permission) {
		return helpers.ErrBadRequest("invalid permission: " + permission)
	}

	leader, err := s.userRepo.FindByID(ctx, leaderID)
	if err != nil || leader == nil {
		return helpers.ErrNotFound
	}
	if leader.Role != models.RoleLeader {
		return helpers.ErrBadRequest("can only grant permissions to leaders")
	}

	return s.userRepo.GrantLeaderPermission(ctx, leaderID, permission, &grantedBy)
}

// RevokeLeaderPermission removes an administrative permission from a leader.
func (s *AuthService) RevokeLeaderPermission(ctx context.Context, leaderID uuid.UUID, permission string) error {
	if !models.IsValidPermission(permission) {
		return helpers.ErrBadRequest("invalid permission: " + permission)
	}

	return s.userRepo.RevokeLeaderPermission(ctx, leaderID, permission)
}

// GetMyPermissions returns current user's permissions.
func (s *AuthService) GetMyPermissions(ctx context.Context, userID uuid.UUID, role string) ([]string, error) {
	if role == models.RoleAdmin {
		return []string{
			models.PermissionUserView,
			models.PermissionUserCreate,
			models.PermissionUserUpdate,
			models.PermissionUserDelete,
			models.PermissionManagerManage,
			models.PermissionSystemSettingsManage,
		}, nil
	}
	if role == models.RoleLeader {
		return s.userRepo.GetLeaderPermissions(ctx, userID)
	}
	return []string{}, nil
}
