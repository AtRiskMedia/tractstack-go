// Package templates provides Bunny video widget placeholder
package templates

import (
	"bytes"
	"html/template"
	"log"
)

var bunnyWidgetTmpl = template.Must(template.New("bunnyWidget").Parse(
	`<div class="{{.ClassNames}}"><div>Bunny Video Widget: {{.VideoURL}} - {{.Title}}</div></div>`,
))

type bunnyWidgetData struct {
	ClassNames string
	VideoURL   string
	Title      string
}

// RenderBunny renders a Bunny video widget placeholder securely
func RenderBunny(classNames, videoURL, title string) string {
	data := bunnyWidgetData{
		ClassNames: classNames,
		VideoURL:   videoURL,
		Title:      title,
	}

	var buf bytes.Buffer
	err := bunnyWidgetTmpl.Execute(&buf, data)
	if err != nil {
		log.Printf("ERROR: Failed to execute bunny widget template: %v", err)
		return `<!-- template error -->`
	}

	return buf.String()
}
