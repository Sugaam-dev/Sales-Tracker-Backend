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

	// 3. Verify stage seed data has exactly 7 rows
	var stageCount int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM lead_stages").Scan(&stageCount)
	if err != nil || stageCount != 7 {
		t.Errorf("Expected 7 stages, got %d (err: %v)", stageCount, err)
	}

	// 4. Verify Idempotency of migrations (run connection/migration again)
	pool2, err := conf.ConnectDB(cfg.DB, log)
	if err != nil {
		t.Fatalf("Failed to run ConnectDB again: %v", err)
	}
	defer pool2.Close()

	err = pool2.QueryRow(ctx, "SELECT COUNT(*) FROM lead_stages").Scan(&stageCount)
	if err != nil || stageCount != 7 {
		t.Errorf("Expected exactly 7 stages after second migration run, got %d (err: %v)", stageCount, err)
	}

	// Initialize repositories and service
	userRepo := repository.NewUserRepository(pool)
	leadRepo := repository.NewLeadRepository(pool)
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
	if len(stages) != 7 {
		t.Errorf("Expected 7 stages, got %d", len(stages))
	}
	// Verify sorting order
	for idx, s := range stages {
		if idx > 0 && s.SortOrder < stages[idx-1].SortOrder {
			t.Error("Stages are not sorted by sort_order ASC")
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

func TestMain(m *testing.M) {
	// Set test environment configuration
	os.Setenv("APP_ENV", "test")
	os.Setenv("JWT_SECRET", "super-secret-key-32-characters-long")
	
	code := m.Run()

	os.Unsetenv("APP_ENV")
	os.Unsetenv("JWT_SECRET")
	os.Exit(code)
}
