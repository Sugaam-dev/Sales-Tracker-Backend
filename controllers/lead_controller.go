package controllers

import (
	"log/slog"
	"net/http"
	"regexp"
	"strconv"

	"github.com/gin-gonic/gin"

	"crm-auth-service/helpers"
	"crm-auth-service/models"
	"crm-auth-service/services"
)

var leadIDRegex = regexp.MustCompile(`^L-\d+$`)

type LeadController struct {
	leadService *services.LeadService
	log         *slog.Logger
}

func NewLeadController(leadService *services.LeadService, log *slog.Logger) *LeadController {
	return &LeadController{
		leadService: leadService,
		log:         log,
	}
}

func (ac *LeadController) GetCurrentUsers(c *gin.Context) {
	users, err := ac.leadService.GetCurrentUsers(c.Request.Context())
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
	stages, err := ac.leadService.GetMasterStages(c.Request.Context())
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

	leads, pagination, err := ac.leadService.ListLeads(c.Request.Context(), page, limit, search, owner, priority, stage, sortBy, sortOrder)
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

	// Validate public Lead ID format (L-0001)
	if !leadIDRegex.MatchString(id) {
		helpers.ErrorResponse(c, http.StatusNotFound, "Lead not found")
		return
	}

	lead, err := ac.leadService.GetLead(c.Request.Context(), id)
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

func (ac *LeadController) UpdateLead(c *gin.Context) {
	id := c.Param("id")

	// Validate public Lead ID format (L-0001)
	if !leadIDRegex.MatchString(id) {
		helpers.ErrorResponse(c, http.StatusNotFound, "Lead not found")
		return
	}

	var req models.UpdateLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "invalid request body")
		return
	}

	lead, err := ac.leadService.UpdateLead(c.Request.Context(), id, req)
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
