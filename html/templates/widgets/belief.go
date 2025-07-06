// Package templates provides Belief widget implementation
package templates

import (
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// RenderBelief renders an accessible native HTML select component for belief scales
func RenderBelief(ctx *models.RenderContext, classNames, slug, scale, extra string) string {
	userBeliefs := getUserBeliefs(ctx)
	currentBelief := getCurrentBeliefState(userBeliefs, slug)

	placeholder := getPlaceholderText(scale)
	actualOptions := filterPlaceholderOptions(getScaleOptions(scale))

	selectedValue := ""
	if currentBelief != nil {
		for _, option := range actualOptions {
			if option.Slug == currentBelief.Verb {
				selectedValue = option.Slug
				break
			}
		}
	}

	selectID := fmt.Sprintf("belief-select-%s", slug)
	labelID := fmt.Sprintf("belief-label-%s", slug)
	helpID := fmt.Sprintf("belief-help-%s", slug)

	html := fmt.Sprintf(`<div id="belief-container-%s" class="%s" data-belief="%s">`, slug, classNames, slug)

	// Add label for accessibility
	labelText := extra
	if labelText == "" {
		labelText = placeholder
	}
	html += fmt.Sprintf(`
    <label id="%s" for="%s" class="block text-sm font-medium text-gray-900 mb-2">
        %s
    </label>`, labelID, selectID, labelText)

	// Add accessible select with proper ARIA attributes
	html += fmt.Sprintf(`
    <select id="%s" 
            name="beliefValue" 
            class="block w-fit min-w-48 max-w-xs px-3 py-2 border border-gray-300 rounded-md shadow-sm bg-white text-sm focus:outline-none focus:ring-2 focus:ring-cyan-600 focus:border-cyan-600 disabled:bg-gray-50 disabled:text-gray-500" 
            data-belief-id="%s" 
            data-belief-type="Belief"
            aria-labelledby="%s"
            aria-describedby="%s"
            aria-required="false">`,
		selectID, slug, labelID, helpID)

	// Add default option that's properly accessible
	if selectedValue == "" {
		html += fmt.Sprintf(`
        <option value="" selected aria-label="No selection made">%s</option>`, placeholder)
	} else {
		html += fmt.Sprintf(`
        <option value="" aria-label="Reset to no selection">%s</option>`, placeholder)
	}

	// Add actual options with proper accessibility
	for _, option := range actualOptions {
		isSelected := option.Slug == selectedValue
		selectedAttr := ""
		if isSelected {
			selectedAttr = " selected"
		}

		html += fmt.Sprintf(`
        <option value="%s"%s aria-label="%s">
            %s
        </option>`,
			option.Slug,
			selectedAttr,
			option.Name,
			option.Name,
		)
	}

	html += `
    </select>`

	// Add help text for screen readers
	html += fmt.Sprintf(`
    <div id="%s" class="sr-only">
        Select your belief level. This choice affects the content you see and helps personalize your experience.
    </div>`, helpID)

	html += `
    </div>`

	return html
}

func getScaleOptions(scale string) []ScaleOption {
	switch scale {
	case "likert":
		return []ScaleOption{
			{ID: 0, Slug: "0", Name: "Choose your agreement level", Color: "bg-orange-500"},
			{ID: 1, Slug: "STRONGLY_AGREES", Name: "Strongly agree", Color: "bg-teal-400"},
			{ID: 2, Slug: "AGREES", Name: "Agree", Color: "bg-lime-400"},
			{ID: 3, Slug: "NEITHER_AGREES_NOR_DISAGREES", Name: "Neither agree nor disagree", Color: "bg-slate-200"},
			{ID: 4, Slug: "DISAGREES", Name: "Disagree", Color: "bg-amber-400"},
			{ID: 5, Slug: "STRONGLY_DISAGREES", Name: "Strongly disagree", Color: "bg-red-400"},
		}
	case "agreement":
		return []ScaleOption{
			{ID: 0, Slug: "0", Name: "Choose: Agree or Disagree?", Color: "bg-orange-500"},
			{ID: 1, Slug: "AGREES", Name: "Agree", Color: "bg-lime-400"},
			{ID: 2, Slug: "DISAGREES", Name: "Disagree", Color: "bg-amber-400"},
		}
	case "interest":
		return []ScaleOption{
			{ID: 0, Slug: "0", Name: "Are you interested?", Color: "bg-orange-500"},
			{ID: 1, Slug: "INTERESTED", Name: "Interested", Color: "bg-lime-400"},
			{ID: 2, Slug: "NOT_INTERESTED", Name: "Not Interested", Color: "bg-amber-400"},
		}
	case "yn":
		return []ScaleOption{
			{ID: 0, Slug: "0", Name: "Choose: Yes or No?", Color: "bg-orange-500"},
			{ID: 1, Slug: "BELIEVES_YES", Name: "Yes", Color: "bg-lime-400"},
			{ID: 2, Slug: "BELIEVES_NO", Name: "No", Color: "bg-amber-400"},
		}
	case "tf":
		return []ScaleOption{
			{ID: 0, Slug: "0", Name: "Choose: True or False?", Color: "bg-orange-500"},
			{ID: 1, Slug: "BELIEVES_TRUE", Name: "True", Color: "bg-lime-400"},
			{ID: 2, Slug: "BELIEVES_FALSE", Name: "False", Color: "bg-amber-400"},
		}
	default:
		return []ScaleOption{
			{ID: 0, Slug: "0", Name: "Choose an option", Color: "bg-orange-500"},
		}
	}
}

func getPlaceholderText(scale string) string {
	scaleOptions := getScaleOptions(scale)
	if len(scaleOptions) > 0 {
		return scaleOptions[0].Name
	}
	return "Choose an option"
}

func filterPlaceholderOptions(scaleOptions []ScaleOption) []ScaleOption {
	var actualOptions []ScaleOption
	for _, option := range scaleOptions {
		if option.ID != 0 {
			actualOptions = append(actualOptions, option)
		}
	}
	return actualOptions
}
