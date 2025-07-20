// Package email provides email client functionality
package email

import (
	"fmt"
	"os"

	"github.com/AtRiskMedia/tractstack-go/email/templates"
	"github.com/resendlabs/resend-go"
)

type Client struct {
	resend    *resend.Client
	fromEmail string
	fromName  string
}

func NewClient() (*Client, error) {
	apiKey := os.Getenv("RESEND_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("RESEND_API_KEY environment variable is required")
	}

	fromEmail := os.Getenv("TENANT_EMAIL_FROM")
	if fromEmail == "" {
		fromEmail = "noreply@yourdomain.com"
	}

	fromName := os.Getenv("TENANT_EMAIL_FROM_NAME")
	if fromName == "" {
		fromName = "TractStack"
	}

	client := resend.NewClient(apiKey)

	return &Client{
		resend:    client,
		fromEmail: fromEmail,
		fromName:  fromName,
	}, nil
}

func (c *Client) SendTenantActivationEmail(tenantID, name, email, activationURL string) error {
	subject := "Activate your TractStack tenant"

	content := templates.GetActivationEmailContent(templates.ActivationEmailProps{
		Name:          name,
		ActivationURL: activationURL,
		TenantID:      tenantID,
	})

	htmlContent := templates.GetEmailLayout(templates.EmailLayoutProps{
		Content: content,
	})

	request := &resend.SendEmailRequest{
		From:    fmt.Sprintf("%s <%s>", c.fromName, c.fromEmail),
		To:      []string{email},
		Subject: subject,
		Html:    htmlContent,
	}

	_, err := c.resend.Emails.Send(request)
	if err != nil {
		return fmt.Errorf("failed to send activation email: %w", err)
	}

	return nil
}
