package helpers

import (
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
)

// EmailService defines the operations for sending email notifications.
type EmailService interface {
	// SendOTP delivers a multi-factor authentication code to the specified email address.
	SendOTP(toEmail, otp string) error
	// SendPasswordReset delivers a password reset link to the specified email address.
	SendPasswordReset(toEmail, resetLink string) error
	// SendWelcomeEmail delivers an account creation welcome email with initial temporary credentials.
	SendWelcomeEmail(toEmail, name, initialPassword, role, managerName string) error
}

// SMTPConfig holds credentials for SMTP email delivery.
type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
}

// NewEmailService constructs an EmailService. Uses SMTP if host and username are configured,
// otherwise falls back to console logging dev mode.
func NewEmailService(cfg SMTPConfig, log *slog.Logger) EmailService {
	if cfg.Host != "" && cfg.Username != "" {
		return &smtpEmailService{cfg: cfg, log: log}
	}
	return &consoleEmailService{log: log}
}

// consoleEmailService logs emails to the logger.
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

// SendWelcomeEmail logs the welcome email to the console without logging sensitive password info.
func (s *consoleEmailService) SendWelcomeEmail(toEmail, name, initialPassword, role, managerName string) error {
	s.log.Info("Welcome Email (console dev mode)", "to", toEmail, "name", name, "role", role, "manager", managerName)
	return nil
}

// smtpEmailService sends emails through an SMTP server.
type smtpEmailService struct {
	cfg SMTPConfig
	log *slog.Logger
}

func (s *smtpEmailService) send(toEmail, subject, body string) error {
	addr := net.JoinHostPort(s.cfg.Host, s.cfg.Port)
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	from := s.cfg.Username
	if from == "" {
		from = "no-reply@salestracker.com"
	}
	msg := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s", from, toEmail, subject, body))
	return smtp.SendMail(addr, auth, from, []string{toEmail}, msg)
}

func (s *smtpEmailService) SendOTP(toEmail, otp string) error {
	subject := "Your Sales Tracker Verification Code"
	body := fmt.Sprintf("Your verification code is: %s\n\nThis code expires in 5 minutes.", otp)
	return s.send(toEmail, subject, body)
}

func (s *smtpEmailService) SendPasswordReset(toEmail, resetLink string) error {
	subject := "Reset Your Sales Tracker Password"
	body := fmt.Sprintf("Click the following link to reset your password:\n\n%s\n\nThis link expires in 15 minutes.", resetLink)
	return s.send(toEmail, subject, body)
}

func (s *smtpEmailService) SendWelcomeEmail(toEmail, name, initialPassword, role, managerName string) error {
	subject := "Your Sales Tracker Account"
	body := fmt.Sprintf(`Hello %s,

Your Sales Tracker account has been created.

Account Details
----------------
Name: %s
Email: %s
Role: %s
Assigned Sales Manager: %s

Initial Password:
%s

Please use these credentials to log in to Sales Tracker.

For security, this is a temporary password. You will be required to create a new password after your first successful login.

Regards,
Sales Tracker Team`, name, name, toEmail, role, managerName, initialPassword)

	if err := s.send(toEmail, subject, body); err != nil {
		s.log.Error("failed to deliver welcome email via SMTP", "to", toEmail, "error", err)
		return err
	}
	s.log.Info("welcome email delivered successfully via SMTP", "to", toEmail, "role", role)
	return nil
}
