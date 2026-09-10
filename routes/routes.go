package routes

import (
	"crm-auth-service/controllers"
	"crm-auth-service/helpers"
	"crm-auth-service/middleware"
	"crm-auth-service/models"
	"crm-auth-service/repository"
	v1 "crm-auth-service/routes/api/v1"
	"github.com/gin-gonic/gin"
	"net/http"
)

// RegisterRoutes mounts all route groups on the given engine.
func RegisterRoutes(
	router *gin.Engine,
	authController *controllers.AuthController,
	leadController *controllers.LeadController,
	commercialController *controllers.CommercialController,
	taskController *controllers.TaskController,
	reportController *controllers.ReportController,
	analyticsController *controllers.AnalyticsController,
	jwtManager *helpers.JWTManager,
	userRepo repository.UserRepository,
) {
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	apiV1 := router.Group("/api/v1")
	v1.RegisterAuthRoutes(apiV1, authController, jwtManager)
	v1.RegisterLeadRoutes(apiV1, leadController, jwtManager)
	v1.RegisterCommercialRoutes(apiV1, commercialController, jwtManager)

	// Task routes
	tasks := apiV1.Group("/tasks")
	tasks.Use(middleware.AuthMiddleware(jwtManager))
	{
		tasks.GET("", taskController.GetTasks)
		tasks.POST("", taskController.CreateTask)
		tasks.PATCH("/:id/status", taskController.UpdateTaskStatus)
		tasks.DELETE("/:id", taskController.DeleteTask)
	}

	// Report routes
	reports := apiV1.Group("/reports")
	reports.Use(middleware.AuthMiddleware(jwtManager))
	{
		reports.GET("/heat-map", reportController.GetHeatMap)
	}

	v1.RegisterAnalyticsRoutes(apiV1, analyticsController, jwtManager)

	// User management routes
	users := apiV1.Group("/users")
	users.Use(middleware.AuthMiddleware(jwtManager))
	{
		users.GET("", middleware.RequirePermission(userRepo, models.PermissionUserView), authController.ListUsers)
		users.POST("", middleware.RequirePermission(userRepo, models.PermissionUserCreate), authController.CreateUser)
		users.PATCH("/:id", middleware.RequirePermission(userRepo, models.PermissionUserUpdate), authController.UpdateUser)
		users.PUT("/:id", middleware.RequirePermission(userRepo, models.PermissionUserUpdate), authController.UpdateUser)
		users.DELETE("/:id", middleware.RequirePermission(userRepo, models.PermissionUserDelete), authController.DeleteUser)
		users.PATCH("/:id/manager", middleware.RequirePermission(userRepo, models.PermissionManagerManage), authController.AssignManager)

		// Leader delegation routes (admin only)
		users.GET("/:id/delegations", middleware.RequireRole(models.RoleAdmin), authController.GetLeaderPermissions)
		users.POST("/:id/delegations", middleware.RequireRole(models.RoleAdmin), authController.GrantLeaderPermission)
		users.DELETE("/:id/delegations/:permission", middleware.RequireRole(models.RoleAdmin), authController.RevokeLeaderPermission)
		users.DELETE("/:id/delegations", middleware.RequireRole(models.RoleAdmin), authController.RevokeLeaderPermission)
	}

	// User convenience endpoint for own permissions
	apiV1.GET("/users/me/permissions", middleware.AuthMiddleware(jwtManager), authController.GetMyPermissions)

	// Lead Module routes from Sahil
	apiV1.GET("/current_users/", middleware.AuthMiddleware(jwtManager), leadController.GetCurrentUsers)
	apiV1.GET("/master/stages", middleware.AuthMiddleware(jwtManager), leadController.GetMasterStages)
	apiV1.GET("/leads", middleware.AuthMiddleware(jwtManager), leadController.ListLeads)
	apiV1.GET("/leads/:id", middleware.AuthMiddleware(jwtManager), leadController.GetLead)
}
