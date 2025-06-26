// Package templates provides Toggle widget placeholder
package templates

import "fmt"

// RenderToggle renders a Toggle belief widget placeholder
func RenderToggle(classNames, belief, prompt string) string {
	return fmt.Sprintf(`<div class="%s" data-belief="%s"><div>Toggle Widget: %s - %s</div></div>`,
		classNames, belief, belief, prompt)
}
