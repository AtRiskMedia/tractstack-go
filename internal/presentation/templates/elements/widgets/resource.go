// Package templates provides Resource widget placeholder
package templates

import (
	"bytes"
	"html/template"
	"log"
)

var resourceWidgetTmpl = template.Must(template.New("resourceWidget").Parse(
	`<div class="{{.ClassNames}}"><div><strong>Resource Template (not yet implemented):</strong> {{.Value1}}, {{.Value2}}</div></div>`,
))

type resourceWidgetData struct {
	ClassNames string
	Value1     string
	Value2     string
}

// RenderResource renders a Resource widget placeholder securely
func RenderResource(classNames, value1, value2 string) string {
	data := resourceWidgetData{
		ClassNames: classNames,
		Value1:     value1,
		Value2:     value2,
	}

	var buf bytes.Buffer
	err := resourceWidgetTmpl.Execute(&buf, data)
	if err != nil {
		log.Printf("ERROR: Failed to execute resource widget template: %v", err)
		return `<!-- template error -->`
	}

	return buf.String()
}
