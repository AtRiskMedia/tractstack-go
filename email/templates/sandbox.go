// Package templates provides email sandbox template
package templates

import (
	"bytes"
	"html/template"
	"log"
)

// SandboxEmailProps holds the dynamic data for the sandbox email.
type SandboxEmailProps struct {
	Name       string
	ActionURL  string
	ActionText string
}

// sandboxGreetingTmpl is a pre-parsed template for the greeting.
// Using html/template automatically escapes the user-provided name.
var sandboxGreetingTmpl = template.Must(template.New("sandboxGreeting").Parse("Hi {{.}},"))

// GetSandboxEmailContent generates the secure HTML content for a sandbox email.
func GetSandboxEmailContent(props SandboxEmailProps) string {
	name := props.Name
	if name == "" {
		name = "there"
	}

	actionURL := props.ActionURL
	if actionURL == "" {
		actionURL = "https://tractstack.com"
	}

	actionText := props.ActionText
	if actionText == "" {
		actionText = "Visit Your Sandbox"
	}

	// Use the pre-parsed template to securely render the greeting.
	// This replaces the insecure fmt.Sprintf() call.
	var greeting bytes.Buffer
	err := sandboxGreetingTmpl.Execute(&greeting, name)
	if err != nil {
		log.Printf("ERROR: Failed to render sandbox email greeting: %v", err)
		// Fallback to a safe, non-personalized greeting on error
		greeting.WriteString("Hi there,")
	}

	content := GetParagraph(greeting.String()) +
		GetParagraph("Welcome to your new TractStack sandbox! This environment lets you experiment with all the features of TractStack before going live.") +
		GetButton(ButtonProps{
			Text: actionText,
			URL:  actionURL,
		}) +
		GetParagraph("Your sandbox is ready to use. You can create content, test your site, and prepare for launch.") +
		GetParagraph("If you have any questions, please reach out to our support team.")

	return content
}
