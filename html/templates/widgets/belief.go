// Package templates provides Belief widget implementation
package templates

import (
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// RenderBelief renders a Shoelace select component for belief scales
func RenderBelief(ctx *models.RenderContext, classNames, slug, scale, extra string) string {
	// Get user's current beliefs
	userBeliefs := getUserBeliefs(ctx)
	currentBelief := getCurrentBeliefState(userBeliefs, slug)

	// Determine scale options based on scale type
	scaleOptions := getScaleOptions(scale)
	selectedOption := findSelectedOption(currentBelief, scaleOptions)

	// Generate unique ID for this widget instance
	selectID := fmt.Sprintf("belief-select-%s", slug)

	// Determine session handling strategy
	sessionStrategy := getSessionStrategy(ctx)

	// Build the Shoelace select component HTML
	html := fmt.Sprintf(`<div class="%s" data-belief="%s">`, classNames, slug)

	// Add extra text if provided
	if extra != "" {
		html += fmt.Sprintf(`<span class="mr-2">%s</span>`, extra)
	}

	// Build sl-select component
	html += fmt.Sprintf(`
        <sl-select data-shoelace="select" id="%s" name="belief" class="block mt-3 w-fit %s" value="%s" hx-post="' + window.TRACTSTACK_CONFIG.backendUrl + '/api/v1/state" hx-vals='{"event": {"id": "%s", "type": "Belief", "verb": "{{this.value}}"}}' %s style="--option-border-color: %s;">`,
		selectID,
		getBorderColorClass(selectedOption),
		selectedOption.Slug,
		slug,
		sessionStrategy,
		getBorderColorStyle(selectedOption),
	)

	// Add scale options
	for _, option := range scaleOptions {
		isSelected := option.Slug == selectedOption.Slug
		html += fmt.Sprintf(`
            <sl-option value="%s" class="%s">
                <span class="%s inline-block h-2 w-2 flex-shrink-0 rounded-full mr-2" aria-hidden="true"></span>
                %s
            </sl-option>`,
			option.Slug,
			getItemClasses(isSelected),
			option.Color,
			option.Name,
		)
	}

	html += `
        </sl-select>
    </div>`

	return html
}

func getScaleOptions(scale string) []ScaleOption {
	switch scale {
	case "likert":
		return []ScaleOption{
			{ID: 0, Slug: "0", Name: "Agree or Disagree?", Color: "bg-orange-500"},
			{ID: 1, Slug: "STRONGLY_AGREES", Name: "Strongly agree", Color: "bg-teal-400"},
			{ID: 2, Slug: "AGREES", Name: "Agree", Color: "bg-lime-400"},
			{ID: 3, Slug: "NEITHER_AGREES_NOR_DISAGREES", Name: "Neither agree nor disagree", Color: "bg-slate-200"},
			{ID: 4, Slug: "DISAGREES", Name: "Disagree", Color: "bg-amber-400"},
			{ID: 5, Slug: "STRONGLY_DISAGREES", Name: "Strongly disagree", Color: "bg-red-400"},
		}
	case "agreement":
		return []ScaleOption{
			{ID: 0, Slug: "0", Name: "Agree or Disagree?", Color: "bg-orange-500"},
			{ID: 1, Slug: "AGREES", Name: "Agree", Color: "bg-lime-400"},
			{ID: 2, Slug: "DISAGREES", Name: "Disagree", Color: "bg-amber-400"},
		}
	case "interest":
		return []ScaleOption{
			{ID: 0, Slug: "0", Name: "Are you Interested?", Color: "bg-orange-500"},
			{ID: 1, Slug: "INTERESTED", Name: "Interested", Color: "bg-lime-400"},
			{ID: 2, Slug: "NOT_INTERESTED", Name: "Not Interested", Color: "bg-amber-400"},
		}
	case "yn":
		return []ScaleOption{
			{ID: 0, Slug: "0", Name: "Yes or No?", Color: "bg-orange-500"},
			{ID: 1, Slug: "BELIEVES_YES", Name: "Yes", Color: "bg-lime-400"},
			{ID: 2, Slug: "BELIEVES_NO", Name: "No", Color: "bg-amber-400"},
		}
	case "tf":
		return []ScaleOption{
			{ID: 0, Slug: "0", Name: "True or False?", Color: "bg-orange-500"},
			{ID: 1, Slug: "BELIEVES_TRUE", Name: "True", Color: "bg-lime-400"},
			{ID: 2, Slug: "BELIEVES_FALSE", Name: "False", Color: "bg-amber-400"},
		}
	default:
		return []ScaleOption{
			{ID: 0, Slug: "0", Name: "Choose an option", Color: "bg-orange-500"},
		}
	}
}

func findSelectedOption(currentBelief *BeliefState, scaleOptions []ScaleOption) ScaleOption {
	// Default to first option (the prompt)
	defaultOption := scaleOptions[0]

	if currentBelief == nil {
		return defaultOption
	}

	// Find matching option based on verb
	for _, option := range scaleOptions {
		if option.Slug == currentBelief.Verb {
			return option
		}
	}

	return defaultOption
}

func getBorderColorClass(option ScaleOption) string {
	if option.ID == 0 {
		return "border-slate-200"
	}
	// Extract color from bg-color-shade format and convert to border
	colorPart := strings.TrimPrefix(option.Color, "bg-")
	return "border-" + colorPart
}

func getBorderColorStyle(option ScaleOption) string {
	if option.ID == 0 {
		return "#e5e7eb" // Matches border-slate-200
	}
	switch option.Color {
	case "bg-orange-500":
		return "#f97316"
	case "bg-teal-400":
		return "#2dd4bf"
	case "bg-lime-400":
		return "#a3e635"
	case "bg-slate-200":
		return "#e5e7eb"
	case "bg-amber-400":
		return "#f59e0b"
	case "bg-red-400":
		return "#f87171"
	default:
		return "#f97316" // Fallback to orange-500
	}
}

func getItemClasses(isSelected bool) string {
	if isSelected {
		return "bg-slate-100 text-cyan-600"
	}
	return "hover:bg-slate-50"
}
