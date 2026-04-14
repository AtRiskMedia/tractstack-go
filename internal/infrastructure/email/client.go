// Package email provides the email client for sending transactional emails.
package email

import (
	"fmt"
	"os"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/resend/resend-go/v2"
)

// Service defines the generic interface for dispatching compiled HTML emails.
type Service interface {
	SendDynamicHTML(tenantID string, to []string, subject string, htmlBody string) error
}

// ResendClient is the concrete implementation of the email Service using the Resend API.
type ResendClient struct {
	fromEmail string
	fromName  string
}

// NewService creates a new email service client.
// It initializes defaults but delegates API key resolution to the runtime methods.
func NewService() Service {
	fromEmail := os.Getenv("TENANT_EMAIL_FROM")
	if fromEmail == "" {
		fromEmail = "noreply@tractstack.com"
	}

	fromName := os.Getenv("TENANT_EMAIL_FROM_NAME")
	if fromName == "" {
		fromName = "TractStack"
	}

	return &ResendClient{
		fromEmail: fromEmail,
		fromName:  fromName,
	}
}

// SendDynamicHTML dispatches a pre-compiled HTML payload via Resend.
// It resolves the tenant-specific API key at runtime to ensure absolute domain isolation.
func (c *ResendClient) SendDynamicHTML(tenantID string, to []string, subject string, htmlBody string) error {
	config, err := tenant.LoadTenantConfig(tenantID, nil)
	if err != nil {
		return fmt.Errorf("failed to load tenant config for email sending: %w", err)
	}

	if config.ResendAPIKey == "" {
		return fmt.Errorf("resend API key not configured for tenant %s", tenantID)
	}

	client := resend.NewClient(config.ResendAPIKey)

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("%s <%s>", c.fromName, c.fromEmail),
		To:      to,
		Subject: subject,
		Html:    htmlBody,
	}

	_, err = client.Emails.Send(params)
	if err != nil {
		return fmt.Errorf("failed to send dynamic html email via Resend: %w", err)
	}

	return nil
}
