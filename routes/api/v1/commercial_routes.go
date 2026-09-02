package v1

import (
	"github.com/gin-gonic/gin"

	"crm-auth-service/controllers"
	"crm-auth-service/helpers"
	"crm-auth-service/middleware"
)

// RegisterCommercialRoutes registers Commercial Estimation endpoints under /api/v1/leads/:id/commercial
func RegisterCommercialRoutes(rg *gin.RouterGroup, commercialController *controllers.CommercialController, jwtManager *helpers.JWTManager) {
	comm := rg.Group("/leads/:id/commercial")
	comm.Use(middleware.AuthMiddleware(jwtManager))
	{
		comm.GET("", commercialController.GetCommercial)
		comm.PATCH("", commercialController.UpdateCommercial)
		comm.GET("/analytics", commercialController.GetAnalytics)
	}
}
