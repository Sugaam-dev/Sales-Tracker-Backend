package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

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
	FindLeads(ctx context.Context, scope helpers.DataScope, page, limit int, search, owner, priority, stage, sortBy, sortOrder string) ([]*models.Lead, int64, error)
	FindByID(ctx context.Context, leadID string) (*models.Lead, error)
	UpdateLeadStatusAndStage(ctx context.Context, leadID string, status, stage string, lostReason *string) error
	GetHeatMapAggregation(ctx context.Context) ([]models.LeadAggregationRow, error)

	CreateActivity(activity *models.Activity) error
	GetActivityByID(id uint) (*models.Activity, error)
	UpdateActivityCompleted(id uint, completed bool) error
	CheckEmailExists(email string) (bool, error)
	CheckCompanyExists(company string) (bool, error)
	CheckUserActive(name string) (bool, error)
	CheckStageExistsCaseInsensitive(stage string) (bool, string, error)

	FindActivitiesFeed(ctx context.Context, scope helpers.DataScope, q models.GetActivitiesQuery) ([]models.ActivityFeedItemResponse, int64, models.ActivityTypeCounts, error)
	FindLeadByCompanyOrContact(ctx context.Context, name string) (*models.Lead, error)
	GetActivitiesSummary(ctx context.Context, scope helpers.DataScope) (*models.ActivitySummaryData, error)
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
	var est models.CommercialEstimation
	if err := r.db.Preload("Resources").Preload("Expenses").Where("lead_id = ?", leadID).First(&est).Error; err == nil {
		var resRev float64
		for _, res := range est.Resources {
			resRev += res.TotalRevenue
		}
		var expCost float64
		for _, exp := range est.Expenses {
			expCost += exp.Cost
		}
		if (resRev > 0 || expCost > 0) || (est.ManualSellingPrice != nil && *est.ManualSellingPrice > 0) {
			mult := 1.0 + (est.MarkupPercent/100.0) - (est.DiscountPercent/100.0)
			calcPrice := math.Round((resRev*mult+expCost)*100) / 100
			effPrice := calcPrice
			if est.ManualSellingPrice != nil && *est.ManualSellingPrice > 0 {
				effPrice = math.Round(*est.ManualSellingPrice*100) / 100
			}
			lead.Value = &effPrice
		} else {
			lead.Value = nil
		}
	} else {
		lead.Value = nil
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
// Team Methods (PGX Pool)
// ---------------------------------------------

func (r *leadRepository) FindStages(ctx context.Context) ([]*models.LeadStage, error) {
	query := `SELECT id, name, status, sort_order, is_active FROM lead_stages WHERE is_active = true ORDER BY sort_order ASC`
	rows, err := r.pgx.Query(ctx, query)
	if err != nil {
		return nil, err
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stages, nil
}

func (r *leadRepository) FindLeads(ctx context.Context, scope helpers.DataScope, page, limit int, search, owner, priority, stage, sortBy, sortOrder string) ([]*models.Lead, int64, error) {
	var whereClauses []string
	var args []any
	argCount := 1

	whereClauses = append(whereClauses, "l.deleted_at IS NULL")

	if !scope.IsUnrestricted {
		placeholder := fmt.Sprintf("$%d", argCount)
		whereClauses = append(whereClauses, fmt.Sprintf("(l.assigned_to = ANY(%s) OR l.created_by = ANY(%s))", placeholder, placeholder))
		args = append(args, scope.AllowedUserIDs)
		argCount++
	}

	if search != "" {
		placeholder := fmt.Sprintf("$%d", argCount)
		whereClauses = append(whereClauses, fmt.Sprintf("(l.company ILIKE %s OR l.contact ILIKE %s OR l.lead_id ILIKE %s)", placeholder, placeholder, placeholder))
		args = append(args, "%"+search+"%")
		argCount++
	}
	if owner != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("l.owner = $%d", argCount))
		args = append(args, owner)
		argCount++
	}
	if priority != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("l.priority = $%d", argCount))
		args = append(args, priority)
		argCount++
	}
	if stage != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("LOWER(l.stage) = LOWER($%d)", argCount))
		args = append(args, stage)
		argCount++
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	countQuery := "SELECT COUNT(*) FROM leads l" + whereSQL
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
		orderByCol = "l.lead_id"
	case "value":
		orderByCol = "calculated_value"
	case "createdAt":
		orderByCol = "l.created_at"
	default:
		orderByCol = "l.created_at"
	}

	dir := "DESC"
	if strings.ToLower(sortOrder) == "asc" {
		dir = "ASC"
	}

	offset := (page - 1) * limit
	limitOffsetSQL := fmt.Sprintf(" ORDER BY %s %s LIMIT $%d OFFSET $%d", orderByCol, dir, argCount, argCount+1)
	args = append(args, limit, offset)

	dataQuery := `SELECT 
		l.id, l.lead_id, l.company, l.project_name, l.contact, l.email, l.phone, l.phone_country, 
		l.office_phone, l.office_phone_country, l.owner, l.industry, l.size, l.region, l.source, 
		l.stage, l.status, l.sentiment, l.priority, 
		COALESCE(
			ce.manual_selling_price,
			CASE WHEN ce.id IS NOT NULL AND (COALESCE(r.total_res_revenue, 0) > 0 OR COALESCE(e.total_exp_cost, 0) > 0) THEN
				ROUND(
					COALESCE(r.total_res_revenue, 0) * (1.0 + (COALESCE(ce.markup_percent, 0) - COALESCE(ce.discount_percent, 0)) / 100.0) 
					+ COALESCE(e.total_exp_cost, 0),
					2
				)
			ELSE NULL
			END
		) AS calculated_value,
		l.lost_reason, l.best_time, l.lifecycle_template, l.kam_name, l.designation, 
		l.best_time_to_connect, l.alternate_phone, l.alternate_phone_country, 
		l.linkedin_profile_url, l.linkedin_company_page_url, l.estimated_requirement_date, 
		l.last_contact_date, l.next_follow_up, l.basic_requirements, l.notes, 
		l.request_details, l.request_type, l.created_by, l.assigned_to, l.created_at, l.updated_at
	FROM leads l
	LEFT JOIN commercial_estimations ce ON ce.lead_id = l.lead_id
	LEFT JOIN (
		SELECT commercial_estimation_id, SUM(total_revenue) AS total_res_revenue
		FROM commercial_resources
		GROUP BY commercial_estimation_id
	) r ON r.commercial_estimation_id = ce.id
	LEFT JOIN (
		SELECT commercial_estimation_id, SUM(cost) AS total_exp_cost
		FROM commercial_expenses
		GROUP BY commercial_estimation_id
	) e ON e.commercial_estimation_id = ce.id` + whereSQL + limitOffsetSQL

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
			&l.Email, &l.Phone, &l.PhoneCountry, &l.OfficePhone, &l.OfficePhoneCountry, &l.Owner,
			&l.Industry, &l.Size, &l.Region, &l.Source, &l.Stage, &l.Status,
			&l.Sentiment, &l.Priority, &l.Value, &l.LostReason, &l.BestTime,
			&l.LifecycleTemplate, &l.KamName, &l.Designation, &l.BestTimeToConnect,
			&l.AlternatePhone, &l.AlternatePhoneCountry, &l.LinkedinProfileURL, &l.LinkedinCompanyPageURL,
			&l.EstimatedRequirementDate, &l.LastContactDate, &l.NextFollowUp, &l.BasicRequirements, &l.Notes,
			&l.RequestDetails, &l.RequestType,
			&l.CreatedBy, &l.AssignedTo,
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
	query := `SELECT 
		l.id, l.lead_id, l.company, l.project_name, l.contact, l.email, l.phone, l.phone_country, 
		l.office_phone, l.office_phone_country, l.owner, l.industry, l.size, l.region, l.source, 
		l.stage, l.status, l.sentiment, l.priority, 
		COALESCE(
			ce.manual_selling_price,
			CASE WHEN ce.id IS NOT NULL AND (COALESCE(r.total_res_revenue, 0) > 0 OR COALESCE(e.total_exp_cost, 0) > 0) THEN
				ROUND(
					COALESCE(r.total_res_revenue, 0) * (1.0 + (COALESCE(ce.markup_percent, 0) - COALESCE(ce.discount_percent, 0)) / 100.0) 
					+ COALESCE(e.total_exp_cost, 0),
					2
				)
			ELSE NULL
			END
		) AS calculated_value,
		l.lost_reason, l.best_time, l.lifecycle_template, l.kam_name, l.designation, 
		l.best_time_to_connect, l.alternate_phone, l.alternate_phone_country, 
		l.linkedin_profile_url, l.linkedin_company_page_url, l.estimated_requirement_date, 
		l.last_contact_date, l.next_follow_up, l.basic_requirements, l.notes, 
		l.request_details, l.request_type, l.created_by, l.assigned_to, l.created_at, l.updated_at 
	FROM leads l
	LEFT JOIN commercial_estimations ce ON ce.lead_id = l.lead_id
	LEFT JOIN (
		SELECT commercial_estimation_id, SUM(total_revenue) AS total_res_revenue
		FROM commercial_resources
		GROUP BY commercial_estimation_id
	) r ON r.commercial_estimation_id = ce.id
	LEFT JOIN (
		SELECT commercial_estimation_id, SUM(cost) AS total_exp_cost
		FROM commercial_expenses
		GROUP BY commercial_estimation_id
	) e ON e.commercial_estimation_id = ce.id
	WHERE l.lead_id = $1 AND l.deleted_at IS NULL`
	row := r.pgx.QueryRow(ctx, query, leadID)

	var l models.Lead
	err := row.Scan(
		&l.ID, &l.LeadID, &l.Company, &l.ProjectName, &l.Contact,
		&l.Email, &l.Phone, &l.PhoneCountry, &l.OfficePhone, &l.OfficePhoneCountry, &l.Owner,
		&l.Industry, &l.Size, &l.Region, &l.Source, &l.Stage, &l.Status,
		&l.Sentiment, &l.Priority, &l.Value, &l.LostReason, &l.BestTime,
		&l.LifecycleTemplate, &l.KamName, &l.Designation, &l.BestTimeToConnect,
		&l.AlternatePhone, &l.AlternatePhoneCountry, &l.LinkedinProfileURL, &l.LinkedinCompanyPageURL,
		&l.EstimatedRequirementDate, &l.LastContactDate, &l.NextFollowUp, &l.BasicRequirements, &l.Notes,
		&l.RequestDetails, &l.RequestType,
		&l.CreatedBy, &l.AssignedTo,
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

func (r *leadRepository) GetHeatMapAggregation(ctx context.Context) ([]models.LeadAggregationRow, error) {
	query := `
		SELECT COALESCE(owner, '') AS owner, COALESCE(stage, '') AS stage, COALESCE(SUM(value), 0) AS value, COUNT(id) AS leads
		FROM leads
		WHERE deleted_at IS NULL
		GROUP BY owner, stage
	`
	rows, err := r.pgx.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.LeadAggregationRow
	for rows.Next() {
		var row models.LeadAggregationRow
		if err := rows.Scan(&row.Owner, &row.Stage, &row.Value, &row.Count); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

type rawActivityFeedDBItem struct {
	ID        uint       `json:"id"`
	Type      string     `json:"type"`
	Desc      string     `json:"desc"`
	Outcome   *string    `json:"outcome"`
	DueDate   *time.Time `json:"due_date"`
	Completed bool       `json:"completed"`
	CreatedAt time.Time  `json:"created_at"`
	LeadName  *string    `json:"lead_name"`
	LeadID    string     `json:"lead_id"`
	Company   string     `json:"company"`
	Geography *string    `json:"geography"`
	Industry  *string    `json:"industry"`
	DealSize  *string    `json:"deal_size"`
	UserName  *string    `json:"user_name"`
	UserEmail *string    `json:"user_email"`
	Priority  *string    `json:"priority"`
	Status    *string    `json:"status"`
}

type activityTypeCountDBRow struct {
	Type  string `json:"type"`
	Count int64  `json:"count"`
}

func (r *leadRepository) FindActivitiesFeed(ctx context.Context, scope helpers.DataScope, q models.GetActivitiesQuery) ([]models.ActivityFeedItemResponse, int64, models.ActivityTypeCounts, error) {
	var baseWhereClauses []string
	var args []interface{}
	argCount := 1

	baseWhereClauses = append(baseWhereClauses, "l.deleted_at IS NULL")

	if !scope.IsUnrestricted {
		placeholder := fmt.Sprintf("$%d", argCount)
		baseWhereClauses = append(baseWhereClauses, fmt.Sprintf("(l.assigned_to = ANY(%s) OR l.created_by = ANY(%s))", placeholder, placeholder))
		args = append(args, scope.AllowedUserIDs)
		argCount++
	}

	repFilter := q.Rep
	if repFilter == "" {
		repFilter = q.UserID
	}
	if repFilter != "" {
		placeholder := fmt.Sprintf("$%d", argCount)
		baseWhereClauses = append(baseWhereClauses, fmt.Sprintf("(u.id::text = %s OR u.name ILIKE %s OR u.email ILIKE %s OR a.rep::text = %s)", placeholder, placeholder, placeholder, placeholder))
		args = append(args, repFilter)
		argCount++
	}

	if q.LeadID != "" {
		placeholder := fmt.Sprintf("$%d", argCount)
		baseWhereClauses = append(baseWhereClauses, fmt.Sprintf("a.lead_id = %s", placeholder))
		args = append(args, q.LeadID)
		argCount++
	}

	if q.Geography != "" {
		placeholder := fmt.Sprintf("$%d", argCount)
		baseWhereClauses = append(baseWhereClauses, fmt.Sprintf("l.region ILIKE %s", placeholder))
		args = append(args, "%"+q.Geography+"%")
		argCount++
	}

	if q.Industry != "" {
		placeholder := fmt.Sprintf("$%d", argCount)
		baseWhereClauses = append(baseWhereClauses, fmt.Sprintf("l.industry ILIKE %s", placeholder))
		args = append(args, "%"+q.Industry+"%")
		argCount++
	}

	if q.DealSize != "" {
		placeholder := fmt.Sprintf("$%d", argCount)
		baseWhereClauses = append(baseWhereClauses, fmt.Sprintf("l.size ILIKE %s", placeholder))
		args = append(args, "%"+q.DealSize+"%")
		argCount++
	}

	if q.DueStatus == "overdue" {
		baseWhereClauses = append(baseWhereClauses, "a.due_date < CURRENT_DATE AND a.completed = FALSE")
	} else if q.DueStatus == "upcoming" {
		baseWhereClauses = append(baseWhereClauses, "a.due_date >= CURRENT_DATE AND a.due_date <= CURRENT_DATE + INTERVAL '7 days' AND a.completed = FALSE")
	}

	baseWhereSQL := " WHERE " + strings.Join(baseWhereClauses, " AND ")

	filteredWhereSQL := "WHERE 1=1"
	if q.Type != "" {
		placeholder := fmt.Sprintf("$%d", argCount)
		filteredWhereSQL += fmt.Sprintf(" AND LOWER(type) = LOWER(%s)", placeholder)
		args = append(args, q.Type)
		argCount++
	}

	page := q.Page
	if page < 1 {
		page = 1
	}
	limit := q.Limit
	if limit < 1 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	limitPlaceholder := fmt.Sprintf("$%d", argCount)
	offsetPlaceholder := fmt.Sprintf("$%d", argCount+1)
	args = append(args, limit, offset)

	query := fmt.Sprintf(`
	WITH base_activities AS (
		SELECT a.id, a.type, a."desc", a.outcome, a.due_date, a.completed, a.created_at,
		       l.contact AS lead_name, l.lead_id, l.company, l.region AS geography, l.industry, l.size AS deal_size,
		       u.name AS user_name, u.email AS user_email,
		       l.priority, l.status
		FROM activities a
		INNER JOIN leads l ON a.lead_id = l.lead_id
		LEFT JOIN users u ON a.rep = u.id
		%s
	),
	type_counts AS (
		SELECT type, COUNT(*) AS cnt
		FROM base_activities
		GROUP BY type
	),
	filtered_activities AS (
		SELECT *
		FROM base_activities
		%s
	),
	total_count AS (
		SELECT COUNT(*) AS total FROM filtered_activities
	),
	paged_activities AS (
		SELECT *
		FROM filtered_activities
		ORDER BY created_at DESC
		LIMIT %s OFFSET %s
	)
	SELECT
		(SELECT total FROM total_count) AS total,
		COALESCE((SELECT json_agg(json_build_object('type', tc.type, 'count', tc.cnt)) FROM type_counts tc), '[]'::json) AS type_counts_json,
		COALESCE((SELECT json_agg(json_build_object(
			'id', pa.id,
			'type', pa.type,
			'desc', pa."desc",
			'outcome', pa.outcome,
			'due_date', pa.due_date,
			'completed', pa.completed,
			'created_at', pa.created_at,
			'lead_name', pa.lead_name,
			'lead_id', pa.lead_id,
			'company', pa.company,
			'geography', pa.geography,
			'industry', pa.industry,
			'deal_size', pa.deal_size,
			'user_name', pa.user_name,
			'user_email', pa.user_email,
			'priority', pa.priority,
			'status', pa.status
		) ORDER BY pa.created_at DESC) FROM paged_activities pa), '[]'::json) AS items_json;`,
		baseWhereSQL, filteredWhereSQL, limitPlaceholder, offsetPlaceholder)

	var total int64
	var typeCountsBytes, itemsBytes []byte
	err := r.pgx.QueryRow(ctx, query, args...).Scan(&total, &typeCountsBytes, &itemsBytes)
	if err != nil {
		return nil, 0, models.ActivityTypeCounts{}, fmt.Errorf("repo: find activities feed: %w", err)
	}

	var typeCounts models.ActivityTypeCounts
	var tcRows []activityTypeCountDBRow
	if err := json.Unmarshal(typeCountsBytes, &tcRows); err == nil {
		for _, tc := range tcRows {
			typeCounts.All += tc.Count
			normType := strings.ToLower(strings.TrimSpace(tc.Type))
			switch normType {
			case "call":
				typeCounts.Call += tc.Count
			case "email":
				typeCounts.Email += tc.Count
			case "meeting":
				typeCounts.Meeting += tc.Count
			case "demo":
				typeCounts.Demo += tc.Count
			case "linkedin":
				typeCounts.Linkedin += tc.Count
			case "proposal_sent", "proposal sent", "proposalsent":
				typeCounts.ProposalSent += tc.Count
			default:
				typeCounts.Other += tc.Count
			}
		}
	}

	var rawItems []rawActivityFeedDBItem
	if err := json.Unmarshal(itemsBytes, &rawItems); err != nil {
		return nil, 0, typeCounts, fmt.Errorf("repo: unmarshal activity feed items: %w", err)
	}

	items := make([]models.ActivityFeedItemResponse, 0, len(rawItems))
	for _, row := range rawItems {
		var repStr *string
		if row.UserName != nil && *row.UserName != "" {
			repStr = row.UserName
		} else if row.UserEmail != nil && *row.UserEmail != "" {
			repStr = row.UserEmail
		}

		var dueDateStr *string
		if row.DueDate != nil {
			d := row.DueDate.Local().Format("2006-01-02")
			dueDateStr = &d
		}

		items = append(items, models.ActivityFeedItemResponse{
			ID:        row.ID,
			Type:      row.Type,
			Desc:      row.Desc,
			LeadName:  row.LeadName,
			LeadID:    row.LeadID,
			Company:   row.Company,
			Rep:       repStr,
			Timestamp: row.CreatedAt.Local().Format(time.RFC3339),
			Outcome:   row.Outcome,
			Geography: row.Geography,
			Industry:  row.Industry,
			DealSize:  row.DealSize,
			DueDate:   dueDateStr,
			Completed: row.Completed,
			Priority:  row.Priority,
			Status:    row.Status,
		})
	}

	return items, total, typeCounts, nil
}

func (r *leadRepository) FindLeadByCompanyOrContact(ctx context.Context, name string) (*models.Lead, error) {
	nameTrimmed := strings.TrimSpace(name)
	query := `SELECT id, lead_id, company, project_name, contact, email, phone, phone_country, office_phone, office_phone_country, owner, industry, size, region, source, stage, status, sentiment, priority, value, lost_reason, best_time, lifecycle_template, kam_name, designation, best_time_to_connect, alternate_phone, alternate_phone_country, linkedin_profile_url, linkedin_company_page_url, estimated_requirement_date, last_contact_date, next_follow_up, basic_requirements, notes, request_details, request_type, created_by, assigned_to, created_at, updated_at 
			  FROM leads 
			  WHERE (LOWER(company) = LOWER($1) OR LOWER(contact) = LOWER($1)) AND deleted_at IS NULL
			  ORDER BY created_at DESC LIMIT 1`
	row := r.pgx.QueryRow(ctx, query, nameTrimmed)

	var l models.Lead
	err := row.Scan(
		&l.ID, &l.LeadID, &l.Company, &l.ProjectName, &l.Contact,
		&l.Email, &l.Phone, &l.PhoneCountry, &l.OfficePhone, &l.OfficePhoneCountry, &l.Owner,
		&l.Industry, &l.Size, &l.Region, &l.Source, &l.Stage, &l.Status,
		&l.Sentiment, &l.Priority, &l.Value, &l.LostReason, &l.BestTime,
		&l.LifecycleTemplate, &l.KamName, &l.Designation, &l.BestTimeToConnect,
		&l.AlternatePhone, &l.AlternatePhoneCountry, &l.LinkedinProfileURL, &l.LinkedinCompanyPageURL,
		&l.EstimatedRequirementDate, &l.LastContactDate, &l.NextFollowUp, &l.BasicRequirements, &l.Notes,
		&l.RequestDetails, &l.RequestType,
		&l.CreatedBy, &l.AssignedTo,
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

func (r *leadRepository) GetActivitiesSummary(ctx context.Context, scope helpers.DataScope) (*models.ActivitySummaryData, error) {
	whereClause := "WHERE l.deleted_at IS NULL"
	var args []any
	if !scope.IsUnrestricted {
		whereClause += " AND (l.assigned_to = ANY($1) OR l.created_by = ANY($1))"
		args = append(args, scope.AllowedUserIDs)
	}

	query := fmt.Sprintf(`SELECT 
		COALESCE(COUNT(*) FILTER (WHERE a.created_at::date = CURRENT_DATE), 0) AS velocity_today_logged,
		COALESCE(COUNT(*) FILTER (WHERE a.created_at::date = CURRENT_DATE AND a.completed = TRUE), 0) AS velocity_today_completed,
		COALESCE(COUNT(*) FILTER (WHERE a.created_at::date = CURRENT_DATE AND a.completed = FALSE), 0) AS velocity_today_planned,
		COALESCE(COUNT(*) FILTER (WHERE a.due_date < CURRENT_DATE AND a.completed = FALSE), 0) AS overdue_count,
		COALESCE(COUNT(*) FILTER (WHERE a.due_date >= CURRENT_DATE AND a.due_date <= CURRENT_DATE + INTERVAL '7 days' AND a.completed = FALSE), 0) AS upcoming_count
	FROM activities a
	INNER JOIN leads l ON a.lead_id = l.lead_id
	%s`, whereClause)

	var s models.ActivitySummaryData
	err := r.pgx.QueryRow(ctx, query, args...).Scan(
		&s.VelocityTodayLogged,
		&s.VelocityTodayCompleted,
		&s.VelocityTodayPlanned,
		&s.OverdueCount,
		&s.UpcomingCount,
	)
	if err != nil {
		return nil, fmt.Errorf("repo: get activities summary: %w", err)
	}
	return &s, nil
}
