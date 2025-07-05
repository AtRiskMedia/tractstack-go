// Package templates provides Toggle widget implementation
package templates

import (
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// RenderToggle renders a Zag.js switch component for binary belief toggles
func RenderToggle(ctx *models.RenderContext, classNames, slug, prompt string) string {
	// Get user's current beliefs
	userBeliefs := getUserBeliefs(ctx)
	currentBelief := getCurrentBeliefState(userBeliefs, slug)

	// Determine current toggle state
	isEnabled := getToggleState(currentBelief)

	// Generate unique IDs for this widget instance
	switchID := fmt.Sprintf("toggle-switch-%s", slug)
	controlID := fmt.Sprintf("toggle-control-%s", slug)
	thumbID := fmt.Sprintf("toggle-thumb-%s", slug)
	labelID := fmt.Sprintf("toggle-label-%s", slug)

	// Determine session handling strategy
	sessionStrategy := getSessionStrategy(ctx)

	// Build the Zag.js switch component HTML
	html := fmt.Sprintf(`<div class="%s flex items-center mt-6" data-belief="%s">`, classNames, slug)

	html += fmt.Sprintf(`
		<div data-scope="switch" data-part="root" id="%s" class="inline-flex items-center">
			<button
				data-part="control"
				id="%s"
				type="button"
				role="switch"
				aria-checked="%t"
				aria-labelledby="%s"
				class="relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent %s transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-cyan-600 focus:ring-offset-2"
      hx-post="' + window.TRACTSTACK_CONFIG.backendUrl + '/api/v1/state"
				hx-vals='{"event": {"id": "%s", "type": "Belief", "verb": "%s"}}'
				%s
			>
				<span
					data-part="thumb"
					id="%s"
					class="pointer-events-none inline-block h-5 w-5 rounded-full bg-white shadow ring-0 transition-transform duration-200 ease-in-out %s"
				></span>
			</button>
			
			<input 
				data-part="hidden-input"
				type="checkbox" 
				%s
				class="sr-only"
			/>
			
			<div class="flex items-center h-6 ml-3">
				<label 
					data-part="label"
					id="%s"
					for="%s"
					class="cursor-pointer"
				>
					<span>%s</span>
				</label>
			</div>
		</div>
	</div>`,
		switchID,
		controlID,
		isEnabled,
		labelID,
		getSwitchBackgroundClass(isEnabled),
		slug,
		getToggleVerb(isEnabled),
		sessionStrategy,
		thumbID,
		getThumbPositionClass(isEnabled),
		getCheckedAttribute(isEnabled),
		labelID,
		controlID,
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

func getSwitchBackgroundClass(isEnabled bool) string {
	if isEnabled {
		return "bg-cyan-600"
	}
	return "bg-blue-600" // myblue equivalent
}

func getThumbPositionClass(isEnabled bool) string {
	if isEnabled {
		return "translate-x-5"
	}
	return "translate-x-0 motion-safe:animate-pulse" // animate-wig equivalent
}

func getToggleVerb(isEnabled bool) string {
	if isEnabled {
		return "BELIEVES_NO" // Toggle to opposite state
	}
	return "BELIEVES_YES"
}

func getCheckedAttribute(isEnabled bool) string {
	if isEnabled {
		return `checked="checked"`
	}
	return ""
}
