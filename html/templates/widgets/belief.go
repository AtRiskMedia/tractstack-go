// Package templates provides Belief widget implementation
package templates

import (
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// RenderBelief renders a Shoelace select component for belief scales
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

	html := fmt.Sprintf(`<div class="%s" data-belief="%s">`, classNames, slug)

	if extra != "" {
		html += fmt.Sprintf(`<span class="mr-2">%s</span>`, extra)
	}

	html += fmt.Sprintf(`
    <sl-select data-shoelace="select" id="%s" name="beliefValue" class="block mt-3 w-fit" value="%s" placeholder="%s" data-belief-id="%s" data-belief-type="Belief">`,
		selectID,
		selectedValue,
		placeholder,
		slug,
	)

	for _, option := range actualOptions {
		isSelected := option.Slug == selectedValue
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

func getItemClasses(isSelected bool) string {
	if isSelected {
		return "bg-slate-100 text-cyan-600"
	}
	return "hover:bg-slate-50"
}
