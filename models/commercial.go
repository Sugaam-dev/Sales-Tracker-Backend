package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	CommercialStatusDraft     = "DRAFT"
	CommercialStatusSubmitted = "SUBMITTED"
	CommercialStatusApproved  = "APPROVED"
	CommercialStatusRejected  = "REJECTED"

	CurrencyUSD = "USD"
	CurrencyEUR = "EUR"
	CurrencyGBP = "GBP"
	CurrencyINR = "INR"
)

// CommercialEstimation represents the 1:1 commercial estimation attached to a Lead.
type CommercialEstimation struct {
	ID                      uuid.UUID            `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	LeadID                  string               `json:"leadId" gorm:"type:varchar;uniqueIndex;not null"`
	Currency                string               `json:"currency" gorm:"type:varchar(3);not null;default:'USD'"`
	BillingType             string               `json:"billingType" gorm:"type:varchar(50);not null;default:'T&M'"`
	StartDate               time.Time            `json:"startDate" gorm:"type:date;not null"`
	EstimatedDurationMonths int                  `json:"estimatedDurationMonths" gorm:"type:int;not null;default:1"`
	EstimatedEndDate        time.Time            `json:"estimatedEndDate" gorm:"type:date;not null"`
	MarkupPercent           float64              `json:"markupPercent" gorm:"type:numeric(5,2);not null;default:0.00"`
	DiscountPercent         float64              `json:"discountPercent" gorm:"type:numeric(5,2);not null;default:0.00"`
	ManualSellingPrice      *float64             `json:"manualSellingPrice" gorm:"type:numeric(15,2)"`
	Status                  string               `json:"status" gorm:"type:varchar(30);not null;default:'DRAFT'"`
	Resources               []CommercialResource `json:"resources,omitempty" gorm:"foreignKey:CommercialEstimationID;references:ID;constraint:OnDelete:CASCADE"`
	Expenses                []CommercialExpense  `json:"expenses,omitempty" gorm:"foreignKey:CommercialEstimationID;references:ID;constraint:OnDelete:CASCADE"`
	SDLCAllocations         []SDLCAllocation     `json:"sdlcAllocations,omitempty" gorm:"foreignKey:CommercialEstimationID;references:ID;constraint:OnDelete:CASCADE"`
	CreatedAt               time.Time            `json:"createdAt" gorm:"not null;autoCreateTime"`
	UpdatedAt               time.Time            `json:"updatedAt" gorm:"not null;autoUpdateTime"`
}

func (CommercialEstimation) TableName() string { return "commercial_estimations" }

// CommercialResource represents a billable role/grade resource entry.
type CommercialResource struct {
	ID                     uuid.UUID `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CommercialEstimationID uuid.UUID `json:"commercialEstimationId" gorm:"type:uuid;not null;index"`
	Role                   string    `json:"role" gorm:"type:varchar(100);not null"`
	Grade                  string    `json:"grade" gorm:"type:varchar(50);not null"`
	OnsiteDays             int       `json:"onsiteDays" gorm:"type:int;not null;default:0"`
	OffshoreDays           int       `json:"offshoreDays" gorm:"type:int;not null;default:0"`
	DailyCost              float64   `json:"dailyCost" gorm:"type:numeric(15,2);not null;default:0.00"`
	BillingRate            float64   `json:"billingRate" gorm:"type:numeric(15,2);not null;default:0.00"`
	TotalCost              float64   `json:"totalCost" gorm:"type:numeric(15,2);not null;default:0.00"`
	TotalRevenue           float64   `json:"totalRevenue" gorm:"type:numeric(15,2);not null;default:0.00"`
	CreatedAt              time.Time `json:"createdAt" gorm:"not null;autoCreateTime"`
	UpdatedAt              time.Time `json:"updatedAt" gorm:"not null;autoUpdateTime"`
}

func (CommercialResource) TableName() string { return "commercial_resources" }

// CommercialExpense represents an auxiliary cost item for the project.
type CommercialExpense struct {
	ID                     uuid.UUID `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CommercialEstimationID uuid.UUID `json:"commercialEstimationId" gorm:"type:uuid;not null;index"`
	ExpenseType            string    `json:"expenseType" gorm:"type:varchar(100);not null"`
	Cost                   float64   `json:"cost" gorm:"type:numeric(15,2);not null;default:0.00"`
	Remarks                *string   `json:"remarks" gorm:"type:text"`
	CreatedAt              time.Time `json:"createdAt" gorm:"not null;autoCreateTime"`
	UpdatedAt              time.Time `json:"updatedAt" gorm:"not null;autoUpdateTime"`
}

func (CommercialExpense) TableName() string { return "commercial_expenses" }

// SDLCAllocation represents man-days allocated to an SDLC phase.
type SDLCAllocation struct {
	ID                     uuid.UUID `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CommercialEstimationID uuid.UUID `json:"commercialEstimationId" gorm:"type:uuid;not null;index"`
	Phase                  string    `json:"phase" gorm:"type:varchar(100);not null"`
	ManDays                int       `json:"manDays" gorm:"type:int;not null;default:0"`
	CreatedAt              time.Time `json:"createdAt" gorm:"not null;autoCreateTime"`
	UpdatedAt              time.Time `json:"updatedAt" gorm:"not null;autoUpdateTime"`
}

func (SDLCAllocation) TableName() string { return "sdlc_allocations" }
