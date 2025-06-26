// Package templates provides IdentifyAs widget placeholder
package templates

import "fmt"

// RenderIdentifyAs renders an IdentifyAs widget placeholder
func RenderIdentifyAs(classNames, slug, target, extra string) string {
	return fmt.Sprintf(`<div class="%s" data-belief="%s"><div>IdentifyAs Widget: %s - %s - %s</div></div>`,
		classNames, slug, slug, target, extra)
}
