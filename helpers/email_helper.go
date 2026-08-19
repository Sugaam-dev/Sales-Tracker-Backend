package helpers

import (
	"log/slog"
)

// EmailService represents the interface for sending emails
type EmailService interface {
	SendOTP(email, otp string) error
}

type consoleEmailService struct {
	log *slog.Logger
}

// NewConsoleEmailService creates a new console email service
func NewConsoleEmailService(log *slog.Logger) EmailService {
	return &consoleEmailService{log: log}
}

// SendOTP simulates sending an OTP by writing to the log
func (s *consoleEmailService) SendOTP(email, otp string) error {
	s.log.Info("simulated email send", "to", email, "subject", "Verify your email — CRM", "otp", otp)
	return nil
}
