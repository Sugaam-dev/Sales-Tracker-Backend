package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"

	"crm-auth-service/helpers"
	"crm-auth-service/models"
)

type resourceJSON struct {
	ID                     uuid.UUID `json:"id"`
	CommercialEstimationID uuid.UUID `json:"commercial_estimation_id"`
	Role                   string    `json:"role"`
	Grade                  string    `json:"grade"`
	OnsiteDays             int       `json:"onsite_days"`
	OffshoreDays           int       `json:"offshore_days"`
	DailyCost              float64   `json:"daily_cost"`
	BillingRate            float64   `json:"billing_rate"`
	TotalCost              float64   `json:"total_cost"`
	TotalRevenue           float64   `json:"total_revenue"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type expenseJSON struct {
	ID                     uuid.UUID `json:"id"`
	CommercialEstimationID uuid.UUID `json:"commercial_estimation_id"`
	ExpenseType            string    `json:"expense_type"`
	Cost                   float64   `json:"cost"`
	Remarks                *string   `json:"remarks"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type sdlcJSON struct {
	ID                     uuid.UUID `json:"id"`
	CommercialEstimationID uuid.UUID `json:"commercial_estimation_id"`
	Phase                  string    `json:"phase"`
	ManDays                int       `json:"man_days"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

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

func (r *commercialRepository) findByLeadIDSingleQuery(ctx context.Context, leadID string) (*models.CommercialEstimation, error) {
	query := `
		SELECT 
			ce.id,
			ce.lead_id,
			ce.currency,
			ce.billing_type,
			ce.start_date,
			ce.estimated_duration_months,
			ce.estimated_end_date,
			ce.markup_percent,
			ce.discount_percent,
			ce.manual_selling_price,
			ce.status,
			ce.created_at,
			ce.updated_at,
			COALESCE((
				SELECT json_agg(json_build_object(
					'id', cr.id,
					'commercial_estimation_id', cr.commercial_estimation_id,
					'role', cr.role,
					'grade', cr.grade,
					'onsite_days', cr.onsite_days,
					'offshore_days', cr.offshore_days,
					'daily_cost', cr.daily_cost,
					'billing_rate', cr.billing_rate,
					'total_cost', cr.total_cost,
					'total_revenue', cr.total_revenue,
					'created_at', cr.created_at,
					'updated_at', cr.updated_at
				) ORDER BY cr.created_at ASC)
				FROM commercial_resources cr
				WHERE cr.commercial_estimation_id = ce.id
			), '[]'::json) AS resources_json,
			COALESCE((
				SELECT json_agg(json_build_object(
					'id', cx.id,
					'commercial_estimation_id', cx.commercial_estimation_id,
					'expense_type', cx.expense_type,
					'cost', cx.cost,
					'remarks', cx.remarks,
					'created_at', cx.created_at,
					'updated_at', cx.updated_at
				) ORDER BY cx.created_at ASC)
				FROM commercial_expenses cx
				WHERE cx.commercial_estimation_id = ce.id
			), '[]'::json) AS expenses_json,
			COALESCE((
				SELECT json_agg(json_build_object(
					'id', sa.id,
					'commercial_estimation_id', sa.commercial_estimation_id,
					'phase', sa.phase,
					'man_days', sa.man_days,
					'created_at', sa.created_at,
					'updated_at', sa.updated_at
				) ORDER BY sa.created_at ASC)
				FROM sdlc_allocations sa
				WHERE sa.commercial_estimation_id = ce.id
			), '[]'::json) AS sdlc_json
		FROM commercial_estimations ce
		WHERE ce.lead_id = $1
		LIMIT 1;
	`

	var est models.CommercialEstimation
	var resJSONBytes, expJSONBytes, sdlcJSONBytes []byte

	err := r.pgx.QueryRow(ctx, query, leadID).Scan(
		&est.ID,
		&est.LeadID,
		&est.Currency,
		&est.BillingType,
		&est.StartDate,
		&est.EstimatedDurationMonths,
		&est.EstimatedEndDate,
		&est.MarkupPercent,
		&est.DiscountPercent,
		&est.ManualSellingPrice,
		&est.Status,
		&est.CreatedAt,
		&est.UpdatedAt,
		&resJSONBytes,
		&expJSONBytes,
		&sdlcJSONBytes,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, helpers.ErrNotFound
		}
		return nil, fmt.Errorf("repo: get commercial estimation single query: %w", err)
	}

	var rawRes []resourceJSON
	var rawExp []expenseJSON
	var rawSDLC []sdlcJSON

	if err := json.Unmarshal(resJSONBytes, &rawRes); err != nil {
		return nil, fmt.Errorf("repo: unmarshal resources: %w", err)
	}
	if err := json.Unmarshal(expJSONBytes, &rawExp); err != nil {
		return nil, fmt.Errorf("repo: unmarshal expenses: %w", err)
	}
	if err := json.Unmarshal(sdlcJSONBytes, &rawSDLC); err != nil {
		return nil, fmt.Errorf("repo: unmarshal sdlc: %w", err)
	}

	est.Resources = make([]models.CommercialResource, len(rawRes))
	for i, r := range rawRes {
		est.Resources[i] = models.CommercialResource{
			ID:                     r.ID,
			CommercialEstimationID: r.CommercialEstimationID,
			Role:                   r.Role,
			Grade:                  r.Grade,
			OnsiteDays:             r.OnsiteDays,
			OffshoreDays:           r.OffshoreDays,
			DailyCost:              r.DailyCost,
			BillingRate:            r.BillingRate,
			TotalCost:              r.TotalCost,
			TotalRevenue:           r.TotalRevenue,
			CreatedAt:              r.CreatedAt,
			UpdatedAt:              r.UpdatedAt,
		}
	}

	est.Expenses = make([]models.CommercialExpense, len(rawExp))
	for i, e := range rawExp {
		est.Expenses[i] = models.CommercialExpense{
			ID:                     e.ID,
			CommercialEstimationID: e.CommercialEstimationID,
			ExpenseType:            e.ExpenseType,
			Cost:                   e.Cost,
			Remarks:                e.Remarks,
			CreatedAt:              e.CreatedAt,
			UpdatedAt:              e.UpdatedAt,
		}
	}

	est.SDLCAllocations = make([]models.SDLCAllocation, len(rawSDLC))
	for i, s := range rawSDLC {
		est.SDLCAllocations[i] = models.SDLCAllocation{
			ID:                     s.ID,
			CommercialEstimationID: s.CommercialEstimationID,
			Phase:                  s.Phase,
			ManDays:                s.ManDays,
			CreatedAt:              s.CreatedAt,
			UpdatedAt:              s.UpdatedAt,
		}
	}

	return &est, nil
}

func (r *commercialRepository) GetLeadContext(ctx context.Context, leadID string) (*models.LeadContextDTO, error) {
	query := `SELECT lead_id, company, project_name, owner, estimated_requirement_date 
	          FROM leads 
	          WHERE lead_id = $1 AND deleted_at IS NULL`
	var dto models.LeadContextDTO
	var estReqDate *time.Time
	err := r.pgx.QueryRow(ctx, query, leadID).Scan(&dto.LeadID, &dto.Company, &dto.ProjectName, &dto.Owner, &estReqDate)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, helpers.ErrNotFound
		}
		return nil, fmt.Errorf("repo: get lead context: %w", err)
	}
	if estReqDate != nil {
		d := estReqDate.Format("2006-01-02")
		dto.EstimatedRequirementDate = &d
	}
	return &dto, nil
}

func (r *commercialRepository) GetOrCreateDraft(ctx context.Context, leadID string) (*models.CommercialEstimation, error) {
	// Try to find existing commercial estimation with single consolidated query
	est, err := r.findByLeadIDSingleQuery(ctx, leadID)
	if err == nil {
		return est, nil
	}

	if !errors.Is(err, helpers.ErrNotFound) {
		return nil, err
	}

	// Atomically create draft if not exists
	now := time.Now()
	startDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	durationMonths := 1
	endDate := startDate.AddDate(0, 1, 0)

	var lead models.Lead
	if leadErr := r.db.WithContext(ctx).Where("lead_id = ? AND deleted_at IS NULL", leadID).First(&lead).Error; leadErr == nil {
		if lead.EstimatedRequirementDate != nil {
			reqDate := *lead.EstimatedRequirementDate
			reqDate = time.Date(reqDate.Year(), reqDate.Month(), reqDate.Day(), 0, 0, 0, 0, time.UTC)
			if reqDate.After(startDate) {
				endDate = reqDate
				months := (endDate.Year()-startDate.Year())*12 + int(endDate.Month()-startDate.Month())
				if endDate.Day() > startDate.Day() {
					months++
				}
				if months < 1 {
					months = 1
				}
				durationMonths = months
			}
		}
	}

	newDraft := models.CommercialEstimation{
		LeadID:                  leadID,
		Currency:                models.CurrencyUSD,
		BillingType:             "T&M",
		StartDate:               startDate,
		EstimatedDurationMonths: durationMonths,
		EstimatedEndDate:        endDate,
		MarkupPercent:           0.00,
		DiscountPercent:         0.00,
		Status:                  models.CommercialStatusDraft,
		Resources:               []models.CommercialResource{},
		Expenses:                []models.CommercialExpense{},
		SDLCAllocations: []models.SDLCAllocation{
			{Phase: "Discovery & Architecture", ManDays: 0},
			{Phase: "UI/UX Design", ManDays: 0},
			{Phase: "Core Development", ManDays: 0},
			{Phase: "QA & Testing", ManDays: 0},
			{Phase: "Deployment & UAT", ManDays: 0},
		},
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
	loaded, loadErr := r.GetByLeadID(ctx, leadID)
	if loadErr == nil {
		syncLeadDealValue(ctx, r.db, leadID, loaded)
	}
	return loaded, loadErr
}

func syncLeadDealValue(ctx context.Context, db *gorm.DB, leadID string, est *models.CommercialEstimation) {
	if est == nil {
		return
	}
	var resRev float64
	for _, res := range est.Resources {
		resRev += res.TotalRevenue
	}
	var expCost float64
	for _, exp := range est.Expenses {
		expCost += exp.Cost
	}
	mult := 1.0 + (est.MarkupPercent/100.0) - (est.DiscountPercent/100.0)
	calcPrice := math.Round((resRev*mult+expCost)*100) / 100
	effPrice := calcPrice
	if est.ManualSellingPrice != nil && *est.ManualSellingPrice > 0 {
		effPrice = math.Round(*est.ManualSellingPrice*100) / 100
	}
	_ = db.WithContext(ctx).Model(&models.Lead{}).Where("lead_id = ? AND deleted_at IS NULL", leadID).Update("value", effPrice).Error
}

func (r *commercialRepository) GetByLeadID(ctx context.Context, leadID string) (*models.CommercialEstimation, error) {
	return r.findByLeadIDSingleQuery(ctx, leadID)
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
		// 1. Verify existence or create new header
		var existing models.CommercialEstimation
		if err := tx.Where("lead_id = ?", leadID).First(&existing).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				existing = models.CommercialEstimation{
					LeadID:                  leadID,
					Currency:                estimation.Currency,
					BillingType:             estimation.BillingType,
					StartDate:               estimation.StartDate,
					EstimatedDurationMonths: estimation.EstimatedDurationMonths,
					EstimatedEndDate:        estimation.EstimatedEndDate,
					MarkupPercent:           estimation.MarkupPercent,
					DiscountPercent:         estimation.DiscountPercent,
					ManualSellingPrice:      estimation.ManualSellingPrice,
					Status:                  estimation.Status,
				}
				if createErr := tx.Create(&existing).Error; createErr != nil {
					return createErr
				}
			} else {
				return err
			}
		} else {
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
