package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"crm-auth-service/controllers"
	v1 "crm-auth-service/routes/api/v1"
)

// RegisterRoutes mounts all route groups on the given engine.
func RegisterRoutes(router *gin.Engine, authController *controllers.AuthController) {
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	apiV1 := router.Group("/api/v1")
	v1.RegisterAuthRoutes(apiV1, authController)
}