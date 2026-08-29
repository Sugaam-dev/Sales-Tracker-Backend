package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
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

	CreateActivity(leadID string, userRole, userEmail string, req models.CreateActivityRequest) (*models.ActivityResponse, error)
	CompleteActivity(activityID uint, userRole, userEmail string, completed bool) (*models.ActivityResponse, error)
	BulkCreateLeads(userRole, userEmail string, req models.BulkCreateLeadsRequest) (*models.BulkCreateResponse, error)
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

	newStatus := lead.Status
	if req.Status != nil {
		newStatus = req.Status
	}
	newStage := lead.Stage
	if req.Stage != nil {
		newStage = req.Stage
	}

	if req.Status != nil && req.Stage == nil {
		var mappedStage string
		switch *req.Status {
		case "Open":
			mappedStage = "Prospecting"
		case "New":
			mappedStage = "Qualification"
		case "Won":
			mappedStage = "Closed Won"
		case "Lost":
			mappedStage = "Closed Lost"
		case "Contacted":
			mappedStage = "Initial Discussion"
		case "Analysis":
			mappedStage = "Needs Analysis"
		case "Interested":
			mappedStage = "Proposal"
		case "Negotiation":
			mappedStage = "Negotiation"
		}
		if mappedStage != "" {
			newStage = &mappedStage
		}
	}

	if newStatus != nil && newStage != nil {
		statusVal := *newStatus
		stageVal := *newStage

		if statusVal == "Won" && stageVal != "Closed Won" {
			return nil, ErrValidation
		}
		if statusVal == "Lost" && stageVal != "Closed Lost" {
			return nil, ErrValidation
		}
		if statusVal == "Open" && stageVal != "Prospecting" {
			return nil, ErrValidation
		}
		if statusVal == "New" && stageVal != "Qualification" {
			return nil, ErrValidation
		}
		if statusVal == "Contacted" && stageVal != "Initial Discussion" {
			return nil, ErrValidation
		}
		if statusVal == "Analysis" && stageVal != "Needs Analysis" {
			return nil, ErrValidation
		}
		if statusVal == "Interested" && stageVal != "Proposal" {
			return nil, ErrValidation
		}
		if statusVal == "Negotiation" && stageVal != "Negotiation" {
			return nil, ErrValidation
		}
	}

	if newStatus != nil && *newStatus == "Lost" {
		lostReasonVal := ""
		if req.LostReason != nil {
			lostReasonVal = strings.TrimSpace(*req.LostReason)
		} else if lead.LostReason != nil {
			lostReasonVal = strings.TrimSpace(*lead.LostReason)
		}
		if lostReasonVal == "" {
			return nil, ErrValidation
		}
	}

	updates := make(map[string]interface{})
	if req.Owner != nil {
		updates["owner"] = *req.Owner
	}
	if newStage != nil {
		updates["stage"] = *newStage
	}
	if newStatus != nil {
		updates["status"] = *newStatus
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

	if stage != "" {
		exists, err := s.leadRepo.CheckStageExists(stage)
		if err != nil || !exists {
			return nil, nil, &helpers.AppError{Status: http.StatusBadRequest, Message: "invalid stage filter"}
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
		CreatedAt:          l.CreatedAt.Format(time.RFC3339),
		UpdatedAt:          l.UpdatedAt.Format(time.RFC3339),
	}
	return resp
}

func (s *leadService) CreateActivity(leadID string, userRole, userEmail string, req models.CreateActivityRequest) (*models.ActivityResponse, error) {
	lead, err := s.leadRepo.GetLeadByLeadID(leadID)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	userName, err := s.leadRepo.GetUserNameByEmail(userEmail)
	if err != nil && userRole != models.RoleAdmin {
		return nil, ErrUnauthorized
	}

	leadOwner := ""
	if lead.Owner != nil {
		leadOwner = *lead.Owner
	}

	if userRole != models.RoleAdmin && leadOwner != userName {
		return nil, ErrUnauthorized
	}

	var dueDate *time.Time
	if req.DueDate != "" {
		t, err := time.Parse("2006-01-02", req.DueDate)
		if err != nil {
			return nil, ErrValidation
		}
		dueDate = &t
	}

	activity := &models.Activity{
		LeadID:    lead.LeadID,
		Type:      req.Type,
		Desc:      req.Desc,
		Outcome:   req.Outcome,
		DueDate:   dueDate,
		Completed: false,
	}

	err = s.leadRepo.CreateActivity(activity)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	resp := models.ToActivityResponse(*activity)
	return &resp, nil
}

func (s *leadService) CompleteActivity(activityID uint, userRole, userEmail string, completed bool) (*models.ActivityResponse, error) {
	activity, err := s.leadRepo.GetActivityByID(activityID)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	lead, err := s.leadRepo.GetLeadByLeadID(activity.LeadID)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	userName, err := s.leadRepo.GetUserNameByEmail(userEmail)
	if err != nil && userRole != models.RoleAdmin {
		return nil, ErrUnauthorized
	}

	leadOwner := ""
	if lead.Owner != nil {
		leadOwner = *lead.Owner
	}

	if userRole != models.RoleAdmin && leadOwner != userName {
		return nil, ErrUnauthorized
	}

	if activity.Completed == completed {
		resp := models.ToActivityResponse(*activity)
		return &resp, nil
	}

	err = s.leadRepo.UpdateActivityCompleted(activityID, completed)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	activity.Completed = completed
	resp := models.ToActivityResponse(*activity)
	return &resp, nil
}

func (s *leadService) BulkCreateLeads(userRole, userEmail string, req models.BulkCreateLeadsRequest) (*models.BulkCreateResponse, error) {
	_, err := s.leadRepo.GetUserNameByEmail(userEmail)
	if err != nil && userRole != models.RoleAdmin {
		return nil, ErrUnauthorized
	}

	total := len(req.Leads)
	created := make([]models.BulkCreateCreatedResponse, 0)
	failed := make([]models.BulkCreateFailedResponse, 0)

	batchEmails := make(map[string]bool)
	batchCompanies := make(map[string]bool)

	for i, item := range req.Leads {
		errorsMap := make(map[string]string)

		companyTrimmed := strings.TrimSpace(item.Company)
		if companyTrimmed == "" {
			errorsMap["company"] = "Company is required"
		}
		contactTrimmed := strings.TrimSpace(item.Contact)
		if contactTrimmed == "" {
			errorsMap["contact"] = "Contact is required"
		}
		emailTrimmed := strings.ToLower(strings.TrimSpace(item.Email))
		if emailTrimmed == "" {
			errorsMap["email"] = "Email is required"
		} else {
			emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
			if !emailRegex.MatchString(emailTrimmed) {
				errorsMap["email"] = "Invalid email format"
			}
		}

		if len(item.Phone) != 10 {
			errorsMap["phone"] = "Phone must contain exactly 10 digits"
		} else {
			numericRegex := regexp.MustCompile(`^[0-9]+$`)
			if !numericRegex.MatchString(item.Phone) {
				errorsMap["phone"] = "Phone must contain exactly 10 digits"
			}
		}

		if len(item.OfficePhone) != 10 {
			errorsMap["officePhone"] = "Office phone must contain exactly 10 digits"
		} else {
			numericRegex := regexp.MustCompile(`^[0-9]+$`)
			if !numericRegex.MatchString(item.OfficePhone) {
				errorsMap["officePhone"] = "Office phone must contain exactly 10 digits"
			}
		}

		if strings.TrimSpace(item.Owner) == "" {
			errorsMap["owner"] = "Owner is required"
		} else {
			exists, err := s.leadRepo.CheckUserExists(item.Owner)
			if err != nil || !exists {
				errorsMap["owner"] = "Owner does not exist"
			} else {
				active, err := s.leadRepo.CheckUserActive(item.Owner)
				if err != nil || !active {
					errorsMap["owner"] = "Owner must be active"
				}
			}
		}

		var normalizedStage string
		if strings.TrimSpace(item.Stage) == "" {
			errorsMap["stage"] = "Stage is required"
		} else {
			exists, dbStageName, err := s.leadRepo.CheckStageExistsCaseInsensitive(item.Stage)
			if err != nil || !exists {
				errorsMap["stage"] = "Stage does not exist"
			} else {
				normalizedStage = dbStageName
			}
		}

		validStatuses := map[string]bool{"Open": true, "In Progress": true, "Won": true, "Lost": true}
		if !validStatuses[item.Status] {
			errorsMap["status"] = "Status must be Open, In Progress, Won, or Lost"
		}

		validSentiments := map[string]bool{"Positive": true, "Neutral": true, "Negative": true}
		if !validSentiments[item.Sentiment] {
			errorsMap["sentiment"] = "Sentiment must be Positive, Neutral, or Negative"
		}

		validPriorities := map[string]bool{"Low": true, "Normal": true, "High": true, "Urgent": true}
		if !validPriorities[item.Priority] {
			errorsMap["priority"] = "Priority must be Low, Normal, High, or Urgent"
		}

		if emailTrimmed != "" && errorsMap["email"] == "" {
			if batchEmails[emailTrimmed] {
				errorsMap["email"] = "duplicate email within request"
			}
		}
		if companyTrimmed != "" && errorsMap["company"] == "" {
			compLower := strings.ToLower(companyTrimmed)
			if batchCompanies[compLower] {
				errorsMap["company"] = "duplicate company within request"
			}
		}

		if emailTrimmed != "" && errorsMap["email"] == "" {
			exists, err := s.leadRepo.CheckEmailExists(emailTrimmed)
			if err == nil && exists {
				errorsMap["email"] = "duplicate email"
			}
		}
		if companyTrimmed != "" && errorsMap["company"] == "" {
			exists, err := s.leadRepo.CheckCompanyExists(companyTrimmed)
			if err == nil && exists {
				errorsMap["company"] = "duplicate company"
			}
		}

		if len(errorsMap) > 0 {
			failed = append(failed, models.BulkCreateFailedResponse{
				Index:   i,
				Company: item.Company,
				Errors:  errorsMap,
			})
			continue
		}

		batchEmails[emailTrimmed] = true
		batchCompanies[strings.ToLower(companyTrimmed)] = true

		lead := &models.Lead{
			Company:            companyTrimmed,
			Contact:            &item.Contact,
			Email:              &emailTrimmed,
			Phone:              &item.Phone,
			OfficePhone:        &item.OfficePhone,
			OfficePhoneCountry: &item.OfficePhoneCountry,
			Owner:              &item.Owner,
			Stage:              &normalizedStage,
			Status:             &item.Status,
			Sentiment:          &item.Sentiment,
			Priority:           &item.Priority,
		}
		if item.ProjectName != "" {
			lead.ProjectName = &item.ProjectName
		}
		if item.Industry != "" {
			lead.Industry = &item.Industry
		}
		if item.Size != "" {
			lead.Size = &item.Size
		}
		if item.Region != "" {
			lead.Region = &item.Region
		}
		if item.Source != "" {
			lead.Source = &item.Source
		}

		err = s.leadRepo.CreateLead(lead)
		if err != nil {
			failed = append(failed, models.BulkCreateFailedResponse{
				Index:   i,
				Company: item.Company,
				Errors:  map[string]string{"database": err.Error()},
			})
		} else {
			created = append(created, models.BulkCreateCreatedResponse{
				LeadID:  lead.LeadID,
				Company: lead.Company,
			})
		}
	}

	return &models.BulkCreateResponse{
		Success: true,
		Summary: models.BulkCreateSummary{
			Total:   total,
			Created: len(created),
			Failed:  len(failed),
		},
		Created: created,
		Failed:  failed,
	}, nil
}
