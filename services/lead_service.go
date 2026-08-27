package services

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"crm-auth-service/models"
	"crm-auth-service/repository"
)

var (
	ErrValidation        = errors.New("validation failed")
	ErrDuplicateConflict = errors.New("duplicate conflict")
	ErrNotFound          = errors.New("lead not found")
	ErrUnauthorized      = errors.New("unauthorized action")
)

type LeadService interface {
	CreateLead(req models.CreateLeadRequest) (*models.LeadResponse, error)
	UpdateLead(leadID string, userRole, userEmail string, req models.UpdateLeadRequest) (*models.LeadResponse, error)
	DeleteLead(leadID string, userRole string) error
	GetLeadActivities(leadID string, userRole, userEmail string) ([]models.ActivityResponse, error)
}

type leadService struct {
	repo repository.LeadRepository
}

func NewLeadService(repo repository.LeadRepository) LeadService {
	return &leadService{repo: repo}
}

func isValidStatus(s string) bool {
	valid := []string{"Open", "In Progress", "Won", "Lost"}
	for _, v := range valid {
		if s == v {
			return true
		}
	}
	return false
}

func isValidSentiment(s string) bool {
	valid := []string{"Positive", "Neutral", "Negative"}
	for _, v := range valid {
		if s == v {
			return true
		}
	}
	return false
}

func (s *leadService) validateMasterData(owner, stage, status, sentiment string) error {
	if owner != "" {
		exists, err := s.repo.CheckUserExists(owner)
		if err != nil || !exists {
			return ErrValidation
		}
	}
	if stage != "" {
		exists, err := s.repo.CheckStageExists(stage)
		if err != nil || !exists {
			return ErrValidation
		}
	}
	if status != "" && !isValidStatus(status) {
		return ErrValidation
	}
	if sentiment != "" && !isValidSentiment(sentiment) {
		return ErrValidation
	}
	return nil
}

func (s *leadService) handleDBError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrDuplicateConflict
	}
	return err
}

func (s *leadService) CreateLead(req models.CreateLeadRequest) (*models.LeadResponse, error) {
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if err := s.validateMasterData(req.Owner, req.Stage, req.Status, req.Sentiment); err != nil {
		return nil, err
	}

	lead := &models.Lead{
		Company:            req.Company,
		ProjectName:        req.ProjectName,
		Contact:            req.Contact,
		Email:              req.Email,
		Phone:              req.Phone,
		OfficePhone:        req.OfficePhone,
		OfficePhoneCountry: req.OfficePhoneCountry,
		Owner:              req.Owner,
		Industry:           req.Industry,
		Size:               req.Size,
		Region:             req.Region,
		Source:             req.Source,
		Stage:              req.Stage,
		Status:             req.Status,
		Sentiment:          req.Sentiment,
		Priority:           req.Priority,
	}

	err := s.repo.CreateLead(lead)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	resp := models.ToLeadResponse(*lead)
	return &resp, nil
}

func (s *leadService) UpdateLead(leadID string, userRole, userEmail string, req models.UpdateLeadRequest) (*models.LeadResponse, error) {
	lead, err := s.repo.GetLeadByLeadID(leadID)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	userName, err := s.repo.GetUserNameByEmail(userEmail)
	if err != nil && userRole != models.RoleAdmin {
		return nil, ErrUnauthorized
	}

	// KAM can only update their own leads
	if userRole != models.RoleAdmin && lead.Owner != userName {
		return nil, ErrUnauthorized
	}

	updates := make(map[string]interface{})
	
	ownerToCheck := ""
	stageToCheck := ""
	statusToCheck := ""
	
	if req.Owner != nil {
		updates["owner"] = *req.Owner
		ownerToCheck = *req.Owner
	}
	if req.Stage != nil {
		updates["stage"] = *req.Stage
		stageToCheck = *req.Stage
	}
	if req.Status != nil {
		updates["status"] = *req.Status
		statusToCheck = *req.Status
	}
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}
	if req.Contact != nil {
		updates["contact"] = *req.Contact
	}
	if req.Email != nil {
		email := strings.ToLower(strings.TrimSpace(*req.Email))
		updates["email"] = email
	}
	if req.Phone != nil {
		updates["phone"] = *req.Phone
	}
	if req.Value != nil {
		updates["value"] = *req.Value
	}

	if err := s.validateMasterData(ownerToCheck, stageToCheck, statusToCheck, ""); err != nil {
		return nil, err
	}

	err = s.repo.UpdateLead(leadID, updates)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	// Fetch updated lead
	updatedLead, err := s.repo.GetLeadByLeadID(leadID)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	resp := models.ToLeadResponse(*updatedLead)
	return &resp, nil
}

func (s *leadService) DeleteLead(leadID string, userRole string) error {
	// Only Admin can delete leads
	if userRole != models.RoleAdmin {
		return ErrUnauthorized
	}

	err := s.repo.DeleteLead(leadID)
	return s.handleDBError(err)
}

func (s *leadService) GetLeadActivities(leadID string, userRole, userEmail string) ([]models.ActivityResponse, error) {
	lead, err := s.repo.GetLeadActivities(leadID)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	userName, err := s.repo.GetUserNameByEmail(userEmail)
	if err != nil && userRole != models.RoleAdmin {
		return nil, ErrUnauthorized
	}

	// KAM can only view activities of their own leads
	if userRole != models.RoleAdmin && lead.Owner != userName {
		return nil, ErrUnauthorized
	}

	resp := make([]models.ActivityResponse, len(lead.Activities))
	for i, a := range lead.Activities {
		resp[i] = models.ToActivityResponse(a)
	}

	return resp, nil
}
