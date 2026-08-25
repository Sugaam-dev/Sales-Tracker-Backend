package models

import (
	"time"

	"github.com/google/uuid"
)

type LeadStage struct {
	ID        int    `json:"id" gorm:"primaryKey"`
	Name      string `json:"name" gorm:"type:varchar;not null"`
	SortOrder int    `json:"sortOrder" gorm:"type:int;not null"`
	IsActive  bool   `json:"isActive" gorm:"not null;default:true"`
}

func (LeadStage) TableName() string { return "lead_stages" }

type Lead struct {
	ID                 uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	LeadID             string     `json:"leadId" gorm:"type:varchar;uniqueIndex;not null"`
	Company            string     `json:"company" gorm:"type:varchar;not null"`
	ProjectName        *string    `json:"projectName" gorm:"type:varchar"`
	Contact            *string    `json:"contact" gorm:"type:varchar"`
	Email              *string    `json:"email" gorm:"type:varchar;uniqueIndex"`
	Phone              *string    `json:"phone" gorm:"type:varchar"`
	OfficePhone        *string    `json:"officePhone" gorm:"type:varchar"`
	OfficePhoneCountry *string    `json:"officePhoneCountry" gorm:"type:varchar"`
	Owner              *string    `json:"owner" gorm:"type:varchar"`
	Industry           *string    `json:"industry" gorm:"type:varchar"`
	Size               *string    `json:"size" gorm:"type:varchar"`
	Region             *string    `json:"region" gorm:"type:varchar"`
	Source             *string    `json:"source" gorm:"type:varchar"`
	Stage              *string    `json:"stage" gorm:"type:varchar"`
	Status             *string    `json:"status" gorm:"type:varchar"`
	Sentiment          *string    `json:"sentiment" gorm:"type:varchar"`
	Priority           *string    `json:"priority" gorm:"type:varchar"`
	Value              *float64   `json:"value" gorm:"type:numeric(15,2)"`
	CreatedAt          time.Time  `json:"createdAt" gorm:"not null;autoCreateTime"`
	UpdatedAt          time.Time  `json:"updatedAt" gorm:"not null;autoUpdateTime"`
	DeletedAt          *time.Time `json:"deletedAt,omitempty" gorm:"index"`
}

func (Lead) TableName() string { return "leads" }

type ActiveUserResponse struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	IsActive bool      `json:"isActive"`
}

type LeadResponse struct {
	ID                 string     `json:"id"`
	Company            string     `json:"company"`
	ProjectName        *string    `json:"projectName,omitempty"`
	Contact            *string    `json:"contact,omitempty"`
	Email              *string    `json:"email,omitempty"`
	Phone              *string    `json:"phone,omitempty"`
	OfficePhone        *string    `json:"officePhone,omitempty"`
	OfficePhoneCountry *string    `json:"officePhoneCountry,omitempty"`
	Owner              *string    `json:"owner,omitempty"`
	Industry           *string    `json:"industry,omitempty"`
	Size               *string    `json:"size,omitempty"`
	Region             *string    `json:"region,omitempty"`
	Source             *string    `json:"source,omitempty"`
	Stage              *string    `json:"stage,omitempty"`
	Status             *string    `json:"status,omitempty"`
	Sentiment          *string    `json:"sentiment,omitempty"`
	Priority           *string    `json:"priority,omitempty"`
	Value              *string    `json:"value,omitempty"`
	CreatedAt          string     `json:"createdAt"`
	UpdatedAt          string     `json:"updatedAt"`
}

type PaginationMetadata struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"totalPages"`
}

type LeadListResponse struct {
	Success    bool           `json:"success"`
	Data       []LeadResponse `json:"data"`
	Pagination *PaginationMetadata `json:"pagination,omitempty"`
}
