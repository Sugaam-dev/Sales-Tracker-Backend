package controllers

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"crm-auth-service/helpers"
	"crm-auth-service/middleware"
	"crm-auth-service/services"
)

type AnalyticsController struct {
	service services.AnalyticsService
	log     *slog.Logger
}

func NewAnalyticsController(service services.AnalyticsService, log *slog.Logger) *AnalyticsController {
	return &AnalyticsController{
		service: service,
		log:     log,
	}
}

// GetDashboardSummary handles GET /api/v1/dashboard/summary
func (ctrl *AnalyticsController) GetDashboardSummary(c *gin.Context) {
	userID, userRole, userEmail, err := middleware.GetAuthUser(c)
	if err != nil {
		helpers.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	owner := c.Query("owner")
	region := c.Query("region")

	summary, err := ctrl.service.GetDashboardSummary(c.Request.Context(), userID, userRole, userEmail, owner, region)
	if err != nil {
		helpers.RespondError(c, err, ctrl.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, gin.H{
		"success": true,
		"data":    summary,
	})
}

// GetReportsAnalytics handles GET /api/v1/reports/analytics
func (ctrl *AnalyticsController) GetReportsAnalytics(c *gin.Context) {
	userID, userRole, userEmail, err := middleware.GetAuthUser(c)
	if err != nil {
		helpers.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	owner := c.Query("owner")
	region := c.Query("region")

	dateFromStr := c.Query("date_from")
	if dateFromStr == "" {
		dateFromStr = c.Query("dateFrom")
	}
	dateToStr := c.Query("date_to")
	if dateToStr == "" {
		dateToStr = c.Query("dateTo")
	}

	var dateFrom, dateTo *time.Time
	if strings.TrimSpace(dateFromStr) != "" {
		t, err := time.Parse("2006-01-02", strings.TrimSpace(dateFromStr))
		if err != nil {
			tRFC, errRFC := time.Parse(time.RFC3339, strings.TrimSpace(dateFromStr))
			if errRFC != nil {
				helpers.ErrorResponse(c, http.StatusBadRequest, "Invalid date_from format. Expected YYYY-MM-DD or RFC3339.")
				return
			}
			t = tRFC
		}
		dateFrom = &t
	}

	if strings.TrimSpace(dateToStr) != "" {
		t, err := time.Parse("2006-01-02", strings.TrimSpace(dateToStr))
		if err != nil {
			tRFC, errRFC := time.Parse(time.RFC3339, strings.TrimSpace(dateToStr))
			if errRFC != nil {
				helpers.ErrorResponse(c, http.StatusBadRequest, "Invalid date_to format. Expected YYYY-MM-DD or RFC3339.")
				return
			}
			t = tRFC
		} else {
			// Include full day if only date given
			t = t.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
		}
		dateTo = &t
	}

	analytics, err := ctrl.service.GetReportsAnalytics(c.Request.Context(), userID, userRole, userEmail, dateFrom, dateTo, owner, region)
	if err != nil {
		helpers.RespondError(c, err, ctrl.log)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, gin.H{
		"success": true,
		"data":    analytics,
	})
}
