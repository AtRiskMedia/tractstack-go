// Package templates provides YouTube widget placeholder
package templates

import "fmt"

// RenderYouTube renders a YouTube widget placeholder
func RenderYouTube(classNames, embedCode, title string) string {
	return fmt.Sprintf(`<div class="%s"><div>YouTube Widget: %s - %s</div></div>`,
		classNames, embedCode, title)
}
