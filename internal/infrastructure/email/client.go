// Package email provides the email client for sending transactional emails.
package email

import (
	"fmt"
	"os"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/email/templates"
	"github.com/resendlabs/resend-go"
)

// Service defines the interface for sending emails, allowing for mock implementations in tests.
type Service interface {
	SendTenantActivationEmail(toEmail, tenantID, activationURL string) error
}

// ResendClient is the concrete implementation of the email Service using the Resend API.
type ResendClient struct {
	client    *resend.Client
	fromEmail string
	fromName  string
}

// NewService creates a new email service client, returning the Service interface.
func NewService() (Service, error) {
	apiKey := os.Getenv("RESEND_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("RESEND_API_KEY environment variable is required")
	}

	fromEmail := os.Getenv("TENANT_EMAIL_FROM")
	if fromEmail == "" {
		fromEmail = "noreply@tractstack.com" // Default from address
	}

	fromName := os.Getenv("TENANT_EMAIL_FROM_NAME")
	if fromName == "" {
		fromName = "TractStack" // Default from name
	}

	return &ResendClient{
		client:    resend.NewClient(apiKey),
		fromEmail: fromEmail,
		fromName:  fromName,
	}, nil
}

// SendTenantActivationEmail composes and sends the tenant activation email.
func (c *ResendClient) SendTenantActivationEmail(toEmail, tenantID, activationURL string) error {
	subject := "Activate your TractStack tenant"

	content := templates.GetActivationEmailContent(templates.ActivationEmailProps{
		Name:            "there",
		ActivationURL:   activationURL,
		TenantID:        tenantID,
		ExpirationHours: 48,
	})

	htmlContent := templates.GetEmailLayout(templates.EmailLayoutProps{
		Content: content,
	})

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("%s <%s>", c.fromName, c.fromEmail),
		To:      []string{toEmail},
		Subject: subject,
		Html:    htmlContent,
	}

	_, err := c.client.Emails.Send(params)
	if err != nil {
		return fmt.Errorf("failed to send activation email via Resend: %w", err)
	}

	return nil
}
