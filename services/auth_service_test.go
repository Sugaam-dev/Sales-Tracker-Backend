package services

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"crm-auth-service/helpers"
	"crm-auth-service/models"
)

// mockOAuthStateRepo implements repository.OAuthStateRepository
type mockOAuthStateRepo struct {
	verifyFunc func(ctx context.Context, stateToken string) (*models.OAuthState, error)
}

func (m *mockOAuthStateRepo) SaveOAuthState(ctx context.Context, state *models.OAuthState) error {
	return nil
}

func (m *mockOAuthStateRepo) VerifyOAuthState(ctx context.Context, stateToken string) (*models.OAuthState, error) {
	return m.verifyFunc(ctx, stateToken)
}

// mockUserRepo implements repository.UserRepository
type mockUserRepo struct{}

func (m *mockUserRepo) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	return nil, nil
}
func (m *mockUserRepo) FindByMobile(ctx context.Context, mobile string) (*models.User, error) {
	return nil, nil
}
func (m *mockUserRepo) FindByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	return nil, nil
}
func (m *mockUserRepo) FindBySSOID(ctx context.Context, provider, subjectID string) (*models.User, error) {
	return nil, nil
}
func (m *mockUserRepo) Create(ctx context.Context, user *models.User) error {
	return nil
}
func (m *mockUserRepo) Update(ctx context.Context, user *models.User) error {
	return nil
}
func (m *mockUserRepo) UpdatePasswordAndFirstLogin(ctx context.Context, id uuid.UUID, newPasswordHash string) error {
	return nil
}
func (m *mockUserRepo) UpdateEmailVerified(ctx context.Context, id uuid.UUID) error {
	return nil
}
func (m *mockUserRepo) UpdateMobileVerified(ctx context.Context, id uuid.UUID) error {
	return nil
}
func (m *mockUserRepo) UpdatePassword(ctx context.Context, id uuid.UUID, newPasswordHash string) error {
	return nil
}
func (m *mockUserRepo) FindActiveUsers(ctx context.Context) ([]*models.User, error) {
	return nil, nil
}
func (m *mockUserRepo) FindAllUsers(ctx context.Context) ([]*models.User, error) {
	return nil, nil
}
func (m *mockUserRepo) GetManagedExecutiveIDs(ctx context.Context, managerID uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}
func (m *mockUserRepo) GetManagedExecutives(ctx context.Context, managerID uuid.UUID) ([]*models.User, error) {
	return nil, nil
}
func (m *mockUserRepo) GetLeaderPermissions(ctx context.Context, leaderID uuid.UUID) ([]string, error) {
	return nil, nil
}
func (m *mockUserRepo) GrantLeaderPermission(ctx context.Context, leaderID uuid.UUID, permission string, grantedBy *uuid.UUID) error {
	return nil
}
func (m *mockUserRepo) RevokeLeaderPermission(ctx context.Context, leaderID uuid.UUID, permission string) error {
	return nil
}
func (m *mockUserRepo) AssignExecutiveToManager(ctx context.Context, executiveID uuid.UUID, managerID *uuid.UUID) error {
	return nil
}
func (m *mockUserRepo) FindUsersScoped(ctx context.Context, scope helpers.DataScope) ([]*models.User, error) {
	return nil, nil
}
func (m *mockUserRepo) UpdateUserDetails(ctx context.Context, id uuid.UUID, name, email, role string, isActive bool, managerID *uuid.UUID) error {
	return nil
}
func (m *mockUserRepo) DeleteUser(ctx context.Context, id uuid.UUID) error {
	return nil
}

// mockRoundTripper intercepts HTTP requests for tests
type mockRoundTripper func(req *http.Request) (*http.Response, error)

func (f mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestSSOCallback_ErrorHandling(t *testing.T) {
	// Set environment variables for test
	os.Setenv("AZURE_CLIENT_ID", "test-client-id")
	os.Setenv("AZURE_CLIENT_SECRET", "super-secret-client-key-12345")
	os.Setenv("SSO_CALLBACK_URL", "http://localhost:8080/callback")
	defer os.Unsetenv("AZURE_CLIENT_ID")
	defer os.Unsetenv("AZURE_CLIENT_SECRET")
	defer os.Unsetenv("SSO_CALLBACK_URL")

	// Captured logs
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	// Mock HTTP Transport
	originalTransport := http.DefaultClient.Transport
	defer func() { http.DefaultClient.Transport = originalTransport }()

	// Sample OAuth error response with sensitive fields we want to make sure are redacted
	mockResponseBody := `{
		"error": "invalid_grant",
		"error_description": "AADSTS70000: The provided value for the 'code' parameter 'auth-code-xyz' is not valid.",
		"client_secret": "super-secret-client-key-12345",
		"access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature",
		"cookie": "session_cookie=sensitive_val"
	}`

	http.DefaultClient.Transport = mockRoundTripper(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(mockResponseBody)),
			Header:     make(http.Header),
		}, nil
	})

	stateRepo := &mockOAuthStateRepo{
		verifyFunc: func(ctx context.Context, stateToken string) (*models.OAuthState, error) {
			return &models.OAuthState{
				State:     "test-state",
				Provider:  "microsoft",
				ExpiresAt: time.Now().Add(10 * time.Minute),
			}, nil
		},
	}

	authService := NewAuthService(
		&mockUserRepo{},
		nil, // sessionRepo
		nil, // otpRepo
		nil, // emailOTPRepo
		nil, // mobileOTPRepo
		nil, // forgotPasswordRepo
		stateRepo,
		nil, // jwtManager
		nil, // emailSvc
		nil, // smsSvc
		nil, // rateLimiter
		logger,
	)

	_, err := authService.SSOCallback(context.Background(), "microsoft", "auth-code-xyz", "test-state")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Verify the error contains the sanitized message details
	errMsg := err.Error()
	if !strings.Contains(errMsg, "invalid_grant") {
		t.Errorf("expected error message to contain invalid_grant, got: %s", errMsg)
	}
	if strings.Contains(errMsg, "auth-code-xyz") {
		t.Errorf("error message leaked auth code: %s", errMsg)
	}
	if strings.Contains(errMsg, "super-secret-client-key-12345") {
		t.Errorf("error message leaked client secret: %s", errMsg)
	}

	// Verify logs
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "Microsoft SSO token exchange failed") {
		t.Errorf("expected log to contain failure message, got: %s", logOutput)
	}

	// Assertions to verify sensitive fields are NOT in the log
	sensitiveItems := []string{
		"super-secret-client-key-12345",
		"auth-code-xyz",
		"session_cookie=sensitive_val",
		"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature",
	}

	for _, item := range sensitiveItems {
		if strings.Contains(logOutput, item) {
			t.Errorf("Log leaked sensitive item: %q\nFull Log:\n%s", item, logOutput)
		}
	}

	// Assertions to verify standard fields ARE logged
	if !strings.Contains(logOutput, "invalid_grant") {
		t.Errorf("expected log to contain 'invalid_grant', got: %s", logOutput)
	}
}

func TestBuildSSORedirectURL(t *testing.T) {
	os.Setenv("AZURE_CLIENT_ID", "test-client-id")
	os.Setenv("AZURE_TENANT_ID", "test-tenant-id")
	os.Setenv("SSO_CALLBACK_URL", "http://localhost:8080/callback")
	defer os.Unsetenv("AZURE_CLIENT_ID")
	defer os.Unsetenv("AZURE_TENANT_ID")
	defer os.Unsetenv("SSO_CALLBACK_URL")

	stateRepo := &mockOAuthStateRepo{}

	authService := NewAuthService(
		&mockUserRepo{},
		nil, // sessionRepo
		nil, // otpRepo
		nil, // emailOTPRepo
		nil, // mobileOTPRepo
		nil, // forgotPasswordRepo
		stateRepo,
		nil, // jwtManager
		nil, // emailSvc
		nil, // smsSvc
		nil, // rateLimiter
		slog.Default(),
	)

	urlStr, err := authService.BuildSSORedirectURL(context.Background(), "microsoft")
	if err != nil {
		t.Fatalf("failed to build redirect URL: %v", err)
	}

	if !strings.Contains(urlStr, "prompt=select_account") {
		t.Errorf("expected URL to contain 'prompt=select_account', got: %s", urlStr)
	}
}
