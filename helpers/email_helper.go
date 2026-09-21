package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"strings"
	"sync"
	"time"
)

// EmailService defines operations for sending email notifications.
type EmailService interface {
	SendOTP(toEmail, otp string) error
	SendPasswordReset(toEmail, resetLink string) error
	SendWelcomeEmail(toEmail, name, initialPassword, role, managerName string) error
	SendLeadCreated(toEmail, leadName, company, leadID, ownerName string) error
	SendLeadAssigned(toEmail, assigneeName, leadID, company, assignedByName string) error
	SendLeadStatusChanged(toEmail, recipientName, leadID, company, oldStatus, newStatus, oldStage, newStage string) error
	SendActivityNotification(toEmail, recipientName, activityType, desc, dueDate, leadID, company string) error
}

// SMTPConfig holds credentials for SMTP email delivery.
type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
}

// AzureGraphConfig holds credentials for Microsoft Graph API email delivery.
type AzureGraphConfig struct {
	ClientID     string
	TenantID     string
	ClientSecret string
	MailSender   string
}

// EmailServiceConfig holds top-level email configuration.
type EmailServiceConfig struct {
	Provider   string // "SMTP" or "AZURE"
	MailSender string
	SMTP       SMTPConfig
	Azure      AzureGraphConfig
}

// EmailDeliveryProvider is the low-level transport interface for sending raw emails.
type EmailDeliveryProvider interface {
	SendMail(toEmail, subject, body string) error
}

// smtpDeliveryProvider sends emails through an SMTP server.
type smtpDeliveryProvider struct {
	cfg        SMTPConfig
	mailSender string
	log        *slog.Logger
}

func (s *smtpDeliveryProvider) SendMail(toEmail, subject, body string) error {
	addr := net.JoinHostPort(s.cfg.Host, s.cfg.Port)
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	from := s.cfg.Username
	if s.mailSender != "" {
		from = s.mailSender
	}
	if from == "" {
		from = "no-reply@salestracker.com"
	}
	msg := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s", from, toEmail, subject, body))
	err := smtp.SendMail(addr, auth, from, []string{toEmail}, msg)
	if err != nil {
		s.log.Error("failed to deliver email via SMTP", "to", toEmail, "error", err)
		return err
	}
	s.log.Info("email delivered successfully via SMTP", "to", toEmail)
	return nil
}

// azureGraphDeliveryProvider sends emails through Microsoft Graph API using OAuth 2.0 Client Credentials.
type azureGraphDeliveryProvider struct {
	cfg        AzureGraphConfig
	log        *slog.Logger
	httpClient *http.Client

	tokenMu   sync.RWMutex
	token     string
	expiresAt time.Time
}

func (p *azureGraphDeliveryProvider) getToken(ctx context.Context) (string, error) {
	p.tokenMu.RLock()
	if p.token != "" && time.Now().Add(2*time.Minute).Before(p.expiresAt) {
		tok := p.token
		p.tokenMu.RUnlock()
		return tok, nil
	}
	p.tokenMu.RUnlock()

	p.tokenMu.Lock()
	defer p.tokenMu.Unlock()

	if p.token != "" && time.Now().Add(2*time.Minute).Before(p.expiresAt) {
		return p.token, nil
	}

	endpoint := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", url.PathEscape(p.cfg.TenantID))
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", p.cfg.ClientID)
	data.Set("client_secret", p.cfg.ClientSecret)
	data.Set("scope", "https://graph.microsoft.com/.default")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("azure graph: create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("azure graph: token endpoint request error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("azure graph: token endpoint returned status %d", resp.StatusCode)
	}

	var res struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("azure graph: decode token response: %w", err)
	}
	if res.AccessToken == "" {
		return "", fmt.Errorf("azure graph: empty access_token returned")
	}

	p.token = res.AccessToken
	p.expiresAt = time.Now().Add(time.Duration(res.ExpiresIn) * time.Second)
	return p.token, nil
}

func (p *azureGraphDeliveryProvider) SendMail(toEmail, subject, body string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	token, err := p.getToken(ctx)
	if err != nil {
		p.log.Error("azure graph token acquisition failed", "to", toEmail, "error", err)
		return fmt.Errorf("azure graph: token acquisition failed: %w", err)
	}

	sender := p.cfg.MailSender
	if sender == "" {
		return fmt.Errorf("azure graph: MailSender configuration is missing")
	}

	sendURL := fmt.Sprintf("https://graph.microsoft.com/v1.0/users/%s/sendMail", url.PathEscape(sender))

	type emailAddress struct {
		Address string `json:"address"`
	}
	type recipient struct {
		EmailAddress emailAddress `json:"emailAddress"`
	}
	type itemBody struct {
		ContentType string `json:"contentType"`
		Content     string `json:"content"`
	}
	type messagePayload struct {
		Subject      string      `json:"subject"`
		Body         itemBody    `json:"body"`
		ToRecipients []recipient `json:"toRecipients"`
	}
	type graphRequest struct {
		Message        messagePayload `json:"message"`
		SaveToSentItem string         `json:"saveToSentItems"`
	}

	payload := graphRequest{
		Message: messagePayload{
			Subject: subject,
			Body: itemBody{
				ContentType: "Text",
				Content:     body,
			},
			ToRecipients: []recipient{
				{EmailAddress: emailAddress{Address: toEmail}},
			},
		},
		SaveToSentItem: "false",
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("azure graph: marshal sendMail payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sendURL, bytes.NewReader(jsonBytes))
	if err != nil {
		return fmt.Errorf("azure graph: create sendMail request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.log.Error("azure graph sendMail HTTP request failed", "to", toEmail, "error", err)
		return fmt.Errorf("azure graph: sendMail HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		p.log.Error("azure graph sendMail API returned failure status", "status", resp.StatusCode, "to", toEmail)
		return fmt.Errorf("azure graph: sendMail API status %d: %s", resp.StatusCode, string(respBody))
	}

	p.log.Info("email delivered successfully via Microsoft Graph API", "to", toEmail)
	return nil
}

// consoleDeliveryProvider logs emails to the logger in dev mode.
type consoleDeliveryProvider struct {
	log *slog.Logger
}

func (c *consoleDeliveryProvider) SendMail(toEmail, subject, body string) error {
	c.log.Info("Email (console dev mode)", "to", toEmail, "subject", subject)
	return nil
}

// defaultEmailService implements EmailService by delegating delivery to an EmailDeliveryProvider.
type defaultEmailService struct {
	provider EmailDeliveryProvider
	log      *slog.Logger
}

// NewEmailService constructs an EmailService supporting both SMTP and Azure Graph API.
func NewEmailService(cfg EmailServiceConfig, log *slog.Logger) EmailService {
	var deliveryProvider EmailDeliveryProvider

	if strings.EqualFold(cfg.Provider, "AZURE") {
		log.Info("initializing Azure Microsoft Graph email provider", "sender", cfg.MailSender)
		deliveryProvider = &azureGraphDeliveryProvider{
			cfg:        cfg.Azure,
			log:        log,
			httpClient: &http.Client{Timeout: 30 * time.Second},
		}
	} else if cfg.SMTP.Host != "" && cfg.SMTP.Username != "" {
		log.Info("initializing SMTP email provider", "host", cfg.SMTP.Host, "username", cfg.SMTP.Username)
		deliveryProvider = &smtpDeliveryProvider{
			cfg:        cfg.SMTP,
			mailSender: cfg.MailSender,
			log:        log,
		}
	} else {
		log.Info("initializing Console Email Provider (dev/log mode)")
		deliveryProvider = &consoleDeliveryProvider{log: log}
	}

	return &defaultEmailService{
		provider: deliveryProvider,
		log:      log,
	}
}

// NewConsoleEmailService constructs a new console-based EmailService.
func NewConsoleEmailService(log *slog.Logger) EmailService {
	return &defaultEmailService{
		provider: &consoleDeliveryProvider{log: log},
		log:      log,
	}
}

func (s *defaultEmailService) SendOTP(toEmail, otp string) error {
	subject := "Your Sales Tracker Verification Code"
	body := fmt.Sprintf("Your verification code is: %s\n\nThis code expires in 5 minutes.", otp)
	return s.provider.SendMail(toEmail, subject, body)
}

func (s *defaultEmailService) SendPasswordReset(toEmail, resetLink string) error {
	subject := "Reset Your Sales Tracker Password"
	body := fmt.Sprintf("Click the following link to reset your password:\n\n%s\n\nThis link expires in 15 minutes.", resetLink)
	return s.provider.SendMail(toEmail, subject, body)
}

func (s *defaultEmailService) SendWelcomeEmail(toEmail, name, initialPassword, role, managerName string) error {
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

	return s.provider.SendMail(toEmail, subject, body)
}

func (s *defaultEmailService) SendLeadCreated(toEmail, leadName, company, leadID, ownerName string) error {
	subject := fmt.Sprintf("Lead Created: %s (%s)", company, leadID)
	body := fmt.Sprintf(`Hello %s,

Thank you for registering with Sales Tracker. Your lead profile has been created successfully.

Lead Summary:
- Lead ID: %s
- Company: %s
- Contact Name: %s
- Assigned Owner: %s

Our sales team will be in touch shortly regarding your requirements.

Regards,
Sales Tracker Team`, leadName, leadID, company, leadName, ownerName)

	return s.provider.SendMail(toEmail, subject, body)
}

func (s *defaultEmailService) SendLeadAssigned(toEmail, assigneeName, leadID, company, assignedByName string) error {
	subject := fmt.Sprintf("Lead Assigned to You: %s (%s)", company, leadID)
	body := fmt.Sprintf(`Hello %s,

You have been assigned a lead in Sales Tracker.

Assignment Details:
- Lead ID: %s
- Company: %s
- Assigned By: %s

Please log into Sales Tracker to review the lead requirements and follow up.

Regards,
Sales Tracker Team`, assigneeName, leadID, company, assignedByName)

	return s.provider.SendMail(toEmail, subject, body)
}

func (s *defaultEmailService) SendLeadStatusChanged(toEmail, recipientName, leadID, company, oldStatus, newStatus, oldStage, newStage string) error {
	subject := fmt.Sprintf("Lead Status Update: %s (%s)", company, leadID)
	body := fmt.Sprintf(`Hello %s,

The status/stage for lead %s (%s) has been updated.

Update Details:
- Lead ID: %s
- Company: %s
- Stage: %s -> %s
- Status: %s -> %s

Log into Sales Tracker for full details.

Regards,
Sales Tracker Team`, recipientName, company, leadID, leadID, company, oldStage, newStage, oldStatus, newStatus)

	return s.provider.SendMail(toEmail, subject, body)
}

func (s *defaultEmailService) SendActivityNotification(toEmail, recipientName, activityType, desc, dueDate, leadID, company string) error {
	subject := fmt.Sprintf("Activity Scheduled: %s for %s (%s)", activityType, company, leadID)
	body := fmt.Sprintf(`Hello %s,

An activity has been scheduled regarding lead %s (%s).

Activity Details:
- Type: %s
- Company: %s
- Lead ID: %s
- Scheduled Date: %s
- Description: %s

Regards,
Sales Tracker Team`, recipientName, company, leadID, activityType, company, leadID, dueDate, desc)

	return s.provider.SendMail(toEmail, subject, body)
}
