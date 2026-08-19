package helpers

import (
	"github.com/gin-gonic/gin"
)

// ErrorResponse writes a standardized JSON error response
func ErrorResponse(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"success": false,
		"message": message,
	})
}

// SuccessResponse writes a standardized JSON success response
func SuccessResponse(c *gin.Context, status int, data any) {
	c.JSON(status, data)
}
