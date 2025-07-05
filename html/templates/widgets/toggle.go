// Package templates provides Toggle widget implementation
package templates

import (
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// RenderToggle renders a Shoelace switch component for binary belief toggles
func RenderToggle(ctx *models.RenderContext, classNames, slug, prompt string) string {
	// Get user's current beliefs
	userBeliefs := getUserBeliefs(ctx)
	currentBelief := getCurrentBeliefState(userBeliefs, slug)

	// Determine current toggle state
	isEnabled := getToggleState(currentBelief)

	// Generate unique ID for this widget instance
	switchID := fmt.Sprintf("toggle-switch-%s", slug)

	// Build the Shoelace switch component HTML
	html := fmt.Sprintf(`<div class="%s flex items-center mt-6" data-belief="%s">`, classNames, slug)

	html += fmt.Sprintf(`
    <sl-switch data-shoelace="switch" id="%s" name="beliefValue" %s class="focus:ring-2 focus:ring-cyan-600 focus:ring-offset-2" data-belief-id="%s" data-belief-type="Belief">
        <span>%s</span>
    </sl-switch>`,
		switchID,
		getCheckedAttribute(isEnabled),
		slug,
		prompt,
	)

	return html
}

func getToggleState(currentBelief *BeliefState) bool {
	if currentBelief == nil {
		return false
	}
	return currentBelief.Verb == "BELIEVES_YES"
}

func getCheckedAttribute(isEnabled bool) string {
	if isEnabled {
		return `checked`
	}
	return ""
}
