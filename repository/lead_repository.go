package repository

import (
	"context"
	"errors"
	"fmt"
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

func (r *leadRepository) FindLeads(ctx context.Context, scope helpers.DataScope, page, limit int, search, owner, priority, stage, sortBy, sortOrder string) ([]*models.Lead, int64, error) {
	var whereClauses []string
	var args []any
	argCount := 1

	whereClauses = append(whereClauses, "deleted_at IS NULL")

	if !scope.IsUnrestricted {
		placeholder := fmt.Sprintf("$%d", argCount)
		whereClauses = append(whereClauses, fmt.Sprintf("(assigned_to = ANY(%s) OR created_by = ANY(%s))", placeholder, placeholder))
		args = append(args, scope.AllowedUserIDs)
		argCount++
	}

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

	dataQuery := `SELECT id, lead_id, company, project_name, contact, email, phone, phone_country, office_phone, office_phone_country, owner, industry, size, region, source, stage, status, sentiment, priority, value, lost_reason, best_time, lifecycle_template, kam_name, designation, best_time_to_connect, alternate_phone, alternate_phone_country, linkedin_profile_url, linkedin_company_page_url, estimated_requirement_date, last_contact_date, next_follow_up, basic_requirements, notes, request_details, request_type, created_by, assigned_to, created_at, updated_at 
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
	query := `SELECT id, lead_id, company, project_name, contact, email, phone, phone_country, office_phone, office_phone_country, owner, industry, size, region, source, stage, status, sentiment, priority, value, lost_reason, best_time, lifecycle_template, kam_name, designation, best_time_to_connect, alternate_phone, alternate_phone_country, linkedin_profile_url, linkedin_company_page_url, estimated_requirement_date, last_contact_date, next_follow_up, basic_requirements, notes, request_details, request_type, created_by, assigned_to, created_at, updated_at 
			  FROM leads 
			  WHERE lead_id = $1 AND deleted_at IS NULL`
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

func (r *leadRepository) FindActivitiesFeed(ctx context.Context, scope helpers.DataScope, q models.GetActivitiesQuery) ([]models.ActivityFeedItemResponse, int64, models.ActivityTypeCounts, error) {
	var whereClauses []string
	var args []interface{}
	argCount := 1

	whereClauses = append(whereClauses, "l.deleted_at IS NULL")

	if !scope.IsUnrestricted {
		placeholder := fmt.Sprintf("$%d", argCount)
		whereClauses = append(whereClauses, fmt.Sprintf("(l.assigned_to = ANY(%s) OR l.created_by = ANY(%s))", placeholder, placeholder))
		args = append(args, scope.AllowedUserIDs)
		argCount++
	}

	repFilter := q.Rep
	if repFilter == "" {
		repFilter = q.UserID
	}
	if repFilter != "" {
		placeholder := fmt.Sprintf("$%d", argCount)
		whereClauses = append(whereClauses, fmt.Sprintf("(u.id::text = %s OR u.name ILIKE %s OR u.email ILIKE %s OR a.rep::text = %s)", placeholder, placeholder, placeholder, placeholder))
		args = append(args, repFilter)
		argCount++
	}

	if q.LeadID != "" {
		placeholder := fmt.Sprintf("$%d", argCount)
		whereClauses = append(whereClauses, fmt.Sprintf("a.lead_id = %s", placeholder))
		args = append(args, q.LeadID)
		argCount++
	}

	if q.Geography != "" {
		placeholder := fmt.Sprintf("$%d", argCount)
		whereClauses = append(whereClauses, fmt.Sprintf("l.region ILIKE %s", placeholder))
		args = append(args, "%"+q.Geography+"%")
		argCount++
	}

	if q.Industry != "" {
		placeholder := fmt.Sprintf("$%d", argCount)
		whereClauses = append(whereClauses, fmt.Sprintf("l.industry ILIKE %s", placeholder))
		args = append(args, "%"+q.Industry+"%")
		argCount++
	}

	if q.DealSize != "" {
		placeholder := fmt.Sprintf("$%d", argCount)
		whereClauses = append(whereClauses, fmt.Sprintf("l.size ILIKE %s", placeholder))
		args = append(args, "%"+q.DealSize+"%")
		argCount++
	}

	if q.DueStatus == "overdue" {
		whereClauses = append(whereClauses, "a.due_date < CURRENT_DATE AND a.completed = FALSE")
	} else if q.DueStatus == "upcoming" {
		whereClauses = append(whereClauses, "a.due_date >= CURRENT_DATE AND a.due_date <= CURRENT_DATE + INTERVAL '7 days' AND a.completed = FALSE")
	}

	// Query 2: Type Counts (unaffected by selected type filter and unaffected by pagination)
	typeCountWhereSQL := ""
	if len(whereClauses) > 0 {
		typeCountWhereSQL = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	typeCountsQuery := `SELECT a.type, COUNT(*) 
						FROM activities a 
						INNER JOIN leads l ON a.lead_id = l.lead_id 
						LEFT JOIN users u ON a.rep = u.id` + typeCountWhereSQL + ` GROUP BY a.type`

	var typeCounts models.ActivityTypeCounts
	typeRows, err := r.pgx.Query(ctx, typeCountsQuery, args...)
	if err != nil {
		return nil, 0, typeCounts, fmt.Errorf("repo: type counts: %w", err)
	}
	defer typeRows.Close()

	for typeRows.Next() {
		var rawType string
		var cnt int64
		if err := typeRows.Scan(&rawType, &cnt); err == nil {
			typeCounts.All += cnt
			normType := strings.ToLower(strings.TrimSpace(rawType))
			switch normType {
			case "call":
				typeCounts.Call += cnt
			case "email":
				typeCounts.Email += cnt
			case "meeting":
				typeCounts.Meeting += cnt
			case "demo":
				typeCounts.Demo += cnt
			case "linkedin":
				typeCounts.Linkedin += cnt
			case "proposal_sent", "proposal sent", "proposalsent":
				typeCounts.ProposalSent += cnt
			default:
				typeCounts.Other += cnt
			}
		}
	}

	// Query 1: Filtered Paginated Data
	dataWhereClauses := append([]string{}, whereClauses...)
	dataArgs := append([]interface{}{}, args...)
	dataArgCount := argCount

	if q.Type != "" {
		placeholder := fmt.Sprintf("$%d", dataArgCount)
		dataWhereClauses = append(dataWhereClauses, fmt.Sprintf("LOWER(a.type) = LOWER(%s)", placeholder))
		dataArgs = append(dataArgs, q.Type)
		dataArgCount++
	}

	dataWhereSQL := ""
	if len(dataWhereClauses) > 0 {
		dataWhereSQL = " WHERE " + strings.Join(dataWhereClauses, " AND ")
	}

	countQuery := `SELECT COUNT(*) 
				   FROM activities a 
				   INNER JOIN leads l ON a.lead_id = l.lead_id 
				   LEFT JOIN users u ON a.rep = u.id` + dataWhereSQL
	var total int64
	err = r.pgx.QueryRow(ctx, countQuery, dataArgs...).Scan(&total)
	if err != nil {
		return nil, 0, typeCounts, fmt.Errorf("repo: count activities: %w", err)
	}

	if total == 0 {
		return []models.ActivityFeedItemResponse{}, 0, typeCounts, nil
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

	limitOffsetSQL := fmt.Sprintf(" ORDER BY a.created_at DESC LIMIT $%d OFFSET $%d", dataArgCount, dataArgCount+1)
	dataArgs = append(dataArgs, limit, offset)

	dataQuery := `SELECT a.id, a.type, a."desc", a.outcome, a.due_date, a.completed, a.created_at, 
						 l.contact, l.lead_id, l.company, l.region, l.industry, l.size,
						 u.name, u.email,
						 l.priority, l.status
				  FROM activities a 
				  INNER JOIN leads l ON a.lead_id = l.lead_id 
				  LEFT JOIN users u ON a.rep = u.id` + dataWhereSQL + limitOffsetSQL

	rows, err := r.pgx.Query(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, typeCounts, fmt.Errorf("repo: find activities feed: %w", err)
	}
	defer rows.Close()

	var items []models.ActivityFeedItemResponse
	for rows.Next() {
		var id uint
		var actType, desc string
		var outcome *string
		var dueDate *time.Time
		var completed bool
		var createdAt time.Time
		var leadName, geography, industry, dealSize *string
		var leadID, company string
		var userName, userEmail *string
		var priority, status *string

		err := rows.Scan(
			&id, &actType, &desc, &outcome, &dueDate, &completed, &createdAt,
			&leadName, &leadID, &company, &geography, &industry, &dealSize,
			&userName, &userEmail,
			&priority, &status,
		)
		if err != nil {
			return nil, 0, typeCounts, fmt.Errorf("repo: scan activity feed: %w", err)
		}

		var repStr *string
		if userName != nil && *userName != "" {
			repStr = userName
		} else if userEmail != nil && *userEmail != "" {
			repStr = userEmail
		}

		var dueDateStr *string
		if dueDate != nil {
			d := dueDate.Format("2006-01-02")
			dueDateStr = &d
		}

		items = append(items, models.ActivityFeedItemResponse{
			ID:        id,
			Type:      actType,
			Desc:      desc,
			LeadName:  leadName,
			LeadID:    leadID,
			Company:   company,
			Rep:       repStr,
			Timestamp: createdAt.Format(time.RFC3339),
			Outcome:   outcome,
			Geography: geography,
			Industry:  industry,
			DealSize:  dealSize,
			DueDate:   dueDateStr,
			Completed: completed,
			Priority:  priority,
			Status:    status,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, typeCounts, err
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
