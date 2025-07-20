// Package templates provides email activation template
package templates

import "fmt"

type ActivationEmailProps struct {
	Name            string
	ActivationURL   string
	TenantID        string
	ExpirationHours int
}

func GetActivationEmailContent(props ActivationEmailProps) string {
	expirationHours := props.ExpirationHours
	if expirationHours == 0 {
		expirationHours = 48
	}

	content := GetParagraph(fmt.Sprintf("Hello %s,", props.Name)) +
		GetParagraph("Thank you for creating your TractStack tenant. Please click the button below to activate your tenant:") +
		GetButton(ButtonProps{
			Text: "Activate Your Tenant",
			URL:  props.ActivationURL,
		}) +
		GetParagraph("Once activated, you'll be able to access your tenant at:") +
		GetParagraph(fmt.Sprintf("<strong>https://%s.sandbox.freewebpress.com</strong>", props.TenantID)) +
		GetParagraph(fmt.Sprintf("This activation link will expire in %d hours.", expirationHours))

	return content
}
