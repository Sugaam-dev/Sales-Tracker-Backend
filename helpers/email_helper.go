package helpers

import "log/slog"

// EmailService defines the operations for sending email notifications.
type EmailService interface {
	// SendOTP delivers a multi-factor authentication code to the specified email address.
	SendOTP(toEmail, otp string) error
	// SendPasswordReset delivers a password reset link to the specified email address.
	SendPasswordReset(toEmail, resetLink string) error
}

// consoleEmailService logs OTP codes to the logger.
type consoleEmailService struct {
	log *slog.Logger
}

// NewConsoleEmailService constructs a new console-based EmailService.
func NewConsoleEmailService(log *slog.Logger) EmailService {
	return &consoleEmailService{log: log}
}

// SendOTP logs the multi-factor authentication OTP to the console.
func (s *consoleEmailService) SendOTP(toEmail, otp string) error {
	s.log.Info("MFA OTP (console dev mode)", "to", toEmail, "otp", otp)
	return nil
}

// SendPasswordReset logs the password reset link to the console.
func (s *consoleEmailService) SendPasswordReset(toEmail, resetLink string) error {
	s.log.Info("Password Reset Link (console dev mode)", "to", toEmail, "link", resetLink)
	return nil
}
