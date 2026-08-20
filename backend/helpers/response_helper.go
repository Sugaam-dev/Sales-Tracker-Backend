package helpers

import (
	"errors"
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"
)

// errorBody defines the standard structured JSON response schema for error responses.
type errorBody struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// SuccessResponse writes the API data payload to the response with a specified HTTP status code.
func SuccessResponse(c *gin.Context, status int, data any) {
	c.JSON(status, data)
}

// ErrorResponse writes a structured API error response message to the response.
func ErrorResponse(c *gin.Context, status int, message string) {
	c.JSON(status, errorBody{
		Success: false,
		Message: message,
	})
}

// RespondError translates application or system errors to structured HTTP responses.
func RespondError(c *gin.Context, err error, log *slog.Logger) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		if appErr.RetryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(int(appErr.RetryAfter.Seconds())))
		}
		ErrorResponse(c, appErr.Status, appErr.Message)
		return
	}

	log.Error("unhandled error", "error", err)
	internal := ErrInternal()
	ErrorResponse(c, internal.Status, internal.Message)
}