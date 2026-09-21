package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"crm-auth-service/helpers"
	"crm-auth-service/middleware"
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
	CreateLead(ctx context.Context, callerID uuid.UUID, callerRole, callerEmail string, req models.CreateLeadRequest) (*models.LeadResponse, error)
	UpdateLead(ctx context.Context, callerID uuid.UUID, callerRole, callerEmail string, leadID string, req models.UpdateLeadRequest) (*models.LeadResponse, error)
	DeleteLead(ctx context.Context, callerID uuid.UUID, callerRole, callerEmail string, leadID string) error
	GetLeadActivities(ctx context.Context, callerID uuid.UUID, callerRole, callerEmail string, leadID string) ([]models.ActivityResponse, error)

	GetCurrentUsers(ctx context.Context, callerID uuid.UUID, callerRole string) ([]models.ActiveUserResponse, error)
	GetMasterStages(ctx context.Context) ([]*models.LeadStage, error)
	ListLeads(ctx context.Context, callerID uuid.UUID, callerRole string, page, limit int, search, owner, priority, stage, sortBy, sortOrder string) ([]models.LeadResponse, *models.PaginationMetadata, error)
	GetLead(ctx context.Context, callerID uuid.UUID, callerRole string, leadID string) (*models.LeadResponse, error)
	GetHeatMap(ctx context.Context) (*models.HeatMapResponse, error)

	CreateActivity(ctx context.Context, callerID uuid.UUID, callerRole, callerEmail string, leadID string, req models.CreateActivityRequest) (*models.ActivityResponse, error)
	CompleteActivity(ctx context.Context, callerID uuid.UUID, callerRole, callerEmail string, activityID uint, completed bool) (*models.ActivityResponse, error)
	BulkCreateLeads(ctx context.Context, callerID uuid.UUID, callerRole, callerEmail string, req models.BulkCreateLeadsRequest) (*models.BulkCreateResponse, error)

	GetActivities(ctx context.Context, callerID uuid.UUID, callerRole string, query models.GetActivitiesQuery) (*models.ActivitiesFeedResponse, error)
	LogActivity(ctx context.Context, userID uuid.UUID, userRole, userEmail string, req models.LogActivityRequest) (*models.ActivityFeedItemResponse, error)
	GetActivitiesSummary(ctx context.Context, callerID uuid.UUID, callerRole string) (*models.ActivitySummaryData, error)
}

type leadService struct {
	leadRepo repository.LeadRepository
	userRepo repository.UserRepository
	emailSvc helpers.EmailService
	log      *slog.Logger
}

func NewLeadService(leadRepo repository.LeadRepository, userRepo repository.UserRepository, emailSvc helpers.EmailService, log *slog.Logger) LeadService {
	return &leadService{
		leadRepo: leadRepo,
		userRepo: userRepo,
		emailSvc: emailSvc,
		log:      log,
	}
}

func safeDerefString(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
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

func (s *leadService) CreateLead(ctx context.Context, callerID uuid.UUID, callerRole, callerEmail string, req models.CreateLeadRequest) (*models.LeadResponse, error) {
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

	scope, err := middleware.ResolveDataScope(ctx, s.userRepo, callerID, callerRole)
	if err != nil {
		return nil, helpers.ErrInternal("failed to resolve user data scope")
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
	if wordCount < 10 || wordCount > 200 {
		return nil, helpers.ErrBadRequest("Request details must be between 10 and 200 words.")
	}

	// 5. Phone & Country Code Validation
	if err := helpers.ValidatePhoneNumber(req.Phone, req.CountryCode); err != nil {
		return nil, helpers.ErrBadRequest(err.Error())
	}
	if req.OfficePhone != "" {
		officeCC := req.OfficePhoneCountry
		if officeCC == "" {
			officeCC = req.CountryCode
		}
		if err := helpers.ValidatePhoneNumber(req.OfficePhone, officeCC); err != nil {
			return nil, helpers.ErrBadRequest(err.Error())
		}
	}
	if req.AlternatePhone != "" {
		altCC := req.AlternatePhoneCountry
		if altCC == "" {
			altCC = req.CountryCode
		}
		if err := helpers.ValidatePhoneNumber(req.AlternatePhone, altCC); err != nil {
			return nil, helpers.ErrBadRequest(err.Error())
		}
	}

	if req.Stage != "" {
		exists, err := s.leadRepo.CheckStageExists(req.Stage)
		if err != nil || !exists {
			return nil, helpers.ErrBadRequest("Stage does not exist.")
		}
	}

	var createdByUUID *uuid.UUID
	if callerID != uuid.Nil {
		createdByUUID = &callerID
	}
	var assignedToUUID *uuid.UUID

	callerUser, _ := s.userRepo.FindByID(ctx, callerID)

	if callerRole == models.RoleSalesExecutive {
		// Sales Executive can only create leads assigned to themselves
		if callerID != uuid.Nil {
			assignedToUUID = &callerID
		}
		if callerUser != nil {
			req.Owner = callerUser.Name
		}
	} else if req.AssignedTo != "" {
		parsedAssigned, err := uuid.Parse(req.AssignedTo)
		if err != nil {
			return nil, helpers.ErrBadRequest("invalid assigned_to user id")
		}
		if !scope.CanAssignLead(parsedAssigned) {
			return nil, helpers.ErrForbidden("cannot assign lead to user outside authorized team")
		}
		targetUser, err := s.userRepo.FindByID(ctx, parsedAssigned)
		if err != nil || targetUser == nil {
			return nil, helpers.ErrBadRequest("assigned user does not exist")
		}
		if parsedAssigned != uuid.Nil {
			assignedToUUID = &parsedAssigned
		}
		req.Owner = targetUser.Name
	} else if req.Owner != "" {
		exists, err := s.leadRepo.CheckUserExists(req.Owner)
		if err != nil || !exists {
			return nil, helpers.ErrBadRequest("Owner does not exist.")
		}
		// Resolve assigned_to UUID from owner name
		users, _ := s.userRepo.FindAllUsers(ctx)
		for _, u := range users {
			if strings.EqualFold(u.Name, req.Owner) {
				if !scope.CanAssignLead(u.ID) {
					return nil, helpers.ErrForbidden("cannot assign lead to owner outside authorized team")
				}
				targetID := u.ID
				if targetID != uuid.Nil {
					assignedToUUID = &targetID
				}
				break
			}
		}
	} else {
		// Default assigned to caller
		if callerID != uuid.Nil {
			assignedToUUID = &callerID
		}
		if callerUser != nil {
			req.Owner = callerUser.Name
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
		CreatedBy:          createdByUUID,
		AssignedTo:         assignedToUUID,
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
		cleanedVal := strings.ReplaceAll(req.Value, ",", "")
		parsedVal, err := strconv.ParseFloat(cleanedVal, 64)
		if err == nil && parsedVal >= 0 {
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

	err = s.leadRepo.CreateLead(lead)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	if s.emailSvc != nil && lead.Email != nil && *lead.Email != "" {
		toEmail := *lead.Email
		leadName := safeDerefString(lead.Contact)
		company := lead.Company
		leadID := lead.LeadID
		ownerName := safeDerefString(lead.Owner)
		emailSvc := s.emailSvc
		logger := s.log
		go func() {
			if err := emailSvc.SendLeadCreated(toEmail, leadName, company, leadID, ownerName); err != nil {
				if logger != nil {
					logger.Error("failed to send lead creation email", "lead_id", leadID, "error", err)
				}
			}
		}()
	}

	resp := s.mapToResponse(lead)
	return &resp, nil
}

func (s *leadService) UpdateLead(ctx context.Context, callerID uuid.UUID, callerRole, callerEmail string, leadID string, req models.UpdateLeadRequest) (*models.LeadResponse, error) {
	lead, err := s.leadRepo.GetLeadByLeadID(leadID)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	scope, err := middleware.ResolveDataScope(ctx, s.userRepo, callerID, callerRole)
	if err != nil {
		return nil, helpers.ErrInternal("failed to resolve user data scope")
	}

	if !scope.CanAccessLead(lead) {
		return nil, ErrUnauthorized
	}

	// If owner reassignment is requested, validate permissions
	if req.Owner != nil && (lead.Owner == nil || !strings.EqualFold(*lead.Owner, *req.Owner)) {
		if callerRole == models.RoleSalesExecutive {
			return nil, helpers.ErrForbidden("sales executives cannot reassign lead ownership")
		}

		users, err := s.userRepo.FindAllUsers(ctx)
		if err != nil {
			return nil, helpers.ErrInternal("failed to lookup users")
		}
		var targetUser *models.User
		for _, u := range users {
			if strings.EqualFold(u.Name, *req.Owner) {
				targetUser = u
				break
			}
		}
		if targetUser == nil {
			return nil, helpers.ErrBadRequest("new owner does not exist")
		}

		if !scope.CanAssignLead(targetUser.ID) {
			return nil, helpers.ErrForbidden("cannot assign lead to user outside authorized team")
		}
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

	// Validate phone fields if updated
	if req.Phone != nil && strings.TrimSpace(*req.Phone) != "" {
		phoneCC := ""
		if req.CountryCode != nil && *req.CountryCode != "" {
			phoneCC = *req.CountryCode
		} else if lead.PhoneCountry != nil {
			phoneCC = *lead.PhoneCountry
		}
		if err := helpers.ValidatePhoneNumber(*req.Phone, phoneCC); err != nil {
			return nil, helpers.ErrBadRequest(err.Error())
		}
	}
	if req.OfficePhone != nil && strings.TrimSpace(*req.OfficePhone) != "" {
		officeCC := ""
		if req.OfficePhoneCountry != nil && *req.OfficePhoneCountry != "" {
			officeCC = *req.OfficePhoneCountry
		} else if lead.OfficePhoneCountry != nil {
			officeCC = *lead.OfficePhoneCountry
		} else if req.CountryCode != nil && *req.CountryCode != "" {
			officeCC = *req.CountryCode
		} else if lead.PhoneCountry != nil {
			officeCC = *lead.PhoneCountry
		}
		if err := helpers.ValidatePhoneNumber(*req.OfficePhone, officeCC); err != nil {
			return nil, helpers.ErrBadRequest(err.Error())
		}
	}
	if req.AlternatePhone != nil && strings.TrimSpace(*req.AlternatePhone) != "" {
		altCC := ""
		if req.AlternatePhoneCountry != nil && *req.AlternatePhoneCountry != "" {
			altCC = *req.AlternatePhoneCountry
		} else if lead.AlternatePhoneCountry != nil {
			altCC = *lead.AlternatePhoneCountry
		} else if req.CountryCode != nil && *req.CountryCode != "" {
			altCC = *req.CountryCode
		} else if lead.PhoneCountry != nil {
			altCC = *lead.PhoneCountry
		}
		if err := helpers.ValidatePhoneNumber(*req.AlternatePhone, altCC); err != nil {
			return nil, helpers.ErrBadRequest(err.Error())
		}
	}

	updates := make(map[string]interface{})
	if req.Owner != nil {
		updates["owner"] = *req.Owner
		// Also update assigned_to if we can find the matching user
		users, _ := s.userRepo.FindAllUsers(ctx)
		for _, u := range users {
			if strings.EqualFold(u.Name, *req.Owner) {
				updates["assigned_to"] = u.ID
				break
			}
		}
	}
	if req.AssignedTo != nil && *req.AssignedTo != "" {
		if callerRole == models.RoleSalesExecutive {
			return nil, helpers.ErrForbidden("sales executives cannot reassign lead ownership")
		}
		parsedAssigned, err := uuid.Parse(*req.AssignedTo)
		if err == nil {
			if !scope.CanAssignLead(parsedAssigned) {
				return nil, helpers.ErrForbidden("cannot assign lead to user outside authorized team")
			}
			targetUser, _ := s.userRepo.FindByID(ctx, parsedAssigned)
			if targetUser != nil {
				updates["assigned_to"] = parsedAssigned
				updates["owner"] = targetUser.Name
			}
		}
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
		cleanedVal := strings.ReplaceAll(*req.Value, ",", "")
		if strings.TrimSpace(cleanedVal) == "" {
			updates["value"] = nil
		} else {
			parsedVal, err := strconv.ParseFloat(cleanedVal, 64)
			if err == nil && parsedVal >= 0 {
				updates["value"] = parsedVal
			}
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
		if wordCount < 10 || wordCount > 200 {
			return nil, helpers.ErrBadRequest("Request details must be between 10 and 200 words.")
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
		if strings.TrimSpace(*req.KamName) == "" {
			return nil, ErrValidation
		}
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

	if s.emailSvc != nil {
		emailSvc := s.emailSvc
		logger := s.log
		userRepo := s.userRepo

		// Lead assignment / reassignment trigger
		if updatedLead.AssignedTo != nil && (lead.AssignedTo == nil || *lead.AssignedTo != *updatedLead.AssignedTo || (lead.Owner == nil || (updatedLead.Owner != nil && *lead.Owner != *updatedLead.Owner))) {
			assignedID := *updatedLead.AssignedTo
			leadIDStr := updatedLead.LeadID
			companyStr := updatedLead.Company
			go func() {
				targetUser, err := userRepo.FindByID(context.Background(), assignedID)
				if err == nil && targetUser != nil && targetUser.Email != "" {
					if err := emailSvc.SendLeadAssigned(targetUser.Email, targetUser.Name, leadIDStr, companyStr, callerEmail); err != nil {
						if logger != nil {
							logger.Error("failed to send lead assignment email", "lead_id", leadIDStr, "error", err)
						}
					}
				}
			}()
		}

		// Status or Stage change trigger
		oldStatus := safeDerefString(lead.Status)
		newStatus := safeDerefString(updatedLead.Status)
		oldStage := safeDerefString(lead.Stage)
		newStage := safeDerefString(updatedLead.Stage)

		if (oldStatus != "" && newStatus != "" && oldStatus != newStatus) || (oldStage != "" && newStage != "" && oldStage != newStage) {
			leadIDStr := updatedLead.LeadID
			companyStr := updatedLead.Company
			contactEmail := safeDerefString(updatedLead.Email)
			contactName := safeDerefString(updatedLead.Contact)
			assignedID := updatedLead.AssignedTo

			go func() {
				if contactEmail != "" {
					if err := emailSvc.SendLeadStatusChanged(contactEmail, contactName, leadIDStr, companyStr, oldStatus, newStatus, oldStage, newStage); err != nil {
						if logger != nil {
							logger.Error("failed to send lead status change email to contact", "lead_id", leadIDStr, "error", err)
						}
					}
				}
				if assignedID != nil {
					targetUser, err := userRepo.FindByID(context.Background(), *assignedID)
					if err == nil && targetUser != nil && targetUser.Email != "" && targetUser.Email != contactEmail {
						if err := emailSvc.SendLeadStatusChanged(targetUser.Email, targetUser.Name, leadIDStr, companyStr, oldStatus, newStatus, oldStage, newStage); err != nil {
							if logger != nil {
								logger.Error("failed to send lead status change email to assigned user", "lead_id", leadIDStr, "error", err)
							}
						}
					}
				}
			}()
		}
	}

	resp := s.mapToResponse(updatedLead)
	return &resp, nil
}

func (s *leadService) DeleteLead(ctx context.Context, callerID uuid.UUID, callerRole, callerEmail string, leadID string) error {
	if callerRole != models.RoleAdmin && callerRole != models.RoleSalesManager {
		return ErrUnauthorized
	}

	lead, err := s.leadRepo.GetLeadByLeadID(leadID)
	if err != nil {
		return s.handleDBError(err)
	}

	scope, err := middleware.ResolveDataScope(ctx, s.userRepo, callerID, callerRole)
	if err != nil {
		return helpers.ErrInternal("failed to resolve user data scope")
	}

	if !scope.CanAccessLead(lead) {
		return ErrUnauthorized
	}

	err = s.leadRepo.DeleteLead(leadID)
	return s.handleDBError(err)
}

func (s *leadService) GetLeadActivities(ctx context.Context, callerID uuid.UUID, callerRole, callerEmail string, leadID string) ([]models.ActivityResponse, error) {
	lead, err := s.leadRepo.GetLeadActivities(leadID)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	scope, err := middleware.ResolveDataScope(ctx, s.userRepo, callerID, callerRole)
	if err != nil {
		return nil, helpers.ErrInternal("failed to resolve user data scope")
	}

	if !scope.CanAccessLead(lead) {
		return nil, ErrUnauthorized
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

func (s *leadService) GetCurrentUsers(ctx context.Context, callerID uuid.UUID, callerRole string) ([]models.ActiveUserResponse, error) {
	users, err := s.userRepo.FindUsersByRoleScope(ctx, callerID, callerRole)
	if err != nil {
		return nil, fmt.Errorf("service: get current users: %w", err)
	}

	res := make([]models.ActiveUserResponse, 0, len(users))
	for _, u := range users {
		res = append(res, models.ActiveUserResponse{
			ID:       u.ID,
			Name:     u.Name,
			Email:    u.Email,
			Role:     u.Role,
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

func (s *leadService) ListLeads(ctx context.Context, callerID uuid.UUID, callerRole string, page, limit int, search, owner, priority, stage, sortBy, sortOrder string) ([]models.LeadResponse, *models.PaginationMetadata, error) {
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

	scope, err := middleware.ResolveDataScope(ctx, s.userRepo, callerID, callerRole)
	if err != nil {
		return nil, nil, fmt.Errorf("service: resolve scope: %w", err)
	}

	leads, total, err := s.leadRepo.FindLeads(ctx, scope, page, limit, search, owner, priority, stage, sortBy, sortOrder)
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

func (s *leadService) GetLead(ctx context.Context, callerID uuid.UUID, callerRole string, leadID string) (*models.LeadResponse, error) {
	lead, err := s.leadRepo.FindByID(ctx, leadID)
	if err != nil {
		return nil, err
	}

	scope, err := middleware.ResolveDataScope(ctx, s.userRepo, callerID, callerRole)
	if err != nil {
		return nil, fmt.Errorf("service: resolve scope: %w", err)
	}

	if !scope.CanAccessLead(lead) {
		return nil, ErrUnauthorized
	}

	res := s.mapToResponse(lead)
	return &res, nil
}

// StageProbabilityMap defines the stage-to-probability mapping (consistent with analytics repository)
var StageProbabilityMap = map[string]float64{
	"Prospecting":        0.10,
	"Qualification":      0.20,
	"Initial Discussion": 0.30,
	"Needs Analysis":     0.35,
	"Proposal":           0.50,
	"Negotiation":        0.80,
	"Closed Won":         1.00,
	"Closed Lost":        0.00,
}

// CalculateStageProbability returns the probability based on stage and status.
func CalculateStageProbability(stage, status string) float64 {
	if status == "Lost" || stage == "Closed Lost" {
		return 0.00
	}
	if status == "Won" || stage == "Closed Won" {
		return 1.00
	}
	if prob, ok := StageProbabilityMap[stage]; ok {
		return prob
	}
	switch status {
	case "Open":
		return 0.10
	case "New":
		return 0.20
	case "Contacted":
		return 0.30
	case "Analysis":
		return 0.35
	case "Interested":
		return 0.50
	case "Negotiation":
		return 0.80
	default:
		return 0.10
	}
}

func (s *leadService) mapToResponse(l *models.Lead) models.LeadResponse {
	var valStr *string
	var expectedValStr *string

	if l.Value != nil && *l.Value > 0 {
		effectiveDealVal := *l.Value
		val := fmt.Sprintf("%.0f", effectiveDealVal)
		valStr = &val

		stageStr := safeDerefString(l.Stage)
		statusStr := safeDerefString(l.Status)
		prob := CalculateStageProbability(stageStr, statusStr)
		expVal := effectiveDealVal * prob
		expStr := fmt.Sprintf("%.0f", expVal)
		expectedValStr = &expStr
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
		CreatedBy:                helpers.UUIDPtrToStringPtr(l.CreatedBy),
		AssignedTo:               helpers.UUIDPtrToStringPtr(l.AssignedTo),
		Industry:                 l.Industry,
		Size:                     l.Size,
		Region:                   l.Region,
		Source:                   l.Source,
		Stage:                    l.Stage,
		Status:                   l.Status,
		Sentiment:                l.Sentiment,
		Priority:                 l.Priority,
		Value:                    valStr,
		ExpectedValue:            expectedValStr,
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

func (s *leadService) CreateActivity(ctx context.Context, callerID uuid.UUID, callerRole, callerEmail string, leadID string, req models.CreateActivityRequest) (*models.ActivityResponse, error) {
	lead, err := s.leadRepo.GetLeadByLeadID(leadID)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	scope, err := middleware.ResolveDataScope(ctx, s.userRepo, callerID, callerRole)
	if err != nil {
		return nil, helpers.ErrInternal("failed to resolve user data scope")
	}

	if !scope.CanAccessLead(lead) {
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
		Rep:       &callerID,
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

	if s.emailSvc != nil {
		emailSvc := s.emailSvc
		logger := s.log
		userRepo := s.userRepo
		actType := req.Type
		actDesc := req.Desc
		actDueDate := req.DueDate
		leadIDStr := lead.LeadID
		companyStr := lead.Company
		assignedID := lead.AssignedTo
		contactEmail := safeDerefString(lead.Email)
		contactName := safeDerefString(lead.Contact)

		go func() {
			recipients := make(map[string]string)
			if assignedID != nil {
				targetUser, err := userRepo.FindByID(context.Background(), *assignedID)
				if err == nil && targetUser != nil && strings.TrimSpace(targetUser.Email) != "" {
					recipients[strings.ToLower(strings.TrimSpace(targetUser.Email))] = targetUser.Name
				}
			}
			if strings.TrimSpace(contactEmail) != "" {
				recipients[strings.ToLower(strings.TrimSpace(contactEmail))] = contactName
			}

			for email, name := range recipients {
				if err := emailSvc.SendActivityNotification(email, name, actType, actDesc, actDueDate, leadIDStr, companyStr); err != nil {
					if logger != nil {
						logger.Error("failed to send activity notification", "lead_id", leadIDStr, "to", email, "error", err)
					}
				}
			}
		}()
	}

	resp := models.ToActivityResponse(*activity)
	return &resp, nil
}

func (s *leadService) CompleteActivity(ctx context.Context, callerID uuid.UUID, callerRole, callerEmail string, activityID uint, completed bool) (*models.ActivityResponse, error) {
	activity, err := s.leadRepo.GetActivityByID(activityID)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	lead, err := s.leadRepo.GetLeadByLeadID(activity.LeadID)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	scope, err := middleware.ResolveDataScope(ctx, s.userRepo, callerID, callerRole)
	if err != nil {
		return nil, helpers.ErrInternal("failed to resolve user data scope")
	}

	if !scope.CanAccessLead(lead) {
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

func (s *leadService) BulkCreateLeads(ctx context.Context, callerID uuid.UUID, callerRole, callerEmail string, req models.BulkCreateLeadsRequest) (*models.BulkCreateResponse, error) {
	scope, err := middleware.ResolveDataScope(ctx, s.userRepo, callerID, callerRole)
	if err != nil {
		return nil, helpers.ErrInternal("failed to resolve user data scope")
	}

	callerUser, _ := s.userRepo.FindByID(ctx, callerID)

	total := len(req.Leads)
	created := make([]models.BulkCreateCreatedResponse, 0)
	failed := make([]models.BulkCreateFailedResponse, 0)

	batchEmails := make(map[string]bool)
	batchCompanies := make(map[string]bool)

	// Pre-fetch all users for owner resolution
	allUsers, _ := s.userRepo.FindAllUsers(ctx)
	userMapByName := make(map[string]*models.User)
	for _, u := range allUsers {
		userMapByName[strings.ToLower(u.Name)] = u
	}

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

		// Phone & Office Phone validation with international support
		phoneCC := item.CountryCode
		if phoneCC == "" {
			phoneCC = item.OfficePhoneCountry
		}
		if phoneCC == "" {
			phoneCC = "IN|+91"
		}
		if err := helpers.ValidatePhoneNumber(item.Phone, phoneCC); err != nil {
			errorsMap["phone"] = err.Error()
		}

		if item.OfficePhone != "" {
			officeCC := item.OfficePhoneCountry
			if officeCC == "" {
				officeCC = phoneCC
			}
			if err := helpers.ValidatePhoneNumber(item.OfficePhone, officeCC); err != nil {
				errorsMap["officePhone"] = err.Error()
			}
		}

		// Owner & AssignedTo resolution based on caller role
		var leadAssignedTo *uuid.UUID
		effectiveOwner := item.Owner

		if callerRole == models.RoleSalesExecutive {
			leadAssignedTo = &callerID
			if callerUser != nil {
				effectiveOwner = callerUser.Name
			}
		} else {
			if strings.TrimSpace(item.Owner) == "" {
				errorsMap["owner"] = "Owner is required"
			} else {
				targetUser, exists := userMapByName[strings.ToLower(strings.TrimSpace(item.Owner))]
				if !exists {
					errorsMap["owner"] = "Owner does not exist"
				} else if !targetUser.IsActive {
					errorsMap["owner"] = "Owner must be active"
				} else if !scope.CanAssignLead(targetUser.ID) {
					errorsMap["owner"] = "Cannot assign lead to user outside authorized team"
				} else {
					targetID := targetUser.ID
					leadAssignedTo = &targetID
					effectiveOwner = targetUser.Name
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
		if item.BasicRequirements == "" && item.RequestDetails != "" {
			item.BasicRequirements = item.RequestDetails
		}
		if item.BasicRequirements == "" {
			errorsMap["basicRequirements"] = "Basic Requirements is required"
		}

		if item.RequestType != "IT Product" && item.RequestType != "IT Service" {
			errorsMap["requestType"] = "Request Type must be either 'IT Product' or 'IT Service'"
		}

		if strings.TrimSpace(item.RequestDetails) == "" {
			errorsMap["requestDetails"] = "Request Details is required"
		}

		if item.AlternatePhone != "" {
			altCC := item.AlternatePhoneCountry
			if altCC == "" {
				altCC = phoneCC
			}
			if err := helpers.ValidatePhoneNumber(item.AlternatePhone, altCC); err != nil {
				errorsMap["alternatePhone"] = err.Error()
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

		var createdByUUID *uuid.UUID
		if callerID != uuid.Nil {
			createdByUUID = &callerID
		}
		lead := &models.Lead{
			Company:            companyTrimmed,
			Contact:            &item.Contact,
			Email:              &emailTrimmed,
			Phone:              &item.Phone,
			OfficePhone:        &item.OfficePhone,
			OfficePhoneCountry: &item.OfficePhoneCountry,
			Owner:              &effectiveOwner,
			CreatedBy:          createdByUUID,
			AssignedTo:         leadAssignedTo,
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

func (s *leadService) GetHeatMap(ctx context.Context) (*models.HeatMapResponse, error) {
	rows, err := s.leadRepo.GetHeatMapAggregation(ctx)
	if err != nil {
		return nil, err
	}

	stages, err := s.leadRepo.FindStages(ctx)
	if err != nil {
		return nil, err
	}

	var stageNames []string
	for _, st := range stages {
		stageNames = append(stageNames, st.Name)
	}

	repMap := make(map[string]map[string]models.HeatMapCell)
	stageTotals := make(map[string]models.HeatMapCell)
	grandTotal := models.HeatMapCell{Stage: "Grand Total"}

	for _, st := range stageNames {
		stageTotals[st] = models.HeatMapCell{Stage: st, Value: 0, Leads: 0}
	}

	for _, row := range rows {
		if _, ok := repMap[row.Owner]; !ok {
			repMap[row.Owner] = make(map[string]models.HeatMapCell)
			for _, st := range stageNames {
				repMap[row.Owner][st] = models.HeatMapCell{Stage: st, Value: 0, Leads: 0}
			}
		}

		if _, ok := repMap[row.Owner][row.Stage]; ok {
			cell := repMap[row.Owner][row.Stage]
			cell.Value += row.Value
			cell.Leads += row.Count
			repMap[row.Owner][row.Stage] = cell

			stTotal := stageTotals[row.Stage]
			stTotal.Value += row.Value
			stTotal.Leads += row.Count
			stageTotals[row.Stage] = stTotal

			grandTotal.Value += row.Value
			grandTotal.Leads += row.Count
		}
	}

	var repRows []models.HeatMapRepRow
	for rep, stMap := range repMap {
		repRow := models.HeatMapRepRow{Rep: rep}
		repTotal := models.HeatMapCell{Stage: "Total", Value: 0, Leads: 0}
		for _, stName := range stageNames {
			cell := stMap[stName]
			repRow.Stages = append(repRow.Stages, cell)
			repTotal.Value += cell.Value
			repTotal.Leads += cell.Leads
		}
		repRow.Total = repTotal
		repRows = append(repRows, repRow)
	}

	var stageTotalArr []models.HeatMapCell
	for _, stName := range stageNames {
		stageTotalArr = append(stageTotalArr, stageTotals[stName])
	}

	resp := &models.HeatMapResponse{
		Reps:       repRows,
		StageTotal: stageTotalArr,
		GrandTotal: grandTotal,
	}
	return resp, nil
}

func (s *leadService) GetActivities(ctx context.Context, callerID uuid.UUID, callerRole string, query models.GetActivitiesQuery) (*models.ActivitiesFeedResponse, error) {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.Limit < 1 {
		query.Limit = 20
	} else if query.Limit > 100 {
		query.Limit = 100
	}

	scope, err := middleware.ResolveDataScope(ctx, s.userRepo, callerID, callerRole)
	if err != nil {
		return nil, fmt.Errorf("service: resolve scope: %w", err)
	}

	items, total, typeCounts, err := s.leadRepo.FindActivitiesFeed(ctx, scope, query)
	if err != nil {
		return nil, fmt.Errorf("service: get activities feed: %w", err)
	}

	totalPages := int(math.Ceil(float64(total) / float64(query.Limit)))
	if total == 0 {
		totalPages = 0
	}

	pagination := &models.PaginationMetadata{
		Page:       query.Page,
		Limit:      query.Limit,
		Total:      total,
		TotalPages: totalPages,
	}

	return &models.ActivitiesFeedResponse{
		Success:    true,
		Data:       items,
		Pagination: pagination,
		TypeCounts: typeCounts,
	}, nil
}

func (s *leadService) LogActivity(ctx context.Context, userID uuid.UUID, userRole, userEmail string, req models.LogActivityRequest) (*models.ActivityFeedItemResponse, error) {
	var normalizedType string
	switch strings.ToLower(strings.TrimSpace(req.Type)) {
	case "call":
		normalizedType = "Call"
	case "email":
		normalizedType = "Email"
	case "meeting":
		normalizedType = "Meeting"
	case "demo":
		normalizedType = "Demo"
	case "linkedin":
		normalizedType = "LinkedIn"
	case "proposal sent", "proposal_sent", "proposalsent":
		normalizedType = "Proposal Sent"
	case "other":
		normalizedType = "Other"
	default:
		return nil, helpers.ErrBadRequest("Invalid activity type. Allowed types: Call, Email, Meeting, Demo, LinkedIn, Proposal Sent, Other")
	}

	descTrimmed := strings.TrimSpace(req.Desc)
	if descTrimmed == "" {
		return nil, helpers.ErrBadRequest("Activity description is required")
	}

	var parsedDueDate *time.Time
	if req.DueDate != "" {
		t, err := time.Parse("2006-01-02", req.DueDate)
		if err != nil {
			t2, err2 := time.Parse(time.RFC3339, req.DueDate)
			if err2 != nil {
				return nil, helpers.ErrBadRequest("Invalid dueDate format. Use YYYY-MM-DD")
			}
			parsedDueDate = &t2
		} else {
			parsedDueDate = &t
		}
	}

	var lead *models.Lead
	var err error

	leadIDTrimmed := strings.TrimSpace(req.LeadID)
	leadNameTrimmed := strings.TrimSpace(req.Lead)

	if leadIDTrimmed != "" {
		lead, err = s.leadRepo.GetLeadByLeadID(leadIDTrimmed)
		if err != nil {
			return nil, helpers.ErrNotFound
		}
	} else if leadNameTrimmed != "" {
		lead, err = s.leadRepo.FindLeadByCompanyOrContact(ctx, leadNameTrimmed)
		if err != nil {
			return nil, helpers.ErrNotFound
		}
	} else {
		return nil, helpers.ErrBadRequest("leadId or lead is required")
	}

	scope, err := middleware.ResolveDataScope(ctx, s.userRepo, userID, userRole)
	if err != nil {
		return nil, helpers.ErrInternal("failed to resolve user data scope")
	}

	if !scope.CanAccessLead(lead) {
		return nil, ErrUnauthorized
	}

	activity := &models.Activity{
		LeadID:    lead.LeadID,
		Rep:       &userID,
		Type:      normalizedType,
		Desc:      descTrimmed,
		Outcome:   req.Outcome,
		DueDate:   parsedDueDate,
		Completed: false,
	}

	err = s.leadRepo.CreateActivity(activity)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	if s.emailSvc != nil {
		emailSvc := s.emailSvc
		logger := s.log
		userRepo := s.userRepo
		actType := normalizedType
		actDesc := descTrimmed
		actDueDate := req.DueDate
		leadIDStr := lead.LeadID
		companyStr := lead.Company
		assignedID := lead.AssignedTo
		contactEmail := safeDerefString(lead.Email)
		contactName := safeDerefString(lead.Contact)

		go func() {
			recipients := make(map[string]string)
			if assignedID != nil {
				targetUser, err := userRepo.FindByID(context.Background(), *assignedID)
				if err == nil && targetUser != nil && strings.TrimSpace(targetUser.Email) != "" {
					recipients[strings.ToLower(strings.TrimSpace(targetUser.Email))] = targetUser.Name
				}
			}
			if strings.TrimSpace(contactEmail) != "" {
				recipients[strings.ToLower(strings.TrimSpace(contactEmail))] = contactName
			}

			for email, name := range recipients {
				if err := emailSvc.SendActivityNotification(email, name, actType, actDesc, actDueDate, leadIDStr, companyStr); err != nil {
					if logger != nil {
						logger.Error("failed to send activity notification", "lead_id", leadIDStr, "to", email, "error", err)
					}
				}
			}
		}()
	}

	var userName *string
	if name, err := s.leadRepo.GetUserNameByEmail(userEmail); err == nil && name != "" {
		userName = &name
	} else if userEmail != "" {
		userName = &userEmail
	}

	var dueDateStr *string
	if parsedDueDate != nil {
		d := parsedDueDate.Format("2006-01-02")
		dueDateStr = &d
	}

	var outcomeStr *string
	if activity.Outcome != "" {
		outcomeStr = &activity.Outcome
	}

	resp := &models.ActivityFeedItemResponse{
		ID:        activity.ID,
		Type:      activity.Type,
		Desc:      activity.Desc,
		LeadName:  lead.Contact,
		LeadID:    lead.LeadID,
		Company:   lead.Company,
		Rep:       userName,
		Timestamp: activity.CreatedAt.Format(time.RFC3339),
		Outcome:   outcomeStr,
		Geography: lead.Region,
		Industry:  lead.Industry,
		DealSize:  lead.Size,
		DueDate:   dueDateStr,
		Completed: activity.Completed,
	}

	return resp, nil
}

func (s *leadService) GetActivitiesSummary(ctx context.Context, callerID uuid.UUID, callerRole string) (*models.ActivitySummaryData, error) {
	scope, err := middleware.ResolveDataScope(ctx, s.userRepo, callerID, callerRole)
	if err != nil {
		return nil, fmt.Errorf("service: resolve scope: %w", err)
	}

	summary, err := s.leadRepo.GetActivitiesSummary(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("service: get activities summary: %w", err)
	}
	return summary, nil
}
