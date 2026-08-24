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
func RegisterRoutes(router *gin.Engine, authController *controllers.AuthController, jwtManager *helpers.JWTManager) {
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	apiV1 := router.Group("/api/v1")
	v1.RegisterAuthRoutes(apiV1, authController, jwtManager)
	apiV1.POST("/users", middleware.AuthMiddleware(jwtManager), authController.CreateUser)
}
