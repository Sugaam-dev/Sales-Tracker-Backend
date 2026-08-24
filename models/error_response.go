package models

// ErrorResponse represents a standard API error response.
type ErrorResponse struct {
	Message string `json:"message" example:"Invalid credentials"`
} 