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

type AuthService struct {
	userRepo       repository.UserRepository
	sessionRepo    repository.SessionRepository
	otpRepo        repository.UserEmailOTPRepository
	oauthStateRepo repository.OAuthStateRepository
	jwtManager     *helpers.JWTManager
	emailSvc       helpers.EmailService
	smsSvc         helpers.SMSService
	rateLimiter    *middleware.RateLimiter
	log            *slog.Logger
}

func NewAuthService(
	userRepo repository.UserRepository,
	sessionRepo repository.SessionRepository,
	otpRepo repository.UserEmailOTPRepository,
	oauthStateRepo repository.OAuthStateRepository,
	jwtManager *helpers.JWTManager,
	emailSvc helpers.EmailService,
	smsSvc helpers.SMSService,
	rateLimiter *middleware.RateLimiter,
	log *slog.Logger,
) *AuthService {
	return &AuthService{
		userRepo:       userRepo,
		sessionRepo:    sessionRepo,
		otpRepo:        otpRepo,
		oauthStateRepo: oauthStateRepo,
		jwtManager:     jwtManager,
		emailSvc:       emailSvc,
		smsSvc:         smsSvc,
		rateLimiter:    rateLimiter,
		log:            log,
	}
}

func (s *AuthService) Login(ctx context.Context, req models.LoginRequest, clientIP string) (any, error) {
	rateLimitKey := req.Identifier + "|" + clientIP

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
			s.rateLimiter.RecordFailure(rateLimitKey)
			return nil, helpers.ErrInvalidCredentials()
		}
		return nil, fmt.Errorf("services: find user: %w", err)
	}

	if !helpers.ComparePassword(req.Password, user.PasswordHash) {
		s.rateLimiter.RecordFailure(rateLimitKey)
		return nil, helpers.ErrInvalidCredentials()
	}

	s.rateLimiter.Reset(rateLimitKey)

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

	if user.MFAEnabled {
		return s.triggerMFA(ctx, user)
	}

	return s.issueSession(ctx, user)
}

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

func (s *AuthService) Refresh(ctx context.Context, req models.RefreshRequest) (*models.RefreshResponse, error) {
	tokenHash := helpers.HashRefreshToken(req.RefreshToken)

	tokenRecord, err := s.sessionRepo.FindActiveByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, helpers.ErrNotFound) {
			return nil, helpers.ErrTokenInvalid("Invalid or expired token")
		}
		return nil, fmt.Errorf("services: find refresh token: %w", err)
	}

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
	// Single Tenant Default Tenant ID: a58ed82f-bd29-465a-85aa-8352e1f17715
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

	// Single Tenant Default Tenant ID: a58ed82f-bd29-465a-85aa-8352e1f17715
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
