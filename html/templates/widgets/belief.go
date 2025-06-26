// Package templates provides Belief widget placeholder
package templates

import "fmt"

// RenderBelief renders a Belief widget placeholder
func RenderBelief(classNames, slug, scale, extra string) string {
	return fmt.Sprintf(`<div class="%s" data-belief="%s"><div>Belief Widget: %s - %s - %s</div></div>`,
		classNames, slug, slug, scale, extra)
}
