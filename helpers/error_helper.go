package helpers

import (
	"errors"
	"net/http"
	"time"
)

// ErrNotFound is returned by repositories when a record is not found.
var ErrNotFound = errors.New("record not found")

// AppError represents a user-facing application error.
type AppError struct {
	Status     int
	Message    string
	RetryAfter time.Duration
}

// Error implements the standard error interface.
func (e *AppError) Error() string {
	return e.Message
}

// ErrInvalidCredentials constructs a 401 AppError for authentication failures.
func ErrInvalidCredentials() *AppError {
	return &AppError{
		Status:  http.StatusUnauthorized,
		Message: "Invalid credentials",
	}
}

// ErrTokenInvalid constructs a 401 AppError for invalid/expired tokens.
func ErrTokenInvalid(message string) *AppError {
	return &AppError{
		Status:  http.StatusUnauthorized,
		Message: message,
	}
}

// ErrRateLimited constructs a 429 AppError indicating too many login requests.
func ErrRateLimited(retryAfter time.Duration) *AppError {
	return &AppError{
		Status:     http.StatusTooManyRequests,
		Message:    "Too many login attempts. Please try again later.",
		RetryAfter: retryAfter,
	}
}

// ErrBadRequest constructs a 400 AppError for request payload validation issues.
func ErrBadRequest(message string) *AppError {
	return &AppError{
		Status:  http.StatusBadRequest,
		Message: message,
	}
}

// ErrInternal constructs a 500 AppError for internal runtime exceptions.
func ErrInternal() *AppError {
	return &AppError{
		Status:  http.StatusInternalServerError,
		Message: "Something went wrong. Please try again.",
	}
}

// ErrConflict constructs a 409 AppError for duplicate resource registration.
func ErrConflict(message string) *AppError {
	return &AppError{
		Status:  http.StatusConflict,
		Message: message,
	}
}

// ErrForbidden constructs a 403 AppError for access authorization failures.
func ErrForbidden(message string) *AppError {
	return &AppError{
		Status:  http.StatusForbidden,
		Message: message,
	}
}