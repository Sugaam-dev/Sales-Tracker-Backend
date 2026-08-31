package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"

	"crm-auth-service/helpers"
	"crm-auth-service/models"
)

type CommercialRepository interface {
	GetOrCreateDraft(ctx context.Context, leadID string) (*models.CommercialEstimation, error)
	GetByLeadID(ctx context.Context, leadID string) (*models.CommercialEstimation, error)
	UpdateAggregate(
		ctx context.Context,
		leadID string,
		estimation *models.CommercialEstimation,
		resources *[]models.CommercialResource,
		expenses *[]models.CommercialExpense,
		sdlc *[]models.SDLCAllocation,
	) (*models.CommercialEstimation, error)
	GetLeadContext(ctx context.Context, leadID string) (*models.LeadContextDTO, error)
}

type commercialRepository struct {
	pgx *pgxpool.Pool
	db  *gorm.DB
}

func NewCommercialRepository(pgx *pgxpool.Pool, db *gorm.DB) CommercialRepository {
	return &commercialRepository{
		pgx: pgx,
		db:  db,
	}
}

func (r *commercialRepository) GetLeadContext(ctx context.Context, leadID string) (*models.LeadContextDTO, error) {
	query := `SELECT lead_id, company, project_name, owner 
	          FROM leads 
	          WHERE lead_id = $1 AND deleted_at IS NULL`
	var dto models.LeadContextDTO
	err := r.pgx.QueryRow(ctx, query, leadID).Scan(&dto.LeadID, &dto.Company, &dto.ProjectName, &dto.Owner)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, helpers.ErrNotFound
		}
		return nil, fmt.Errorf("repo: get lead context: %w", err)
	}
	return &dto, nil
}

func (r *commercialRepository) GetOrCreateDraft(ctx context.Context, leadID string) (*models.CommercialEstimation, error) {
	// First, ensure the Lead exists
	var leadCount int64
	err := r.db.WithContext(ctx).Model(&models.Lead{}).Where("lead_id = ? AND deleted_at IS NULL", leadID).Count(&leadCount).Error
	if err != nil {
		return nil, err
	}
	if leadCount == 0 {
		return nil, helpers.ErrNotFound
	}

	// Try to find existing commercial estimation with preloaded children
	var est models.CommercialEstimation
	err = r.db.WithContext(ctx).
		Preload("Resources", func(db *gorm.DB) *gorm.DB { return db.Order("created_at asc") }).
		Preload("Expenses", func(db *gorm.DB) *gorm.DB { return db.Order("created_at asc") }).
		Preload("SDLCAllocations", func(db *gorm.DB) *gorm.DB { return db.Order("created_at asc") }).
		Where("lead_id = ?", leadID).
		First(&est).Error

	if err == nil {
		return &est, nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// Atomically create draft if not exists
	now := time.Now()
	startDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	endDate := startDate.AddDate(0, 1, 0)

	newDraft := models.CommercialEstimation{
		LeadID:                  leadID,
		Currency:                models.CurrencyUSD,
		BillingType:             "T&M",
		StartDate:               startDate,
		EstimatedDurationMonths: 1,
		EstimatedEndDate:        endDate,
		MarkupPercent:           0.00,
		DiscountPercent:         0.00,
		Status:                  models.CommercialStatusDraft,
		Resources:               []models.CommercialResource{},
		Expenses:                []models.CommercialExpense{},
		SDLCAllocations:         []models.SDLCAllocation{},
	}

	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Double check within transaction
		var existing models.CommercialEstimation
		if txErr := tx.Where("lead_id = ?", leadID).First(&existing).Error; txErr == nil {
			newDraft = existing
			return nil
		}
		return tx.Create(&newDraft).Error
	})

	if err != nil {
		return nil, err
	}

	// Reload with relationships
	return r.GetByLeadID(ctx, leadID)
}

func (r *commercialRepository) GetByLeadID(ctx context.Context, leadID string) (*models.CommercialEstimation, error) {
	var est models.CommercialEstimation
	err := r.db.WithContext(ctx).
		Preload("Resources", func(db *gorm.DB) *gorm.DB { return db.Order("created_at asc") }).
		Preload("Expenses", func(db *gorm.DB) *gorm.DB { return db.Order("created_at asc") }).
		Preload("SDLCAllocations", func(db *gorm.DB) *gorm.DB { return db.Order("created_at asc") }).
		Where("lead_id = ?", leadID).
		First(&est).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, helpers.ErrNotFound
		}
		return nil, err
	}

	return &est, nil
}

func (r *commercialRepository) UpdateAggregate(
	ctx context.Context,
	leadID string,
	estimation *models.CommercialEstimation,
	resources *[]models.CommercialResource,
	expenses *[]models.CommercialExpense,
	sdlc *[]models.SDLCAllocation,
) (*models.CommercialEstimation, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Verify existence
		var existing models.CommercialEstimation
		if err := tx.Where("lead_id = ?", leadID).First(&existing).Error; err != nil {
			return err
		}

		// 2. Update estimation header fields
		updates := map[string]interface{}{
			"currency":                  estimation.Currency,
			"billing_type":              estimation.BillingType,
			"start_date":                estimation.StartDate,
			"estimated_duration_months": estimation.EstimatedDurationMonths,
			"estimated_end_date":        estimation.EstimatedEndDate,
			"markup_percent":            estimation.MarkupPercent,
			"discount_percent":          estimation.DiscountPercent,
			"manual_selling_price":      estimation.ManualSellingPrice,
			"status":                    estimation.Status,
			"updated_at":                time.Now(),
		}

		if err := tx.Model(&existing).Updates(updates).Error; err != nil {
			return err
		}

		// 3. Replace Child Resources if explicitly provided
		if resources != nil {
			if err := tx.Where("commercial_estimation_id = ?", existing.ID).Delete(&models.CommercialResource{}).Error; err != nil {
				return err
			}
			if len(*resources) > 0 {
				resList := *resources
				for i := range resList {
					resList[i].CommercialEstimationID = existing.ID
				}
				if err := tx.Create(&resList).Error; err != nil {
					return err
				}
			}
		}

		// 4. Replace Child Expenses if explicitly provided
		if expenses != nil {
			if err := tx.Where("commercial_estimation_id = ?", existing.ID).Delete(&models.CommercialExpense{}).Error; err != nil {
				return err
			}
			if len(*expenses) > 0 {
				expList := *expenses
				for i := range expList {
					expList[i].CommercialEstimationID = existing.ID
				}
				if err := tx.Create(&expList).Error; err != nil {
					return err
				}
			}
		}

		// 5. Replace Child SDLC Allocations if explicitly provided
		if sdlc != nil {
			if err := tx.Where("commercial_estimation_id = ?", existing.ID).Delete(&models.SDLCAllocation{}).Error; err != nil {
				return err
			}
			if len(*sdlc) > 0 {
				sdlcList := *sdlc
				for i := range sdlcList {
					sdlcList[i].CommercialEstimationID = existing.ID
				}
				if err := tx.Create(&sdlcList).Error; err != nil {
					return err
				}
			}
		}

		return nil
	})

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, helpers.ErrNotFound
		}
		return nil, err
	}

	return r.GetByLeadID(ctx, leadID)
}
