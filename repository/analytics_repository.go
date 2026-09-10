package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"crm-auth-service/helpers"
	"crm-auth-service/models"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
)

type AnalyticsRepository interface {
	GetDashboardSummary(ctx context.Context, scope helpers.DataScope, owner, region string) (*models.DashboardSummaryData, error)
	GetReportsAnalytics(ctx context.Context, scope helpers.DataScope, dateFrom, dateTo *time.Time, owner, region string) (*models.ReportsAnalyticsData, error)
}

type analyticsRepository struct {
	pool *pgxpool.Pool
	db   *gorm.DB
}

func NewAnalyticsRepository(pool *pgxpool.Pool, db *gorm.DB) AnalyticsRepository {
	return &analyticsRepository{
		pool: pool,
		db:   db,
	}
}

func (r *analyticsRepository) GetDashboardSummary(ctx context.Context, scope helpers.DataScope, owner, region string) (*models.DashboardSummaryData, error) {
	data := &models.DashboardSummaryData{
		StageDistribution:  make([]models.StageDistributionItem, 0),
		RegionDistribution: make([]models.RegionDistributionItem, 0),
	}

	// 1. Build common WHERE filter for leads
	var leadWhereClauses []string
	var leadArgs []any
	argIdx := 1

	leadWhereClauses = append(leadWhereClauses, "deleted_at IS NULL")

	if !scope.IsUnrestricted {
		leadWhereClauses = append(leadWhereClauses, fmt.Sprintf("(assigned_to = ANY($%d) OR created_by = ANY($%d))", argIdx, argIdx))
		leadArgs = append(leadArgs, scope.AllowedUserIDs)
		argIdx++
	}

	if strings.TrimSpace(owner) != "" {
		leadWhereClauses = append(leadWhereClauses, fmt.Sprintf("LOWER(owner) = LOWER($%d)", argIdx))
		leadArgs = append(leadArgs, strings.TrimSpace(owner))
		argIdx++
	}
	if strings.TrimSpace(region) != "" {
		leadWhereClauses = append(leadWhereClauses, fmt.Sprintf("LOWER(region) = LOWER($%d)", argIdx))
		leadArgs = append(leadArgs, strings.TrimSpace(region))
		argIdx++
	}

	whereSQL := " WHERE " + strings.Join(leadWhereClauses, " AND ")

	// 2. Aggregate core KPI metrics
	metricsQuery := fmt.Sprintf(`
		SELECT
			COALESCE(SUM(CASE WHEN (status IS NULL OR status NOT IN ('Won', 'Lost')) AND (stage IS NULL OR stage NOT IN ('Closed Won', 'Closed Lost')) THEN value ELSE 0 END), 0) AS pipeline_value,
			COUNT(CASE WHEN (status IS NULL OR status NOT IN ('Won', 'Lost')) AND (stage IS NULL OR stage NOT IN ('Closed Won', 'Closed Lost')) THEN 1 END) AS open_deals_count,
			COALESCE(SUM(
				CASE WHEN (status IS NULL OR status NOT IN ('Won', 'Lost')) AND (stage IS NULL OR stage NOT IN ('Closed Won', 'Closed Lost')) THEN
					value * (CASE 
						WHEN stage = 'Prospecting' THEN 0.10
						WHEN stage = 'Qualification' THEN 0.20
						WHEN stage = 'Initial Discussion' THEN 0.30
						WHEN stage = 'Needs Analysis' THEN 0.35
						WHEN stage = 'Proposal' THEN 0.50
						WHEN stage = 'Negotiation' THEN 0.80
						ELSE 0.10
					END)
				ELSE 0 END
			), 0) AS expected_value,
			COUNT(CASE WHEN status = 'Won' OR stage = 'Closed Won' THEN 1 END) AS won_leads_count,
			COUNT(CASE WHEN status = 'Lost' OR stage = 'Closed Lost' THEN 1 END) AS lost_leads_count
		FROM leads
		%s
	`, whereSQL)

	err := r.pool.QueryRow(ctx, metricsQuery, leadArgs...).Scan(
		&data.PipelineValue,
		&data.OpenDealsCount,
		&data.ExpectedValue,
		&data.WonLeadsCount,
		&data.LostLeadsCount,
	)
	if err != nil {
		return nil, fmt.Errorf("analytics repo: query dashboard metrics: %w", err)
	}

	// 3. Overdue Tasks Count: activities with completed = false AND due_date < CURRENT_TIMESTAMP
	var actWhereClauses []string
	var actArgs []any
	actArgIdx := 1

	actWhereClauses = append(actWhereClauses, "a.completed = false", "a.due_date < CURRENT_TIMESTAMP", "l.deleted_at IS NULL")

	if !scope.IsUnrestricted {
		actWhereClauses = append(actWhereClauses, fmt.Sprintf("(l.assigned_to = ANY($%d) OR l.created_by = ANY($%d))", actArgIdx, actArgIdx))
		actArgs = append(actArgs, scope.AllowedUserIDs)
		actArgIdx++
	}

	if strings.TrimSpace(owner) != "" {
		actWhereClauses = append(actWhereClauses, fmt.Sprintf("LOWER(l.owner) = LOWER($%d)", actArgIdx))
		actArgs = append(actArgs, strings.TrimSpace(owner))
		actArgIdx++
	}
	if strings.TrimSpace(region) != "" {
		actWhereClauses = append(actWhereClauses, fmt.Sprintf("LOWER(l.region) = LOWER($%d)", actArgIdx))
		actArgs = append(actArgs, strings.TrimSpace(region))
		actArgIdx++
	}

	overdueQuery := fmt.Sprintf(`
		SELECT COUNT(a.id)
		FROM activities a
		JOIN leads l ON a.lead_id = l.lead_id
		WHERE %s
	`, strings.Join(actWhereClauses, " AND "))

	err = r.pool.QueryRow(ctx, overdueQuery, actArgs...).Scan(&data.OverdueCount)
	if err != nil {
		return nil, fmt.Errorf("analytics repo: query overdue tasks: %w", err)
	}

	// 4. Stage Distribution
	stageQuery := fmt.Sprintf(`
		SELECT COALESCE(stage, 'Prospecting') AS stage, COUNT(*), COALESCE(SUM(value), 0)
		FROM leads
		%s
		GROUP BY stage
	`, whereSQL)

	stageRows, err := r.pool.Query(ctx, stageQuery, leadArgs...)
	if err != nil {
		return nil, fmt.Errorf("analytics repo: query stage distribution: %w", err)
	}
	defer stageRows.Close()

	stageMap := make(map[string]struct {
		count int64
		value float64
	})
	for stageRows.Next() {
		var stgName string
		var count int64
		var val float64
		if err := stageRows.Scan(&stgName, &count, &val); err == nil {
			stageMap[stgName] = struct {
				count int64
				value float64
			}{count: count, value: val}
		}
	}

	// Canonical 8 stages and fills
	canonicalStages := []struct {
		name string
		fill string
	}{
		{"Prospecting", "#93C5FD"},
		{"Qualification", "#A7F3D0"},
		{"Initial Discussion", "#99F6E4"},
		{"Needs Analysis", "#FDE68A"},
		{"Proposal", "#C7D2FE"},
		{"Negotiation", "#FBCFE8"},
		{"Closed Won", "#34D399"},
		{"Closed Lost", "#F87171"},
	}

	for _, cs := range canonicalStages {
		stgData := stageMap[cs.name]
		data.StageDistribution = append(data.StageDistribution, models.StageDistributionItem{
			Name:  cs.name,
			Deals: stgData.count,
			Count: stgData.count,
			Value: stgData.value,
			Fill:  cs.fill,
		})
	}

	// 5. Region Distribution
	regionQuery := fmt.Sprintf(`
		SELECT COALESCE(region, 'Other') AS region, COUNT(*), COALESCE(SUM(value), 0)
		FROM leads
		%s
		GROUP BY region
	`, whereSQL)

	regRows, err := r.pool.Query(ctx, regionQuery, leadArgs...)
	if err != nil {
		return nil, fmt.Errorf("analytics repo: query region distribution: %w", err)
	}
	defer regRows.Close()

	regionMap := make(map[string]struct {
		count int64
		value float64
	})
	var totalRegionLeads int64
	for regRows.Next() {
		var regName string
		var count int64
		var val float64
		if err := regRows.Scan(&regName, &count, &val); err == nil {
			regionMap[regName] = struct {
				count int64
				value float64
			}{count: count, value: val}
			totalRegionLeads += count
		}
	}

	canonicalRegions := []struct {
		name string
		fill string
	}{
		{"North America", "#1D4ED8"},
		{"Europe", "#0EA5E9"},
		{"Asia Pacific", "#14B8A6"},
		{"LATAM", "#F59E0B"},
		{"India", "#8B5CF6"},
	}

	for _, cr := range canonicalRegions {
		regData := regionMap[cr.name]
		percent := 0
		if totalRegionLeads > 0 {
			percent = int(float64(regData.count) / float64(totalRegionLeads) * 100)
		}
		data.RegionDistribution = append(data.RegionDistribution, models.RegionDistributionItem{
			Name:    cr.name,
			Count:   regData.count,
			Value:   regData.value,
			Percent: percent,
			Fill:    cr.fill,
		})
	}

	return data, nil
}

func (r *analyticsRepository) GetReportsAnalytics(ctx context.Context, scope helpers.DataScope, dateFrom, dateTo *time.Time, owner, region string) (*models.ReportsAnalyticsData, error) {
	data := &models.ReportsAnalyticsData{
		PipelineByStage:   make([]models.StageDistributionItem, 0),
		PipelineByRegion:  make([]models.RegionDistributionItem, 0),
		RepPerformance:    make([]models.RepPerformanceItem, 0),
		ActivityBreakdown: make([]models.ActivityBreakdownItem, 0),
		PriorityBreakdown: make([]models.PriorityBreakdownItem, 0),
	}

	// 1. Build Base Filters for Leads
	var leadWhere []string
	var leadArgs []any
	argIdx := 1

	leadWhere = append(leadWhere, "deleted_at IS NULL")

	if !scope.IsUnrestricted {
		leadWhere = append(leadWhere, fmt.Sprintf("(assigned_to = ANY($%d) OR created_by = ANY($%d))", argIdx, argIdx))
		leadArgs = append(leadArgs, scope.AllowedUserIDs)
		argIdx++
	}

	if strings.TrimSpace(owner) != "" {
		leadWhere = append(leadWhere, fmt.Sprintf("LOWER(owner) = LOWER($%d)", argIdx))
		leadArgs = append(leadArgs, strings.TrimSpace(owner))
		argIdx++
	}
	if strings.TrimSpace(region) != "" {
		leadWhere = append(leadWhere, fmt.Sprintf("LOWER(region) = LOWER($%d)", argIdx))
		leadArgs = append(leadArgs, strings.TrimSpace(region))
		argIdx++
	}

	// Range filters for closed deals / revenue: use updated_at as closure date
	var dateFilterSQL string
	if dateFrom != nil {
		dateFilterSQL += fmt.Sprintf(" AND updated_at >= $%d", argIdx)
		leadArgs = append(leadArgs, *dateFrom)
		argIdx++
	}
	if dateTo != nil {
		dateFilterSQL += fmt.Sprintf(" AND updated_at <= $%d", argIdx)
		leadArgs = append(leadArgs, *dateTo)
		argIdx++
	}

	whereBaseSQL := " WHERE " + strings.Join(leadWhere, " AND ")

	// 2. Revenue, Total Won, and Average Sales Cycle for Won Leads
	revQuery := fmt.Sprintf(`
		SELECT
			COALESCE(SUM(CASE WHEN (status = 'Won' OR stage = 'Closed Won') THEN value ELSE 0 END), 0) AS total_revenue,
			COUNT(CASE WHEN (status = 'Won' OR stage = 'Closed Won') THEN 1 END) AS won_count,
			COUNT(CASE WHEN (status = 'Lost' OR stage = 'Closed Lost') THEN 1 END) AS lost_count,
			COALESCE(AVG(CASE WHEN (status = 'Won' OR stage = 'Closed Won') THEN EXTRACT(EPOCH FROM (updated_at - created_at))/86400.0 ELSE NULL END), 0) AS avg_sales_cycle_days
		FROM leads
		%s %s
	`, whereBaseSQL, dateFilterSQL)

	err := r.pool.QueryRow(ctx, revQuery, leadArgs...).Scan(
		&data.RevenueSummary.CurrentPeriodRevenue,
		&data.ConversionAnalytics.WonCount,
		&data.ConversionAnalytics.LostCount,
		&data.ConversionAnalytics.AvgSalesCycleDays,
	)
	if err != nil {
		return nil, fmt.Errorf("analytics repo: query revenue & conversion: %w", err)
	}

	// 3. Lifetime Total Won Revenue
	totalRevQuery := fmt.Sprintf(`
		SELECT COALESCE(SUM(CASE WHEN (status = 'Won' OR stage = 'Closed Won') THEN value ELSE 0 END), 0)
		FROM leads
		%s
	`, whereBaseSQL)

	// Lead args without date params
	var baseArgsOnly []any
	baseArgCount := 0
	if !scope.IsUnrestricted {
		baseArgsOnly = append(baseArgsOnly, scope.AllowedUserIDs)
		baseArgCount++
	}
	if strings.TrimSpace(owner) != "" {
		baseArgsOnly = append(baseArgsOnly, strings.TrimSpace(owner))
		baseArgCount++
	}
	if strings.TrimSpace(region) != "" {
		baseArgsOnly = append(baseArgsOnly, strings.TrimSpace(region))
		baseArgCount++
	}

	_ = r.pool.QueryRow(ctx, totalRevQuery, baseArgsOnly...).Scan(&data.RevenueSummary.TotalRevenue)

	// 4. Previous Period Revenue for Growth Calculation
	now := time.Now()
	var prevFrom, prevTo time.Time
	if dateFrom != nil && dateTo != nil {
		duration := dateTo.Sub(*dateFrom)
		prevTo = *dateFrom
		prevFrom = dateFrom.Add(-duration)
	} else {
		// Default to previous month
		currentMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		prevTo = currentMonthStart
		prevFrom = currentMonthStart.AddDate(0, -1, 0)
	}

	prevRevQuery := fmt.Sprintf(`
		SELECT COALESCE(SUM(value), 0)
		FROM leads
		%s AND (status = 'Won' OR stage = 'Closed Won') AND updated_at >= $%d AND updated_at < $%d
	`, whereBaseSQL, baseArgCount+1, baseArgCount+2)

	prevArgs := append(baseArgsOnly, prevFrom, prevTo)
	_ = r.pool.QueryRow(ctx, prevRevQuery, prevArgs...).Scan(&data.RevenueSummary.PreviousPeriodRevenue)

	// Calculate Growth % safely
	if data.RevenueSummary.PreviousPeriodRevenue > 0 {
		data.RevenueSummary.GrowthPercent = ((data.RevenueSummary.CurrentPeriodRevenue - data.RevenueSummary.PreviousPeriodRevenue) / data.RevenueSummary.PreviousPeriodRevenue) * 100
	} else if data.RevenueSummary.CurrentPeriodRevenue > 0 {
		data.RevenueSummary.GrowthPercent = 100.0
	} else {
		data.RevenueSummary.GrowthPercent = 0.0
	}

	if data.RevenueSummary.GrowthPercent >= 0 {
		data.RevenueSummary.RevenueMomentumText = fmt.Sprintf("+%.1f%% MoM", data.RevenueSummary.GrowthPercent)
	} else {
		data.RevenueSummary.RevenueMomentumText = fmt.Sprintf("%.1f%% MoM", data.RevenueSummary.GrowthPercent)
	}

	// 5. Win Rate %
	totalClosed := data.ConversionAnalytics.WonCount + data.ConversionAnalytics.LostCount
	if totalClosed > 0 {
		data.ConversionAnalytics.WinRatePercent = (float64(data.ConversionAnalytics.WonCount) / float64(totalClosed)) * 100
	} else {
		data.ConversionAnalytics.WinRatePercent = 0.0
	}

	// 6. Pipeline by Stage (values and counts)
	stageQuery := fmt.Sprintf(`
		SELECT COALESCE(stage, 'Prospecting') AS stage, COUNT(*), COALESCE(SUM(value), 0)
		FROM leads
		%s
		GROUP BY stage
	`, whereBaseSQL)

	stageRows, err := r.pool.Query(ctx, stageQuery, baseArgsOnly...)
	if err == nil {
		defer stageRows.Close()
		stageMap := make(map[string]struct {
			count int64
			value float64
		})
		for stageRows.Next() {
			var stgName string
			var count int64
			var val float64
			if err := stageRows.Scan(&stgName, &count, &val); err == nil {
				stageMap[stgName] = struct {
					count int64
					value float64
				}{count: count, value: val}
			}
		}

		canonicalStages := []struct {
			name string
			fill string
		}{
			{"Prospecting", "#93C5FD"},
			{"Qualification", "#A7F3D0"},
			{"Initial Discussion", "#99F6E4"},
			{"Needs Analysis", "#FDE68A"},
			{"Proposal", "#C7D2FE"},
			{"Negotiation", "#FBCFE8"},
			{"Closed Won", "#34D399"},
			{"Closed Lost", "#F87171"},
		}

		for _, cs := range canonicalStages {
			stgData := stageMap[cs.name]
			data.PipelineByStage = append(data.PipelineByStage, models.StageDistributionItem{
				Name:  cs.name,
				Deals: stgData.count,
				Count: stgData.count,
				Value: stgData.value,
				Fill:  cs.fill,
			})
		}
	}

	// 7. Pipeline by Region
	regQuery := fmt.Sprintf(`
		SELECT COALESCE(region, 'Other') AS region, COUNT(*), COALESCE(SUM(value), 0)
		FROM leads
		%s
		GROUP BY region
	`, whereBaseSQL)

	regRows, err := r.pool.Query(ctx, regQuery, baseArgsOnly...)
	if err == nil {
		defer regRows.Close()
		regMap := make(map[string]struct {
			count int64
			value float64
		})
		var totalRegValue float64
		for regRows.Next() {
			var regName string
			var count int64
			var val float64
			if err := regRows.Scan(&regName, &count, &val); err == nil {
				regMap[regName] = struct {
					count int64
					value float64
				}{count: count, value: val}
				totalRegValue += val
			}
		}

		canonicalRegions := []struct {
			name string
			fill string
		}{
			{"North America", "#1D4ED8"},
			{"Europe", "#0EA5E9"},
			{"Asia Pacific", "#14B8A6"},
			{"LATAM", "#F59E0B"},
			{"India", "#8B5CF6"},
		}

		for _, cr := range canonicalRegions {
			rData := regMap[cr.name]
			percent := 0
			if totalRegValue > 0 {
				percent = int((rData.value / totalRegValue) * 100)
			}
			displayName := cr.name
			if displayName == "Europe" {
				displayName = "EMEA"
			} else if displayName == "Asia Pacific" {
				displayName = "APAC"
			}
			data.PipelineByRegion = append(data.PipelineByRegion, models.RegionDistributionItem{
				Name:    displayName,
				Count:   rData.count,
				Value:   rData.value,
				Percent: percent,
				Fill:    cr.fill,
			})
		}
	}

	// 8. Sales Rep Performance
	repQuery := fmt.Sprintf(`
		SELECT
			COALESCE(owner, 'Unassigned') AS owner,
			COALESCE(SUM(CASE WHEN status = 'Won' OR stage = 'Closed Won' THEN value ELSE 0 END), 0) AS won,
			COALESCE(SUM(CASE WHEN (status IS NULL OR status NOT IN ('Won', 'Lost')) AND (stage IS NULL OR stage NOT IN ('Closed Won', 'Closed Lost')) THEN value ELSE 0 END), 0) AS pipeline,
			COALESCE(SUM(CASE WHEN status = 'Lost' OR stage = 'Closed Lost' THEN value ELSE 0 END), 0) AS lost,
			COUNT(*) AS total_deals
		FROM leads
		%s AND owner IS NOT NULL AND owner != ''
		GROUP BY owner
		ORDER BY won DESC, pipeline DESC
	`, whereBaseSQL)

	repRows, err := r.pool.Query(ctx, repQuery, baseArgsOnly...)
	if err == nil {
		defer repRows.Close()
		for repRows.Next() {
			var item models.RepPerformanceItem
			if err := repRows.Scan(&item.Name, &item.Won, &item.Pipeline, &item.Lost, &item.TotalDeals); err == nil {
				data.RepPerformance = append(data.RepPerformance, item)
			}
		}
	}

	// 9. Activity Breakdown
	var actWhere []string
	var actArgs []any
	actArgNum := 1

	actWhere = append(actWhere, "l.deleted_at IS NULL")

	if !scope.IsUnrestricted {
		actWhere = append(actWhere, fmt.Sprintf("(l.assigned_to = ANY($%d) OR l.created_by = ANY($%d))", actArgNum, actArgNum))
		actArgs = append(actArgs, scope.AllowedUserIDs)
		actArgNum++
	}

	if strings.TrimSpace(owner) != "" {
		actWhere = append(actWhere, fmt.Sprintf("LOWER(l.owner) = LOWER($%d)", actArgNum))
		actArgs = append(actArgs, strings.TrimSpace(owner))
		actArgNum++
	}
	if strings.TrimSpace(region) != "" {
		actWhere = append(actWhere, fmt.Sprintf("LOWER(l.region) = LOWER($%d)", actArgNum))
		actArgs = append(actArgs, strings.TrimSpace(region))
		actArgNum++
	}

	actQuery := fmt.Sprintf(`
		SELECT COALESCE(a.type, 'Other') AS type, COUNT(a.id)
		FROM activities a
		JOIN leads l ON a.lead_id = l.lead_id
		WHERE %s
		GROUP BY a.type
	`, strings.Join(actWhere, " AND "))

	actRows, err := r.pool.Query(ctx, actQuery, actArgs...)
	if err == nil {
		defer actRows.Close()
		var totalActivities int64
		typeMap := make(map[string]int64)

		for actRows.Next() {
			var aType string
			var count int64
			if err := actRows.Scan(&aType, &count); err == nil {
				typeMap[aType] = count
				totalActivities += count
			}
		}

		// Standard categories
		categories := []struct {
			name string
			keys []string
			fill string
		}{
			{"Calls", []string{"Call", "Calls", "Phone Call"}, "#10B981"},
			{"Emails", []string{"Email", "Emails", "Email Sent"}, "#3B82F6"},
			{"Meetings", []string{"Meeting", "Meetings"}, "#F59E0B"},
			{"Demos", []string{"Demo", "Demos", "Product Demo"}, "#8B5CF6"},
		}

		for _, cat := range categories {
			var catCount int64
			for _, k := range cat.keys {
				catCount += typeMap[k]
			}
			percent := 0
			if totalActivities > 0 {
				percent = int((float64(catCount) / float64(totalActivities)) * 100)
			}
			data.ActivityBreakdown = append(data.ActivityBreakdown, models.ActivityBreakdownItem{
				Name:    cat.name,
				Count:   catCount,
				Percent: percent,
				Fill:    cat.fill,
			})
		}
	}

	// 10. Priority Breakdown
	priQuery := fmt.Sprintf(`
		SELECT COALESCE(priority, 'Normal') AS priority, COUNT(*), COALESCE(SUM(value), 0)
		FROM leads
		%s
		GROUP BY priority
	`, whereBaseSQL)

	priRows, err := r.pool.Query(ctx, priQuery, baseArgsOnly...)
	if err == nil {
		defer priRows.Close()
		priMap := make(map[string]struct {
			count int64
			value float64
		})
		for priRows.Next() {
			var pName string
			var count int64
			var val float64
			if err := priRows.Scan(&pName, &count, &val); err == nil {
				priMap[pName] = struct {
					count int64
					value float64
				}{count: count, value: val}
			}
		}

		priorityTiers := []struct {
			tier         string
			priorityKey  string
			altKey       string
			color        string
			actionStatus string
		}{
			{"Critical", "Urgent", "Critical", "badge-danger", "Requires Daily Review"},
			{"High", "High", "High", "badge-warning", "Weekly Follow-up"},
			{"Medium", "Normal", "Medium", "badge-info", "Standard Cycle"},
			{"Low", "Low", "Low", "badge-success", "Standard Cycle"},
		}

		for _, pt := range priorityTiers {
			pData1 := priMap[pt.priorityKey]
			pData2 := priMap[pt.altKey]
			totalCount := pData1.count
			totalVal := pData1.value
			if pt.priorityKey != pt.altKey {
				totalCount += pData2.count
				totalVal += pData2.value
			}

			data.PriorityBreakdown = append(data.PriorityBreakdown, models.PriorityBreakdownItem{
				Tier:         pt.tier,
				Priority:     pt.priorityKey,
				Count:        totalCount,
				Value:        fmt.Sprintf("$%.0f", totalVal),
				NumericValue: totalVal,
				Color:        pt.color,
				ActionStatus: pt.actionStatus,
			})
		}
	}

	return data, nil
}
