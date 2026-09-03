package services

import (
	"context"
	"testing"
	"time"

	"crm-auth-service/conf"
	"crm-auth-service/helpers"
	"crm-auth-service/models"
	"crm-auth-service/repository"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestDashboardSummaryAndReportsAnalytics(t *testing.T) {
	cfg, err := conf.LoadConfig()
	if err != nil {
		t.Skip("Skipping analytics test; DB credentials not in env")
		return
	}

	log := helpers.NewLogger(cfg.Server.Env)
	pool, err := conf.ConnectDB(cfg.DB, log)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()
	importGormStr := cfg.DB.DSN(cfg.DB.Name)
	gormDB, err := gorm.Open(postgres.Open(importGormStr), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect gorm: %v", err)
	}

	leadRepo := repository.NewLeadRepository(pool, gormDB)
	analyticsRepo := repository.NewAnalyticsRepository(pool, gormDB)
	analyticsService := NewAnalyticsService(analyticsRepo, leadRepo)

	// Clean up any test records
	_, _ = pool.Exec(ctx, "DELETE FROM activities WHERE lead_id IN ('L-9901', 'L-9902', 'L-9903', 'L-9904')")
	_, _ = pool.Exec(ctx, "DELETE FROM leads WHERE lead_id IN ('L-9901', 'L-9902', 'L-9903', 'L-9904')")

	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM activities WHERE lead_id IN ('L-9901', 'L-9902', 'L-9903', 'L-9904')")
		_, _ = pool.Exec(ctx, "DELETE FROM leads WHERE lead_id IN ('L-9901', 'L-9902', 'L-9903', 'L-9904')")
	}()

	// 1. Seed Test Leads
	now := time.Now()
	oneMonthAgo := now.AddDate(0, -1, 0)
	twoDaysAgo := now.AddDate(0, 0, -2)

	// Lead 1: Open deal in Proposal ($50,000, 50% prob) - North America - D. Ghosh
	_, err = pool.Exec(ctx, `
		INSERT INTO leads (lead_id, company, stage, status, value, owner, region, priority, created_at, updated_at)
		VALUES ('L-9901', 'Analytics Co 1', 'Proposal', 'Interested', 50000.00, 'D. Ghosh', 'North America', 'High', $1, $1)
	`, twoDaysAgo)
	if err != nil {
		t.Fatalf("Failed to seed lead 1: %v", err)
	}

	// Lead 2: Open deal in Negotiation ($100,000, 80% prob) - Europe - D. Ghosh
	_, err = pool.Exec(ctx, `
		INSERT INTO leads (lead_id, company, stage, status, value, owner, region, priority, created_at, updated_at)
		VALUES ('L-9902', 'Analytics Co 2', 'Negotiation', 'Negotiation', 100000.00, 'D. Ghosh', 'Europe', 'Urgent', $1, $1)
	`, twoDaysAgo)
	if err != nil {
		t.Fatalf("Failed to seed lead 2: %v", err)
	}

	// Lead 3: Won deal ($80,000) - India - S. Mishra (closed today, cycle = 30 days)
	_, err = pool.Exec(ctx, `
		INSERT INTO leads (lead_id, company, stage, status, value, owner, region, priority, created_at, updated_at)
		VALUES ('L-9903', 'Analytics Co 3', 'Closed Won', 'Won', 80000.00, 'S. Mishra', 'India', 'Normal', $1, $2)
	`, oneMonthAgo, now)
	if err != nil {
		t.Fatalf("Failed to seed lead 3: %v", err)
	}

	// Lead 4: Lost deal ($30,000) - LATAM - S. Mishra
	_, err = pool.Exec(ctx, `
		INSERT INTO leads (lead_id, company, stage, status, value, owner, region, priority, created_at, updated_at)
		VALUES ('L-9904', 'Analytics Co 4', 'Closed Lost', 'Lost', 30000.00, 'S. Mishra', 'LATAM', 'Low', $1, $2)
	`, oneMonthAgo, now)
	if err != nil {
		t.Fatalf("Failed to seed lead 4: %v", err)
	}

	// Seed Activities: 1 Overdue activity on L-9901, 1 Completed activity on L-9902
	yesterday := now.AddDate(0, 0, -1)
	tomorrow := now.AddDate(0, 0, 1)

	_, err = pool.Exec(ctx, `
		INSERT INTO activities (lead_id, type, "desc", due_date, completed)
		VALUES ('L-9901', 'Call', 'Follow-up call with prospect', $1, false)
	`, yesterday)
	if err != nil {
		t.Fatalf("Failed to seed overdue activity: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO activities (lead_id, type, "desc", due_date, completed)
		VALUES ('L-9902', 'Meeting', 'Demo walkthrough', $1, true)
	`, tomorrow)
	if err != nil {
		t.Fatalf("Failed to seed completed activity: %v", err)
	}

	// ----------------------------------------------------
	// TEST 1: GET /api/v1/dashboard/summary (Admin/Global)
	// ----------------------------------------------------
	t.Run("Dashboard Summary Global", func(t *testing.T) {
		summary, err := analyticsService.GetDashboardSummary(ctx, models.RoleAdmin, "admin@example.com", "", "")
		if err != nil {
			t.Fatalf("GetDashboardSummary returned error: %v", err)
		}

		if summary.PipelineValue < 150000.00 {
			t.Errorf("Expected pipeline_value >= 150000, got %f", summary.PipelineValue)
		}
		if summary.OpenDealsCount < 2 {
			t.Errorf("Expected open_deals_count >= 2, got %d", summary.OpenDealsCount)
		}
		// Expected value for L1 (50000 * 0.50 = 25000) + L2 (100000 * 0.80 = 80000) = 105000
		if summary.ExpectedValue < 105000.00 {
			t.Errorf("Expected expected_value >= 105000, got %f", summary.ExpectedValue)
		}
		if summary.WonLeadsCount < 1 {
			t.Errorf("Expected won_leads_count >= 1, got %d", summary.WonLeadsCount)
		}
		if summary.LostLeadsCount < 1 {
			t.Errorf("Expected lost_leads_count >= 1, got %d", summary.LostLeadsCount)
		}
		if summary.OverdueCount < 1 {
			t.Errorf("Expected overdue_count >= 1, got %d", summary.OverdueCount)
		}

		// Verify stage distribution contains 8 stages
		if len(summary.StageDistribution) != 8 {
			t.Errorf("Expected 8 canonical stages in stage_distribution, got %d", len(summary.StageDistribution))
		}

		// Verify region distribution contains 5 canonical regions
		if len(summary.RegionDistribution) != 5 {
			t.Errorf("Expected 5 canonical regions in region_distribution, got %d", len(summary.RegionDistribution))
		}
	})

	// ----------------------------------------------------
	// TEST 2: GET /api/v1/dashboard/summary (Filtered by Owner)
	// ----------------------------------------------------
	t.Run("Dashboard Summary Filtered by Owner", func(t *testing.T) {
		summary, err := analyticsService.GetDashboardSummary(ctx, models.RoleAdmin, "admin@example.com", "D. Ghosh", "")
		if err != nil {
			t.Fatalf("GetDashboardSummary filtered returned error: %v", err)
		}

		// D. Ghosh only has L1 ($50k) and L2 ($100k) open deals
		if summary.PipelineValue < 150000.00 {
			t.Errorf("Expected pipeline_value >= 150000 for D. Ghosh, got %f", summary.PipelineValue)
		}
		if summary.OverdueCount < 1 {
			t.Errorf("Expected overdue_count >= 1 for D. Ghosh, got %d", summary.OverdueCount)
		}
	})

	// ----------------------------------------------------
	// TEST 3: GET /api/v1/reports/analytics
	// ----------------------------------------------------
	t.Run("Reports Analytics", func(t *testing.T) {
		analytics, err := analyticsService.GetReportsAnalytics(ctx, models.RoleAdmin, "admin@example.com", nil, nil, "", "")
		if err != nil {
			t.Fatalf("GetReportsAnalytics returned error: %v", err)
		}

		// Total won revenue
		if analytics.RevenueSummary.TotalRevenue < 80000.00 {
			t.Errorf("Expected total_revenue >= 80000, got %f", analytics.RevenueSummary.TotalRevenue)
		}

		// Win rate: won = 1, lost = 1 -> 50%
		if analytics.ConversionAnalytics.WinRatePercent <= 0 {
			t.Errorf("Expected win_rate_percent > 0, got %f", analytics.ConversionAnalytics.WinRatePercent)
		}

		// Avg sales cycle
		if analytics.ConversionAnalytics.AvgSalesCycleDays <= 0 {
			t.Errorf("Expected avg_sales_cycle_days > 0, got %f", analytics.ConversionAnalytics.AvgSalesCycleDays)
		}

		// Rep performance
		if len(analytics.RepPerformance) == 0 {
			t.Errorf("Expected rep_performance to have entries, got 0")
		}

		// Activity breakdown
		if len(analytics.ActivityBreakdown) == 0 {
			t.Errorf("Expected activity_breakdown to have categories, got 0")
		}

		// Priority breakdown
		if len(analytics.PriorityBreakdown) != 4 {
			t.Errorf("Expected 4 priority tiers, got %d", len(analytics.PriorityBreakdown))
		}
	})
}
