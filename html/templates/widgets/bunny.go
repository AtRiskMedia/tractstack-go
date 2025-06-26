// Package templates provides Bunny video widget placeholder
package templates

import "fmt"

// RenderBunny renders a Bunny video widget placeholder
func RenderBunny(classNames, videoURL, title string) string {
	return fmt.Sprintf(`<div class="%s"><div>Bunny Video Widget: %s - %s</div></div>`,
		classNames, videoURL, title)
}
