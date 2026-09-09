package helpers

import "log/slog"

// SMSService defines the operations for sending SMS notifications.
type SMSService interface {
	// SendOTP delivers a multi-factor authentication code to the specified mobile phone number.
	SendOTP(toMobile, otp string) error
}

// consoleSMSService logs OTP codes to the logger.
type consoleSMSService struct {
	log *slog.Logger
}

// NewConsoleSMSService constructs a new console-based SMSService.
func NewConsoleSMSService(log *slog.Logger) SMSService {
	return &consoleSMSService{log: log}
}

// SendOTP logs the multi-factor authentication OTP to the console.
func (s *consoleSMSService) SendOTP(toMobile, otp string) error {
	s.log.Info("MFA OTP (console dev mode)", "to", toMobile, "otp", otp)
	return nil
}
