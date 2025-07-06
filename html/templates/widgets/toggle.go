// Package templates provides Toggle widget implementation
package templates

import (
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// RenderToggle renders an accessible native HTML checkbox toggle for binary belief toggles
func RenderToggle(ctx *models.RenderContext, classNames, slug, prompt string) string {
	// Get user's current beliefs
	userBeliefs := getUserBeliefs(ctx)
	currentBelief := getCurrentBeliefState(userBeliefs, slug)

	// Determine current toggle state
	isEnabled := getToggleState(currentBelief)

	// Generate unique IDs for accessibility
	checkboxID := fmt.Sprintf("toggle-checkbox-%s", slug)
	labelID := fmt.Sprintf("toggle-label-%s", slug)
	helpID := fmt.Sprintf("toggle-help-%s", slug)

	// Build the accessible HTML toggle
	html := fmt.Sprintf(`<div class="%s mt-6" data-belief="%s" data-pane-id="%s">`, classNames, slug, ctx.ContainingPaneID)

	// Accessible toggle with proper ARIA attributes and HTMX integration
	html += fmt.Sprintf(`
    <div class="flex items-center">
        <input type="checkbox" 
               id="%s" 
               name="beliefValue" 
               %s 
               class="sr-only peer" 
               data-belief-id="%s" 
               data-belief-type="Belief"
               data-pane-id="%s"
               role="switch"
               aria-checked="%s"
               aria-labelledby="%s"
               aria-describedby="%s"
               hx-post="/api/v1/state"
               hx-trigger="change"
               hx-swap="none"
               hx-vals='{"beliefId": "%s", "beliefType": "Belief", "paneId": "%s"}'
               hx-include="[name='beliefValue']">
        
        <!-- Visual toggle switch -->
        <label for="%s" 
               class="relative inline-flex items-center cursor-pointer"
               id="%s">
            <!-- Toggle track -->
            <div class="w-11 h-6 bg-gray-200 peer-focus:outline-none peer-focus:ring-4 peer-focus:ring-cyan-300 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-cyan-600">
            </div>
            
            <!-- Label text -->
            <span class="ml-3 text-sm text-gray-900">%s</span>
        </label>
    </div>`,
		checkboxID,
		getCheckedAttribute(isEnabled),
		slug,
		ctx.ContainingPaneID,
		getBooleanString(isEnabled),
		labelID,
		helpID,
		slug,
		ctx.ContainingPaneID,
		checkboxID,
		labelID,
		prompt,
	)

	// Add screen reader help text
	html += fmt.Sprintf(`
    <div id="%s" class="sr-only">
        Toggle switch. Currently %s. Press space to toggle.
    </div>`,
		helpID,
		getToggleStateText(isEnabled),
	)

	html += `</div>`

	return html
}

// getToggleState determines if the toggle should be checked based on current belief
func getToggleState(currentBelief *BeliefState) bool {
	if currentBelief == nil {
		return false
	}

	// Check for positive belief verbs that indicate "enabled"
	switch currentBelief.Verb {
	case "BELIEVES_YES", "ENABLE", "ACTIVATE", "TOGGLE_ON":
		return true
	default:
		return false
	}
}

// getCheckedAttribute returns the checked attribute string if enabled
func getCheckedAttribute(isEnabled bool) string {
	if isEnabled {
		return "checked"
	}
	return ""
}

// getBooleanString converts boolean to string for ARIA attributes
func getBooleanString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

// getToggleStateText returns human-readable state for screen readers
func getToggleStateText(isEnabled bool) string {
	if isEnabled {
		return "enabled"
	}
	return "disabled"
}
