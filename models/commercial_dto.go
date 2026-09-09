package models

import "github.com/google/uuid"

// LeadContextDTO holds the read-only Lead fields displayed on the Commercial page.
type LeadContextDTO struct {
	LeadID      string  `json:"leadId"`
	Company     string  `json:"company"`
	ProjectName *string `json:"projectName,omitempty"`
	Owner       *string `json:"owner,omitempty"`
}

// CommercialResourceDTO represents input/output of a single resource row.
type CommercialResourceDTO struct {
	ID           *uuid.UUID `json:"id,omitempty"`
	Role         string     `json:"role" binding:"required"`
	Grade        string     `json:"grade" binding:"required"`
	OnsiteDays   int        `json:"onsiteDays"`
	OffshoreDays int        `json:"offshoreDays"`
	DailyCost    float64    `json:"dailyCost"`
	BillingRate  float64    `json:"billingRate"`
	TotalDays    int        `json:"totalDays,omitempty"`
	TotalCost    float64    `json:"totalCost,omitempty"`
	TotalRevenue float64    `json:"totalRevenue,omitempty"`
}

// CommercialExpenseDTO represents input/output of an expense item.
type CommercialExpenseDTO struct {
	ID          *uuid.UUID `json:"id,omitempty"`
	ExpenseType string     `json:"expenseType" binding:"required"`
	Cost        float64    `json:"cost"`
	Remarks     *string    `json:"remarks,omitempty"`
}

// SDLCAllocationDTO represents input/output of an SDLC phase allocation.
type SDLCAllocationDTO struct {
	ID         *uuid.UUID `json:"id,omitempty"`
	Phase      string     `json:"phase" binding:"required"`
	ManDays    int        `json:"manDays"`
	Percentage float64    `json:"percentage,omitempty"`
}

// FinancialSummaryDTO holds computed financial overview metrics.
type FinancialSummaryDTO struct {
	TotalResourceCost      float64  `json:"totalResourceCost"`
	TotalExpenses          float64  `json:"totalExpenses"`
	TotalProjectCost       float64  `json:"totalProjectCost"`
	CalculatedSellingPrice float64  `json:"calculatedSellingPrice"`
	EffectiveSellingPrice  float64  `json:"effectiveSellingPrice"`
	GrossProfit            float64  `json:"grossProfit"`
	MarginPercent          float64  `json:"marginPercent"`
	ROIPercent             float64  `json:"roiPercent"`
	BreakEvenMonth         *int     `json:"breakEvenMonth,omitempty"`
	MaximumCashOut         float64  `json:"maximumCashOut"`
	NPV                    *float64 `json:"npv,omitempty"`
}

// CommercialEstimationDetailsDTO holds the full aggregate response for a Commercial Estimation.
type CommercialEstimationDetailsDTO struct {
	ID                      uuid.UUID               `json:"id"`
	LeadID                  string                  `json:"leadId"`
	Currency                string                  `json:"currency"`
	BillingType             string                  `json:"billingType"`
	StartDate               string                  `json:"startDate"`
	EstimatedDurationMonths int                     `json:"estimatedDurationMonths"`
	EstimatedEndDate        string                  `json:"estimatedEndDate"`
	MarkupPercent           float64                 `json:"markupPercent"`
	DiscountPercent         float64                 `json:"discountPercent"`
	ManualSellingPrice      *float64                `json:"manualSellingPrice,omitempty"`
	Status                  string                  `json:"status"`
	Resources               []CommercialResourceDTO `json:"resources"`
	Expenses                []CommercialExpenseDTO  `json:"expenses"`
	SDLCAllocations         []SDLCAllocationDTO     `json:"sdlcAllocations"`
	FinancialSummary        FinancialSummaryDTO     `json:"financialSummary"`
	CreatedAt               string                  `json:"createdAt"`
	UpdatedAt               string                  `json:"updatedAt"`
}

// GetCommercialResponse represents the complete GET /leads/:id/commercial response payload.
type GetCommercialResponse struct {
	LeadContext          LeadContextDTO                 `json:"leadContext"`
	CommercialEstimation CommercialEstimationDetailsDTO `json:"commercialEstimation"`
}

// UpdateCommercialRequest represents the body for PATCH /leads/:id/commercial.
type UpdateCommercialRequest struct {
	Currency                *string                  `json:"currency,omitempty"`
	BillingType             *string                  `json:"billingType,omitempty"`
	StartDate               *string                  `json:"startDate,omitempty"`
	EstimatedDurationMonths *int                     `json:"estimatedDurationMonths,omitempty"`
	EstimatedEndDate        *string                  `json:"estimatedEndDate,omitempty"`
	MarkupPercent           *float64                 `json:"markupPercent,omitempty"`
	DiscountPercent         *float64                 `json:"discountPercent,omitempty"`
	ManualSellingPrice      *float64                 `json:"manualSellingPrice,omitempty"`
	Status                  *string                  `json:"status,omitempty"`
	Resources               *[]CommercialResourceDTO `json:"resources,omitempty"`
	Expenses                *[]CommercialExpenseDTO  `json:"expenses,omitempty"`
	SDLCAllocations         *[]SDLCAllocationDTO     `json:"sdlcAllocations,omitempty"`
}

// ChartDatasetDTO represents a generic dataset for Chart.js / Recharts.
type ChartDatasetDTO struct {
	Label string    `json:"label,omitempty"`
	Data  []float64 `json:"data"`
}

// ChartDataDTO represents labels + datasets format for visualization.
type ChartDataDTO struct {
	Labels   []string          `json:"labels"`
	Datasets []ChartDatasetDTO `json:"datasets"`
}

// CommercialAnalyticsResponse represents the unified analytics endpoint payload.
type CommercialAnalyticsResponse struct {
	Currency               string              `json:"currency"`
	FinancialSummary       FinancialSummaryDTO `json:"financialSummary"`
	GradeWiseAllocation    ChartDataDTO        `json:"gradeWiseAllocation"`
	PhaseWiseAllocation    ChartDataDTO        `json:"phaseWiseAllocation"`
	CostBreakdown          ChartDataDTO        `json:"costBreakdown"`
	RevenueVsCost          ChartDataDTO        `json:"revenueVsCost"`
	SDLCEffortDistribution ChartDataDTO        `json:"sdlcEffortDistribution"`
	CumulativeCashFlow     ChartDataDTO        `json:"cumulativeCashFlow"`
}
