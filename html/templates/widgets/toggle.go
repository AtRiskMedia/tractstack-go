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
	html := fmt.Sprintf(`<div class="%s mt-6" data-belief="%s">`, classNames, slug)

	// Accessible toggle with proper ARIA attributes
	html += fmt.Sprintf(`
    <div class="flex items-center">
        <input type="checkbox" 
               id="%s" 
               name="beliefValue" 
               %s 
               class="sr-only peer" 
               data-belief-id="%s" 
               data-belief-type="Belief"
               role="switch"
               aria-checked="%s"
               aria-labelledby="%s"
               aria-describedby="%s">
        
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
		getBooleanString(isEnabled),
		labelID,
		helpID,
		checkboxID,
		labelID,
		prompt,
	)

	// Add screen reader help text
	html += fmt.Sprintf(`
    <div id="%s" class="sr-only">
        Toggle switch. Currently %s. Press space to toggle.
    </div>`, helpID, getStateDescription(isEnabled))

	html += `
    </div>

    <style>
    /* Enhanced focus styles for accessibility */
    .peer:focus + label > div {
        box-shadow: 0 0 0 2px #ffffff, 0 0 0 4px #0891b2;
    }
    
    /* High contrast mode support */
    @media (prefers-contrast: high) {
        .peer:checked + label > div {
            background-color: #000000;
        }
        .peer + label > div::after {
            border-color: #000000;
        }
    }
    
    /* Reduced motion support */
    @media (prefers-reduced-motion: reduce) {
        .peer + label > div::after {
            transition: none;
        }
    }
    </style>`

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

func getBooleanString(isEnabled bool) string {
	if isEnabled {
		return "true"
	}
	return "false"
}

func getStateDescription(isEnabled bool) string {
	if isEnabled {
		return "enabled"
	}
	return "disabled"
}
