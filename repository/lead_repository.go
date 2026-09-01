package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"

	"crm-auth-service/helpers"
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

	FindStages(ctx context.Context) ([]*models.LeadStage, error)
	FindLeads(ctx context.Context, page, limit int, search, owner, priority, stage, sortBy, sortOrder string) ([]*models.Lead, int64, error)
	FindByID(ctx context.Context, leadID string) (*models.Lead, error)
	UpdateLeadStatusAndStage(ctx context.Context, leadID string, status, stage string, lostReason *string) error

	CreateActivity(activity *models.Activity) error
	GetActivityByID(id uint) (*models.Activity, error)
	UpdateActivityCompleted(id uint, completed bool) error
	CheckEmailExists(email string) (bool, error)
	CheckCompanyExists(company string) (bool, error)
	CheckUserActive(name string) (bool, error)
	CheckStageExistsCaseInsensitive(stage string) (bool, string, error)
}

type leadRepository struct {
	pgx *pgxpool.Pool
	db  *gorm.DB
}

func NewLeadRepository(pgx *pgxpool.Pool, db *gorm.DB) LeadRepository {
	return &leadRepository{
		pgx: pgx,
		db:  db,
	}
}

// ---------------------------------------------
// My Methods (GORM)
// ---------------------------------------------

func (r *leadRepository) GetUserNameByEmail(email string) (string, error) {
	var user models.User
	err := r.db.Where("email = ?", email).First(&user).Error
	return user.Name, err
}

func (r *leadRepository) CheckUserExists(name string) (bool, error) {
	var count int64
	err := r.db.Model(&models.User{}).Where("name = ?", name).Count(&count).Error
	return count > 0, err
}

func (r *leadRepository) CheckStageExists(stage string) (bool, error) {
	var count int64
	err := r.db.Model(&models.LeadStage{}).Where("name = ? AND is_active = ?", stage, true).Count(&count).Error
	return count > 0, err
}

func (r *leadRepository) CheckStageExistsCaseInsensitive(stage string) (bool, string, error) {
	var leadStage models.LeadStage
	err := r.db.Where("LOWER(name) = LOWER(?) AND is_active = ?", stage, true).First(&leadStage).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, "", nil
		}
		return false, "", err
	}
	return true, leadStage.Name, nil
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
	err := r.db.Where("lead_id = ? AND deleted_at IS NULL", leadID).First(&lead).Error
	if err != nil {
		return nil, err
	}
	return &lead, nil
}

func (r *leadRepository) UpdateLead(leadID string, updates map[string]interface{}) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var lead models.Lead
		if err := tx.Where("lead_id = ? AND deleted_at IS NULL", leadID).First(&lead).Error; err != nil {
			return err
		}
		return tx.Model(&lead).Updates(updates).Error
	})
}

func (r *leadRepository) DeleteLead(leadID string) error {
	res := r.db.Where("lead_id = ? AND deleted_at IS NULL", leadID).Delete(&models.Lead{})
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
	err := r.db.Preload("Activities", func(db *gorm.DB) *gorm.DB {
		return db.Order("created_at desc")
	}).Where("lead_id = ? AND deleted_at IS NULL", leadID).First(&lead).Error
	if err != nil {
		return nil, err
	}
	return &lead, nil
}

func (r *leadRepository) CreateActivity(activity *models.Activity) error {
	return r.db.Create(activity).Error
}

func (r *leadRepository) GetActivityByID(id uint) (*models.Activity, error) {
	var activity models.Activity
	err := r.db.Where("id = ?", id).First(&activity).Error
	if err != nil {
		return nil, err
	}
	return &activity, nil
}

func (r *leadRepository) UpdateActivityCompleted(id uint, completed bool) error {
	return r.db.Model(&models.Activity{}).Where("id = ?", id).Update("completed", completed).Error
}

func (r *leadRepository) CheckEmailExists(email string) (bool, error) {
	var count int64
	err := r.db.Model(&models.Lead{}).Where("email = ?", email).Count(&count).Error
	return count > 0, err
}

func (r *leadRepository) CheckCompanyExists(company string) (bool, error) {
	var count int64
	err := r.db.Model(&models.Lead{}).Where("LOWER(company) = LOWER(?)", company).Count(&count).Error
	return count > 0, err
}

func (r *leadRepository) CheckUserActive(name string) (bool, error) {
	var count int64
	err := r.db.Model(&models.User{}).Where("name = ? AND is_active = ?", name, true).Count(&count).Error
	return count > 0, err
}

// ---------------------------------------------
// Sahil's Methods (PGX)
// ---------------------------------------------

func (r *leadRepository) FindStages(ctx context.Context) ([]*models.LeadStage, error) {
	query := "SELECT id, name, status, sort_order, is_active FROM lead_stages WHERE is_active = TRUE ORDER BY sort_order ASC"
	rows, err := r.pgx.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("repo: find stages: %w", err)
	}
	defer rows.Close()

	var stages []*models.LeadStage
	for rows.Next() {
		var s models.LeadStage
		if err := rows.Scan(&s.ID, &s.Name, &s.Status, &s.SortOrder, &s.IsActive); err != nil {
			return nil, err
		}
		stages = append(stages, &s)
	}
	return stages, nil
}

func (r *leadRepository) FindLeads(ctx context.Context, page, limit int, search, owner, priority, stage, sortBy, sortOrder string) ([]*models.Lead, int64, error) {
	var whereClauses []string
	var args []any
	argCount := 1

	whereClauses = append(whereClauses, "deleted_at IS NULL")

	if search != "" {
		placeholder := fmt.Sprintf("$%d", argCount)
		whereClauses = append(whereClauses, fmt.Sprintf("(company ILIKE %s OR contact ILIKE %s OR lead_id ILIKE %s)", placeholder, placeholder, placeholder))
		args = append(args, "%"+search+"%")
		argCount++
	}
	if owner != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("owner = $%d", argCount))
		args = append(args, owner)
		argCount++
	}
	if priority != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("priority = $%d", argCount))
		args = append(args, priority)
		argCount++
	}
	if stage != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("LOWER(stage) = LOWER($%d)", argCount))
		args = append(args, stage)
		argCount++
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	countQuery := "SELECT COUNT(*) FROM leads" + whereSQL
	var total int64
	err := r.pgx.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("repo: count leads: %w", err)
	}
	if total == 0 {
		return []*models.Lead{}, 0, nil
	}

	var orderByCol string
	switch sortBy {
	case "id":
		orderByCol = "lead_id"
	case "value":
		orderByCol = "value"
	case "createdAt":
		orderByCol = "created_at"
	default:
		orderByCol = "created_at"
	}

	dir := "DESC"
	if strings.ToLower(sortOrder) == "asc" {
		dir = "ASC"
	}

	offset := (page - 1) * limit
	limitOffsetSQL := fmt.Sprintf(" ORDER BY %s %s LIMIT $%d OFFSET $%d", orderByCol, dir, argCount, argCount+1)
	args = append(args, limit, offset)

	dataQuery := `SELECT id, lead_id, company, project_name, contact, email, phone, office_phone, office_phone_country, owner, industry, size, region, source, stage, status, sentiment, priority, value, lost_reason, best_time, lifecycle_template, kam_name, designation, best_time_to_connect, alternate_phone, alternate_phone_country, linkedin_profile_url, linkedin_company_page_url, estimated_requirement_date, last_contact_date, next_follow_up, basic_requirements, notes, product_service, request_type, created_at, updated_at 
				  FROM leads` + whereSQL + limitOffsetSQL

	rows, err := r.pgx.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("repo: find leads data: %w", err)
	}
	defer rows.Close()

	var leads []*models.Lead
	for rows.Next() {
		var l models.Lead
		err := rows.Scan(
			&l.ID, &l.LeadID, &l.Company, &l.ProjectName, &l.Contact,
			&l.Email, &l.Phone, &l.OfficePhone, &l.OfficePhoneCountry, &l.Owner,
			&l.Industry, &l.Size, &l.Region, &l.Source, &l.Stage, &l.Status,
			&l.Sentiment, &l.Priority, &l.Value, &l.LostReason, &l.BestTime,
			&l.LifecycleTemplate, &l.KamName, &l.Designation, &l.BestTimeToConnect,
			&l.AlternatePhone, &l.AlternatePhoneCountry, &l.LinkedinProfileURL, &l.LinkedinCompanyPageURL,
			&l.EstimatedRequirementDate, &l.LastContactDate, &l.NextFollowUp, &l.BasicRequirements, &l.Notes,
			&l.ProductService, &l.RequestType,
			&l.CreatedAt, &l.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("repo: scan lead: %w", err)
		}
		leads = append(leads, &l)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return leads, total, nil
}

func (r *leadRepository) FindByID(ctx context.Context, leadID string) (*models.Lead, error) {
	query := `SELECT id, lead_id, company, project_name, contact, email, phone, office_phone, office_phone_country, owner, industry, size, region, source, stage, status, sentiment, priority, value, lost_reason, best_time, lifecycle_template, kam_name, designation, best_time_to_connect, alternate_phone, alternate_phone_country, linkedin_profile_url, linkedin_company_page_url, estimated_requirement_date, last_contact_date, next_follow_up, basic_requirements, notes, product_service, request_type, created_at, updated_at 
			  FROM leads 
			  WHERE lead_id = $1 AND deleted_at IS NULL`
	row := r.pgx.QueryRow(ctx, query, leadID)

	var l models.Lead
	err := row.Scan(
		&l.ID, &l.LeadID, &l.Company, &l.ProjectName, &l.Contact,
		&l.Email, &l.Phone, &l.OfficePhone, &l.OfficePhoneCountry, &l.Owner,
		&l.Industry, &l.Size, &l.Region, &l.Source, &l.Stage, &l.Status,
		&l.Sentiment, &l.Priority, &l.Value, &l.LostReason, &l.BestTime,
		&l.LifecycleTemplate, &l.KamName, &l.Designation, &l.BestTimeToConnect,
		&l.AlternatePhone, &l.AlternatePhoneCountry, &l.LinkedinProfileURL, &l.LinkedinCompanyPageURL,
		&l.EstimatedRequirementDate, &l.LastContactDate, &l.NextFollowUp, &l.BasicRequirements, &l.Notes,
		&l.ProductService, &l.RequestType,
		&l.CreatedAt, &l.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, helpers.ErrNotFound
		}
		return nil, err
	}
	return &l, nil
}

func (r *leadRepository) UpdateLeadStatusAndStage(ctx context.Context, leadID string, status, stage string, lostReason *string) error {
	query := `UPDATE leads 
			  SET status = $1, stage = $2, lost_reason = $3, updated_at = NOW() 
			  WHERE lead_id = $4 AND deleted_at IS NULL`
	_, err := r.pgx.Exec(ctx, query, status, stage, lostReason, leadID)
	return err
}
