package models

import (
	"time"

	"github.com/google/uuid"
)

type LeadStage struct {
	ID        int    `json:"id" gorm:"primaryKey"`
	Name      string `json:"name" gorm:"type:varchar;not null"`
	Status    string `json:"status" gorm:"type:varchar;not null"`
	SortOrder int    `json:"sortOrder" gorm:"type:int;not null"`
	IsActive  bool   `json:"isActive" gorm:"not null;default:true"`
}

func (LeadStage) TableName() string { return "lead_stages" }

type Lead struct {
	ID                 uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	LeadID             string     `json:"leadId" gorm:"type:varchar;uniqueIndex;not null"`
	Company            string     `json:"company" gorm:"type:varchar;not null"`
	ProjectName        *string    `json:"projectName" gorm:"type:varchar"`
	Designation        *string    `json:"designation" gorm:"type:varchar"`
	Contact            *string    `json:"contact" gorm:"type:varchar"`
	Email              *string    `json:"email" gorm:"type:varchar;uniqueIndex"`
	Phone              *string    `json:"phone" gorm:"type:varchar"`
	PhoneCountry       *string    `json:"countryCode,omitempty" gorm:"column:phone_country;type:varchar"`
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
	LostReason         *string    `json:"lostReason" gorm:"column:lost_reason;type:varchar"`
	BestTime           *string    `json:"bestTime" gorm:"column:best_time;type:varchar"`
	RequestDetails     *string    `json:"requestDetails,omitempty" gorm:"column:request_details;type:text"`
	RequestType        *string    `json:"requestType" gorm:"column:request_type;type:varchar"`
	LifecycleTemplate  *string    `json:"lifecycleTemplate" gorm:"column:lifecycle_template;type:varchar"`
	KamName            *string    `json:"kamName" gorm:"column:kam_name;type:varchar"`
	BestTimeToConnect  *string    `json:"bestTimeToConnect" gorm:"column:best_time_to_connect;type:varchar"`
	AlternatePhone     *string    `json:"alternatePhone" gorm:"column:alternate_phone;type:varchar"`
	AlternatePhoneCountry *string `json:"alternatePhoneCountry" gorm:"column:alternate_phone_country;type:varchar"`
	LinkedinProfileURL *string    `json:"linkedinProfileUrl" gorm:"column:linkedin_profile_url;type:varchar"`
	LinkedinCompanyPageURL *string `json:"linkedinCompanyPageUrl" gorm:"column:linkedin_company_page_url;type:varchar"`
	EstimatedRequirementDate *time.Time `json:"estimatedRequirementDate" gorm:"column:estimated_requirement_date;type:date"`
	LastContactDate    *time.Time `json:"lastContactDate" gorm:"column:last_contact_date;type:timestamp with time zone"`
	NextFollowUp       *time.Time `json:"nextFollowUp" gorm:"column:next_follow_up;type:timestamp with time zone"`
	BasicRequirements  *string    `json:"basicRequirements" gorm:"column:basic_requirements;type:text"`
	Notes              *string    `json:"notes" gorm:"column:notes;type:text"`
	Activities         []Activity `json:"activities,omitempty" gorm:"foreignKey:LeadID;references:LeadID"`
	CreatedAt          time.Time  `json:"createdAt" gorm:"not null;autoCreateTime"`
	UpdatedAt          time.Time  `json:"updatedAt" gorm:"not null;autoUpdateTime"`
	DeletedAt          *time.Time `json:"deletedAt,omitempty" gorm:"index"`
}

func (Lead) TableName() string { return "leads" }

type Activity struct {
	ID        uint       `gorm:"primaryKey"`
	LeadID    string     `gorm:"index;not null;type:varchar"`
	Type      string     `gorm:"not null"`
	Desc      string     `gorm:"not null"`
	Outcome   string
	DueDate   *time.Time
	Completed bool       `gorm:"default:false"`
	CreatedAt time.Time
}

func (Activity) TableName() string { return "activities" }

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
	Designation        *string    `json:"designation,omitempty"`
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
	LostReason         *string    `json:"lostReason,omitempty"`
	BestTime           *string    `json:"bestTime,omitempty"`
	RequestDetails     *string    `json:"requestDetails,omitempty"`
	RequestType        *string    `json:"requestType,omitempty"`
	CountryCode        *string    `json:"countryCode,omitempty"`
	LifecycleTemplate  *string    `json:"lifecycleTemplate,omitempty"`
	KamName            *string    `json:"kamName,omitempty"`
	BestTimeToConnect  *string    `json:"bestTimeToConnect,omitempty"`
	AlternatePhone     *string    `json:"alternatePhone,omitempty"`
	AlternatePhoneCountry *string `json:"alternatePhoneCountry,omitempty"`
	LinkedinProfileURL *string    `json:"linkedinProfileUrl,omitempty"`
	LinkedinCompanyPageURL *string `json:"linkedinCompanyPageUrl,omitempty"`
	EstimatedRequirementDate *string `json:"estimatedRequirementDate,omitempty"`
	LastContactDate    *string    `json:"lastContactDate,omitempty"`
	NextFollowUp       *string    `json:"nextFollowUp,omitempty"`
	BasicRequirements  *string    `json:"basicRequirements,omitempty"`
	Notes              *string    `json:"notes,omitempty"`
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

type UpdateLeadRequest struct {
	RequestDetails     *string `json:"requestDetails,omitempty"`
	RequestType        *string `json:"requestType,omitempty"`
	CountryCode        *string `json:"countryCode,omitempty"`
	Owner                    *string `json:"owner,omitempty"`
	Stage                    *string `json:"stage,omitempty"`
	Status                   *string `json:"status,omitempty"`
	Priority                 *string `json:"priority,omitempty"`
	Contact                  *string `json:"contact,omitempty"`
	Email                    *string `json:"email,omitempty"`
	Phone                    *string `json:"phone,omitempty"`
	Value                    *string `json:"value,omitempty"`
	LostReason               *string `json:"lostReason,omitempty"`
	BestTime                 *string `json:"bestTime,omitempty"`
	Company                  *string `json:"company,omitempty"`
	ProjectName              *string `json:"projectName,omitempty"`
	Designation              *string `json:"designation,omitempty"`
	Industry                 *string `json:"industry,omitempty"`
	Size                     *string `json:"size,omitempty"`
	Region                   *string `json:"region,omitempty"`
	Source                   *string `json:"source,omitempty"`
	Sentiment                *string `json:"sentiment,omitempty"`
	OfficePhone              *string `json:"officePhone,omitempty"`
	OfficePhoneCountry       *string `json:"officePhoneCountry,omitempty"`
	LifecycleTemplate        *string `json:"lifecycleTemplate,omitempty"`
	KamName                  *string `json:"kamName,omitempty"`
	BestTimeToConnect        *string `json:"bestTimeToConnect,omitempty"`
	AlternatePhone           *string `json:"alternatePhone,omitempty"`
	AlternatePhoneCountry    *string `json:"alternatePhoneCountry,omitempty"`
	LinkedinProfileURL       *string `json:"linkedinProfileUrl,omitempty"`
	LinkedinCompanyPageURL   *string `json:"linkedinCompanyPageUrl,omitempty"`
	EstimatedRequirementDate *string `json:"estimatedRequirementDate,omitempty"`
	LastContactDate          *string `json:"lastContactDate,omitempty"`
	NextFollowUp             *string `json:"nextFollowUp,omitempty"`
	BasicRequirements        *string `json:"basicRequirements,omitempty"`
	Notes                    *string `json:"notes,omitempty"`
}
