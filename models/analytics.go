package models

// StageDistributionItem represents a pipeline stage summary.
type StageDistributionItem struct {
	Name  string  `json:"name"`
	Deals int64   `json:"deals"`
	Count int64   `json:"count"`
	Value float64 `json:"value"`
	Fill  string  `json:"fill"`
}

// RegionDistributionItem represents a region pipeline summary.
type RegionDistributionItem struct {
	Name    string  `json:"name"`
	Count   int64   `json:"count"`
	Value   float64 `json:"value"`
	Percent int     `json:"percent"`
	Fill    string  `json:"fill,omitempty"`
}

// DashboardSummaryData contains all KPIs and chart distributions for Dashboard.jsx.
type DashboardSummaryData struct {
	PipelineValue      float64                  `json:"pipeline_value"`
	OpenDealsCount     int64                    `json:"open_deals_count"`
	ExpectedValue      float64                  `json:"expected_value"`
	OverdueCount       int64                    `json:"overdue_count"`
	WonLeadsCount      int64                    `json:"won_leads_count"`
	LostLeadsCount     int64                    `json:"lost_leads_count"`
	StageDistribution  []StageDistributionItem  `json:"stage_distribution"`
	RegionDistribution []RegionDistributionItem `json:"region_distribution"`
}

// DashboardSummaryResponse wraps the summary data payload.
type DashboardSummaryResponse struct {
	Success bool                 `json:"success"`
	Data    DashboardSummaryData `json:"data"`
}

// RevenueSummary holds revenue totals, period comparisons, and growth momentum.
type RevenueSummary struct {
	TotalRevenue          float64 `json:"total_revenue"`
	CurrentPeriodRevenue  float64 `json:"current_period_revenue"`
	PreviousPeriodRevenue float64 `json:"previous_period_revenue"`
	GrowthPercent         float64 `json:"growth_percent"`
	RevenueMomentumText   string  `json:"revenue_momentum_text"`
}

// ConversionAnalytics holds win rate and sales cycle metrics.
type ConversionAnalytics struct {
	WinRatePercent    float64 `json:"win_rate_percent"`
	AvgSalesCycleDays float64 `json:"avg_sales_cycle_days"`
	WonCount          int64   `json:"won_count"`
	LostCount         int64   `json:"lost_count"`
}

// RepPerformanceItem captures Won, Pipeline, and Lost deal values per sales owner.
type RepPerformanceItem struct {
	Name       string  `json:"name"`
	Won        float64 `json:"won"`
	Lost       float64 `json:"lost"`
	Pipeline   float64 `json:"pipeline"`
	TotalDeals int64   `json:"total_deals"`
}

// ActivityBreakdownItem captures task volume distribution by activity type.
type ActivityBreakdownItem struct {
	Name    string `json:"name"`
	Count   int64  `json:"count"`
	Percent int    `json:"percent"`
	Fill    string `json:"fill"`
}

// PriorityBreakdownItem represents deal volume and pipeline value by priority level.
type PriorityBreakdownItem struct {
	Tier         string  `json:"tier"`
	Priority     string  `json:"priority"`
	Count        int64   `json:"count"`
	Value        string  `json:"value"`
	NumericValue float64 `json:"numeric_value"`
	Color        string  `json:"color"`
	ActionStatus string  `json:"action_status"`
}

// ReportsAnalyticsData represents the complete payload for Reports.jsx.
type ReportsAnalyticsData struct {
	RevenueSummary      RevenueSummary           `json:"revenue_summary"`
	ConversionAnalytics ConversionAnalytics      `json:"conversion_analytics"`
	PipelineByStage     []StageDistributionItem  `json:"pipeline_by_stage"`
	PipelineByRegion    []RegionDistributionItem `json:"pipeline_by_region"`
	RepPerformance      []RepPerformanceItem     `json:"rep_performance"`
	ActivityBreakdown   []ActivityBreakdownItem  `json:"activity_breakdown"`
	PriorityBreakdown   []PriorityBreakdownItem  `json:"priority_breakdown"`
}

// ReportsAnalyticsResponse wraps the reports analytics payload.
type ReportsAnalyticsResponse struct {
	Success bool                 `json:"success"`
	Data    ReportsAnalyticsData `json:"data"`
}
