// Package templates provides Resource widget placeholder
package templates

import "fmt"

// RenderResource renders a Resource widget placeholder
func RenderResource(classNames, value1, value2 string) string {
	return fmt.Sprintf(`<div class="%s"><div><strong>Resource Template (not yet implemented):</strong> %s, %s</div></div>`,
		classNames, value1, value2)
}
