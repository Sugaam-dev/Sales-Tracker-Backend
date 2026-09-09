package v1

import (
	"github.com/gin-gonic/gin"

	"crm-auth-service/controllers"
	"crm-auth-service/helpers"
	"crm-auth-service/middleware"
)

// RegisterAnalyticsRoutes mounts the dashboard summary and reports analytics endpoints.
func RegisterAnalyticsRoutes(rg *gin.RouterGroup, analyticsController *controllers.AnalyticsController, jwtManager *helpers.JWTManager) {
	authGroup := rg.Group("")
	authGroup.Use(middleware.AuthMiddleware(jwtManager))
	{
		authGroup.GET("/dashboard/summary", analyticsController.GetDashboardSummary)
		authGroup.GET("/reports/analytics", analyticsController.GetReportsAnalytics)
	}
}
