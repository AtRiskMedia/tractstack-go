// Package email provides the email client for sending transactional emails.
package email

import (
	"fmt"
	"os"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/email/templates"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/resendlabs/resend-go"
)

// Service defines the interface for sending emails, allowing for mock implementations in tests.
type Service interface {
	SendTenantActivationEmail(toEmail, tenantID, activationURL string) error
}

// ResendClient is the concrete implementation of the email Service using the Resend API.
type ResendClient struct {
	fromEmail string
	fromName  string
}

// NewService creates a new email service client, returning the Service interface.
// It initializes defaults but delegates API key resolution to the runtime methods.
func NewService() Service {
	fromEmail := os.Getenv("TENANT_EMAIL_FROM")
	if fromEmail == "" {
		fromEmail = "noreply@tractstack.com" // Default from address
	}

	fromName := os.Getenv("TENANT_EMAIL_FROM_NAME")
	if fromName == "" {
		fromName = "TractStack" // Default from name
	}

	return &ResendClient{
		fromEmail: fromEmail,
		fromName:  fromName,
	}
}

// SendTenantActivationEmail composes and sends the tenant activation email.
// It retrieves the Resend API key from the tenant's specific configuration.
func (c *ResendClient) SendTenantActivationEmail(toEmail, tenantID, activationURL string) error {
	// Load the tenant config to get the tenant-specific Resend API Key
	config, err := tenant.LoadTenantConfig(tenantID, nil)
	if err != nil {
		return fmt.Errorf("failed to load tenant config for email sending: %w", err)
	}

	if config.ResendAPIKey == "" {
		return fmt.Errorf("resend API key not configured for tenant %s", tenantID)
	}

	// Create a new client for this specific request using the tenant's key
	client := resend.NewClient(config.ResendAPIKey)

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

	_, err = client.Emails.Send(params)
	if err != nil {
		return fmt.Errorf("failed to send activation email via Resend: %w", err)
	}

	return nil
}
