package services

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"crm-auth-service/helpers"
	"crm-auth-service/models"
	"crm-auth-service/repository"
)

type LeadService struct {
	leadRepo repository.LeadRepository
	userRepo repository.UserRepository
}

func NewLeadService(leadRepo repository.LeadRepository, userRepo repository.UserRepository) *LeadService {
	return &LeadService{
		leadRepo: leadRepo,
		userRepo: userRepo,
	}
}

func (s *LeadService) GetCurrentUsers(ctx context.Context) ([]models.ActiveUserResponse, error) {
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

func (s *LeadService) GetMasterStages(ctx context.Context) ([]*models.LeadStage, error) {
	stages, err := s.leadRepo.FindStages(ctx)
	if err != nil {
		return nil, fmt.Errorf("service: get master stages: %w", err)
	}
	return stages, nil
}

func (s *LeadService) ListLeads(ctx context.Context, page, limit int, search, owner, priority, stage, sortBy, sortOrder string) ([]models.LeadResponse, *models.PaginationMetadata, error) {
	// Validation
	if page < 1 {
		return nil, nil, &helpers.AppError{Status: http.StatusBadRequest, Message: "page must be greater than or equal to 1"}
	}
	if limit < 1 || limit > 100 {
		return nil, nil, &helpers.AppError{Status: http.StatusBadRequest, Message: "limit must be between 1 and 100"}
	}

	if priority != "" {
		validPriorities := map[string]bool{"Low": true, "Normal": true, "High": true, "Urgent": true}
		if !validPriorities[priority] {
			return nil, nil, &helpers.AppError{Status: http.StatusBadRequest, Message: "invalid priority filter value"}
		}
	}

	if sortBy != "" {
		validSortBy := map[string]bool{"id": true, "value": true, "createdAt": true}
		if !validSortBy[sortBy] {
			return nil, nil, &helpers.AppError{Status: http.StatusBadRequest, Message: "invalid sortBy value"}
		}
	}

	if sortOrder != "" {
		validSortOrder := map[string]bool{"asc": true, "desc": true}
		if !validSortOrder[strings.ToLower(sortOrder)] {
			return nil, nil, &helpers.AppError{Status: http.StatusBadRequest, Message: "invalid sortOrder value"}
		}
	}

	// Validate stage if provided
	if stage != "" {
		stages, err := s.leadRepo.FindStages(ctx)
		if err != nil {
			return nil, nil, err
		}
		found := false
		for _, st := range stages {
			if strings.EqualFold(st.Name, stage) {
				found = true
				break
			}
		}
		if !found {
			return nil, nil, &helpers.AppError{Status: http.StatusBadRequest, Message: "invalid stage filter value"}
		}
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

func (s *LeadService) GetLead(ctx context.Context, leadID string) (*models.LeadResponse, error) {
	lead, err := s.leadRepo.FindByID(ctx, leadID)
	if err != nil {
		return nil, err
	}

	res := s.mapToResponse(lead)
	return &res, nil
}

func (s *LeadService) mapToResponse(l *models.Lead) models.LeadResponse {
	var valStr *string
	if l.Value != nil {
		s := fmt.Sprintf("%.0f", *l.Value)
		valStr = &s
	}

	return models.LeadResponse{
		ID:                 l.LeadID,
		Company:            l.Company,
		ProjectName:        l.ProjectName,
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
		CreatedAt:          l.CreatedAt.Format(time.RFC3339),
		UpdatedAt:          l.UpdatedAt.Format(time.RFC3339),
	}
}
