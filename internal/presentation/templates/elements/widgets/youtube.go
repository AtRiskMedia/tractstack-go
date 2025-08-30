// Package templates provides YouTube widget placeholder
package templates

import (
	"bytes"
	"html/template"
	"log"
)

var youtubeWidgetTmpl = template.Must(template.New("youtubeWidget").Parse(
	`<div class="{{.ClassNames}}"><div>YouTube Widget: {{.EmbedCode}} - {{.Title}}</div></div>`,
))

type youtubeWidgetData struct {
	ClassNames string
	EmbedCode  string
	Title      string
}

// RenderYouTube renders a YouTube widget placeholder securely
func RenderYouTube(classNames, embedCode, title string) string {
	data := youtubeWidgetData{
		ClassNames: classNames,
		EmbedCode:  embedCode,
		Title:      title,
	}

	var buf bytes.Buffer
	err := youtubeWidgetTmpl.Execute(&buf, data)
	if err != nil {
		log.Printf("ERROR: Failed to execute youtube widget template: %v", err)
		return `<!-- template error -->`
	}

	return buf.String()
}
