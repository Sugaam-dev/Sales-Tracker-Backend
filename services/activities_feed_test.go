package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"crm-auth-service/conf"
	"crm-auth-service/helpers"
	"crm-auth-service/models"
	"crm-auth-service/repository"
)

func TestActivitiesFeedAndSummaryAPIs(t *testing.T) {
	cfg, err := conf.LoadConfig()
	if err != nil {
		t.Skip("Skipping integration test: database configuration not available")
		return
	}

	log := helpers.NewLogger(cfg.Server.Env)
	pool, err := conf.ConnectDB(cfg.DB, log)
	if err != nil {
		t.Skipf("Skipping integration test: cannot connect to database: %v", err)
		return
	}
	defer pool.Close()

	dsn := cfg.DB.DSN(cfg.DB.Name)
	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("Skipping integration test: cannot connect to gorm: %v", err)
		return
	}

	ctx := context.Background()
	userRepo := repository.NewUserRepository(pool)
	leadRepo := repository.NewLeadRepository(pool, gormDB)
	leadService := NewLeadService(leadRepo, userRepo)

	// Seed test user
	testUserUUID := uuid.New()
	testUser := models.User{
		ID:           testUserUUID,
		Name:         "Sneha Rep",
		Email:        fmt.Sprintf("sneha_rep_%d@example.com", time.Now().UnixNano()),
		PasswordHash: "dummyhash",
		Role:         models.RoleSalesExecutive,
		IsActive:     true,
	}
	_ = gormDB.Create(&testUser).Error

	// Seed test lead
	p := time.Now().UnixNano()
	testLead := models.Lead{
		LeadID:                   fmt.Sprintf("L-ACT-%d", p%10000),
		Company:                  fmt.Sprintf("Activities Test Corp %d", p),
		Contact:                  strPtr("Test Contact"),
		Email:                    strPtr(fmt.Sprintf("act_lead_%d@example.com", p)),
		Phone:                    strPtr("9876543210"),
		Owner:                    strPtr("KAM One"),
		Stage:                    strPtr("Prospecting"),
		Status:                   strPtr("Open"),
		Sentiment:                strPtr("Positive"),
		Priority:                 strPtr("High"),
		Region:                   strPtr("Asia"),
		Industry:                 strPtr("Technology"),
		Size:                     strPtr("Enterprise"),
		RequestType:              strPtr("IT Product"),
		RequestDetails:           strPtr("We require a comprehensive CRM solution for our sales team with full lead lifecycle and activities management word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word"),
	}
	_ = gormDB.Create(&testLead).Error

	t.Run("API-18: POST /api/v1/activities - Lead Resolution & Logging", func(t *testing.T) {
		// 1. Create activity using leadId
		req1 := models.LogActivityRequest{
			Type:    "Call",
			LeadID:  testLead.LeadID,
			Desc:    "Initial discovery call with client",
			Outcome: "Interested in demo",
			DueDate: time.Now().AddDate(0, 0, 2).Format("2006-01-02"),
		}
		act1, err := leadService.LogActivity(ctx, testUser.ID, testUser.Role, testUser.Email, req1)
		if err != nil {
			t.Fatalf("LogActivity with leadId failed: %v", err)
		}
		if act1.LeadID != testLead.LeadID {
			t.Errorf("Expected leadId %s, got %s", testLead.LeadID, act1.LeadID)
		}
		if act1.Type != "Call" {
			t.Errorf("Expected type Call, got %s", act1.Type)
		}
		if act1.Rep == nil || (*act1.Rep != testUser.Name && *act1.Rep != testUser.Email) {
			t.Errorf("Expected rep %s, got %v", testUser.Name, act1.Rep)
		}

		// 2. Create activity using company name fallback
		req2 := models.LogActivityRequest{
			Type:    "email",
			Lead:    testLead.Company,
			Desc:    "Follow up email sent with proposal draft",
			Outcome: "Awaiting response",
		}
		act2, err := leadService.LogActivity(ctx, testUser.ID, testUser.Role, testUser.Email, req2)
		if err != nil {
			t.Fatalf("LogActivity with company name failed: %v", err)
		}
		if act2.LeadID != testLead.LeadID {
			t.Errorf("Expected leadId %s from company lookup, got %s", testLead.LeadID, act2.LeadID)
		}
		if act2.Type != "Email" {
			t.Errorf("Expected normalized type Email, got %s", act2.Type)
		}

		// 3. Both leadId and lead supplied -> leadId takes precedence
		req3 := models.LogActivityRequest{
			Type:    "Meeting",
			LeadID:  testLead.LeadID,
			Lead:    "Non Existent Company",
			Desc:    "Executive strategy meeting",
			Outcome: "Positive",
		}
		act3, err := leadService.LogActivity(ctx, testUser.ID, testUser.Role, testUser.Email, req3)
		if err != nil {
			t.Fatalf("LogActivity with both leadId and lead failed: %v", err)
		}
		if act3.LeadID != testLead.LeadID {
			t.Errorf("Expected leadId %s, got %s", testLead.LeadID, act3.LeadID)
		}

		// 4. Invalid lead -> 404
		req4 := models.LogActivityRequest{
			Type:   "Demo",
			LeadID: "L-NONEXISTENT-9999",
			Desc:   "Product demonstration",
		}
		_, err = leadService.LogActivity(ctx, testUser.ID, testUser.Role, testUser.Email, req4)
		if err == nil {
			t.Errorf("Expected error for non-existent lead, got nil")
		}

		// 5. Invalid activity type -> 400
		req5 := models.LogActivityRequest{
			Type:   "InvalidTypeXYZ",
			LeadID: testLead.LeadID,
			Desc:   "Invalid type test",
		}
		_, err = leadService.LogActivity(ctx, testUser.ID, testUser.Role, testUser.Email, req5)
		if err == nil {
			t.Errorf("Expected error for invalid activity type, got nil")
		}

		// 6. Invalid due date format -> 400
		req6 := models.LogActivityRequest{
			Type:    "Call",
			LeadID:  testLead.LeadID,
			Desc:    "Bad date test",
			DueDate: "09-10-2026", // Invalid format
		}
		_, err = leadService.LogActivity(ctx, testUser.ID, testUser.Role, testUser.Email, req6)
		if err == nil {
			t.Errorf("Expected error for invalid dueDate format, got nil")
		}
	})

	t.Run("API-17: GET /api/v1/activities - Feed, Filters, Pagination, and Type Counts", func(t *testing.T) {
		// 1. Fetch all activities for the test user
		feedResp, err := leadService.GetActivities(ctx, testUser.ID, testUser.Role, models.GetActivitiesQuery{
			UserID: testUser.ID.String(),
			Page:   1,
			Limit:  20,
		})
		if err != nil {
			t.Fatalf("GetActivities failed: %v", err)
		}
		if len(feedResp.Data) == 0 {
			t.Errorf("Expected at least 1 activity in feed, got 0")
		}
		if feedResp.TypeCounts.All == 0 {
			t.Errorf("Expected type_counts.all > 0, got 0")
		}

		// 2. Fetch with type=Call filter: data should only contain Calls, but type_counts should reflect all types
		callFeedResp, err := leadService.GetActivities(ctx, testUser.ID, testUser.Role, models.GetActivitiesQuery{
			UserID: testUser.ID.String(),
			Type:   "Call",
			Page:   1,
			Limit:  20,
		})
		if err != nil {
			t.Fatalf("GetActivities with type filter failed: %v", err)
		}
		for _, item := range callFeedResp.Data {
			if item.Type != "Call" {
				t.Errorf("Expected item type Call, got %s", item.Type)
			}
		}
		// type_counts.all and type_counts.email should NOT be reset by type=Call filter
		if callFeedResp.TypeCounts.All != feedResp.TypeCounts.All {
			t.Errorf("Expected type_counts.all to be %d, got %d", feedResp.TypeCounts.All, callFeedResp.TypeCounts.All)
		}
		if callFeedResp.TypeCounts.Email != feedResp.TypeCounts.Email {
			t.Errorf("Expected type_counts.email to be %d, got %d", feedResp.TypeCounts.Email, callFeedResp.TypeCounts.Email)
		}

		// 3. Multi-filter AND semantics
		multiResp, err := leadService.GetActivities(ctx, testUser.ID, testUser.Role, models.GetActivitiesQuery{
			LeadID:    testLead.LeadID,
			Geography: "Asia",
			Industry:  "Technology",
			DealSize:  "Enterprise",
			Page:      1,
			Limit:     10,
		})
		if err != nil {
			t.Fatalf("GetActivities multi-filter failed: %v", err)
		}
		if len(multiResp.Data) == 0 {
			t.Errorf("Expected matching activities for test lead with matching filters, got 0")
		}

		// 4. Pagination
		paginatedResp, err := leadService.GetActivities(ctx, testUser.ID, testUser.Role, models.GetActivitiesQuery{
			LeadID: testLead.LeadID,
			Page:   1,
			Limit:  1,
		})
		if err != nil {
			t.Fatalf("GetActivities pagination failed: %v", err)
		}
		if len(paginatedResp.Data) != 1 {
			t.Errorf("Expected exactly 1 item with limit=1, got %d", len(paginatedResp.Data))
		}
		if paginatedResp.Pagination == nil || paginatedResp.Pagination.Limit != 1 {
			t.Errorf("Expected pagination limit 1, got %v", paginatedResp.Pagination)
		}
	})

	t.Run("API-19: GET /api/v1/activities/summary - Velocity Metrics", func(t *testing.T) {
		// Seed overdue activity: due_date in past, completed = false
		pastDate := time.Now().AddDate(0, 0, -3)
		overdueAct := models.Activity{
			LeadID:    testLead.LeadID,
			Rep:       &testUser.ID,
			Type:      "Call",
			Desc:      "Overdue call",
			DueDate:   &pastDate,
			Completed: false,
		}
		_ = gormDB.Create(&overdueAct).Error

		// Seed upcoming activity: due_date in next 3 days, completed = false
		futureDate := time.Now().AddDate(0, 0, 3)
		upcomingAct := models.Activity{
			LeadID:    testLead.LeadID,
			Rep:       &testUser.ID,
			Type:      "Meeting",
			Desc:      "Upcoming meeting",
			DueDate:   &futureDate,
			Completed: false,
		}
		_ = gormDB.Create(&upcomingAct).Error

		summary, err := leadService.GetActivitiesSummary(ctx, testUser.ID, testUser.Role)
		if err != nil {
			t.Fatalf("GetActivitiesSummary failed: %v", err)
		}

		if summary.VelocityTodayLogged <= 0 {
			t.Errorf("Expected velocity_today_logged > 0, got %d", summary.VelocityTodayLogged)
		}
		if summary.OverdueCount <= 0 {
			t.Errorf("Expected overdue_count > 0, got %d", summary.OverdueCount)
		}
		if summary.UpcomingCount <= 0 {
			t.Errorf("Expected upcoming_count > 0, got %d", summary.UpcomingCount)
		}
	})
}
