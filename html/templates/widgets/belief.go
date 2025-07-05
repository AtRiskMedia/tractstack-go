// Package templates provides Belief widget implementation
package templates

import (
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// RenderBelief renders a Zag.js select component for belief scales
func RenderBelief(ctx *models.RenderContext, classNames, slug, scale, extra string) string {
	// Get user's current beliefs
	userBeliefs := getUserBeliefs(ctx)
	currentBelief := getCurrentBeliefState(userBeliefs, slug)

	// Determine scale options based on scale type
	scaleOptions := getScaleOptions(scale)
	selectedOption := findSelectedOption(currentBelief, scaleOptions)

	// Generate unique IDs for this widget instance
	selectID := fmt.Sprintf("belief-select-%s", slug)
	triggerID := fmt.Sprintf("belief-trigger-%s", slug)
	contentID := fmt.Sprintf("belief-content-%s", slug)

	// Determine session handling strategy
	sessionStrategy := getSessionStrategy(ctx)

	// Build the Zag.js select component HTML
	html := fmt.Sprintf(`<div class="%s" data-belief="%s">`, classNames, slug)

	// Add extra text if provided
	if extra != "" {
		html += fmt.Sprintf(`<span class="mr-2">%s</span>`, extra)
	}

	// Build select component with Zag.js attributes
	html += fmt.Sprintf(`
	<div class="block mt-3 w-fit">
		<div data-scope="select" data-part="root" id="%s">
			<button 
				data-part="trigger" 
				id="%s"
				class="relative w-full cursor-default rounded-md border %s bg-white text-black py-2 pl-3 pr-10 text-left shadow-sm focus:border-orange-500 focus:outline-none focus:ring-1 focus:ring-orange-500"
				aria-expanded="false"
				aria-haspopup="listbox"
			>
				<span class="flex items-center">
					<span 
						aria-label="Color swatch for belief"
						class="motion-safe:animate-pulse %s inline-block h-2 w-2 flex-shrink-0 rounded-full"
					></span>
					<span class="ml-3 block truncate" data-part="value-text">%s</span>
				</span>
				<span class="pointer-events-none absolute inset-y-0 right-0 flex items-center pr-2">
					<svg class="h-5 w-5 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 9l4-4 4 4m0 6l-4 4-4-4"></path>
					</svg>
				</span>
			</button>
			
			<div 
				data-part="positioner"
				style="position: absolute; z-index: 50;"
			>
				<div 
					data-part="content" 
					id="%s"
					class="absolute mt-1 max-h-60 overflow-auto rounded-md bg-white py-1 text-sm shadow-lg ring-1 ring-black ring-opacity-5 focus:outline-none z-50"
					style="display: none;"
				>`,
		selectID,
		triggerID,
		getBorderColorClass(selectedOption),
		getColorClass(selectedOption),
		getDisplayText(selectedOption),
		contentID,
	)

	// Add scale options
	for _, option := range scaleOptions {
		isSelected := option.Slug == selectedOption.Slug
		html += fmt.Sprintf(`
					<div 
						data-part="item" 
						data-value="%s"
						class="belief-item relative cursor-default select-none py-2 pl-3 pr-9 text-black %s"
      hx-post="' + window.TRACTSTACK_CONFIG.backendUrl + '/api/v1/state"
						hx-vals='{"event": {"id": "%s", "type": "Belief", "verb": "%s"}}'
						%s
					>
						<div class="flex items-center">
							<span class="%s inline-block h-2 w-2 flex-shrink-0 rounded-full" aria-hidden="true"></span>
							<span class="ml-3 block truncate">%s</span>
						</div>
						%s
					</div>`,
			option.Slug,
			getItemClasses(isSelected),
			slug,
			getVerbForOption(option),
			sessionStrategy,
			option.Color,
			option.Name,
			getCheckIcon(isSelected),
		)
	}

	html += `
				</div>
			</div>
		</div>
	</div>
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

func getColorClass(option ScaleOption) string {
	return option.Color
}

func getDisplayText(option ScaleOption) string {
	return option.Name
}

func getItemClasses(isSelected bool) string {
	if isSelected {
		return "bg-slate-100 text-cyan-600"
	}
	return "hover:bg-slate-50"
}

func getVerbForOption(option ScaleOption) string {
	if option.ID == 0 {
		return "UNSET"
	}
	return option.Slug
}

func getCheckIcon(isSelected bool) string {
	if isSelected {
		return `<span class="absolute inset-y-0 right-0 flex items-center px-2 text-cyan-600">
					<svg class="h-5 w-5" fill="currentColor" viewBox="0 0 20 20">
						<path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd"></path>
					</svg>
				</span>`
	}
	return ""
}
