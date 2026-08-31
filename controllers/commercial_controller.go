package controllers

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"

	"crm-auth-service/helpers"
	"crm-auth-service/models"
	"crm-auth-service/services"
)

type CommercialController struct {
	service services.CommercialService
	log     *slog.Logger
}

func NewCommercialController(service services.CommercialService, log *slog.Logger) *CommercialController {
	return &CommercialController{
		service: service,
		log:     log,
	}
}

func (ctrl *CommercialController) handleServiceError(c *gin.Context, err error) {
	if errors.Is(err, services.ErrNotFound) {
		helpers.ErrorResponse(c, http.StatusNotFound, "Lead or Commercial Estimation not found")
	} else if errors.Is(err, services.ErrUnauthorized) {
		helpers.ErrorResponse(c, http.StatusForbidden, "Unauthorized to access this commercial estimation")
	} else if errors.Is(err, services.ErrValidation) {
		helpers.ErrorResponse(c, http.StatusBadRequest, "Validation failed")
	} else if errors.Is(err, services.ErrDuplicateConflict) {
		helpers.ErrorResponse(c, http.StatusConflict, "Commercial estimation conflict")
	} else {
		ctrl.log.Error("commercial internal error", "error", err)
		helpers.ErrorResponse(c, http.StatusInternalServerError, "Internal server error")
	}
}

func (ctrl *CommercialController) getUserContext(c *gin.Context) (string, string) {
	userEmail := ""
	if email, exists := c.Get("email"); exists {
		userEmail = email.(string)
	}

	userRole := ""
	if role, exists := c.Get("role"); exists {
		userRole = role.(string)
	}

	return userRole, userEmail
}

var leadCommercialIDRegex = regexp.MustCompile(`^L-\d+$`)

// GetCommercial handles GET /api/v1/leads/:id/commercial.
func (ctrl *CommercialController) GetCommercial(c *gin.Context) {
	id := c.Param("id")
	if !leadCommercialIDRegex.MatchString(id) {
		helpers.ErrorResponse(c, http.StatusNotFound, "Lead not found")
		return
	}

	userRole, userEmail := ctrl.getUserContext(c)
	currency := c.Query("currency")

	resp, err := ctrl.service.GetCommercial(c.Request.Context(), id, userRole, userEmail, currency)
	if err != nil {
		ctrl.handleServiceError(c, err)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
	})
}

// UpdateCommercial handles PATCH /api/v1/leads/:id/commercial.
func (ctrl *CommercialController) UpdateCommercial(c *gin.Context) {
	id := c.Param("id")
	if !leadCommercialIDRegex.MatchString(id) {
		helpers.ErrorResponse(c, http.StatusNotFound, "Lead not found")
		return
	}

	userRole, userEmail := ctrl.getUserContext(c)
	currency := c.Query("currency")

	var req models.UpdateCommercialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Validation failed",
			"errors":  err.Error(),
		})
		return
	}

	resp, err := ctrl.service.UpdateCommercial(c.Request.Context(), id, userRole, userEmail, currency, req)
	if err != nil {
		ctrl.handleServiceError(c, err)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
	})
}

// GetAnalytics handles GET /api/v1/leads/:id/commercial/analytics.
func (ctrl *CommercialController) GetAnalytics(c *gin.Context) {
	id := c.Param("id")
	if !leadCommercialIDRegex.MatchString(id) {
		helpers.ErrorResponse(c, http.StatusNotFound, "Lead not found")
		return
	}

	userRole, userEmail := ctrl.getUserContext(c)
	currency := c.Query("currency")

	resp, err := ctrl.service.GetAnalytics(c.Request.Context(), id, userRole, userEmail, currency)
	if err != nil {
		ctrl.handleServiceError(c, err)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
	})
}
