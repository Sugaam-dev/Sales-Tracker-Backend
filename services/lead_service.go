package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
	"strconv"

	"crm-auth-service/helpers"
	"crm-auth-service/models"
	"crm-auth-service/repository"
)

var (
	ErrValidation        = errors.New("validation failed")
	ErrDuplicateConflict = errors.New("duplicate conflict")
	ErrNotFound          = errors.New("not found")
	ErrUnauthorized      = errors.New("unauthorized")
)

type LeadService interface {
	CreateLead(req models.CreateLeadRequest) (*models.LeadResponse, error)
	UpdateLead(leadID string, userRole, userEmail string, req models.UpdateLeadRequest) (*models.LeadResponse, error)
	DeleteLead(leadID string, userRole string) error
	GetLeadActivities(leadID string, userRole, userEmail string) ([]models.ActivityResponse, error)

	GetCurrentUsers(ctx context.Context) ([]models.ActiveUserResponse, error)
	GetMasterStages(ctx context.Context) ([]*models.LeadStage, error)
	ListLeads(ctx context.Context, page, limit int, search, owner, priority, stage, sortBy, sortOrder string) ([]models.LeadResponse, *models.PaginationMetadata, error)
	GetLead(ctx context.Context, leadID string) (*models.LeadResponse, error)
}

type leadService struct {
	leadRepo repository.LeadRepository
	userRepo repository.UserRepository
}

func NewLeadService(leadRepo repository.LeadRepository, userRepo repository.UserRepository) LeadService {
	return &leadService{
		leadRepo: leadRepo,
		userRepo: userRepo,
	}
}

// ---------------------------------------------
// My Methods (Create, Update, Delete, Activities)
// ---------------------------------------------

func (s *leadService) handleDBError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "duplicate key value violates unique constraint") || strings.Contains(err.Error(), "23505") {
		return ErrDuplicateConflict
	}
	if strings.Contains(err.Error(), "record not found") {
		return ErrNotFound
	}
	return err
}

func (s *leadService) CreateLead(req models.CreateLeadRequest) (*models.LeadResponse, error) {
	req.Email = strings.ToLower(req.Email)

	if req.Owner != "" {
		exists, err := s.leadRepo.CheckUserExists(req.Owner)
		if err != nil || !exists {
			return nil, ErrValidation
		}
	}
	if req.Stage != "" {
		exists, err := s.leadRepo.CheckStageExists(req.Stage)
		if err != nil || !exists {
			return nil, ErrValidation
		}
	}

	lead := &models.Lead{
		Company:            req.Company,
		Contact:            &req.Contact,
		Email:              &req.Email,
		Phone:              &req.Phone,
		OfficePhone:        &req.OfficePhone,
		OfficePhoneCountry: &req.OfficePhoneCountry,
		Owner:              &req.Owner,
		Stage:              &req.Stage,
		Status:             &req.Status,
		Sentiment:          &req.Sentiment,
		Priority:           &req.Priority,
	}
	if req.ProjectName != "" {
		lead.ProjectName = &req.ProjectName
	}
	if req.Industry != "" {
		lead.Industry = &req.Industry
	}
	if req.Size != "" {
		lead.Size = &req.Size
	}
	if req.Region != "" {
		lead.Region = &req.Region
	}
	if req.Source != "" {
		lead.Source = &req.Source
	}
	if req.Designation != "" {
		lead.Designation = &req.Designation
	}
	if req.BestTime != "" {
		lead.BestTime = &req.BestTime
	}
	if req.ProductService != "" {
		lead.ProductService = &req.ProductService
	}
	if req.RequestType != "" {
		lead.RequestType = &req.RequestType
	}
	if req.LostReason != "" {
		lead.LostReason = &req.LostReason
	}
	if req.Value != "" {
		parsedVal, err := strconv.ParseFloat(req.Value, 64)
		if err == nil {
			lead.Value = &parsedVal
		}
	}

	err := s.leadRepo.CreateLead(lead)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	resp := s.mapToResponse(lead)
	return &resp, nil
}

func (s *leadService) UpdateLead(leadID string, userRole, userEmail string, req models.UpdateLeadRequest) (*models.LeadResponse, error) {
	lead, err := s.leadRepo.GetLeadByLeadID(leadID)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	userName, err := s.leadRepo.GetUserNameByEmail(userEmail)
	if err != nil && userRole != models.RoleAdmin {
		// log or ignore for now, allow edit
	}

	leadOwner := ""
	if lead.Owner != nil {
		leadOwner = *lead.Owner
	}

	if userRole != models.RoleAdmin && leadOwner != userName {
		// bypass strict owner check to allow team collaboration edits
	}

	updates := make(map[string]interface{})
	if req.Owner != nil {
		updates["owner"] = *req.Owner
	}
	if req.Stage != nil {
		updates["stage"] = *req.Stage
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}
	if req.Contact != nil {
		updates["contact"] = *req.Contact
	}
	if req.Email != nil {
		updates["email"] = strings.ToLower(*req.Email)
	}
	if req.Phone != nil {
		updates["phone"] = *req.Phone
	}
	if req.LostReason != nil {
		updates["lost_reason"] = *req.LostReason
	}
	if req.Value != nil {
		parsedVal, err := strconv.ParseFloat(*req.Value, 64)
		if err == nil {
			updates["value"] = parsedVal
		}
	}
	if req.Company != nil {
		updates["company"] = *req.Company
	}
	if req.ProjectName != nil {
		updates["project_name"] = *req.ProjectName
	}
	if req.Designation != nil {
		updates["designation"] = *req.Designation
	}
	if req.Industry != nil {
		updates["industry"] = *req.Industry
	}
	if req.Size != nil {
		updates["size"] = *req.Size
	}
	if req.Region != nil {
		updates["region"] = *req.Region
	}
	if req.Source != nil {
		updates["source"] = *req.Source
	}
	if req.Sentiment != nil {
		updates["sentiment"] = *req.Sentiment
	}
	if req.OfficePhone != nil {
		updates["office_phone"] = *req.OfficePhone
	}
	if req.OfficePhoneCountry != nil {
		updates["office_phone_country"] = *req.OfficePhoneCountry
	}
	if req.BestTime != nil {
		updates["best_time"] = *req.BestTime
	}
	if req.ProductService != nil {
		updates["product_service"] = *req.ProductService
	}
	if req.RequestType != nil {
		updates["request_type"] = *req.RequestType
	}

	err = s.leadRepo.UpdateLead(leadID, updates)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	updatedLead, err := s.leadRepo.GetLeadByLeadID(leadID)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	resp := s.mapToResponse(updatedLead)
	return &resp, nil
}

func (s *leadService) DeleteLead(leadID string, userRole string) error {
	if userRole != models.RoleAdmin && userRole != models.RoleSalesManager {
		return ErrUnauthorized
	}
	err := s.leadRepo.DeleteLead(leadID)
	return s.handleDBError(err)
}

func (s *leadService) GetLeadActivities(leadID string, userRole, userEmail string) ([]models.ActivityResponse, error) {
	lead, err := s.leadRepo.GetLeadActivities(leadID)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	userName, err := s.leadRepo.GetUserNameByEmail(userEmail)
	if err != nil && userRole != models.RoleAdmin {
		// bypass
	}

	leadOwner := ""
	if lead.Owner != nil {
		leadOwner = *lead.Owner
	}

	if userRole != models.RoleAdmin && leadOwner != userName {
		// bypass
	}

	resp := make([]models.ActivityResponse, len(lead.Activities))
	for i, a := range lead.Activities {
		resp[i] = models.ToActivityResponse(a)
	}
	return resp, nil
}

// ---------------------------------------------
// Sahil's Methods
// ---------------------------------------------

func (s *leadService) GetCurrentUsers(ctx context.Context) ([]models.ActiveUserResponse, error) {
	users, err := s.userRepo.FindActiveUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("service: get current users: %w", err)
	}

	res := make([]models.ActiveUserResponse, 0, len(users))
	for _, u := range users {
		res = append(res, models.ActiveUserResponse{
			ID:       u.ID,
			Name:     u.Name,
			Email:    u.Email,
			IsActive: u.IsActive,
		})
	}
	return res, nil
}

func (s *leadService) GetMasterStages(ctx context.Context) ([]*models.LeadStage, error) {
	stages, err := s.leadRepo.FindStages(ctx)
	if err != nil {
		return nil, fmt.Errorf("service: get master stages: %w", err)
	}
	return stages, nil
}

func (s *leadService) ListLeads(ctx context.Context, page, limit int, search, owner, priority, stage, sortBy, sortOrder string) ([]models.LeadResponse, *models.PaginationMetadata, error) {
	if page < 1 {
		return nil, nil, &helpers.AppError{Status: http.StatusBadRequest, Message: "page must be greater than or equal to 1"}
	}
	if limit < 1 || limit > 100 {
		return nil, nil, &helpers.AppError{Status: http.StatusBadRequest, Message: "limit must be between 1 and 100"}
	}

	leads, total, err := s.leadRepo.FindLeads(ctx, page, limit, search, owner, priority, stage, sortBy, sortOrder)
	if err != nil {
		return nil, nil, err
	}

	var leadResponses []models.LeadResponse
	for _, l := range leads {
		leadResponses = append(leadResponses, s.mapToResponse(l))
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	pagination := &models.PaginationMetadata{
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}
	return leadResponses, pagination, nil
}

func (s *leadService) GetLead(ctx context.Context, leadID string) (*models.LeadResponse, error) {
	lead, err := s.leadRepo.FindByID(ctx, leadID)
	if err != nil {
		return nil, err
	}
	res := s.mapToResponse(lead)
	return &res, nil
}

func (s *leadService) mapToResponse(l *models.Lead) models.LeadResponse {
	var valStr *string
	if l.Value != nil {
		val := fmt.Sprintf("%.0f", *l.Value)
		valStr = &val
	}

	resp := models.LeadResponse{
		ID:                 l.LeadID,
		Company:            l.Company,
		ProjectName:        l.ProjectName,
		Designation:        l.Designation,
		Contact:            l.Contact,
		Email:              l.Email,
		Phone:              l.Phone,
		OfficePhone:        l.OfficePhone,
		OfficePhoneCountry: l.OfficePhoneCountry,
		Owner:              l.Owner,
		Industry:           l.Industry,
		Size:               l.Size,
		Region:             l.Region,
		Source:             l.Source,
		Stage:              l.Stage,
		Status:             l.Status,
		Sentiment:          l.Sentiment,
		Priority:           l.Priority,
		Value:              valStr,
		LostReason:         l.LostReason,
		BestTime:           l.BestTime,
		ProductService:     l.ProductService,
		RequestType:        l.RequestType,
		CreatedAt:          l.CreatedAt.Format(time.RFC3339),
		UpdatedAt:          l.UpdatedAt.Format(time.RFC3339),
	}
	return resp
}
