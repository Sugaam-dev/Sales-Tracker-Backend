package routes

import (
	"net/http"
	"github.com/gin-gonic/gin"
	"crm-auth-service/controllers"
	"crm-auth-service/helpers"
	"crm-auth-service/middleware"
	v1 "crm-auth-service/routes/api/v1"
)

// RegisterRoutes mounts all route groups on the given engine.
func RegisterRoutes(
	router *gin.Engine,
	authController *controllers.AuthController,
	leadController *controllers.LeadController,
	commercialController *controllers.CommercialController,
	analyticsController *controllers.AnalyticsController,
	jwtManager *helpers.JWTManager,
) {
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	apiV1 := router.Group("/api/v1")
	v1.RegisterAuthRoutes(apiV1, authController, jwtManager)
	v1.RegisterLeadRoutes(apiV1, leadController, jwtManager)
	v1.RegisterCommercialRoutes(apiV1, commercialController, jwtManager)
	v1.RegisterAnalyticsRoutes(apiV1, analyticsController, jwtManager)
	apiV1.POST("/users", middleware.AuthMiddleware(jwtManager), authController.CreateUser)

	// Lead Module routes from Sahil
	apiV1.GET("/current_users/", middleware.AuthMiddleware(jwtManager), leadController.GetCurrentUsers)
	apiV1.GET("/master/stages", middleware.AuthMiddleware(jwtManager), leadController.GetMasterStages)
	apiV1.GET("/leads", middleware.AuthMiddleware(jwtManager), leadController.ListLeads)
	apiV1.GET("/leads/:id", middleware.AuthMiddleware(jwtManager), leadController.GetLead)
}
