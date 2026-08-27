package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Lead struct {
	ID                 uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	LeadID             string         `gorm:"uniqueIndex;size:10;not null"`
	Company            string         `gorm:"uniqueIndex:idx_company_lower,expression:lower(company);not null"`
	ProjectName        string
	Contact            string         `gorm:"not null"`
	Email              string         `gorm:"uniqueIndex;not null"`
	Phone              string         `gorm:"size:10;not null"`
	OfficePhone        string         `gorm:"size:10;not null"`
	OfficePhoneCountry string         `gorm:"size:6"`
	Owner              string         `gorm:"not null"`
	Industry           string
	Size               string
	Region             string
	Source             string
	Stage              string         `gorm:"not null"`
	Status             string         `gorm:"not null"`
	Sentiment          string         `gorm:"not null"`
	Priority           string         `gorm:"not null"`
	Value              *string
	Activities         []Activity     `gorm:"foreignKey:LeadID"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          gorm.DeletedAt `gorm:"index"`
}

type Activity struct {
	ID        uint       `gorm:"primaryKey"`
	LeadID    uint       `gorm:"index;not null"`
	Type      string     `gorm:"not null"`
	Desc      string     `gorm:"not null"`
	Outcome   string
	DueDate   *time.Time
	Completed bool       `gorm:"default:false"`
	CreatedAt time.Time
}

type LeadStage struct {
	ID        uint   `gorm:"primaryKey"`
	Name      string `gorm:"uniqueIndex;not null"`
	SortOrder int
	IsActive  bool   `gorm:"default:true"`
}
