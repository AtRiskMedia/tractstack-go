// Package email provides the email client for sending transactional emails.
package email

import (
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/resend/resend-go/v2"
)

// Service defines the generic interface for dispatching compiled HTML emails.
type Service interface {
	SendDynamicHTML(tenantID string, to []string, subject string, htmlBody string) error
}

// ResendClient is the concrete implementation of the email Service using the Resend API.
type ResendClient struct{}

// NewService creates a new email service client.
// Tenant key and From address are resolved per send via LoadTenantConfig.
func NewService() Service {
	return &ResendClient{}
}

// SendDynamicHTML dispatches a pre-compiled HTML payload via Resend.
// It resolves the tenant-specific API key and From address at runtime.
func (c *ResendClient) SendDynamicHTML(tenantID string, to []string, subject string, htmlBody string) error {
	config, err := tenant.LoadTenantConfig(tenantID, nil)
	if err != nil {
		return fmt.Errorf("failed to load tenant config for email sending: %w", err)
	}

	if strings.TrimSpace(config.ResendAPIKey) == "" {
		return fmt.Errorf("resend API key not configured for tenant %s", tenantID)
	}

	var adminEmail, adminEmailName string
	if config.BrandConfig != nil {
		adminEmail = config.BrandConfig.AdminEmail
		adminEmailName = config.BrandConfig.AdminEmailName
	}
	if strings.TrimSpace(adminEmail) == "" {
		return fmt.Errorf("admin email not configured for tenant %s", tenantID)
	}
	if strings.TrimSpace(adminEmailName) == "" {
		return fmt.Errorf("admin email name not configured for tenant %s", tenantID)
	}

	client := resend.NewClient(config.ResendAPIKey)

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("%s <%s>", adminEmailName, adminEmail),
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
