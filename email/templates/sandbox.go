// Package templates provides email sandbox template
package templates

import "fmt"

type SandboxEmailProps struct {
	Name       string
	ActionURL  string
	ActionText string
}

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

	content := GetParagraph(fmt.Sprintf("Hi %s,", name)) +
		GetParagraph("Welcome to your new TractStack sandbox! This environment lets you experiment with all the features of TractStack before going live.") +
		GetButton(ButtonProps{
			Text: actionText,
			URL:  actionURL,
		}) +
		GetParagraph("Your sandbox is ready to use. You can create content, test your site, and prepare for launch.") +
		GetParagraph("If you have any questions, please reach out to our support team.")

	return content
}
