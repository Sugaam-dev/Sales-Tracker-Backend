package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"crm-auth-service/helpers"
	"crm-auth-service/models"
)

type LeadRepository interface {
	FindStages(ctx context.Context) ([]*models.LeadStage, error)
	FindLeads(ctx context.Context, page, limit int, search, owner, priority, stage, sortBy, sortOrder string) ([]*models.Lead, int64, error)
	FindByID(ctx context.Context, leadID string) (*models.Lead, error)
}

type leadRepository struct {
	db *pgxpool.Pool
}

func NewLeadRepository(db *pgxpool.Pool) LeadRepository {
	return &leadRepository{db: db}
}

func (r *leadRepository) FindStages(ctx context.Context) ([]*models.LeadStage, error) {
	query := "SELECT id, name, sort_order, is_active FROM lead_stages WHERE is_active = TRUE ORDER BY sort_order ASC"
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("repo: find stages: %w", err)
	}
	defer rows.Close()

	var stages []*models.LeadStage
	for rows.Next() {
		var s models.LeadStage
		if err := rows.Scan(&s.ID, &s.Name, &s.SortOrder, &s.IsActive); err != nil {
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

	// Soft-delete check
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

	// 1. Get Count
	countQuery := "SELECT COUNT(*) FROM leads" + whereSQL
	var total int64
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("repo: count leads: %w", err)
	}

	if total == 0 {
		return []*models.Lead{}, 0, nil
	}

	// 2. Sorting & Pagination
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

	dataQuery := `SELECT id, lead_id, company, project_name, contact, email, phone, office_phone, office_phone_country, owner, industry, size, region, source, stage, status, sentiment, priority, value, created_at, updated_at 
				  FROM leads` + whereSQL + limitOffsetSQL

	rows, err := r.db.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("repo: find leads data: %w", err)
	}
	defer rows.Close()

	var leads []*models.Lead
	for rows.Next() {
		var l models.Lead
		err := rows.Scan(
			&l.ID,
			&l.LeadID,
			&l.Company,
			&l.ProjectName,
			&l.Contact,
			&l.Email,
			&l.Phone,
			&l.OfficePhone,
			&l.OfficePhoneCountry,
			&l.Owner,
			&l.Industry,
			&l.Size,
			&l.Region,
			&l.Source,
			&l.Stage,
			&l.Status,
			&l.Sentiment,
			&l.Priority,
			&l.Value,
			&l.CreatedAt,
			&l.UpdatedAt,
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
	query := `SELECT id, lead_id, company, project_name, contact, email, phone, office_phone, office_phone_country, owner, industry, size, region, source, stage, status, sentiment, priority, value, created_at, updated_at 
			  FROM leads 
			  WHERE lead_id = $1 AND deleted_at IS NULL`
	row := r.db.QueryRow(ctx, query, leadID)

	var l models.Lead
	err := row.Scan(
		&l.ID,
		&l.LeadID,
		&l.Company,
		&l.ProjectName,
		&l.Contact,
		&l.Email,
		&l.Phone,
		&l.OfficePhone,
		&l.OfficePhoneCountry,
		&l.Owner,
		&l.Industry,
		&l.Size,
		&l.Region,
		&l.Source,
		&l.Stage,
		&l.Status,
		&l.Sentiment,
		&l.Priority,
		&l.Value,
		&l.CreatedAt,
		&l.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, helpers.ErrNotFound
		}
		return nil, err
	}
	return &l, nil
}
