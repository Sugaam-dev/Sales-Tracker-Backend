package controllers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"crm-auth-service/helpers"
	"crm-auth-service/models"
	"crm-auth-service/services"
)

var leadIDRegex = regexp.MustCompile(`^L-\d+$`)

type LeadController struct {
	service services.LeadService
	log     *slog.Logger
}

func NewLeadController(service services.LeadService, log *slog.Logger) *LeadController {
	return &LeadController{
		service: service,
		log:     log,
	}
}

func (ctrl *LeadController) errorResponse(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"success": false,
		"message": message,
	})
}

func (ctrl *LeadController) handleServiceError(c *gin.Context, err error) {
	var appErr *helpers.AppError
	if errors.As(err, &appErr) {
		ctrl.errorResponse(c, appErr.Status, appErr.Message)
		return
	}
	if errors.Is(err, services.ErrNotFound) {
		ctrl.errorResponse(c, http.StatusNotFound, "Lead not found")
	} else if errors.Is(err, services.ErrUnauthorized) {
		ctrl.errorResponse(c, http.StatusForbidden, "Unauthorized to access this lead")
	} else if errors.Is(err, services.ErrValidation) {
		ctrl.errorResponse(c, http.StatusBadRequest, "Validation failed")
	} else if errors.Is(err, services.ErrDuplicateConflict) {
		ctrl.errorResponse(c, http.StatusConflict, "Duplicate email or company conflict")
	} else {
		ctrl.errorResponse(c, http.StatusInternalServerError, "Internal server error")
	}
}

// ---------------------------------------------
// My Endpoints (Create, Update, Delete, Activities)
// ---------------------------------------------

func (ctrl *LeadController) CreateLead(c *gin.Context) {
	var req models.CreateLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fmt.Println("CREATE LEAD VALIDATION ERROR:", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Validation failed",
			"errors":  err.Error(),
		})
		return
	}

	resp, err := ctrl.service.CreateLead(req)
	if err != nil {
		ctrl.handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    resp,
	})
}

func (ctrl *LeadController) UpdateLead(c *gin.Context) {
	id := c.Param("id")
	userEmail := ""
	if email, exists := c.Get("email"); exists {
		userEmail = email.(string)
	}

	userRole := ""
	if role, exists := c.Get("role"); exists {
		userRole = role.(string)
	}

	if !leadIDRegex.MatchString(id) {
		ctrl.errorResponse(c, http.StatusNotFound, "Lead not found")
		return
	}

	var req models.UpdateLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Validation failed",
			"errors":  err.Error(),
		})
		return
	}

	resp, err := ctrl.service.UpdateLead(id, userRole, userEmail, req)
	if err != nil {
		ctrl.handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
	})
}

func (ctrl *LeadController) DeleteLead(c *gin.Context) {
	id := c.Param("id")
	userRole := ""
	if role, exists := c.Get("role"); exists {
		userRole = role.(string)
	}

	err := ctrl.service.DeleteLead(id, userRole)
	if err != nil {
		ctrl.handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Lead deleted successfully",
	})
}

func (ctrl *LeadController) GetLeadActivities(c *gin.Context) {
	id := c.Param("id")
	userEmail := ""
	if email, exists := c.Get("email"); exists {
		userEmail = email.(string)
	}

	userRole := ""
	if role, exists := c.Get("role"); exists {
		userRole = role.(string)
	}

	resp, err := ctrl.service.GetLeadActivities(id, userRole, userEmail)
	if err != nil {
		ctrl.handleServiceError(c, err)
		return
	}

	if len(resp) == 0 {
		resp = []models.ActivityResponse{}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
	})
}

// ---------------------------------------------
// Sahil's Endpoints
// ---------------------------------------------

func (ac *LeadController) GetCurrentUsers(c *gin.Context) {
	users, err := ac.service.GetCurrentUsers(c.Request.Context())
	if err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, gin.H{
		"success": true,
		"data":    users,
	})
}

func (ac *LeadController) GetMasterStages(c *gin.Context) {
	stages, err := ac.service.GetMasterStages(c.Request.Context())
	if err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, gin.H{
		"success": true,
		"data":    stages,
	})
}

func (ac *LeadController) ListLeads(c *gin.Context) {
	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "5")
	search := c.Query("search")
	owner := c.Query("owner")
	priority := c.Query("priority")
	stage := c.Query("stage")
	sortBy := c.DefaultQuery("sortBy", "createdAt")
	sortOrder := c.DefaultQuery("sortOrder", "desc")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		helpers.ErrorResponse(c, http.StatusBadRequest, "page must be greater than or equal to 1")
		return
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		helpers.ErrorResponse(c, http.StatusBadRequest, "limit must be between 1 and 100")
		return
	}

	leads, pagination, err := ac.service.ListLeads(c.Request.Context(), page, limit, search, owner, priority, stage, sortBy, sortOrder)
	if err != nil {
		helpers.RespondError(c, err, ac.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, gin.H{
		"success":    true,
		"data":       leads,
		"pagination": pagination,
	})
}

func (ac *LeadController) GetLead(c *gin.Context) {
	id := c.Param("id")

	if !leadIDRegex.MatchString(id) {
		helpers.ErrorResponse(c, http.StatusNotFound, "Lead not found")
		return
	}

	lead, err := ac.service.GetLead(c.Request.Context(), id)
	if err != nil {
		if err == helpers.ErrNotFound {
			helpers.ErrorResponse(c, http.StatusNotFound, "Lead not found")
			return
		}
		helpers.RespondError(c, err, ac.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, gin.H{
		"success": true,
		"data":    lead,
	})
}

func (ctrl *LeadController) CreateActivity(c *gin.Context) {
	leadID := c.Param("id")

	userEmail := ""
	if email, exists := c.Get("email"); exists {
		userEmail = email.(string)
	}

	userRole := ""
	if role, exists := c.Get("role"); exists {
		userRole = role.(string)
	}

	var req models.CreateActivityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Validation failed",
			"errors":  err.Error(),
		})
		return
	}

	resp, err := ctrl.service.CreateActivity(leadID, userRole, userEmail, req)
	if err != nil {
		ctrl.handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    resp,
	})
}

func (ctrl *LeadController) CompleteActivity(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		ctrl.errorResponse(c, http.StatusBadRequest, "Invalid activity ID")
		return
	}

	userEmail := ""
	if email, exists := c.Get("email"); exists {
		userEmail = email.(string)
	}

	userRole := ""
	if role, exists := c.Get("role"); exists {
		userRole = role.(string)
	}

	var req models.CompleteActivityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Validation failed",
			"errors":  err.Error(),
		})
		return
	}

	resp, err := ctrl.service.CompleteActivity(uint(id), userRole, userEmail, req.Completed)
	if err != nil {
		ctrl.handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
	})
}

func (ctrl *LeadController) BulkCreateLeads(c *gin.Context) {
	userEmail := ""
	if email, exists := c.Get("email"); exists {
		userEmail = email.(string)
	}

	userRole := ""
	if role, exists := c.Get("role"); exists {
		userRole = role.(string)
	}

	var req models.BulkCreateLeadsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Validation failed",
			"errors":  err.Error(),
		})
		return
	}

	resp, err := ctrl.service.BulkCreateLeads(userRole, userEmail, req)
	if err != nil {
		ctrl.handleServiceError(c, err)
		return
	}

	status := http.StatusOK
	if resp.Summary.Created == resp.Summary.Total {
		status = http.StatusCreated
	}

	c.JSON(status, resp)
}

func (ctrl *LeadController) GetActivities(c *gin.Context) {
	var query models.GetActivitiesQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "Invalid query parameters")
		return
	}

	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "20")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		helpers.ErrorResponse(c, http.StatusBadRequest, "page must be greater than or equal to 1")
		return
	}
	query.Page = page

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		helpers.ErrorResponse(c, http.StatusBadRequest, "limit must be between 1 and 100")
		return
	}
	if limit > 100 {
		limit = 100
	}
	query.Limit = limit

	query.Type = c.Query("type")
	query.UserID = c.Query("user_id")
	query.Rep = c.Query("rep")
	query.LeadID = c.Query("lead_id")
	query.Geography = c.Query("geography")
	query.Industry = c.Query("industry")
	query.DealSize = c.Query("deal_size")

	resp, err := ctrl.service.GetActivities(c.Request.Context(), query)
	if err != nil {
		ctrl.handleServiceError(c, err)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, resp)
}

func (ctrl *LeadController) LogActivity(c *gin.Context) {
	val, exists := c.Get("user_id")
	if !exists {
		helpers.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	userID, ok := val.(uuid.UUID)
	if !ok {
		if idStr, isStr := val.(string); isStr {
			parsed, err := uuid.Parse(idStr)
			if err != nil {
				helpers.ErrorResponse(c, http.StatusUnauthorized, "Invalid user identifier in token")
				return
			}
			userID = parsed
		} else {
			helpers.ErrorResponse(c, http.StatusUnauthorized, "Invalid user identifier in token")
			return
		}
	}

	userEmail := ""
	if email, exists := c.Get("email"); exists {
		userEmail = email.(string)
	}

	userRole := ""
	if role, exists := c.Get("role"); exists {
		userRole = role.(string)
	}

	var req models.LogActivityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	resp, err := ctrl.service.LogActivity(c.Request.Context(), userID, userRole, userEmail, req)
	if err != nil {
		ctrl.handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    resp,
	})
}

func (ctrl *LeadController) GetActivitiesSummary(c *gin.Context) {
	summary, err := ctrl.service.GetActivitiesSummary(c.Request.Context())
	if err != nil {
		ctrl.handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    summary,
	})
}
