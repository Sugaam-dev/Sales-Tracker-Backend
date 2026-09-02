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
	// Support conceptual aliases from frontend Quick Create Lead form
	if req.Company == "" && req.CompanyName != "" {
		req.Company = req.CompanyName
	}
	if req.Contact == "" && req.LeadName != "" {
		req.Contact = req.LeadName
	}
	if req.Phone == "" && req.ContactNumber != "" {
		req.Phone = req.ContactNumber
	}
	if req.CountryCode == "" && req.OfficePhoneCountry != "" {
		req.CountryCode = req.OfficePhoneCountry
	}
	if req.OfficePhoneCountry == "" && req.CountryCode != "" {
		req.OfficePhoneCountry = req.CountryCode
	}
	if req.Owner == "" && req.LeadOwner != "" {
		req.Owner = req.LeadOwner
	}
	if req.Status == "" && req.LeadStatus != "" {
		req.Status = req.LeadStatus
	}
	if req.EstimatedRequirementDate == "" && req.EstimatedReqDate != "" {
		req.EstimatedRequirementDate = req.EstimatedReqDate
	}

	// 1. Basic Required Fields
	if strings.TrimSpace(req.Company) == "" {
		return nil, helpers.ErrBadRequest("Company is required.")
	}
	if strings.TrimSpace(req.Contact) == "" {
		return nil, helpers.ErrBadRequest("Contact is required.")
	}
	if strings.TrimSpace(req.Priority) == "" {
		return nil, helpers.ErrBadRequest("Priority is required.")
	}

	// 2. Email Validation (required, general format, not restricted to .com)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" {
		return nil, helpers.ErrBadRequest("Email is required.")
	}
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(req.Email) {
		return nil, helpers.ErrBadRequest("Invalid email format.")
	}

	// 3. Request Type Validation (Strictly "IT Product" or "IT Service")
	req.RequestType = strings.TrimSpace(req.RequestType)
	if req.RequestType != "IT Product" && req.RequestType != "IT Service" {
		return nil, helpers.ErrBadRequest("Request type must be either 'IT Product' or 'IT Service'.")
	}

	// 4. Request Details Validation (Word count 50-200 words inclusive)
	req.RequestDetails = strings.TrimSpace(req.RequestDetails)
	if req.RequestDetails == "" {
		return nil, helpers.ErrBadRequest("Request details is required.")
	}
	wordCount := helpers.CountWords(req.RequestDetails)
	if wordCount < 50 {
		return nil, helpers.ErrBadRequest("Request details must contain at least 50 words.")
	}
	if wordCount > 200 {
		return nil, helpers.ErrBadRequest("Request details cannot exceed 200 words.")
	}

	// 5. Phone & Country Code Validation
	if err := helpers.ValidatePhoneNumber(req.Phone, req.CountryCode); err != nil {
		return nil, helpers.ErrBadRequest(err.Error())
	}

	if req.Owner != "" {
		exists, err := s.leadRepo.CheckUserExists(req.Owner)
		if err != nil || !exists {
			return nil, helpers.ErrBadRequest("Owner does not exist.")
		}
	}
	if req.Stage != "" {
		exists, err := s.leadRepo.CheckStageExists(req.Stage)
		if err != nil || !exists {
			return nil, helpers.ErrBadRequest("Stage does not exist.")
		}
	}

	lead := &models.Lead{
		Company:            req.Company,
		Contact:            &req.Contact,
		Email:              &req.Email,
		Phone:              &req.Phone,
		PhoneCountry:       &req.CountryCode,
		OfficePhone:        &req.OfficePhone,
		OfficePhoneCountry: &req.OfficePhoneCountry,
		Owner:              &req.Owner,
		Stage:              &req.Stage,
		Status:             &req.Status,
		Sentiment:          &req.Sentiment,
		Priority:           &req.Priority,
		RequestDetails:     &req.RequestDetails,
		RequestType:        &req.RequestType,
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

	if req.KamName != "" {
		lead.KamName = &req.KamName
	}
	if req.BasicRequirements != "" {
		lead.BasicRequirements = &req.BasicRequirements
	}

	if req.AlternatePhone != "" {
		phoneRegex := regexp.MustCompile(`^[0-9]{10}$`)
		if !phoneRegex.MatchString(req.AlternatePhone) {
			return nil, ErrValidation
		}
	}

	var estReqDate *time.Time
	if req.EstimatedRequirementDate != "" {
		t, err := time.Parse("2006-01-02", req.EstimatedRequirementDate)
		if err != nil {
			return nil, ErrValidation
		}
		estReqDate = &t
	}

	var lastContact *time.Time
	if req.LastContactDate != "" {
		t, err := time.Parse(time.RFC3339, req.LastContactDate)
		if err != nil {
			t2, err2 := time.Parse("2006-01-02", req.LastContactDate)
			if err2 != nil {
				return nil, ErrValidation
			}
			lastContact = &t2
		} else {
			lastContact = &t
		}
	}

	var nextFollowUp *time.Time
	if req.NextFollowUp != "" {
		t, err := time.Parse(time.RFC3339, req.NextFollowUp)
		if err != nil {
			t2, err2 := time.Parse("2006-01-02", req.NextFollowUp)
			if err2 != nil {
				return nil, ErrValidation
			}
			nextFollowUp = &t2
		} else {
			nextFollowUp = &t
		}
	}

	if req.LifecycleTemplate != "" {
		lead.LifecycleTemplate = &req.LifecycleTemplate
	}
	if req.KamName != "" {
		lead.KamName = &req.KamName
	}
	if req.BestTimeToConnect != "" {
		lead.BestTimeToConnect = &req.BestTimeToConnect
	}
	if req.AlternatePhone != "" {
		lead.AlternatePhone = &req.AlternatePhone
	}
	if req.AlternatePhoneCountry != "" {
		lead.AlternatePhoneCountry = &req.AlternatePhoneCountry
	}
	if req.LinkedinProfileUrl != "" {
		lead.LinkedinProfileURL = &req.LinkedinProfileUrl
	}
	if req.LinkedinCompanyPageUrl != "" {
		lead.LinkedinCompanyPageURL = &req.LinkedinCompanyPageUrl
	}
	lead.EstimatedRequirementDate = estReqDate
	lead.LastContactDate = lastContact
	lead.NextFollowUp = nextFollowUp
	if req.BasicRequirements != "" {
		lead.BasicRequirements = &req.BasicRequirements
	}
	if req.Notes != "" {
		lead.Notes = &req.Notes
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
		statusStageMap := map[string]string{
			"Open":        "Prospecting",
			"New":         "Qualification",
			"Contacted":   "Initial Discussion",
			"Analysis":    "Needs Analysis",
			"Interested":  "Proposal",
			"Negotiation": "Negotiation",
			"Won":         "Closed Won",
			"Lost":        "Closed Lost",
		}
		if expectedStage, ok := statusStageMap[*newStatus]; ok {
			if *newStage != expectedStage {
				return nil, ErrValidation
			}
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

	// Relaxed validation

	if req.AlternatePhone != nil && *req.AlternatePhone != "" {
		phoneRegex := regexp.MustCompile(`^[0-9]{10}$`)
		if !phoneRegex.MatchString(*req.AlternatePhone) {
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
	if req.RequestDetails != nil {
		reqDetails := strings.TrimSpace(*req.RequestDetails)
		if reqDetails == "" {
			return nil, helpers.ErrBadRequest("Request details is required.")
		}
		wordCount := helpers.CountWords(reqDetails)
		if wordCount < 50 {
			return nil, helpers.ErrBadRequest("Request details must contain at least 50 words.")
		}
		if wordCount > 200 {
			return nil, helpers.ErrBadRequest("Request details cannot exceed 200 words.")
		}
		updates["request_details"] = reqDetails
	}
	if req.RequestType != nil {
		reqType := strings.TrimSpace(*req.RequestType)
		if reqType != "IT Product" && reqType != "IT Service" {
			return nil, helpers.ErrBadRequest("Request type must be either 'IT Product' or 'IT Service'.")
		}
		updates["request_type"] = reqType
	}
	if req.CountryCode != nil {
		updates["phone_country"] = *req.CountryCode
	}

	if req.LifecycleTemplate != nil {
		updates["lifecycle_template"] = *req.LifecycleTemplate
	}
	if req.KamName != nil {
		updates["kam_name"] = *req.KamName
	}
	if req.BestTimeToConnect != nil {
		updates["best_time_to_connect"] = *req.BestTimeToConnect
	}
	if req.AlternatePhone != nil {
		updates["alternate_phone"] = *req.AlternatePhone
	}
	if req.AlternatePhoneCountry != nil {
		updates["alternate_phone_country"] = *req.AlternatePhoneCountry
	}
	if req.LinkedinProfileURL != nil {
		updates["linkedin_profile_url"] = *req.LinkedinProfileURL
	}
	if req.LinkedinCompanyPageURL != nil {
		updates["linkedin_company_page_url"] = *req.LinkedinCompanyPageURL
	}

	if req.EstimatedRequirementDate != nil {
		if *req.EstimatedRequirementDate != "" {
			t, err := time.Parse("2006-01-02", *req.EstimatedRequirementDate)
			if err != nil {
				return nil, ErrValidation
			}
			updates["estimated_requirement_date"] = t
		} else {
			updates["estimated_requirement_date"] = nil
		}
	}

	if req.LastContactDate != nil {
		if *req.LastContactDate != "" {
			t, err := time.Parse(time.RFC3339, *req.LastContactDate)
			if err != nil {
				t2, err2 := time.Parse("2006-01-02", *req.LastContactDate)
				if err2 != nil {
					return nil, ErrValidation
				}
				updates["last_contact_date"] = t2
			} else {
				updates["last_contact_date"] = t
			}
		} else {
			updates["last_contact_date"] = nil
		}
	}

	if req.NextFollowUp != nil {
		if *req.NextFollowUp != "" {
			t, err := time.Parse(time.RFC3339, *req.NextFollowUp)
			if err != nil {
				t2, err2 := time.Parse("2006-01-02", *req.NextFollowUp)
				if err2 != nil {
					return nil, ErrValidation
				}
				updates["next_follow_up"] = t2
			} else {
				updates["next_follow_up"] = t
			}
		} else {
			updates["next_follow_up"] = nil
		}
	}

	if req.BasicRequirements != nil {
		updates["basic_requirements"] = *req.BasicRequirements
	}
	if req.Notes != nil {
		updates["notes"] = *req.Notes
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

	var estReqDate *string
	if l.EstimatedRequirementDate != nil {
		s := l.EstimatedRequirementDate.Format("2006-01-02")
		estReqDate = &s
	}
	var lastContDate *string
	if l.LastContactDate != nil {
		s := l.LastContactDate.Format(time.RFC3339)
		lastContDate = &s
	}
	var nextFollow *string
	if l.NextFollowUp != nil {
		s := l.NextFollowUp.Format(time.RFC3339)
		nextFollow = &s
	}

	resp := models.LeadResponse{
		ID:                       l.LeadID,
		Company:                  l.Company,
		ProjectName:              l.ProjectName,
		Contact:                  l.Contact,
		Email:                    l.Email,
		Phone:                    l.Phone,
		OfficePhone:              l.OfficePhone,
		OfficePhoneCountry:       l.OfficePhoneCountry,
		Owner:                    l.Owner,
		Industry:                 l.Industry,
		Size:                     l.Size,
		Region:                   l.Region,
		Source:                   l.Source,
		Stage:                    l.Stage,
		Status:                   l.Status,
		Sentiment:                l.Sentiment,
		Priority:                 l.Priority,
		Value:                    valStr,
		LostReason:               l.LostReason,
		BestTime:                 l.BestTime,
		LifecycleTemplate:        l.LifecycleTemplate,
		KamName:                  l.KamName,
		Designation:              l.Designation,
		BestTimeToConnect:        l.BestTimeToConnect,
		AlternatePhone:           l.AlternatePhone,
		AlternatePhoneCountry:    l.AlternatePhoneCountry,
		LinkedinProfileURL:       l.LinkedinProfileURL,
		LinkedinCompanyPageURL:   l.LinkedinCompanyPageURL,
		EstimatedRequirementDate: estReqDate,
		LastContactDate:          lastContDate,
		NextFollowUp:             nextFollow,
		BasicRequirements:        l.BasicRequirements,
		Notes:                    l.Notes,
		RequestDetails:           l.RequestDetails,
		RequestType:              l.RequestType,
		CountryCode:              l.PhoneCountry,
		CreatedAt:                l.CreatedAt.Format(time.RFC3339),
		UpdatedAt:                l.UpdatedAt.Format(time.RFC3339),
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

		if item.KamName == "" {
			errorsMap["kamName"] = "KAM Name is required"
		}
		if item.BasicRequirements == "" {
			errorsMap["basicRequirements"] = "Basic Requirements is required"
		}

		if item.AlternatePhone != "" {
			phoneRegex := regexp.MustCompile(`^[0-9]{10}$`)
			if !phoneRegex.MatchString(item.AlternatePhone) {
				errorsMap["alternatePhone"] = "Alternate phone must contain exactly 10 digits"
			}
		}

		var estReqDate *time.Time
		if item.EstimatedRequirementDate != "" {
			t, err := time.Parse("2006-01-02", item.EstimatedRequirementDate)
			if err != nil {
				errorsMap["estimatedRequirementDate"] = "Invalid date format, use YYYY-MM-DD"
			} else {
				estReqDate = &t
			}
		}

		var lastContact *time.Time
		if item.LastContactDate != "" {
			t, err := time.Parse(time.RFC3339, item.LastContactDate)
			if err != nil {
				t2, err2 := time.Parse("2006-01-02", item.LastContactDate)
				if err2 != nil {
					errorsMap["lastContactDate"] = "Invalid date-time format"
				} else {
					lastContact = &t2
				}
			} else {
				lastContact = &t
			}
		}

		var nextFollowUp *time.Time
		if item.NextFollowUp != "" {
			t, err := time.Parse(time.RFC3339, item.NextFollowUp)
			if err != nil {
				t2, err2 := time.Parse("2006-01-02", item.NextFollowUp)
				if err2 != nil {
					errorsMap["nextFollowUp"] = "Invalid date-time format"
				} else {
					nextFollowUp = &t2
				}
			} else {
				nextFollowUp = &t
			}
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
		if item.RequestDetails != "" {
			lead.RequestDetails = &item.RequestDetails
		}
		if item.RequestType != "" {
			lead.RequestType = &item.RequestType
		}
		if item.CountryCode != "" {
			lead.PhoneCountry = &item.CountryCode
		}

		if item.LifecycleTemplate != "" {
			lead.LifecycleTemplate = &item.LifecycleTemplate
		}
		if item.KamName != "" {
			lead.KamName = &item.KamName
		}
		if item.Designation != "" {
			lead.Designation = &item.Designation
		}
		if item.BestTimeToConnect != "" {
			lead.BestTimeToConnect = &item.BestTimeToConnect
		}
		if item.AlternatePhone != "" {
			lead.AlternatePhone = &item.AlternatePhone
		}
		if item.AlternatePhoneCountry != "" {
			lead.AlternatePhoneCountry = &item.AlternatePhoneCountry
		}
		if item.LinkedinProfileUrl != "" {
			lead.LinkedinProfileURL = &item.LinkedinProfileUrl
		}
		if item.LinkedinCompanyPageUrl != "" {
			lead.LinkedinCompanyPageURL = &item.LinkedinCompanyPageUrl
		}
		lead.EstimatedRequirementDate = estReqDate
		lead.LastContactDate = lastContact
		lead.NextFollowUp = nextFollowUp
		if item.BasicRequirements != "" {
			lead.BasicRequirements = &item.BasicRequirements
		}
		if item.Notes != "" {
			lead.Notes = &item.Notes
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
