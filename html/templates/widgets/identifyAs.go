// Package templates provides IdentifyAs widget implementation
package templates

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// RenderIdentifyAs renders button group components for exclusive selection widgets
func RenderIdentifyAs(ctx *models.RenderContext, classNames, slug, targets, extra string) string {
	// Parse comma-separated targets
	targetsList := parseTargets(targets)
	if len(targetsList) == 0 {
		return ""
	}

	// Get user's current beliefs
	userBeliefs := getUserBeliefs(ctx)
	currentBelief := getCurrentBeliefState(userBeliefs, slug)

	// Determine current selection state
	selectedTarget := getSelectedTarget(currentBelief)
	isOtherSelected := isOtherTargetSelected(currentBelief, targetsList)

	// Build the widget HTML
	html := fmt.Sprintf(`<div class="%s" data-belief="%s" data-pane-id="%s">`,
		classNames, slug, ctx.ContainingPaneID)

	// Add extra text if provided (unless it's empty which means noprompt)
	if extra != "" {
		html += fmt.Sprintf(`<span class="mr-2">%s</span>`, extra)
	}

	// Container for button group
	html += `<div class="flex flex-wrap gap-2">`

	// Generate a button for each target
	for _, target := range targetsList {
		html += renderIdentifyAsButton(slug, target, selectedTarget, isOtherSelected, extra == "", ctx)
	}

	html += `</div></div>`

	return html
}

// renderIdentifyAsButton generates a single button for the IdentifyAs widget
func renderIdentifyAsButton(beliefSlug, target, selectedTarget string, isOtherSelected bool, noprompt bool, ctx *models.RenderContext) string {
	// Determine button state
	isSelected := target == selectedTarget
	buttonTitle := getButtonTitle(target, noprompt)

	// Generate unique ID for this button
	buttonID := fmt.Sprintf("identifyas-%s-%s", beliefSlug, sanitizeID(target))

	// Determine button classes and indicator color
	buttonClasses := getIdentifyAsButtonClasses(isSelected, isOtherSelected)
	indicatorColor := getIdentifyAsIndicatorColor(isSelected, isOtherSelected)

	// Create JSON for hx-vals using proper marshaling
	hxValsMap := map[string]string{
		"beliefId":     beliefSlug,
		"beliefType":   "Belief",
		"beliefObject": target,
		"paneId":       ctx.ContainingPaneID,
	}
	hxValsBytes, _ := json.Marshal(hxValsMap)
	hxVals := html.EscapeString(string(hxValsBytes))

	return fmt.Sprintf(`
		<div class="block mt-3 w-fit">
			<button
				type="button"
				id="%s"
				class="%s rounded-md px-3 py-2 text-lg text-black shadow-sm ring-1 ring-inset"
				hx-post="/api/v1/state"
				hx-trigger="click"
				hx-swap="none"
				hx-vals='%s'
				hx-preserve="true"
			>
				<div class="flex items-center">
					<span
						aria-label="Color swatch for belief"
						class="motion-safe:animate-pulse %s inline-block h-2 w-2 flex-shrink-0 rounded-full"
					></span>
					<span class="ml-3 block whitespace-normal text-left w-fit">%s</span>
				</div>
			</button>
		</div>`,
		buttonID, buttonClasses, hxVals, indicatorColor, buttonTitle)
}

func parseTargets(targetsString string) []string {
	if targetsString == "" {
		return []string{}
	}

	rawTargets := strings.Split(targetsString, ",")
	var targets []string

	for _, target := range rawTargets {
		trimmed := strings.TrimSpace(target)
		if trimmed != "" {
			targets = append(targets, trimmed)
		}
	}

	return targets
}

func getSelectedTarget(currentBelief *BeliefState) string {
	if currentBelief == nil || currentBelief.Verb != "IDENTIFY_AS" {
		return ""
	}
	return currentBelief.Object
}

func isOtherTargetSelected(currentBelief *BeliefState, targets []string) bool {
	if currentBelief == nil || currentBelief.Verb != "IDENTIFY_AS" {
		return false
	}

	selectedTarget := currentBelief.Object
	if selectedTarget == "" {
		return false
	}

	// Check if the selected target is NOT in our targets list
	for _, target := range targets {
		if target == selectedTarget {
			return false // Selected target is in our list
		}
	}

	return true // Selected target is not in our list (other selected)
}

func getButtonTitle(target string, noprompt bool) string {
	if noprompt {
		return "Tell me more!"
	}
	return target
}

func sanitizeID(target string) string {
	// Replace spaces and special characters with hyphens for valid HTML IDs
	sanitized := strings.ReplaceAll(target, " ", "-")
	sanitized = strings.ReplaceAll(sanitized, ".", "-")
	sanitized = strings.ReplaceAll(sanitized, ",", "-")
	sanitized = strings.ReplaceAll(sanitized, "'", "-")
	sanitized = strings.ReplaceAll(sanitized, "\"", "-")
	return strings.ToLower(sanitized)
}

func getIdentifyAsButtonClasses(isSelected, isOtherSelected bool) string {
	if isSelected {
		return "bg-gray-100 ring-lime-500"
	}

	if isOtherSelected {
		return "bg-gray-300 hover:bg-lime-200 ring-gray-500"
	}

	return "bg-gray-100 hover:bg-orange-200 ring-orange-500"
}

func getIdentifyAsIndicatorColor(isSelected, isOtherSelected bool) string {
	if isSelected {
		return "bg-lime-400" // Selected state
	}

	if isOtherSelected {
		return "bg-gray-500" // Other target selected
	}

	return "bg-orange-500" // Default/unselected state
}

func getIdentifyAsVerb(isSelected bool) string {
	if isSelected {
		return "UNSET" // Toggle off if already selected
	}
	return "IDENTIFY_AS" // Select this target
}
