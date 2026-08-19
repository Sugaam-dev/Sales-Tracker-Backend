package helpers

import (
	"log/slog"
)

// SMSService represents the interface for sending SMS messages
type SMSService interface {
	SendOTP(mobile, otp string) error
}

type consoleSMSService struct {
	log *slog.Logger
}

// NewConsoleSMSService creates a new console SMS service
func NewConsoleSMSService(log *slog.Logger) SMSService {
	return &consoleSMSService{log: log}
}

// SendOTP simulates sending an OTP via SMS by writing to the log
func (s *consoleSMSService) SendOTP(mobile, otp string) error {
	s.log.Info("simulated SMS send", "to", mobile, "otp", otp)
	return nil
}
