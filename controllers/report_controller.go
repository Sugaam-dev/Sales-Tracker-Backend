package controllers

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"crm-auth-service/helpers"
	"crm-auth-service/services"
)

type ReportController struct {
	leadService services.LeadService
	log         *slog.Logger
}

func NewReportController(leadService services.LeadService, log *slog.Logger) *ReportController {
	return &ReportController{
		leadService: leadService,
		log:         log,
	}
}

func (ctrl *ReportController) GetHeatMap(c *gin.Context) {
	resp, err := ctrl.leadService.GetHeatMap(c.Request.Context())
	if err != nil {
		helpers.RespondError(c, err, ctrl.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
	})
}
