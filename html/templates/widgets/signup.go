// Package templates provides SignUp widget placeholder
package templates

import (
	"bytes"
	"html/template"
	"log"
)

var signUpWidgetTmpl = template.Must(template.New("signUpWidget").Parse(
	`<div class="{{.ClassNames}}"><div>SignUp Widget: {{.Persona}} - {{.Prompt}} (consent: {{.ClarifyConsent}})</div></div>`,
))

type signUpWidgetData struct {
	ClassNames     string
	Persona        string
	Prompt         string
	ClarifyConsent bool
}

// RenderSignUp renders a SignUp widget placeholder securely
func RenderSignUp(classNames, persona, prompt string, clarifyConsent bool) string {
	data := signUpWidgetData{
		ClassNames:     classNames,
		Persona:        persona,
		Prompt:         prompt,
		ClarifyConsent: clarifyConsent,
	}

	var buf bytes.Buffer
	err := signUpWidgetTmpl.Execute(&buf, data)
	if err != nil {
		log.Printf("ERROR: Failed to execute signUp widget template: %v", err)
		return `<!-- template error -->`
	}

	return buf.String()
}
