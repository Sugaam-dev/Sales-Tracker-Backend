package services

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"crm-auth-service/conf"
	"crm-auth-service/helpers"
	"crm-auth-service/models"
	"crm-auth-service/repository"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestActivityAndBulkLeadAPIs(t *testing.T) {
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

	importGormStr := cfg.DB.DSN(cfg.DB.Name)
	gormDB, err := gorm.Open(postgres.Open(importGormStr), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to gorm: %v", err)
	}

	ctx := context.Background()
	userRepo := repository.NewUserRepository(pool)
	leadRepo := repository.NewLeadRepository(pool, gormDB)
	leadService := NewLeadService(leadRepo, userRepo)

	// Clean up existing test data
	_, _ = pool.Exec(ctx, "DELETE FROM activities")
	_, _ = pool.Exec(ctx, "DELETE FROM leads")
	_, _ = pool.Exec(ctx, "DELETE FROM users WHERE email LIKE 'test_%'")

	// Setup users
	adminUser := &models.User{
		Name:         "Test Admin",
		Email:        "test_admin@pmrgsolution.com",
		PasswordHash: "hash",
		Role:         "admin",
		IsActive:     true,
	}
	kamUser1 := &models.User{
		Name:         "KAM One",
		Email:        "test_kam1@pmrgsolution.com",
		PasswordHash: "hash",
		Role:         "sales_executive",
		IsActive:     true,
	}
	kamUser2 := &models.User{
		Name:         "KAM Two",
		Email:        "test_kam2@pmrgsolution.com",
		PasswordHash: "hash",
		Role:         "sales_executive",
		IsActive:     true,
	}
	inactiveOwner := &models.User{
		Name:         "Inactive Owner",
		Email:        "test_inactive@pmrgsolution.com",
		PasswordHash: "hash",
		Role:         "sales_executive",
		IsActive:     false,
	}

	_ = gormDB.Create(adminUser)
	_ = gormDB.Create(kamUser1)
	_ = gormDB.Create(kamUser2)
	_ = gormDB.Create(inactiveOwner)

	// Setup a Lead owned by KAM One
	lead1 := &models.Lead{
		LeadID:            "L-9001",
		Company:           "KAM1 Company",
		Contact:           strPtr("Contact 1"),
		Email:             strPtr("contact1@kam1.com"),
		Phone:             strPtr("9876543210"),
		OfficePhone:       strPtr("9123456780"),
		Owner:             strPtr("KAM One"),
		Stage:             strPtr("Prospecting"),
		Status:            strPtr("Open"),
		Sentiment:         strPtr("Positive"),
		Priority:          strPtr("High"),
		KamName:           strPtr("KAM One"),
		BasicRequirements: strPtr("Initial requirements"),
	}
	_ = gormDB.Create(lead1)

	// Setup a soft-deleted Lead
	leadSoftDeleted := &models.Lead{
		LeadID:            "L-9002",
		Company:           "Soft Deleted Corp",
		Contact:           strPtr("Contact 2"),
		Email:             strPtr("contact2@deleted.com"),
		Phone:             strPtr("9876543210"),
		OfficePhone:       strPtr("9123456780"),
		Owner:             strPtr("KAM One"),
		Stage:             strPtr("Prospecting"),
		Status:            strPtr("Open"),
		Sentiment:         strPtr("Positive"),
		Priority:          strPtr("High"),
		DeletedAt:         timePtr(time.Now()),
		KamName:           strPtr("KAM One"),
		BasicRequirements: strPtr("Some requirements"),
	}
	_ = gormDB.Create(leadSoftDeleted)

	t.Run("Create Activity - Success & Ownership rules", func(t *testing.T) {
		req := models.CreateActivityRequest{
			Type:    "Call",
			Desc:    "Initial callback",
			Outcome: "Interested",
			DueDate: "2026-09-10",
		}

		// Admin can create for any lead
		resp, err := leadService.CreateActivity("L-9001", "admin", "test_admin@pmrgsolution.com", req)
		if err != nil {
			t.Fatalf("Admin failed to create activity: %v", err)
		}
		if resp.Completed {
			t.Error("New activity should have completed = false by default")
		}

		// Owner (KAM One) can create
		_, err = leadService.CreateActivity("L-9001", "sales_executive", "test_kam1@pmrgsolution.com", req)
		if err != nil {
			t.Fatalf("Owner failed to create activity: %v", err)
		}

		// Non-owner (KAM Two) cannot create
		_, err = leadService.CreateActivity("L-9001", "sales_executive", "test_kam2@pmrgsolution.com", req)
		if err != ErrUnauthorized {
			t.Errorf("Expected ErrUnauthorized, got %v", err)
		}
	})

	t.Run("Create Activity - Validation and existence rules", func(t *testing.T) {
		// Non-existent Lead
		req := models.CreateActivityRequest{
			Type: "Call",
			Desc: "Callback",
		}
		_, err := leadService.CreateActivity("L-9999", "admin", "test_admin@pmrgsolution.com", req)
		if err != ErrNotFound {
			t.Errorf("Expected ErrNotFound for non-existent lead, got %v", err)
		}

		// Soft-deleted Lead
		_, err = leadService.CreateActivity("L-9002", "admin", "test_admin@pmrgsolution.com", req)
		if err != ErrNotFound {
			t.Errorf("Expected ErrNotFound for soft-deleted lead, got %v", err)
		}

		// Invalid dueDate format
		reqInvalidDate := models.CreateActivityRequest{
			Type:    "Call",
			Desc:    "Callback",
			DueDate: "invalid-date",
		}
		_, err = leadService.CreateActivity("L-9001", "admin", "test_admin@pmrgsolution.com", reqInvalidDate)
		if err != ErrValidation {
			t.Errorf("Expected ErrValidation for invalid dueDate, got %v", err)
		}
	})

	t.Run("Complete Activity - Access & Idempotency", func(t *testing.T) {
		// Insert test activity under L-9001
		act := &models.Activity{
			LeadID:    "L-9001",
			Type:      "Email",
			Desc:      "Send contract",
			Completed: false,
		}
		_ = gormDB.Create(act)

		// Non-owner KAM Two cannot update
		_, err := leadService.CompleteActivity(act.ID, "sales_executive", "test_kam2@pmrgsolution.com", true)
		if err != ErrUnauthorized {
			t.Errorf("Expected ErrUnauthorized for non-owner, got %v", err)
		}

		// Owner KAM One can update
		resp, err := leadService.CompleteActivity(act.ID, "sales_executive", "test_kam1@pmrgsolution.com", true)
		if err != nil {
			t.Fatalf("Owner failed to complete activity: %v", err)
		}
		if !resp.Completed {
			t.Error("Activity should be marked completed")
		}

		// Idempotent test: complete again
		resp2, err := leadService.CompleteActivity(act.ID, "sales_executive", "test_kam1@pmrgsolution.com", true)
		if err != nil {
			t.Fatalf("Owner failed on duplicate complete: %v", err)
		}
		if !resp2.Completed {
			t.Error("Activity should remain completed")
		}

		// Admin can change to false
		resp3, err := leadService.CompleteActivity(act.ID, "admin", "test_admin@pmrgsolution.com", false)
		if err != nil {
			t.Fatalf("Admin failed to set completed = false: %v", err)
		}
		if resp3.Completed {
			t.Error("Activity should be set to false")
		}

		// Non-existent activity
		_, err = leadService.CompleteActivity(9999, "admin", "test_admin@pmrgsolution.com", true)
		if err != ErrNotFound {
			t.Errorf("Expected ErrNotFound for invalid activity ID, got %v", err)
		}
	})

	t.Run("Lead Profile - CRUD operations and validation", func(t *testing.T) {
		// Test Lead creation with the new fields
		req := models.CreateLeadRequest{
			Company:                  "Profile Test Comp",
			Contact:                  "Jane Doe",
			Email:                    "profile@test.com",
			Phone:                    "9876543210",
			OfficePhone:              "9123456780",
			Owner:                    "KAM One",
			Stage:                    "Prospecting",
			Status:                   "Open",
			Sentiment:                "Positive",
			Priority:                 "High",
			LifecycleTemplate:        "Enterprise Sales",
			KamName:                  "John Doe",
			Designation:              "VP of Sales",
			BestTimeToConnect:        "Morning",
			AlternatePhone:           "9876543211",
			AlternatePhoneCountry:    "+91",
			LinkedinProfileUrl:       "https://linkedin.com/in/johndoe",
			LinkedinCompanyPageUrl:   "https://linkedin.com/company/example",
			EstimatedRequirementDate: "2026-07-15",
			LastContactDate:          "2026-06-15T15:00:00Z",
			NextFollowUp:             "2026-06-20T10:00:00Z",
			BasicRequirements:        "Requirement description",
			Notes:                    "Internal notes",
		}

		leadResp, err := leadService.CreateLead(req)
		if err != nil {
			t.Fatalf("Failed to create Lead with Profile fields: %v", err)
		}

		if leadResp.LifecycleTemplate == nil || *leadResp.LifecycleTemplate != "Enterprise Sales" {
			t.Errorf("Expected LifecycleTemplate 'Enterprise Sales', got %v", leadResp.LifecycleTemplate)
		}
		if leadResp.KamName == nil || *leadResp.KamName != "John Doe" {
			t.Errorf("Expected KamName 'John Doe', got %v", leadResp.KamName)
		}
		if leadResp.BasicRequirements == nil || *leadResp.BasicRequirements != "Requirement description" {
			t.Errorf("Expected BasicRequirements 'Requirement description', got %v", leadResp.BasicRequirements)
		}

		// Test GORM fetching
		fetched, err := leadService.GetLead(ctx, leadResp.ID)
		if err != nil {
			t.Fatalf("Failed to fetch lead: %v", err)
		}
		if fetched.LifecycleTemplate == nil || *fetched.LifecycleTemplate != "Enterprise Sales" {
			t.Errorf("Expected LifecycleTemplate 'Enterprise Sales' on fetch, got %v", fetched.LifecycleTemplate)
		}

		// Test PATCH update
		updateReq := models.UpdateLeadRequest{
			KamName:           strPtr("Jane Smith"),
			BasicRequirements: strPtr("Updated basic requirements"),
		}
		updated, err := leadService.UpdateLead(leadResp.ID, "admin", "test_admin@pmrgsolution.com", updateReq)
		if err != nil {
			t.Fatalf("Failed to update Lead: %v", err)
		}
		if updated.KamName == nil || *updated.KamName != "Jane Smith" {
			t.Errorf("Expected KamName 'Jane Smith', got %v", updated.KamName)
		}

		// Test required validation on update
		invalidUpdate := models.UpdateLeadRequest{
			KamName: strPtr(""), // Cannot be empty
		}
		_, err = leadService.UpdateLead(leadResp.ID, "admin", "test_admin@pmrgsolution.com", invalidUpdate)
		if err == nil {
			t.Error("Expected error when updating KamName to empty, got nil")
		}
	})

	t.Run("Bulk Create Leads - Validation and Duplicates", func(t *testing.T) {
		req := models.BulkCreateLeadsRequest{
			Leads: []models.CreateLeadRequest{
				{
					Company:            "Unique Company A",
					Contact:            "John A",
					Email:              "johnA@company.com",
					Phone:              "9876543210",
					OfficePhone:        "9123456780",
					OfficePhoneCountry: "+91",
					Owner:              "KAM One",
					Stage:              "Prospecting",
					Status:             "Open",
					Sentiment:          "Positive",
					Priority:           "High",
					KamName:            "KAM One",
					BasicRequirements:  "Some requirements",
				},
				{
					Company:            "Unique Company B",
					Contact:            "John B",
					Email:              "johnA@company.com", // Duplicate email
					Phone:              "9876543210",
					OfficePhone:        "9123456780",
					OfficePhoneCountry: "+91",
					Owner:              "KAM One",
					Stage:              "Prospecting",
					Status:             "Open",
					Sentiment:          "Positive",
					Priority:           "High",
					KamName:            "KAM One",
					BasicRequirements:  "Some requirements",
				},
				{
					Company:            "Unique Company C",
					Contact:            "John C",
					Email:              "johnC@company.com",
					Phone:              "123", // Invalid phone
					OfficePhone:        "9123456780",
					OfficePhoneCountry: "+91",
					Owner:              "KAM One",
					Stage:              "Prospecting",
					Status:             "Open",
					Sentiment:          "Positive",
					Priority:           "High",
					KamName:            "KAM One",
					BasicRequirements:  "Some requirements",
				},
				{
					Company:            "Unique Company A", // Case-insensitive duplicate company in request
					Contact:            "John A2",
					Email:              "johnA2@company.com",
					Phone:              "9876543210",
					OfficePhone:        "9123456780",
					OfficePhoneCountry: "+91",
					Owner:              "KAM One",
					Stage:              "Prospecting",
					Status:             "Open",
					Sentiment:          "Positive",
					Priority:           "High",
					KamName:            "KAM One",
					BasicRequirements:  "Some requirements",
				},
			},
		}

		resp, err := leadService.BulkCreateLeads("admin", "test_admin@pmrgsolution.com", req)
		if err != nil {
			t.Fatalf("Bulk creation failed completely: %v", err)
		}

		if resp.Summary.Total != 4 {
			t.Errorf("Expected total 4, got %d", resp.Summary.Total)
		}
		if resp.Summary.Created != 1 {
			t.Errorf("Expected 1 created lead, got %d", resp.Summary.Created)
		}
		if resp.Summary.Failed != 3 {
			t.Errorf("Expected 3 failed leads, got %d", resp.Summary.Failed)
		}

		// Verify duplicate email failure description
		if resp.Failed[0].Errors["email"] == "" {
			t.Errorf("Expected duplicate email error for item 1")
		}
	})

	t.Run("Bulk Create Leads - Concurrency lead_id_seq safety", func(t *testing.T) {
		// Run concurrent bulk creations to check lead_id sequence uniqueness
		var wg sync.WaitGroup
		concurrencyCount := 3
		wg.Add(concurrencyCount)

		for i := 0; i < concurrencyCount; i++ {
			go func(idx int) {
				defer wg.Done()
				compName := fmt.Sprintf("Concurrent Comp %d", idx)
				email := fmt.Sprintf("concurrent%d@comp.com", idx)
				req := models.BulkCreateLeadsRequest{
					Leads: []models.CreateLeadRequest{
						{
							Company:            compName,
							Contact:            "John Doe",
							Email:              email,
							Phone:              "9876543210",
							OfficePhone:        "9123456780",
							OfficePhoneCountry: "+91",
							Owner:              "KAM One",
							Stage:              "prospecting", // tests case insensitivity
							Status:             "Open",
							Sentiment:          "Positive",
							Priority:           "High",
							KamName:            "KAM One",
							BasicRequirements:  "Concurrent test requirements",
						},
					},
				}
				_, _ = leadService.BulkCreateLeads("admin", "test_admin@pmrgsolution.com", req)
			}(i)
		}
		wg.Wait()

		// Verify no duplicate LeadIDs generated
		var leadIDs []string
		_ = gormDB.Model(&models.Lead{}).Pluck("lead_id", &leadIDs).Error
		seen := make(map[string]bool)
		for _, lid := range leadIDs {
			if seen[lid] {
				t.Errorf("Found duplicate LeadID in database: %s", lid)
			}
			seen[lid] = true
		}
	})
}

func timePtr(t time.Time) *time.Time {
	return &t
}
