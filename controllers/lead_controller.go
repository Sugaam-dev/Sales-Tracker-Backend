package controllers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"crm-auth-service/models"
	"crm-auth-service/services"
)

type LeadController struct {
	service services.LeadService
}

func NewLeadController(service services.LeadService) *LeadController {
	return &LeadController{service: service}
}

func errorResponse(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"success": false,
		"message": message,
	})
}

func handleServiceError(c *gin.Context, err error) {
	if errors.Is(err, services.ErrNotFound) {
		errorResponse(c, http.StatusNotFound, "Lead not found")
	} else if errors.Is(err, services.ErrUnauthorized) {
		errorResponse(c, http.StatusForbidden, "Unauthorized to access this lead")
	} else if errors.Is(err, services.ErrValidation) {
		errorResponse(c, http.StatusBadRequest, "Validation failed")
	} else if errors.Is(err, services.ErrDuplicateConflict) {
		errorResponse(c, http.StatusConflict, "Duplicate email or company conflict")
	} else {
		errorResponse(c, http.StatusInternalServerError, "Internal server error")
	}
}

func (ctrl *LeadController) CreateLead(c *gin.Context) {
	var req models.CreateLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Validation failed",
			"errors":  err.Error(),
		})
		return
	}

	resp, err := ctrl.service.CreateLead(req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    resp,
	})
}

func (ctrl *LeadController) UpdateLead(c *gin.Context) {
	id := c.Param("id")
	userName := ""
	if email, exists := c.Get("email"); exists {
		userName = email.(string)
	}

	userRole := ""
	if role, exists := c.Get("role"); exists {
		userRole = role.(string)
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

	resp, err := ctrl.service.UpdateLead(id, userRole, userName, req)
	if err != nil {
		handleServiceError(c, err)
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
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Lead deleted successfully",
	})
}

func (ctrl *LeadController) GetLeadActivities(c *gin.Context) {
	id := c.Param("id")
	userName := ""
	if email, exists := c.Get("email"); exists {
		userName = email.(string)
	}

	userRole := ""
	if role, exists := c.Get("role"); exists {
		userRole = role.(string)
	}

	resp, err := ctrl.service.GetLeadActivities(id, userRole, userName)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	if len(resp) == 0 {
		// return empty array per spec
		resp = []models.ActivityResponse{}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
	})
}
