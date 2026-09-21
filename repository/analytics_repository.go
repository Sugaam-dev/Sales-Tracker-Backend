package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"crm-auth-service/helpers"
	"crm-auth-service/models"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
)

type stageItemJSON struct {
	Stage string  `json:"stage"`
	Count int64   `json:"count"`
	Value float64 `json:"value"`
}

type regionItemJSON struct {
	Region string  `json:"region"`
	Count  int64   `json:"count"`
	Value  float64 `json:"value"`
}

type repItemJSON struct {
	Name       string  `json:"name"`
	Won        float64 `json:"won"`
	Pipeline   float64 `json:"pipeline"`
	Lost       float64 `json:"lost"`
	TotalDeals int64   `json:"total_deals"`
}

type actItemJSON struct {
	Type  string `json:"type"`
	Count int64  `json:"count"`
}

type priItemJSON struct {
	Priority string  `json:"priority"`
	Count    int64   `json:"count"`
	Value    float64 `json:"value"`
}

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

	whereSQL := strings.Join(leadWhereClauses, " AND ")

	// Consolidated Single-Round-Trip Dashboard Query
	dashboardQuery := fmt.Sprintf(`
		WITH filtered_leads AS (
			SELECT id, lead_id, company, project_name, owner, stage, status, value, region, assigned_to, created_by, deleted_at
			FROM leads
			WHERE %s
		),
		metrics AS (
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
			FROM filtered_leads
		),
		stage_dist AS (
			SELECT COALESCE(stage, 'Prospecting') AS stage, COUNT(*) AS count, COALESCE(SUM(value), 0) AS value
			FROM filtered_leads
			GROUP BY stage
		),
		region_dist AS (
			SELECT COALESCE(region, 'Other') AS region, COUNT(*) AS count, COALESCE(SUM(value), 0) AS value
			FROM filtered_leads
			GROUP BY region
		),
		overdue AS (
			SELECT COUNT(a.id) AS overdue_count
			FROM activities a
			JOIN filtered_leads fl ON a.lead_id = fl.lead_id
			WHERE a.completed = false AND a.due_date < CURRENT_DATE
		)
		SELECT
			m.pipeline_value,
			m.open_deals_count,
			m.expected_value,
			m.won_leads_count,
			m.lost_leads_count,
			o.overdue_count,
			COALESCE((SELECT json_agg(json_build_object('stage', sd.stage, 'count', sd.count, 'value', sd.value)) FROM stage_dist sd), '[]'::json) AS stage_json,
			COALESCE((SELECT json_agg(json_build_object('region', rd.region, 'count', rd.count, 'value', rd.value)) FROM region_dist rd), '[]'::json) AS region_json
		FROM metrics m
		CROSS JOIN overdue o;
	`, whereSQL)

	var stgJSONBytes, regJSONBytes []byte
	err := r.pool.QueryRow(ctx, dashboardQuery, leadArgs...).Scan(
		&data.PipelineValue,
		&data.OpenDealsCount,
		&data.ExpectedValue,
		&data.WonLeadsCount,
		&data.LostLeadsCount,
		&data.OverdueCount,
		&stgJSONBytes,
		&regJSONBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("analytics repo: query dashboard summary: %w", err)
	}

	var stgItems []stageItemJSON
	var regItems []regionItemJSON
	_ = json.Unmarshal(stgJSONBytes, &stgItems)
	_ = json.Unmarshal(regJSONBytes, &regItems)

	stageMap := make(map[string]struct {
		count int64
		value float64
	})
	for _, item := range stgItems {
		stageMap[item.Stage] = struct {
			count int64
			value float64
		}{count: item.Count, value: item.Value}
	}

	regionMap := make(map[string]struct {
		count int64
		value float64
	})
	var totalRegionLeads int64
	for _, item := range regItems {
		regionMap[item.Region] = struct {
			count int64
			value float64
		}{count: item.Count, value: item.Value}
		totalRegionLeads += item.Count
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

	// Canonical 5 regions and fills
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

	// Current period date filter for revenue & conversion
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

	// Previous period date filter for growth calculation
	now := time.Now()
	var prevFrom, prevTo time.Time
	if dateFrom != nil && dateTo != nil {
		duration := dateTo.Sub(*dateFrom)
		prevTo = *dateFrom
		prevFrom = dateFrom.Add(-duration)
	} else {
		currentMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		prevTo = currentMonthStart
		prevFrom = currentMonthStart.AddDate(0, -1, 0)
	}
	prevDateFilterSQL := fmt.Sprintf("updated_at >= $%d AND updated_at < $%d", argIdx, argIdx+1)
	leadArgs = append(leadArgs, prevFrom, prevTo)
	argIdx += 2

	whereBaseSQL := strings.Join(leadWhere, " AND ")

	// Consolidated Single-Round-Trip Reports Query
	reportsQuery := fmt.Sprintf(`
		WITH filtered_leads AS (
			SELECT id, lead_id, company, project_name, owner, stage, status, value, region, priority, created_at, updated_at, assigned_to, created_by, deleted_at
			FROM leads
			WHERE %s
		),
		lifetime_rev AS (
			SELECT COALESCE(SUM(CASE WHEN (status = 'Won' OR stage = 'Closed Won') THEN value ELSE 0 END), 0) AS total_revenue
			FROM filtered_leads
		),
		current_period_rev AS (
			SELECT
				COALESCE(SUM(CASE WHEN (status = 'Won' OR stage = 'Closed Won') THEN value ELSE 0 END), 0) AS current_period_revenue,
				COUNT(CASE WHEN (status = 'Won' OR stage = 'Closed Won') THEN 1 END) AS won_count,
				COUNT(CASE WHEN (status = 'Lost' OR stage = 'Closed Lost') THEN 1 END) AS lost_count,
				COALESCE(AVG(CASE WHEN (status = 'Won' OR stage = 'Closed Won') THEN EXTRACT(EPOCH FROM (updated_at - created_at))/86400.0 ELSE NULL END), 0) AS avg_sales_cycle_days
			FROM filtered_leads
			WHERE 1=1 %s
		),
		prev_period_rev AS (
			SELECT COALESCE(SUM(value), 0) AS prev_revenue
			FROM filtered_leads
			WHERE (status = 'Won' OR stage = 'Closed Won') AND %s
		),
		stage_dist AS (
			SELECT COALESCE(stage, 'Prospecting') AS stage, COUNT(*) AS count, COALESCE(SUM(value), 0) AS value
			FROM filtered_leads
			GROUP BY stage
		),
		region_dist AS (
			SELECT COALESCE(region, 'Other') AS region, COUNT(*) AS count, COALESCE(SUM(value), 0) AS value
			FROM filtered_leads
			GROUP BY region
		),
		rep_perf AS (
			SELECT
				COALESCE(owner, 'Unassigned') AS owner,
				COALESCE(SUM(CASE WHEN status = 'Won' OR stage = 'Closed Won' THEN value ELSE 0 END), 0) AS won,
				COALESCE(SUM(CASE WHEN (status IS NULL OR status NOT IN ('Won', 'Lost')) AND (stage IS NULL OR stage NOT IN ('Closed Won', 'Closed Lost')) THEN value ELSE 0 END), 0) AS pipeline,
				COALESCE(SUM(CASE WHEN status = 'Lost' OR stage = 'Closed Lost' THEN value ELSE 0 END), 0) AS lost,
				COUNT(*) AS total_deals
			FROM filtered_leads
			WHERE owner IS NOT NULL AND owner != ''
			GROUP BY owner
			ORDER BY won DESC, pipeline DESC
		),
		act_dist AS (
			SELECT COALESCE(a.type, 'Other') AS type, COUNT(a.id) AS count
			FROM activities a
			JOIN filtered_leads fl ON a.lead_id = fl.lead_id
			GROUP BY a.type
		),
		pri_dist AS (
			SELECT COALESCE(priority, 'Normal') AS priority, COUNT(*) AS count, COALESCE(SUM(value), 0) AS value
			FROM filtered_leads
			GROUP BY priority
		)
		SELECT
			lr.total_revenue,
			cpr.current_period_revenue,
			cpr.won_count,
			cpr.lost_count,
			cpr.avg_sales_cycle_days,
			ppr.prev_revenue,
			COALESCE((SELECT json_agg(json_build_object('stage', sd.stage, 'count', sd.count, 'value', sd.value)) FROM stage_dist sd), '[]'::json) AS stage_json,
			COALESCE((SELECT json_agg(json_build_object('region', rd.region, 'count', rd.count, 'value', rd.value)) FROM region_dist rd), '[]'::json) AS region_json,
			COALESCE((SELECT json_agg(json_build_object('name', rp.owner, 'won', rp.won, 'pipeline', rp.pipeline, 'lost', rp.lost, 'total_deals', rp.total_deals) ORDER BY rp.won DESC, rp.pipeline DESC) FROM rep_perf rp), '[]'::json) AS rep_json,
			COALESCE((SELECT json_agg(json_build_object('type', ad.type, 'count', ad.count)) FROM act_dist ad), '[]'::json) AS act_json,
			COALESCE((SELECT json_agg(json_build_object('priority', pd.priority, 'count', pd.count, 'value', pd.value)) FROM pri_dist pd), '[]'::json) AS pri_json
		FROM lifetime_rev lr
		CROSS JOIN current_period_rev cpr
		CROSS JOIN prev_period_rev ppr;
	`, whereBaseSQL, dateFilterSQL, prevDateFilterSQL)

	var rStgJSON, rRegJSON, rRepJSON, rActJSON, rPriJSON []byte
	err := r.pool.QueryRow(ctx, reportsQuery, leadArgs...).Scan(
		&data.RevenueSummary.TotalRevenue,
		&data.RevenueSummary.CurrentPeriodRevenue,
		&data.ConversionAnalytics.WonCount,
		&data.ConversionAnalytics.LostCount,
		&data.ConversionAnalytics.AvgSalesCycleDays,
		&data.RevenueSummary.PreviousPeriodRevenue,
		&rStgJSON,
		&rRegJSON,
		&rRepJSON,
		&rActJSON,
		&rPriJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("analytics repo: query reports analytics: %w", err)
	}

	var rStgItems []stageItemJSON
	var rRegItems []regionItemJSON
	var rRepItems []repItemJSON
	var rActItems []actItemJSON
	var rPriItems []priItemJSON

	_ = json.Unmarshal(rStgJSON, &rStgItems)
	_ = json.Unmarshal(rRegJSON, &rRegItems)
	_ = json.Unmarshal(rRepJSON, &rRepItems)
	_ = json.Unmarshal(rActJSON, &rActItems)
	_ = json.Unmarshal(rPriJSON, &rPriItems)

	stageMap := make(map[string]struct {
		count int64
		value float64
	})
	for _, item := range rStgItems {
		stageMap[item.Stage] = struct {
			count int64
			value float64
		}{count: item.Count, value: item.Value}
	}

	regMap := make(map[string]struct {
		count int64
		value float64
	})
	var totalRegValue float64
	for _, item := range rRegItems {
		regMap[item.Region] = struct {
			count int64
			value float64
		}{count: item.Count, value: item.Value}
		totalRegValue += item.Value
	}

	typeMap := make(map[string]int64)
	var totalActivities int64
	for _, item := range rActItems {
		typeMap[item.Type] = item.Count
		totalActivities += item.Count
	}

	priMap := make(map[string]struct {
		count int64
		value float64
	})
	for _, item := range rPriItems {
		priMap[item.Priority] = struct {
			count int64
			value float64
		}{count: item.Count, value: item.Value}
	}

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

	// Win Rate %
	totalClosed := data.ConversionAnalytics.WonCount + data.ConversionAnalytics.LostCount
	if totalClosed > 0 {
		data.ConversionAnalytics.WinRatePercent = (float64(data.ConversionAnalytics.WonCount) / float64(totalClosed)) * 100
	} else {
		data.ConversionAnalytics.WinRatePercent = 0.0
	}

	// Canonical 8 stages
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

	// Canonical 5 regions
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

	// Rep Performance
	data.RepPerformance = make([]models.RepPerformanceItem, len(rRepItems))
	for i, item := range rRepItems {
		data.RepPerformance[i] = models.RepPerformanceItem{
			Name:       item.Name,
			Won:        item.Won,
			Lost:       item.Lost,
			Pipeline:   item.Pipeline,
			TotalDeals: item.TotalDeals,
		}
	}

	// Activity Breakdown
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

	// Priority Breakdown
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

	return data, nil
}
