package services

import (
	"context"
	"os"
	"testing"
	"time"

	"crm-auth-service/conf"
	"crm-auth-service/helpers"
	"crm-auth-service/models"
	"crm-auth-service/repository"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func strPtr(s string) *string {
	return &s
}

func TestLeadServiceAndMigrationsIntegration(t *testing.T) {
	// 1. Load configuration and attempt database connection
	cfg, err := conf.LoadConfig()
	if err != nil {
		t.Skip("Skipping integration test; config not loaded or database credentials not found in env")
		return
	}

	log := helpers.NewLogger(cfg.Server.Env)
	pool, err := conf.ConnectDB(cfg.DB, log)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()

	// 2. Verify Table and Sequence creation
	var exists bool
	err = pool.QueryRow(ctx, "SELECT EXISTS (SELECT FROM pg_tables WHERE schemaname = 'public' AND tablename = 'leads')").Scan(&exists)
	if err != nil || !exists {
		t.Errorf("leads table does not exist: %v", err)
	}

	err = pool.QueryRow(ctx, "SELECT EXISTS (SELECT FROM pg_tables WHERE schemaname = 'public' AND tablename = 'lead_stages')").Scan(&exists)
	if err != nil || !exists {
		t.Errorf("lead_stages table does not exist: %v", err)
	}

	err = pool.QueryRow(ctx, "SELECT EXISTS (SELECT FROM pg_tables WHERE schemaname = 'public' AND tablename = 'activities')").Scan(&exists)
	if err != nil || !exists {
		t.Errorf("activities table does not exist: %v", err)
	}

	err = pool.QueryRow(ctx, "SELECT EXISTS (SELECT FROM pg_class WHERE relkind = 'S' AND relname = 'lead_id_seq')").Scan(&exists)
	if err != nil || !exists {
		t.Errorf("lead_id_seq sequence does not exist: %v", err)
	}

	// 3. Verify stage seed data has exactly 8 rows
	var stageCount int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM lead_stages").Scan(&stageCount)
	if err != nil || stageCount != 8 {
		t.Errorf("Expected 8 stages, got %d (err: %v)", stageCount, err)
	}

	// 4. Verify Idempotency of migrations (run connection/migration again)
	pool2, err := conf.ConnectDB(cfg.DB, log)
	if err != nil {
		t.Fatalf("Failed to run ConnectDB again: %v", err)
	}
	defer pool2.Close()

	err = pool2.QueryRow(ctx, "SELECT COUNT(*) FROM lead_stages").Scan(&stageCount)
	if err != nil || stageCount != 8 {
		t.Errorf("Expected exactly 8 stages after second migration run, got %d (err: %v)", stageCount, err)
	}

	// Initialize GORM
	importGormStr := cfg.DB.DSN(cfg.DB.Name)
	gormDB, err := gorm.Open(postgres.Open(importGormStr), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect gorm: %v", err)
	}

	// Initialize repositories and service
	userRepo := repository.NewUserRepository(pool)
	leadRepo := repository.NewLeadRepository(pool, gormDB)
	leadService := NewLeadService(leadRepo, userRepo)

	// Clean up existing test data safely
	_, _ = pool.Exec(ctx, "DELETE FROM activities")
	_, _ = pool.Exec(ctx, "DELETE FROM leads")
	_, _ = pool.Exec(ctx, "DELETE FROM users WHERE email LIKE 'test_%'")

	// 5. Test API 1: GetCurrentUsers
	user1 := &models.User{
		Name:         "Alice",
		Email:        "test_alice@example.com",
		PasswordHash: "hash",
		Role:         "agent",
		IsActive:     true,
	}
	user2 := &models.User{
		Name:         "Charlie",
		Email:        "test_charlie@example.com",
		PasswordHash: "hash",
		Role:         "admin",
		IsActive:     false, // inactive
	}
	user3 := &models.User{
		Name:         "Bob",
		Email:        "test_bob@example.com",
		PasswordHash: "hash",
		Role:         "agent",
		IsActive:     true,
	}
	_ = userRepo.Create(ctx, user1)
	_ = userRepo.Create(ctx, user2)
	_ = userRepo.Create(ctx, user3)

	activeUsers, err := leadService.GetCurrentUsers(ctx)
	if err != nil {
		t.Fatalf("GetCurrentUsers failed: %v", err)
	}

	if len(activeUsers) < 2 {
		t.Errorf("Expected at least 2 active users, got %d", len(activeUsers))
	}

	// Verify inactive user is excluded, and active users are sorted by name ASC
	var foundAlice, foundBob, foundCharlie bool
	var aliceIndex, bobIndex int
	for idx, u := range activeUsers {
		if u.Email == "test_alice@example.com" {
			foundAlice = true
			aliceIndex = idx
		}
		if u.Email == "test_bob@example.com" {
			foundBob = true
			bobIndex = idx
		}
		if u.Email == "test_charlie@example.com" {
			foundCharlie = true
		}
	}
	if !foundAlice || !foundBob {
		t.Error("Active users test_alice or test_bob not returned")
	}
	if foundCharlie {
		t.Error("Inactive user test_charlie was returned")
	}
	if aliceIndex > bobIndex {
		t.Error("Active users are not sorted alphabetically by name")
	}

	// 6. Test API 2: GetMasterStages
	stages, err := leadService.GetMasterStages(ctx)
	if err != nil {
		t.Fatalf("GetMasterStages failed: %v", err)
	}
	if len(stages) != 8 {
		t.Errorf("Expected 8 stages, got %d", len(stages))
	}
	// Verify sorting order and status mapping
	expectedMappings := map[string]string{
		"Prospecting":        "Open",
		"Qualification":      "New",
		"Initial Discussion": "Contacted",
		"Needs Analysis":     "Analysis",
		"Proposal":           "Interested",
		"Negotiation":        "Negotiation",
		"Closed Won":         "Won",
		"Closed Lost":        "Lost",
	}

	for idx, s := range stages {
		if idx > 0 && s.SortOrder < stages[idx-1].SortOrder {
			t.Error("Stages are not sorted by sort_order ASC")
		}
		expectedStatus, exists := expectedMappings[s.Name]
		if !exists {
			t.Errorf("Unexpected stage name: %s", s.Name)
		} else if s.Status != expectedStatus {
			t.Errorf("Expected stage %s to map to status %s, got %s", s.Name, expectedStatus, s.Status)
		}
	}

	// 7. Test API 3: ListLeads & Filters
	// Insert dummy leads
	val1 := 100000.0
	val2 := 250000.0
	lead1 := &models.Lead{
		LeadID:      "L-1001",
		Company:     "Acme Corporation",
		Contact:     strPtr("John Doe"),
		Priority:    strPtr("High"),
		Stage:       strPtr("Prospecting"),
		Owner:       strPtr("test_alice@example.com"),
		Value:       &val1,
		CreatedAt:   time.Now().Add(-10 * time.Minute),
	}
	lead2 := &models.Lead{
		LeadID:      "L-1002",
		Company:     "Globex Corp",
		Contact:     strPtr("Jane Smith"),
		Priority:    strPtr("Low"),
		Stage:       strPtr("Qualification"),
		Owner:       strPtr("test_bob@example.com"),
		Value:       &val2,
		CreatedAt:   time.Now(),
	}
	
	// Create raw in DB for testing
	_, err = pool.Exec(ctx, `INSERT INTO leads (lead_id, company, contact, priority, stage, owner, value, created_at) VALUES
		($1, $2, $3, $4, $5, $6, $7, $8)`, lead1.LeadID, lead1.Company, lead1.Contact, lead1.Priority, lead1.Stage, lead1.Owner, lead1.Value, lead1.CreatedAt)
	if err != nil {
		t.Fatalf("Failed to seed lead1: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO leads (lead_id, company, contact, priority, stage, owner, value, created_at) VALUES
		($1, $2, $3, $4, $5, $6, $7, $8)`, lead2.LeadID, lead2.Company, lead2.Contact, lead2.Priority, lead2.Stage, lead2.Owner, lead2.Value, lead2.CreatedAt)
	if err != nil {
		t.Fatalf("Failed to seed lead2: %v", err)
	}

	// Test default pagination and listing
	leadsRes, pagRes, err := leadService.ListLeads(ctx, 1, 5, "", "", "", "", "createdAt", "desc")
	if err != nil {
		t.Fatalf("ListLeads failed: %v", err)
	}
	if len(leadsRes) != 2 {
		t.Errorf("Expected 2 leads, got %d", len(leadsRes))
	}
	if pagRes.Total != 2 || pagRes.TotalPages != 1 {
		t.Errorf("Pagination total mismatch: %+v", pagRes)
	}

	// Test partial search matching
	leadsRes, _, _ = leadService.ListLeads(ctx, 1, 5, "Acme", "", "", "", "", "")
	if len(leadsRes) != 1 || leadsRes[0].Company != "Acme Corporation" {
		t.Error("Search by company failed")
	}

	leadsRes, _, _ = leadService.ListLeads(ctx, 1, 5, "1002", "", "", "", "", "")
	if len(leadsRes) != 1 || leadsRes[0].ID != "L-1002" {
		t.Error("Search by lead_id (partial) failed")
	}

	// Test stage validation (bad stage returns 400 AppError)
	_, _, err = leadService.ListLeads(ctx, 1, 5, "", "", "", "InvalidStage", "", "")
	if err == nil {
		t.Error("Expected error on invalid stage filter, got nil")
	}

	// Test sorting (value asc)
	leadsRes, _, _ = leadService.ListLeads(ctx, 1, 5, "", "", "", "", "value", "asc")
	if len(leadsRes) == 2 && *leadsRes[0].Value != "100000" {
		t.Errorf("Sorting by value failed, first element value should be 100000, got %s", *leadsRes[0].Value)
	}

	// 8. Test API 4: GetLead (Single Lead profile)
	singleLead, err := leadService.GetLead(ctx, "L-1001")
	if err != nil {
		t.Fatalf("GetLead failed: %v", err)
	}
	if singleLead.Company != "Acme Corporation" {
		t.Errorf("Expected Acme Corporation, got %s", singleLead.Company)
	}

	// Test malformed/invalid/non-existent lead id
	_, err = leadService.GetLead(ctx, "L-9999")
	if err != helpers.ErrNotFound {
		t.Errorf("Expected ErrNotFound for non-existent lead, got %v", err)
	}

	// Test soft-delete (mark lead1 as deleted)
	_, _ = pool.Exec(ctx, "UPDATE leads SET deleted_at = NOW() WHERE lead_id = 'L-1001'")
	_, err = leadService.GetLead(ctx, "L-1001")
	if err != helpers.ErrNotFound {
		t.Errorf("Expected ErrNotFound for soft-deleted lead, got %v", err)
	}
}

func TestLeadLifecycleUpdate(t *testing.T) {
	cfg, err := conf.LoadConfig()
	if err != nil {
		t.Skip("Skipping integration test; config not loaded or database credentials not found in env")
		return
	}

	log := helpers.NewLogger(cfg.Server.Env)
	pool, err := conf.ConnectDB(cfg.DB, log)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()
	// Initialize GORM
	importGormStr := cfg.DB.DSN(cfg.DB.Name)
	gormDB, err := gorm.Open(postgres.Open(importGormStr), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect gorm: %v", err)
	}

	userRepo := repository.NewUserRepository(pool)
	leadRepo := repository.NewLeadRepository(pool, gormDB)
	leadService := NewLeadService(leadRepo, userRepo)

	// Clean up and seed one lead for status testing
	_, _ = pool.Exec(ctx, "DELETE FROM leads WHERE lead_id = 'L-9999'")
	
	_, err = pool.Exec(ctx, `INSERT INTO leads (lead_id, company, contact, priority, stage, status, value) VALUES
		('L-9999', 'Lifecycle Corp', 'Test Contact', 'High', 'Prospecting', 'Open', 50000.0)`)
	if err != nil {
		t.Fatalf("Failed to seed lead L-9999: %v", err)
	}

	// 1. Test status mapping update (Open -> Prospecting)
	res, err := leadService.UpdateLead("L-9999", models.RoleAdmin, "admin@example.com", models.UpdateLeadRequest{
		Status: strPtr("Open"),
	})
	if err != nil || res.Stage == nil || *res.Stage != "Prospecting" {
		t.Errorf("Expected Stage Prospecting for Open status, got %v (err: %v)", res, err)
	}

	// 2. Test status mapping update (New -> Qualification)
	res, err = leadService.UpdateLead("L-9999", models.RoleAdmin, "admin@example.com", models.UpdateLeadRequest{
		Status: strPtr("New"),
	})
	if err != nil || res.Stage == nil || *res.Stage != "Qualification" {
		t.Errorf("Expected Stage Qualification for New status, got %v (err: %v)", res, err)
	}

	// 3. Test backward progression (New -> Open)
	res, err = leadService.UpdateLead("L-9999", models.RoleAdmin, "admin@example.com", models.UpdateLeadRequest{
		Status: strPtr("Open"),
	})
	if err != nil || res.Status == nil || *res.Status != "Open" || res.Stage == nil || *res.Stage != "Prospecting" {
		t.Errorf("Backward movement to Open failed, got %+v (err: %v)", res, err)
	}

	// 4. Test invalid status-stage combination
	_, err = leadService.UpdateLead("L-9999", models.RoleAdmin, "admin@example.com", models.UpdateLeadRequest{
		Status: strPtr("Won"),
		Stage:  strPtr("Prospecting"),
	})
	if err == nil {
		t.Error("Expected error for inconsistent status-stage combination, got nil")
	}

	// 5. Test Lost Reason validation (missing reason)
	_, err = leadService.UpdateLead("L-9999", models.RoleAdmin, "admin@example.com", models.UpdateLeadRequest{
		Status: strPtr("Lost"),
	})
	if err == nil {
		t.Error("Expected error for Lost status without lostReason, got nil")
	}

	// 6. Test Lost Reason validation (empty spaces)
	_, err = leadService.UpdateLead("L-9999", models.RoleAdmin, "admin@example.com", models.UpdateLeadRequest{
		Status:     strPtr("Lost"),
		LostReason: strPtr("   "),
	})
	if err == nil {
		t.Error("Expected error for Lost status with empty/whitespace lostReason, got nil")
	}

	// 7. Test Lost Reason valid submit
	reason := "Pricing too high compared to competitors"
	res, err = leadService.UpdateLead("L-9999", models.RoleAdmin, "admin@example.com", models.UpdateLeadRequest{
		Status:     strPtr("Lost"),
		LostReason: &reason,
	})
	if err != nil || res.Status == nil || *res.Status != "Lost" || res.Stage == nil || *res.Stage != "Closed Lost" || res.LostReason == nil || *res.LostReason != reason {
		t.Errorf("Expected success for Lost with reason, got %+v (err: %v)", res, err)
	}

	// 8. Test Lost Reason retention when moving away from Lost status
	res, err = leadService.UpdateLead("L-9999", models.RoleAdmin, "admin@example.com", models.UpdateLeadRequest{
		Status: strPtr("Interested"),
	})
	if err != nil || res.Status == nil || *res.Status != "Interested" || res.Stage == nil || *res.Stage != "Proposal" || res.LostReason == nil || *res.LostReason != reason {
		t.Errorf("Expected lostReason to be retained, got %+v (err: %v)", res, err)
	}

	// 9. Test Atomicity: Ensure invalid updates do not modify the lead partially in database
	_, err = leadService.UpdateLead("L-9999", models.RoleAdmin, "admin@example.com", models.UpdateLeadRequest{
		Status: strPtr("Won"),
		Stage:  strPtr("Prospecting"), // Inconsistent stage
	})
	if err == nil {
		t.Error("Expected error, but update succeeded")
	}

	// Fetch from DB to confirm lead is still Interested / Proposal
	current, err := leadRepo.FindByID(ctx, "L-9999")
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if current.Status == nil || *current.Status != "Interested" || current.Stage == nil || *current.Stage != "Proposal" {
		t.Errorf("Database updated partially! Expected (Interested, Proposal), got (%v, %v)", current.Status, current.Stage)
	}

	// Cleanup
	_, _ = pool.Exec(ctx, "DELETE FROM leads WHERE lead_id = 'L-9999'")
}

func TestMain(m *testing.M) {
	_ = godotenv.Load("../.env")
	// Set test environment configuration
	os.Setenv("APP_ENV", "test")
	if os.Getenv("JWT_SECRET") == "" {
		os.Setenv("JWT_SECRET", "super-secret-key-32-characters-long")
	}
	
	code := m.Run()

	os.Unsetenv("APP_ENV")
	os.Exit(code)
}
