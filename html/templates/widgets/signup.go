// Package templates provides SignUp widget placeholder
package templates

import "fmt"

// RenderSignUp renders a SignUp widget placeholder
func RenderSignUp(classNames, persona, prompt string, clarifyConsent bool) string {
	return fmt.Sprintf(`<div class="%s"><div>SignUp Widget: %s - %s (consent: %t)</div></div>`,
		classNames, persona, prompt, clarifyConsent)
}
