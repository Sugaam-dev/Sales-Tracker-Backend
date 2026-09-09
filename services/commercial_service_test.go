package services

import (
	"context"
	"math"
	"testing"
	"time"

	"crm-auth-service/conf"
	"crm-auth-service/helpers"
	"crm-auth-service/models"
	"crm-auth-service/repository"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func floatPtr(f float64) *float64 {
	return &f
}

func intPtr(i int) *int {
	return &i
}

func TestDateSolverBusinessLogic(t *testing.T) {
	defaultStart, _ := time.Parse("2006-01-02", "2026-09-01")

	t.Run("Case 1: Start Date + Duration -> End Date", func(t *testing.T) {
		start := "2026-09-01"
		dur := 6
		s, d, e, err := SolveDates(&start, &dur, nil, defaultStart)
		if err != nil {
			t.Fatalf("SolveDates failed: %v", err)
		}
		if d != 6 {
			t.Errorf("Expected duration 6, got %d", d)
		}
		expectedEnd := "2027-03-01"
		if e.Format("2006-01-02") != expectedEnd {
			t.Errorf("Expected end date %s, got %s", expectedEnd, e.Format("2006-01-02"))
		}
		if s.Format("2006-01-02") != "2026-09-01" {
			t.Errorf("Expected start date 2026-09-01, got %s", s.Format("2006-01-02"))
		}
	})

	t.Run("Case 2: Start Date + End Date -> Duration", func(t *testing.T) {
		start := "2026-09-01"
		end := "2027-03-01"
		s, d, e, err := SolveDates(&start, nil, &end, defaultStart)
		if err != nil {
			t.Fatalf("SolveDates failed: %v", err)
		}
		if d != 6 {
			t.Errorf("Expected duration 6 months, got %d", d)
		}
		if s.Format("2006-01-02") != start || e.Format("2006-01-02") != end {
			t.Errorf("Dates mismatch: start=%v, end=%v", s, e)
		}
	})

	t.Run("Case 3: Duration + End Date -> Start Date", func(t *testing.T) {
		dur := 6
		end := "2027-03-01"
		s, d, e, err := SolveDates(nil, &dur, &end, defaultStart)
		if err != nil {
			t.Fatalf("SolveDates failed: %v", err)
		}
		if d != 6 {
			t.Errorf("Expected duration 6, got %d", d)
		}
		expectedStart := "2026-09-01"
		if s.Format("2006-01-02") != expectedStart {
			t.Errorf("Expected start date %s, got %s", expectedStart, s.Format("2006-01-02"))
		}
		if e.Format("2006-01-02") != end {
			t.Errorf("Expected end date %s, got %s", end, e.Format("2006-01-02"))
		}
	})

	t.Run("All Three Provided: Start + Duration Wins", func(t *testing.T) {
		start := "2026-09-01"
		dur := 12
		conflictingEnd := "2027-01-01"
		s, d, e, err := SolveDates(&start, &dur, &conflictingEnd, defaultStart)
		if err != nil {
			t.Fatalf("SolveDates failed: %v", err)
		}
		if d != 12 {
			t.Errorf("Expected duration 12, got %d", d)
		}
		expectedEnd := "2027-09-01"
		if e.Format("2006-01-02") != expectedEnd {
			t.Errorf("Expected recalculated end date %s, got %s", expectedEnd, e.Format("2006-01-02"))
		}
		if s.Format("2006-01-02") != start {
			t.Errorf("Expected start date %s, got %s", start, s.Format("2006-01-02"))
		}
	})

	t.Run("Invalid: End Date Before Start Date", func(t *testing.T) {
		start := "2027-09-01"
		end := "2026-09-01"
		_, _, _, err := SolveDates(&start, nil, &end, defaultStart)
		if err == nil {
			t.Error("Expected error when end date is before start date, got nil")
		}
	})
}

func TestCurrencyConversionAndPrecision(t *testing.T) {
	baseUSD := 100.00

	usd := Convert(baseUSD, models.CurrencyUSD)
	if usd != 100.00 {
		t.Errorf("Expected USD 100.00, got %f", usd)
	}

	eur := Convert(baseUSD, models.CurrencyEUR)
	if eur != 92.00 {
		t.Errorf("Expected EUR 92.00, got %f", eur)
	}

	gbp := Convert(baseUSD, models.CurrencyGBP)
	if gbp != 79.00 {
		t.Errorf("Expected GBP 79.00, got %f", gbp)
	}

	inr := Convert(baseUSD, models.CurrencyINR)
	if inr != 8350.00 {
		t.Errorf("Expected INR 8350.00, got %f", inr)
	}

	// Conversion back to base USD
	inrBack := ConvertToBaseUSD(8350.00, models.CurrencyINR)
	if math.Abs(inrBack-100.00) > 0.01 {
		t.Errorf("Expected INR 8350.00 back to USD 100.00, got %f", inrBack)
	}
}

func TestFinancialCalculationEngine(t *testing.T) {
	resources := []models.CommercialResource{
		{
			Role:         "Tech Lead",
			Grade:        "L3",
			OnsiteDays:   20,
			OffshoreDays: 100,
			DailyCost:    450.00,
			BillingRate:  800.00,
		},
		{
			Role:         "Senior Dev",
			Grade:        "L2",
			OnsiteDays:   0,
			OffshoreDays: 100,
			DailyCost:    300.00,
			BillingRate:  600.00,
		},
	}

	expenses := []models.CommercialExpense{
		{
			ExpenseType: "Travel",
			Cost:        5000.00,
		},
		{
			ExpenseType: "Cloud Hosting",
			Cost:        3000.00,
		},
	}

	// Resource 1: (20 + 100) * 450 = 54,000 cost; (20 + 100) * 800 = 96,000 revenue
	// Resource 2: 100 * 300 = 30,000 cost; 100 * 600 = 60,000 revenue
	// Total Resource Cost = 84,000
	// Total Expenses = 8,000
	// Total Project Cost = 92,000
	// Markup = 20%, Discount = 5%
	// Calculated Selling Price = 92,000 * 1.20 * 0.95 = 104,880.00
	// Gross Profit = 104,880 - 92,000 = 12,880.00
	// Margin % = (12,880 / 104,880) * 100 = 12.28%
	// ROI % = (12,880 / 92,000) * 100 = 14.00%
	summary := CalculateFinancialSummary(resources, expenses, 20.0, 5.0, nil, 6)

	if summary.TotalResourceCost != 84000.00 {
		t.Errorf("Expected resource cost 84000.00, got %f", summary.TotalResourceCost)
	}
	if summary.TotalExpenses != 8000.00 {
		t.Errorf("Expected total expenses 8000.00, got %f", summary.TotalExpenses)
	}
	if summary.TotalProjectCost != 92000.00 {
		t.Errorf("Expected project cost 92000.00, got %f", summary.TotalProjectCost)
	}
	if summary.CalculatedSellingPrice != 104880.00 {
		t.Errorf("Expected calculated selling price 104880.00, got %f", summary.CalculatedSellingPrice)
	}
	if summary.EffectiveSellingPrice != 104880.00 {
		t.Errorf("Expected effective selling price 104880.00, got %f", summary.EffectiveSellingPrice)
	}
	if summary.GrossProfit != 12880.00 {
		t.Errorf("Expected gross profit 12880.00, got %f", summary.GrossProfit)
	}
	if summary.MarginPercent != 12.28 {
		t.Errorf("Expected margin percent 12.28%%, got %f", summary.MarginPercent)
	}
	if summary.ROIPercent != 14.00 {
		t.Errorf("Expected ROI percent 14.00%%, got %f", summary.ROIPercent)
	}

	// Test Manual Selling Price Override
	manualPrice := 120000.00
	summaryWithManual := CalculateFinancialSummary(resources, expenses, 20.0, 5.0, &manualPrice, 6)
	if summaryWithManual.EffectiveSellingPrice != 120000.00 {
		t.Errorf("Expected manual selling price override 120000.00, got %f", summaryWithManual.EffectiveSellingPrice)
	}
	if summaryWithManual.GrossProfit != 28000.00 {
		t.Errorf("Expected gross profit 28000.00, got %f", summaryWithManual.GrossProfit)
	}

	// Zero-division safety test
	zeroSummary := CalculateFinancialSummary(nil, nil, 0, 0, nil, 1)
	if zeroSummary.MarginPercent != 0.0 || zeroSummary.ROIPercent != 0.0 {
		t.Errorf("Expected 0%% margin and ROI for zero inputs, got margin=%f, roi=%f", zeroSummary.MarginPercent, zeroSummary.ROIPercent)
	}
}

func TestCommercialIntegrationLifecycle(t *testing.T) {
	cfg, err := conf.LoadConfig()
	if err != nil {
		t.Skip("Skipping integration test; config not loaded or database credentials not found in env")
		return
	}

	log := helpers.NewLogger(cfg.Server.Env)
	pool, err := conf.ConnectDB(cfg.DB, log)
	if err != nil {
		t.Skipf("Skipping database integration test; database not reachable: %v", err)
		return
	}
	defer pool.Close()

	ctx := context.Background()
	importGormStr := cfg.DB.DSN(cfg.DB.Name)
	gormDB, err := gorm.Open(postgres.Open(importGormStr), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect gorm: %v", err)
	}

	userRepo := repository.NewUserRepository(pool)
	leadRepo := repository.NewLeadRepository(pool, gormDB)
	commRepo := repository.NewCommercialRepository(pool, gormDB)
	commService := NewCommercialService(commRepo, leadRepo, userRepo)

	// Clean up existing test data
	_, _ = pool.Exec(ctx, "DELETE FROM sdlc_allocations WHERE commercial_estimation_id IN (SELECT id FROM commercial_estimations WHERE lead_id LIKE 'L-COMM-%')")
	_, _ = pool.Exec(ctx, "DELETE FROM commercial_expenses WHERE commercial_estimation_id IN (SELECT id FROM commercial_estimations WHERE lead_id LIKE 'L-COMM-%')")
	_, _ = pool.Exec(ctx, "DELETE FROM commercial_resources WHERE commercial_estimation_id IN (SELECT id FROM commercial_estimations WHERE lead_id LIKE 'L-COMM-%')")
	_, _ = pool.Exec(ctx, "DELETE FROM commercial_estimations WHERE lead_id LIKE 'L-COMM-%'")
	_, _ = pool.Exec(ctx, "DELETE FROM activities WHERE lead_id LIKE 'L-COMM-%'")
	_, _ = pool.Exec(ctx, "DELETE FROM leads WHERE lead_id LIKE 'L-COMM-%' OR email LIKE 'test_comm_%'")
	_, _ = pool.Exec(ctx, "DELETE FROM users WHERE email LIKE 'test_comm_%'")

	// Setup test users
	_ = gormDB.Create(&models.User{
		Name:         "Admin User",
		Email:        "test_comm_admin@pmrg.com",
		PasswordHash: "hash",
		Role:         models.RoleAdmin,
		IsActive:     true,
	})
	_ = gormDB.Create(&models.User{
		Name:         "KAM Sahil",
		Email:        "test_comm_sahil@pmrg.com",
		PasswordHash: "hash",
		Role:         models.RoleSalesExecutive,
		IsActive:     true,
	})
	_ = gormDB.Create(&models.User{
		Name:         "KAM Other",
		Email:        "test_comm_other@pmrg.com",
		PasswordHash: "hash",
		Role:         models.RoleSalesExecutive,
		IsActive:     true,
	})

	// Setup test lead
	lead := &models.Lead{
		LeadID:      "L-7001",
		Company:     "Acme International",
		ProjectName: strPtr("ERP Modernization"),
		Contact:     strPtr("John Doe"),
		Email:       strPtr("jdoe@acme.com"),
		Phone:       strPtr("9876543210"),
		OfficePhone: strPtr("9123456780"),
		Owner:       strPtr("KAM Sahil"),
		Stage:       strPtr("Proposal"),
		Status:      strPtr("Interested"),
		Sentiment:   strPtr("Positive"),
		Priority:    strPtr("High"),
	}
	_ = gormDB.Create(lead)

	t.Run("Lazy Initialization & Lead Context Population", func(t *testing.T) {
		// First access triggers lazy draft creation
		resp, err := commService.GetCommercial(ctx, "L-7001", models.RoleSalesExecutive, "test_comm_sahil@pmrg.com", "USD")
		if err != nil {
			t.Fatalf("GetCommercial failed: %v", err)
		}

		if resp.LeadContext.LeadID != "L-7001" || resp.LeadContext.Company != "Acme International" {
			t.Errorf("Lead context mismatch: %+v", resp.LeadContext)
		}
		if resp.CommercialEstimation.Status != models.CommercialStatusDraft {
			t.Errorf("Expected DRAFT status, got %s", resp.CommercialEstimation.Status)
		}
		if resp.CommercialEstimation.Currency != "USD" {
			t.Errorf("Expected USD currency, got %s", resp.CommercialEstimation.Currency)
		}

		// Ensure second call returns same record (1:1 uniqueness)
		var count int64
		_ = gormDB.Model(&models.CommercialEstimation{}).Where("lead_id = ?", "L-7001").Count(&count)
		if count != 1 {
			t.Errorf("Expected exactly 1 commercial estimation for lead, got %d", count)
		}
	})

	t.Run("Authorization Enforcement", func(t *testing.T) {
		// Non-owner KAM Other cannot access lead L-7001
		_, err := commService.GetCommercial(ctx, "L-7001", models.RoleSalesExecutive, "test_comm_other@pmrg.com", "USD")
		if err != ErrUnauthorized {
			t.Errorf("Expected ErrUnauthorized for non-owner, got %v", err)
		}

		// Admin can access lead L-7001
		_, err = commService.GetCommercial(ctx, "L-7001", models.RoleAdmin, "test_comm_admin@pmrg.com", "USD")
		if err != nil {
			t.Errorf("Admin should have access, got error: %v", err)
		}
	})

	t.Run("Save / Update Commercial Aggregate and Currency View", func(t *testing.T) {
		updateReq := models.UpdateCommercialRequest{
			Currency:                strPtr("USD"),
			BillingType:             strPtr("T&M"),
			StartDate:               strPtr("2026-10-01"),
			EstimatedDurationMonths: intPtr(6),
			EstimatedEndDate:        strPtr("2027-04-01"),
			MarkupPercent:           floatPtr(20.0),
			DiscountPercent:         floatPtr(5.0),
			Resources: &[]models.CommercialResourceDTO{
				{
					Role:         "Tech Lead",
					Grade:        "L3",
					OnsiteDays:   20,
					OffshoreDays: 80,
					DailyCost:    500.00,
					BillingRate:  900.00,
				},
			},
			Expenses: &[]models.CommercialExpenseDTO{
				{
					ExpenseType: "Travel",
					Cost:        4000.00,
					Remarks:     strPtr("Client Kickoff"),
				},
			},
			SDLCAllocations: &[]models.SDLCAllocationDTO{
				{
					Phase:   "Requirements",
					ManDays: 20,
				},
				{
					Phase:   "Development",
					ManDays: 80,
				},
			},
		}

		resp, err := commService.UpdateCommercial(ctx, "L-7001", models.RoleSalesExecutive, "test_comm_sahil@pmrg.com", "INR", updateReq)
		if err != nil {
			t.Fatalf("UpdateCommercial failed: %v", err)
		}

		// Verify returned values are in INR (exchange rate: 1 USD = 83.50 INR)
		if resp.CommercialEstimation.Currency != "INR" {
			t.Errorf("Expected INR currency in response, got %s", resp.CommercialEstimation.Currency)
		}

		// Base Resource Cost = (20 + 80) * 500 = 50,000 USD -> 50,000 * 83.50 = 4,175,000 INR
		expectedResCostINR := 50000.0 * 83.50
		if resp.CommercialEstimation.FinancialSummary.TotalResourceCost != expectedResCostINR {
			t.Errorf("Expected resource cost %f INR, got %f", expectedResCostINR, resp.CommercialEstimation.FinancialSummary.TotalResourceCost)
		}

		// Base Expenses = 4,000 USD -> 4,000 * 83.50 = 334,000 INR
		expectedExpCostINR := 4000.0 * 83.50
		if resp.CommercialEstimation.FinancialSummary.TotalExpenses != expectedExpCostINR {
			t.Errorf("Expected expenses %f INR, got %f", expectedExpCostINR, resp.CommercialEstimation.FinancialSummary.TotalExpenses)
		}

		// SDLC percentages: Requirements = 20 / 100 = 20%, Development = 80 / 100 = 80%
		if len(resp.CommercialEstimation.SDLCAllocations) != 2 {
			t.Fatalf("Expected 2 SDLC allocations, got %d", len(resp.CommercialEstimation.SDLCAllocations))
		}
		if resp.CommercialEstimation.SDLCAllocations[0].Percentage != 20.0 {
			t.Errorf("Expected Requirements 20%%, got %f", resp.CommercialEstimation.SDLCAllocations[0].Percentage)
		}
		if resp.CommercialEstimation.SDLCAllocations[1].Percentage != 80.0 {
			t.Errorf("Expected Development 80%%, got %f", resp.CommercialEstimation.SDLCAllocations[1].Percentage)
		}
	})

	t.Run("Partial Update - Preserves Child Records When Omitted", func(t *testing.T) {
		// Update only status and markup, leaving resources, expenses, SDLC allocations nil (omitted)
		partialReq := models.UpdateCommercialRequest{
			Status:        strPtr(models.CommercialStatusSubmitted),
			MarkupPercent: floatPtr(25.0),
		}

		resp, err := commService.UpdateCommercial(ctx, "L-7001", models.RoleSalesExecutive, "test_comm_sahil@pmrg.com", "USD", partialReq)
		if err != nil {
			t.Fatalf("Partial UpdateCommercial failed: %v", err)
		}

		if resp.CommercialEstimation.Status != models.CommercialStatusSubmitted {
			t.Errorf("Expected status %s, got %s", models.CommercialStatusSubmitted, resp.CommercialEstimation.Status)
		}

		// Verify that previously saved resources, expenses, and SDLC allocations are PRESERVED and NOT deleted!
		if len(resp.CommercialEstimation.Resources) != 1 {
			t.Fatalf("Resources wiped on partial update! Expected 1 resource, got %d", len(resp.CommercialEstimation.Resources))
		}
		if len(resp.CommercialEstimation.Expenses) != 1 {
			t.Fatalf("Expenses wiped on partial update! Expected 1 expense, got %d", len(resp.CommercialEstimation.Expenses))
		}
		if len(resp.CommercialEstimation.SDLCAllocations) != 2 {
			t.Fatalf("SDLC allocations wiped on partial update! Expected 2 SDLC allocations, got %d", len(resp.CommercialEstimation.SDLCAllocations))
		}

		// Retrieve again via fresh GetCommercial call to confirm database persistence
		freshGet, err := commService.GetCommercial(ctx, "L-7001", models.RoleSalesExecutive, "test_comm_sahil@pmrg.com", "USD")
		if err != nil {
			t.Fatalf("Fresh GetCommercial failed: %v", err)
		}
		if len(freshGet.CommercialEstimation.Resources) != 1 || len(freshGet.CommercialEstimation.Expenses) != 1 || len(freshGet.CommercialEstimation.SDLCAllocations) != 2 {
			t.Errorf("Database persistence failure on fresh retrieval: %+v", freshGet.CommercialEstimation)
		}
	})

	t.Run("Full Expected Scenario - 4 Resources + 6 Expenses", func(t *testing.T) {
		scenarioReq := models.UpdateCommercialRequest{
			Currency:                strPtr("USD"),
			BillingType:             strPtr("T&M"),
			StartDate:               strPtr("2026-09-01"),
			EstimatedDurationMonths: intPtr(6),
			EstimatedEndDate:        strPtr("2027-03-01"),
			MarkupPercent:           floatPtr(20.0),
			DiscountPercent:         floatPtr(5.0),
			Resources: &[]models.CommercialResourceDTO{
				{Role: "Senior Architect", Grade: "L3 (Senior)", OnsiteDays: 20, OffshoreDays: 40, DailyCost: 380, BillingRate: 650},
				{Role: "Software Developer", Grade: "L1 (Junior)", OnsiteDays: 10, OffshoreDays: 120, DailyCost: 180, BillingRate: 300},
				{Role: "QA Lead", Grade: "L2 (Mid)", OnsiteDays: 5, OffshoreDays: 60, DailyCost: 220, BillingRate: 380},
				{Role: "Project Manager", Grade: "L3 (Senior)", OnsiteDays: 15, OffshoreDays: 30, DailyCost: 380, BillingRate: 580},
			},
			Expenses: &[]models.CommercialExpenseDTO{
				{ExpenseType: "Travel", Cost: 6500, Remarks: strPtr("Client site visits")},
				{ExpenseType: "Accommodation", Cost: 8000, Remarks: strPtr("Hotel stays for onsite crew")},
				{ExpenseType: "Cloud Hosting", Cost: 2400, Remarks: strPtr("AWS testing infrastructure")},
				{ExpenseType: "Software Licenses", Cost: 1800, Remarks: strPtr("Vite & Recharts premium toolsets")},
				{ExpenseType: "Third Party APIs", Cost: 1500, Remarks: strPtr("Payment Gateway Integration")},
				{ExpenseType: "Miscellaneous", Cost: 1200, Remarks: strPtr("Contingency backup")},
			},
			SDLCAllocations: &[]models.SDLCAllocationDTO{
				{Phase: "Requirements", ManDays: 20},
				{Phase: "Development", ManDays: 180},
				{Phase: "Testing", ManDays: 80},
			},
		}

		resp, err := commService.UpdateCommercial(ctx, "L-7001", models.RoleSalesExecutive, "test_comm_sahil@pmrg.com", "USD", scenarioReq)
		if err != nil {
			t.Fatalf("Scenario update failed: %v", err)
		}

		// Expected calculations:
		// Resource Cost: 22800 + 23400 + 14300 + 17100 = 77,600
		// Total Expenses: 6500 + 8000 + 2400 + 1800 + 1500 + 1200 = 21,400
		// Total Project Cost: 77,600 + 21,400 = 99,000
		// Calculated Selling Price: 99,000 * 1.20 * 0.95 = 112,860
		// Gross Profit: 112,860 - 99,000 = 13,860
		// Margin %: (13,860 / 112,860) * 100 = 12.28%
		// ROI %: (13,860 / 99,000) * 100 = 14.00%
		if resp.CommercialEstimation.FinancialSummary.TotalResourceCost != 77600.00 {
			t.Errorf("Expected TotalResourceCost 77600, got %f", resp.CommercialEstimation.FinancialSummary.TotalResourceCost)
		}
		if resp.CommercialEstimation.FinancialSummary.TotalExpenses != 21400.00 {
			t.Errorf("Expected TotalExpenses 21400, got %f", resp.CommercialEstimation.FinancialSummary.TotalExpenses)
		}
		if resp.CommercialEstimation.FinancialSummary.TotalProjectCost != 99000.00 {
			t.Errorf("Expected TotalProjectCost 99000, got %f", resp.CommercialEstimation.FinancialSummary.TotalProjectCost)
		}
		if resp.CommercialEstimation.FinancialSummary.CalculatedSellingPrice != 112860.00 {
			t.Errorf("Expected CalculatedSellingPrice 112860, got %f", resp.CommercialEstimation.FinancialSummary.CalculatedSellingPrice)
		}
		if resp.CommercialEstimation.FinancialSummary.GrossProfit != 13860.00 {
			t.Errorf("Expected GrossProfit 13860, got %f", resp.CommercialEstimation.FinancialSummary.GrossProfit)
		}
		if resp.CommercialEstimation.FinancialSummary.MarginPercent != 12.28 {
			t.Errorf("Expected MarginPercent 12.28, got %f", resp.CommercialEstimation.FinancialSummary.MarginPercent)
		}
		if resp.CommercialEstimation.FinancialSummary.ROIPercent != 14.00 {
			t.Errorf("Expected ROIPercent 14.00, got %f", resp.CommercialEstimation.FinancialSummary.ROIPercent)
		}
	})

	t.Run("Unified Analytics Endpoint Verification", func(t *testing.T) {
		analytics, err := commService.GetAnalytics(ctx, "L-7001", models.RoleSalesExecutive, "test_comm_sahil@pmrg.com", "USD")
		if err != nil {
			t.Fatalf("GetAnalytics failed: %v", err)
		}

		if analytics.Currency != "USD" {
			t.Errorf("Expected USD analytics currency, got %s", analytics.Currency)
		}

		// Verify Grade-wise allocation chart has 3 distinct grades: L1 (Junior), L2 (Mid), L3 (Senior)
		if len(analytics.GradeWiseAllocation.Labels) != 3 {
			t.Errorf("Expected 3 grade labels, got %d (%+v)", len(analytics.GradeWiseAllocation.Labels), analytics.GradeWiseAllocation.Labels)
		}

		// Verify Cost Breakdown chart has 7 categories: Resource Cost + 6 expense types
		if len(analytics.CostBreakdown.Labels) != 7 {
			t.Errorf("Expected 7 cost categories (Resource Cost + 6 expenses), got %d (%+v)", len(analytics.CostBreakdown.Labels), analytics.CostBreakdown.Labels)
		}

		// Verify Cumulative Cash Flow Projection
		if len(analytics.CumulativeCashFlow.Labels) != 6 {
			t.Errorf("Expected 6 month labels for 6-month project, got %d", len(analytics.CumulativeCashFlow.Labels))
		}
		if len(analytics.CumulativeCashFlow.Datasets) != 3 {
			t.Errorf("Expected 3 datasets (Cumulative Cost, Revenue, Cash Position), got %d", len(analytics.CumulativeCashFlow.Datasets))
		}
	})
}
