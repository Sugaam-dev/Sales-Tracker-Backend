package helpers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// ErrNotFound is a sentinel error for resource not found
var ErrNotFound = errors.New("helpers: resource not found")

// AppError represents a client-safe HTTP error with a status code
type AppError struct {
	Status  int
	Message string
	Err     error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%d: %s (%v)", e.Status, e.Message, e.Err)
	}
	return fmt.Sprintf("%d: %s", e.Status, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// ErrBadRequest creates a 400 Bad Request error
func ErrBadRequest(message string) error {
	return &AppError{
		Status:  http.StatusBadRequest,
		Message: message,
	}
}

// ErrTokenInvalid creates a 401 Unauthorized error for invalid tokens
func ErrTokenInvalid(message string) error {
	return &AppError{
		Status:  http.StatusUnauthorized,
		Message: message,
	}
}

// ErrRateLimited creates a 429 Too Many Requests error
func ErrRateLimited(retryAfter time.Duration) error {
	return &AppError{
		Status:  http.StatusTooManyRequests,
		Message: fmt.Sprintf("Too many attempts. Try again in %v.", retryAfter),
	}
}

// ErrInvalidCredentials creates a 401 Unauthorized error with safe credential message
func ErrInvalidCredentials() error {
	return &AppError{
		Status:  http.StatusUnauthorized,
		Message: "Invalid credentials",
	}
}

// RespondError parses the error and writes a standardized error response
func RespondError(c *gin.Context, err error, log *slog.Logger) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		if appErr.Status == http.StatusTooManyRequests {
			// Add Retry-After header for rate limiter
			c.Header("Retry-After", "900") // 15 minutes default
		}
		ErrorResponse(c, appErr.Status, appErr.Message)
		return
	}

	// For unexpected errors, log details but return generic 500
	log.Error("internal server error", "error", err)
	ErrorResponse(c, http.StatusInternalServerError, "An unexpected error occurred")
}
