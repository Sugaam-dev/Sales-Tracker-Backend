package repository

import (
	"fmt"

	"gorm.io/gorm"

	"crm-auth-service/models"
)

type LeadRepository interface {
	CheckUserExists(name string) (bool, error)
	CheckStageExists(stage string) (bool, error)
	CreateLead(lead *models.Lead) error
	GetLeadByLeadID(leadID string) (*models.Lead, error)
	UpdateLead(leadID string, updates map[string]interface{}) error
	DeleteLead(leadID string) error
	GetLeadActivities(leadID string) (*models.Lead, error)
	GetUserNameByEmail(email string) (string, error)
}

type leadRepository struct {
	db *gorm.DB
}

func NewLeadRepository(db *gorm.DB) LeadRepository {
	return &leadRepository{db: db}
}

func (r *leadRepository) GetUserNameByEmail(email string) (string, error) {
	var user models.User
	err := r.db.Where("email = ?", email).First(&user).Error
	return user.Name, err
}

func (r *leadRepository) CheckUserExists(name string) (bool, error) {
	var count int64
	// Checking if user exists by Name. (Skipping is_active check as it's not in the base model, 
	// assuming existing user is valid to minimize safe changes).
	err := r.db.Model(&models.User{}).Where("name = ?", name).Count(&count).Error
	return count > 0, err
}

func (r *leadRepository) CheckStageExists(stage string) (bool, error) {
	var count int64
	err := r.db.Model(&models.LeadStage{}).Where("name = ? AND is_active = ?", stage, true).Count(&count).Error
	return count > 0, err
}

func (r *leadRepository) CreateLead(lead *models.Lead) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var n int64
		if err := tx.Raw("SELECT nextval('lead_id_seq')").Scan(&n).Error; err != nil {
			return err
		}
		lead.LeadID = fmt.Sprintf("L-%04d", n)
		
		return tx.Create(lead).Error
	})
}

func (r *leadRepository) GetLeadByLeadID(leadID string) (*models.Lead, error) {
	var lead models.Lead
	err := r.db.Where("lead_id = ?", leadID).First(&lead).Error
	if err != nil {
		return nil, err
	}
	return &lead, nil
}

func (r *leadRepository) UpdateLead(leadID string, updates map[string]interface{}) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var lead models.Lead
		if err := tx.Where("lead_id = ?", leadID).First(&lead).Error; err != nil {
			return err
		}
		return tx.Model(&lead).Updates(updates).Error
	})
}

func (r *leadRepository) DeleteLead(leadID string) error {
	// GORM will perform a soft delete because models.Lead has a gorm.DeletedAt field
	res := r.db.Where("lead_id = ?", leadID).Delete(&models.Lead{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *leadRepository) GetLeadActivities(leadID string) (*models.Lead, error) {
	var lead models.Lead
	// Ensure the lead exists first (and isn't soft-deleted, which is handled automatically by GORM)
	err := r.db.Preload("Activities", func(db *gorm.DB) *gorm.DB {
		return db.Order("created_at desc")
	}).Where("lead_id = ?", leadID).First(&lead).Error

	if err != nil {
		return nil, err
	}
	return &lead, nil
}
