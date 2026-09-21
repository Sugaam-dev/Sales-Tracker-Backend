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
func (m *mockUserRepo) GetAllLeaderPermissions(ctx context.Context) (map[uuid.UUID][]string, error) {
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
func (m *mockUserRepo) FindUsersByRoleScope(ctx context.Context, callerID uuid.UUID, callerRole string) ([]*models.User, error) {
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

type mockUserRepoForStatus struct {
	mockUserRepo
	users map[uuid.UUID]*models.User
}

func (m *mockUserRepoForStatus) FindAllUsers(ctx context.Context) ([]*models.User, error) {
	var result []*models.User
	for _, u := range m.users {
		result = append(result, u)
	}
	return result, nil
}

func (m *mockUserRepoForStatus) FindByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, helpers.ErrNotFound
	}
	return u, nil
}

func (m *mockUserRepoForStatus) UpdateUserDetails(ctx context.Context, id uuid.UUID, name, email, role string, isActive bool, managerID *uuid.UUID) error {
	u, ok := m.users[id]
	if !ok {
		return helpers.ErrNotFound
	}
	u.Name = name
	u.Email = email
	u.Role = role
	u.IsActive = isActive
	u.ManagerID = managerID
	return nil
}

func (m *mockUserRepoForStatus) GetAllLeaderPermissions(ctx context.Context) (map[uuid.UUID][]string, error) {
	return nil, nil
}

func TestUserStatus_ListUsers_ReturnsCorrectStatus(t *testing.T) {
	userActiveID := uuid.New()
	userInactiveID := uuid.New()

	repo := &mockUserRepoForStatus{
		users: map[uuid.UUID]*models.User{
			userActiveID: {
				ID:       userActiveID,
				Name:     "Active User",
				Email:    "active@example.com",
				Role:     "sales_manager",
				IsActive: true,
			},
			userInactiveID: {
				ID:       userInactiveID,
				Name:     "Inactive User",
				Email:    "inactive@example.com",
				Role:     "sales_executive",
				IsActive: false,
			},
		},
	}

	service := NewAuthService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, slog.Default())

	// TEST 1 & TEST 2: ListUsers returns true for active user and false for inactive user
	summaries, err := service.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}

	if len(summaries) != 2 {
		t.Fatalf("expected 2 user summaries, got %d", len(summaries))
	}

	foundActive := false
	foundInactive := false

	for _, s := range summaries {
		if s.ID == userActiveID {
			foundActive = true
			if !s.IsActive {
				t.Errorf("TEST 1 FAILED: expected is_active=true for active user, got false")
			}
		}
		if s.ID == userInactiveID {
			foundInactive = true
			if s.IsActive {
				t.Errorf("TEST 2 FAILED: expected is_active=false for inactive user, got true")
			}
		}
	}

	if !foundActive || !foundInactive {
		t.Errorf("failed to find all test users in ListUsers summary")
	}
}

func TestUserStatus_UpdateUser_PersistsAndReturnsCorrectStatus(t *testing.T) {
	userID := uuid.New()
	managerID := uuid.New()

	repo := &mockUserRepoForStatus{
		users: map[uuid.UUID]*models.User{
			managerID: {
				ID:       managerID,
				Name:     "Manager User",
				Email:    "manager@example.com",
				Role:     "sales_manager",
				IsActive: true,
			},
			userID: {
				ID:       userID,
				Name:     "Initial User",
				Email:    "initial@example.com",
				Role:     "sales_executive",
				IsActive: true,
			},
		},
	}

	service := NewAuthService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, slog.Default())

	// TEST 4 & TEST 5: Update user to is_active = false
	respInactive, err := service.UpdateUser(context.Background(), userID, "Updated User", "updated@example.com", "sales_executive", false, &managerID)
	if err != nil {
		t.Fatalf("UpdateUser failed: %v", err)
	}

	// TEST 5: Verify UpdateUser response contains is_active = false
	if respInactive.IsActive != false {
		t.Errorf("TEST 5 FAILED: expected UpdateUser response IsActive=false, got %v", respInactive.IsActive)
	}

	// TEST 4: Fetch user via ListUsers and verify is_active = false
	summaries, _ := service.ListUsers(context.Background())
	for _, s := range summaries {
		if s.ID == userID && s.IsActive != false {
			t.Errorf("TEST 4 FAILED: expected ListUsers to return IsActive=false after update, got true")
		}
	}

	// TEST 3 & TEST 6: Update user to is_active = true, and verify name/email/role/manager_id
	respActive, err := service.UpdateUser(context.Background(), userID, "Final Name", "final@example.com", "sales_executive", true, &managerID)
	if err != nil {
		t.Fatalf("UpdateUser failed: %v", err)
	}

	// TEST 5: Verify UpdateUser response contains is_active = true
	if respActive.IsActive != true {
		t.Errorf("TEST 5 FAILED: expected UpdateUser response IsActive=true, got %v", respActive.IsActive)
	}

	// TEST 6: Verify name/email/role/manager_id updated correctly
	if respActive.Name != "Final Name" || respActive.Email != "final@example.com" || respActive.Role != "sales_executive" || respActive.ManagerID == nil || *respActive.ManagerID != managerID {
		t.Errorf("TEST 6 FAILED: user profile fields were not preserved/updated properly: %+v", respActive)
	}

	// TEST 3: Fetch user via ListUsers and verify is_active = true
	summariesActive, _ := service.ListUsers(context.Background())
	for _, s := range summariesActive {
		if s.ID == userID && s.IsActive != true {
			t.Errorf("TEST 3 FAILED: expected ListUsers to return IsActive=true after update, got false")
		}
	}
}

