package services_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/joho/godotenv"

	"crm-auth-service/conf"
	"crm-auth-service/controllers"
	"crm-auth-service/helpers"
	"crm-auth-service/middleware"
	"crm-auth-service/models"
	"crm-auth-service/repository"
	"crm-auth-service/services"
)

// mockRecordingEmailService records welcome emails sent and allows simulating failures
type mockRecordingEmailService struct {
	sentEmails []recordedWelcomeEmail
	failNext   bool
}

type recordedWelcomeEmail struct {
	toEmail         string
	name            string
	initialPassword string
	role            string
	managerName     string
}

func (m *mockRecordingEmailService) SendOTP(toEmail, otp string) error {
	return nil
}

func (m *mockRecordingEmailService) SendPasswordReset(toEmail, resetLink string) error {
	return nil
}

func (m *mockRecordingEmailService) SendWelcomeEmail(toEmail, name, initialPassword, role, managerName string) error {
	if m.failNext {
		return errors.New("smtp connection timeout")
	}
	m.sentEmails = append(m.sentEmails, recordedWelcomeEmail{
		toEmail:         toEmail,
		name:            name,
		initialPassword: initialPassword,
		role:            role,
		managerName:     managerName,
	})
	return nil
}

func (m *mockRecordingEmailService) SendLeadCreated(toEmail, leadName, company, leadID, ownerName string) error {
	return nil
}

func (m *mockRecordingEmailService) SendLeadAssigned(toEmail, assigneeName, leadID, company, assignedByName string) error {
	return nil
}

func (m *mockRecordingEmailService) SendLeadStatusChanged(toEmail, recipientName, leadID, company, oldStatus, newStatus, oldStage, newStage string) error {
	return nil
}

func (m *mockRecordingEmailService) SendActivityNotification(toEmail, recipientName, activityType, desc, dueDate, leadID, company string) error {
	return nil
}

func TestWelcomeEmailAndUserCreation(t *testing.T) {
	_ = godotenv.Load("../.env")
	_ = godotenv.Load(".env")
	cfg, err := conf.LoadConfig()
	if err != nil {
		t.Skipf("Skipping integration test; LoadConfig error: %v", err)
		return
	}

	log := helpers.NewLogger(cfg.Server.Env)
	pool, err := conf.ConnectDB(cfg.DB, log)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()

	// Clean up any test users
	_, _ = pool.Exec(ctx, "DELETE FROM users WHERE email LIKE '%@welcometest.com'")
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE email LIKE '%@welcometest.com'")
	}()

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	otpRepo := repository.NewUserEmailOTPRepository(pool)
	emailOTPRepo := repository.NewEmailOTPRepository(pool)
	mobileOTPRepo := repository.NewMobileOTPRepository(pool)
	forgotPasswordRepo := repository.NewForgotPasswordRepository(pool)
	oauthStateRepo := repository.NewOAuthStateRepository(pool)

	jwtManager := helpers.NewJWTManager(cfg.JWT)
	mockEmail := &mockRecordingEmailService{}
	smsService := helpers.NewConsoleSMSService(log)
	rateLimiter := middleware.NewRateLimiter(5, 15*time.Minute)

	authService := services.NewAuthService(
		userRepo,
		sessionRepo,
		otpRepo,
		emailOTPRepo,
		mobileOTPRepo,
		forgotPasswordRepo,
		oauthStateRepo,
		jwtManager,
		mockEmail,
		smsService,
		rateLimiter,
		log,
	)

	authController := controllers.NewAuthController(authService, log)

	// ==========================================
	// TEST 1: Create admin successfully.
	// ==========================================
	t.Run("TEST 1: Create admin successfully", func(t *testing.T) {
		adminResp, err := authService.CreateUser(ctx, "Test Admin User", "admin@welcometest.com", "", "AdminSecret@123", models.RoleAdmin, nil)
		if err != nil {
			t.Fatalf("Create admin failed: %v", err)
		}
		if !adminResp.Success {
			t.Errorf("Expected Success=true")
		}
		dbAdmin, err := userRepo.FindByID(ctx, adminResp.User.ID)
		if err != nil || dbAdmin == nil {
			t.Fatalf("Admin not found in DB: %v", err)
		}
		if dbAdmin.ManagerID != nil {
			t.Errorf("Expected manager_id = nil, got %v", dbAdmin.ManagerID)
		}
		if !dbAdmin.IsActive {
			t.Errorf("Expected is_active = true")
		}
		if !dbAdmin.IsFirstLogin {
			t.Errorf("Expected is_first_login = true")
		}
		if !dbAdmin.PasswordChangeRequired {
			t.Errorf("Expected password_change_required = true")
		}
	})

	// ==========================================
	// TEST 2: Create leader successfully.
	// ==========================================
	t.Run("TEST 2: Create leader successfully", func(t *testing.T) {
		leaderResp, err := authService.CreateUser(ctx, "Test Leader User", "leader@welcometest.com", "", "LeaderSecret@123", models.RoleLeader, nil)
		if err != nil {
			t.Fatalf("Create leader failed: %v", err)
		}
		if !leaderResp.Success {
			t.Errorf("Expected Success=true")
		}
		dbLeader, err := userRepo.FindByID(ctx, leaderResp.User.ID)
		if err != nil || dbLeader == nil {
			t.Fatalf("Leader not found in DB: %v", err)
		}
		if dbLeader.ManagerID != nil {
			t.Errorf("Expected manager_id = nil, got %v", dbLeader.ManagerID)
		}
		if !dbLeader.IsActive {
			t.Errorf("Expected is_active = true")
		}
		if !dbLeader.IsFirstLogin {
			t.Errorf("Expected is_first_login = true")
		}
		if !dbLeader.PasswordChangeRequired {
			t.Errorf("Expected password_change_required = true")
		}
	})

	// ==========================================
	// TEST 3: Create sales_manager successfully.
	// ==========================================
	var validSalesManagerID uuid.UUID
	t.Run("TEST 3: Create sales_manager successfully", func(t *testing.T) {
		mgrResp, err := authService.CreateUser(ctx, "Test Sales Manager", "manager@welcometest.com", "", "ManagerSecret@123", models.RoleSalesManager, nil)
		if err != nil {
			t.Fatalf("Create sales_manager failed: %v", err)
		}
		if !mgrResp.Success {
			t.Errorf("Expected Success=true")
		}
		validSalesManagerID = mgrResp.User.ID

		dbMgr, err := userRepo.FindByID(ctx, mgrResp.User.ID)
		if err != nil || dbMgr == nil {
			t.Fatalf("Sales manager not found in DB: %v", err)
		}
		if dbMgr.ManagerID != nil {
			t.Errorf("Expected manager_id = nil, got %v", dbMgr.ManagerID)
		}
		if !dbMgr.IsActive {
			t.Errorf("Expected is_active = true")
		}
		if !dbMgr.IsFirstLogin {
			t.Errorf("Expected is_first_login = true")
		}
		if !dbMgr.PasswordChangeRequired {
			t.Errorf("Expected password_change_required = true")
		}
	})

	// ==========================================
	// TEST 4: Create sales_executive with valid active sales_manager UUID.
	// ==========================================
	var execUserID uuid.UUID
	execInitialPassword := "ExecSecret@123"
	t.Run("TEST 4: Create sales_executive with valid active sales_manager UUID", func(t *testing.T) {
		execResp, err := authService.CreateUser(ctx, "Test Sales Executive", "exec@welcometest.com", "", execInitialPassword, models.RoleSalesExecutive, &validSalesManagerID)
		if err != nil {
			t.Fatalf("Create sales_executive failed: %v", err)
		}
		if !execResp.Success || !execResp.EmailSent {
			t.Errorf("Expected Success=true and EmailSent=true, got %+v", execResp)
		}
		execUserID = execResp.User.ID

		dbExec, err := userRepo.FindByID(ctx, execResp.User.ID)
		if err != nil || dbExec == nil {
			t.Fatalf("Sales executive not found in DB: %v", err)
		}
		if dbExec.ManagerID == nil || *dbExec.ManagerID != validSalesManagerID {
			t.Errorf("Expected manager_id=%v, got %v", validSalesManagerID, dbExec.ManagerID)
		}
		if !dbExec.IsActive {
			t.Errorf("Expected is_active = true")
		}
		if !dbExec.IsFirstLogin {
			t.Errorf("Expected is_first_login = true")
		}
		if !dbExec.PasswordChangeRequired {
			t.Errorf("Expected password_change_required = true")
		}
	})

	// ==========================================
	// TEST 5: Create sales_executive without manager_id (should fail).
	// ==========================================
	t.Run("TEST 5: Create sales_executive without manager_id should fail", func(t *testing.T) {
		_, err := authService.CreateUser(ctx, "No Mgr Exec", "nomgr@welcometest.com", "", "ExecSecret@123", models.RoleSalesExecutive, nil)
		if err == nil {
			t.Fatalf("Expected error when manager_id is nil for sales_executive, got nil")
		}
		if !strings.Contains(err.Error(), "manager_id is required") {
			t.Errorf("Expected 'manager_id is required' error message, got: %v", err)
		}
	})

	// ==========================================
	// TEST 6: Create sales_executive with malformed manager_id ("manager@pmrgsolution.com").
	// Tested via HTTP controller handler.
	// ==========================================
	t.Run("TEST 6: Create sales_executive with malformed manager_id should fail", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.POST("/api/v1/users", authController.CreateUser)

		body := map[string]any{
			"name":       "Malformed Mgr Exec",
			"email":      "malformed@welcometest.com",
			"password":   "ExecSecret@123",
			"role":       "sales_executive",
			"manager_id": "manager@pmrgsolution.com",
		}
		jsonBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("Expected status 400 for malformed manager_id, got %d. Body: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "invalid manager_id") {
			t.Errorf("Expected 'invalid manager_id' in response, got: %s", w.Body.String())
		}
	})

	// ==========================================
	// TEST 7: Create sales_executive with nonexistent UUID (should fail).
	// ==========================================
	t.Run("TEST 7: Create sales_executive with nonexistent UUID should fail", func(t *testing.T) {
		nonExistentUUID := uuid.New()
		_, err := authService.CreateUser(ctx, "NonExistent Mgr Exec", "nonexistent@welcometest.com", "", "ExecSecret@123", models.RoleSalesExecutive, &nonExistentUUID)
		if err == nil {
			t.Fatalf("Expected error for nonexistent manager_id, got nil")
		}
		if !strings.Contains(err.Error(), "assigned manager does not exist") {
			t.Errorf("Expected 'assigned manager does not exist', got: %v", err)
		}
	})

	// ==========================================
	// TEST 8: Create sales_executive with inactive sales_manager (should fail).
	// ==========================================
	t.Run("TEST 8: Create sales_executive with inactive sales_manager should fail", func(t *testing.T) {
		// Create an inactive sales manager
		inactiveMgrResp, err := authService.CreateUser(ctx, "Inactive Manager", "inactivemgr@welcometest.com", "", "ManagerSecret@123", models.RoleSalesManager, nil)
		if err != nil {
			t.Fatalf("Failed to create manager: %v", err)
		}
		// Deactivate the manager in DB
		_, err = pool.Exec(ctx, "UPDATE users SET is_active = FALSE WHERE id = $1", inactiveMgrResp.User.ID)
		if err != nil {
			t.Fatalf("Failed to deactivate manager: %v", err)
		}

		inactiveMgrID := inactiveMgrResp.User.ID
		_, err = authService.CreateUser(ctx, "Exec With Inactive Mgr", "exec_inactive@welcometest.com", "", "ExecSecret@123", models.RoleSalesExecutive, &inactiveMgrID)
		if err == nil {
			t.Fatalf("Expected error for inactive manager, got nil")
		}
		if !strings.Contains(err.Error(), "assigned manager is inactive") {
			t.Errorf("Expected 'assigned manager is inactive', got: %v", err)
		}
	})

	// ==========================================
	// TEST 9: Create sales_executive with active user whose role is NOT sales_manager (should fail).
	// ==========================================
	t.Run("TEST 9: Create sales_executive with non-sales_manager role should fail", func(t *testing.T) {
		// An admin user exists from TEST 1
		adminUser, err := userRepo.FindByEmail(ctx, "admin@welcometest.com")
		if err != nil || adminUser == nil {
			t.Fatalf("Admin user from Test 1 not found: %v", err)
		}

		adminID := adminUser.ID
		_, err = authService.CreateUser(ctx, "Exec With Admin Mgr", "exec_adminmgr@welcometest.com", "", "ExecSecret@123", models.RoleSalesExecutive, &adminID)
		if err == nil {
			t.Fatalf("Expected error when manager role is not sales_manager, got nil")
		}
		if !strings.Contains(err.Error(), "must have sales_manager role") {
			t.Errorf("Expected 'must have sales_manager role', got: %v", err)
		}
	})

	// ==========================================
	// TEST 10: Successful welcome email verification.
	// ==========================================
	t.Run("TEST 10: Successful welcome email verification", func(t *testing.T) {
		if len(mockEmail.sentEmails) == 0 {
			t.Fatalf("No welcome emails were recorded")
		}
		// Find email sent to exec@welcometest.com
		var foundEmail *recordedWelcomeEmail
		for _, em := range mockEmail.sentEmails {
			if em.toEmail == "exec@welcometest.com" {
				foundEmail = &em
				break
			}
		}
		if foundEmail == nil {
			t.Fatalf("Welcome email to exec@welcometest.com not found")
		}
		if foundEmail.name != "Test Sales Executive" {
			t.Errorf("Expected name 'Test Sales Executive', got '%s'", foundEmail.name)
		}
		if foundEmail.role != "Sales Executive" {
			t.Errorf("Expected role 'Sales Executive', got '%s'", foundEmail.role)
		}
		if foundEmail.managerName != "Test Sales Manager" {
			t.Errorf("Expected managerName 'Test Sales Manager', got '%s'", foundEmail.managerName)
		}
		if foundEmail.initialPassword != execInitialPassword {
			t.Errorf("Expected initial password in email, got '%s'", foundEmail.initialPassword)
		}
	})

	// ==========================================
	// TEST 11: SMTP/email failure behavior.
	// ==========================================
	t.Run("TEST 11: SMTP/email failure behavior", func(t *testing.T) {
		mockEmail.failNext = true
		failResp, err := authService.CreateUser(ctx, "Email Fail User", "emailfail@welcometest.com", "", "FailSecret@123", models.RoleSalesExecutive, &validSalesManagerID)
		if err != nil {
			t.Fatalf("Expected CreateUser to not return error when email fails, got: %v", err)
		}
		if !failResp.Success {
			t.Errorf("Expected Success = true")
		}
		if failResp.EmailSent {
			t.Errorf("Expected EmailSent = false")
		}
		if failResp.Message != "User created, but welcome email failed." {
			t.Errorf("Expected message 'User created, but welcome email failed.', got '%s'", failResp.Message)
		}

		// User remains in database and is active
		dbFailUser, err := userRepo.FindByID(ctx, failResp.User.ID)
		if err != nil || dbFailUser == nil {
			t.Fatalf("User was rolled back/deleted from database: %v", err)
		}
		if !dbFailUser.IsActive {
			t.Errorf("Expected user to be active")
		}
		if !dbFailUser.IsFirstLogin || !dbFailUser.PasswordChangeRequired {
			t.Errorf("Expected first login flags to be true")
		}
		mockEmail.failNext = false
	})

	// ==========================================
	// TEST 12: First login contract.
	// ==========================================
	var accessToken string
	t.Run("TEST 12: First login contract", func(t *testing.T) {
		loginResult, err := authService.Login(ctx, models.LoginRequest{
			Identifier: "exec@welcometest.com",
			Password:   execInitialPassword,
		}, "127.0.0.1")
		if err != nil {
			t.Fatalf("Login failed: %v", err)
		}

		firstLoginResp, ok := loginResult.(*models.LoginFirstTimeResponse)
		if !ok {
			t.Fatalf("Expected *models.LoginFirstTimeResponse, got %T", loginResult)
		}
		if !firstLoginResp.FirstTimeLogin {
			t.Errorf("Expected first_time_login = true")
		}
		if !firstLoginResp.PasswordChangeRequired {
			t.Errorf("Expected password_change_required = true")
		}
		if firstLoginResp.AccessToken == "" {
			t.Errorf("Expected access_token to be non-empty")
		}
		if firstLoginResp.TokenType != "Bearer" {
			t.Errorf("Expected token_type = Bearer, got %s", firstLoginResp.TokenType)
		}
		if firstLoginResp.ExpiresIn != 900 {
			t.Errorf("Expected expires_in = 900, got %d", firstLoginResp.ExpiresIn)
		}
		accessToken = firstLoginResp.AccessToken
	})

	// ==========================================
	// TEST 13: Change password & token verification.
	// ==========================================
	newStrongPassword := "NewSecurePass456!"
	t.Run("TEST 13: Change password and old password invalidation", func(t *testing.T) {
		err := authService.ChangePassword(ctx, execUserID, newStrongPassword)
		if err != nil {
			t.Fatalf("ChangePassword failed: %v", err)
		}

		dbExecUpdated, err := userRepo.FindByID(ctx, execUserID)
		if err != nil || dbExecUpdated == nil {
			t.Fatalf("Failed to query user: %v", err)
		}
		if dbExecUpdated.IsFirstLogin {
			t.Errorf("Expected is_first_login = false after password change")
		}
		if dbExecUpdated.PasswordChangeRequired {
			t.Errorf("Expected password_change_required = false after password change")
		}

		// Old temporary password must no longer work
		_, err = authService.Login(ctx, models.LoginRequest{
			Identifier: "exec@welcometest.com",
			Password:   execInitialPassword,
		}, "127.0.0.1")
		if err == nil {
			t.Errorf("Expected login with old temporary password to fail, but it succeeded")
		}

		// New password works for normal login
		newLoginResult, err := authService.Login(ctx, models.LoginRequest{
			Identifier: "exec@welcometest.com",
			Password:   newStrongPassword,
		}, "127.0.0.1")
		if err != nil {
			t.Fatalf("Login with new password failed: %v", err)
		}
		_, ok := newLoginResult.(*models.LoginSuccessResponse)
		if !ok {
			t.Fatalf("Expected *models.LoginSuccessResponse, got %T", newLoginResult)
		}
	})

	// ==========================================
	// TEST 14: Weak new password rejected.
	// ==========================================
	t.Run("TEST 14: Weak new password rejected", func(t *testing.T) {
		weakPasswords := []string{
			"short",            // < 8 chars
			"alllowercase1",    // missing uppercase & special
			"ALLUPPERCASE1",    // missing lowercase & special
			"NoSpecialChar123", // missing special
			"NoNumbers!",       // missing numbers
		}

		for _, weak := range weakPasswords {
			err := authService.ChangePassword(ctx, execUserID, weak)
			if err == nil {
				t.Errorf("Expected weak password '%s' to be rejected, but it succeeded", weak)
			}
		}
	})

	_ = accessToken
}
