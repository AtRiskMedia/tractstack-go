// Package templates provides email activation template
package templates

import (
	"bytes"
	"html/template"
	"log"
)

// ActivationEmailProps holds the dynamic data for the activation email.
type ActivationEmailProps struct {
	Name            string
	ActivationURL   string
	TenantID        string
	ExpirationHours int
}

// activationTemplates holds pre-parsed templates for security and performance.
// Using html/template automatically escapes the user-provided data.
var activationTemplates = template.Must(template.New("activation").Parse(`
		{{define "greeting"}}Hello {{.}},{{end}}
		{{define "tenantURL"}}<strong>https://{{.}}.sandbox.freewebpress.com</strong>{{end}}
		{{define "expiration"}}This activation link will expire in {{.}} hours.{{end}}
	`))

// renderTemplate is a helper to execute a specific named template.
func renderTemplate(name string, data interface{}) string {
	var buf bytes.Buffer
	err := activationTemplates.ExecuteTemplate(&buf, name, data)
	if err != nil {
		log.Printf("ERROR: Failed to render activation email template '%s': %v", name, err)
		return "" // Return empty string on error
	}
	return buf.String()
}

// GetActivationEmailContent generates the secure HTML content for an activation email.
func GetActivationEmailContent(props ActivationEmailProps) string {
	expirationHours := props.ExpirationHours
	if expirationHours == 0 {
		expirationHours = 48
	}

	// Each piece of dynamic content is now rendered through a secure template.
	// This replaces the three insecure fmt.Sprintf() calls.
	content := GetParagraph(renderTemplate("greeting", props.Name)) +
		GetParagraph("Thank you for creating your TractStack tenant. Please click the button below to activate your tenant:") +
		GetButton(ButtonProps{
			Text: "Activate Your Tenant",
			URL:  props.ActivationURL,
		}) +
		GetParagraph("Once activated, you'll be able to access your tenant at:") +
		GetParagraphWithHTML(renderTemplate("tenantURL", props.TenantID)) + // Use HTML version for <strong> tags
		GetParagraph(renderTemplate("expiration", expirationHours))

	return content
}
