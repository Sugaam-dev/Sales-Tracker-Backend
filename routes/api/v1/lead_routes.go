package v1

import (
	"github.com/gin-gonic/gin"

	"crm-auth-service/controllers"
	"crm-auth-service/helpers"
	"crm-auth-service/middleware"
)

func RegisterLeadRoutes(rg *gin.RouterGroup, leadController *controllers.LeadController, jwtManager *helpers.JWTManager) {
	// Protected lead routes
	leads := rg.Group("/leads")
	leads.Use(middleware.AuthMiddleware(jwtManager))
	{
		leads.POST("", leadController.CreateLead)
		leads.PATCH("/:id", leadController.UpdateLead)
		leads.DELETE("/:id", leadController.DeleteLead)
		leads.GET("/:id/activities", leadController.GetLeadActivities)
		leads.POST("/:id/activities", leadController.CreateActivity)
		leads.POST("/bulk", leadController.BulkCreateLeads)
	}
}
