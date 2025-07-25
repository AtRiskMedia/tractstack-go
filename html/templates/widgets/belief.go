// Package templates provides Belief widget implementation
package templates

import (
	"bytes"
	"html/template"
	"log"

	"github.com/AtRiskMedia/tractstack-go/models"
)

var beliefWidgetTmpl = template.Must(template.New("beliefWidget").Parse(
	`{{define "wrapper"}}<div id="belief-container-{{.Slug}}" class="{{.ClassNames}}" data-belief="{{.Slug}}">{{end}}` +

		`{{define "label"}}<label id="{{.LabelID}}" for="{{.SelectID}}">{{.LabelText}}</label>{{end}}` +

		`{{define "selectTag"}}<select id="{{.SelectID}}" name="beliefValue" class="my-6 block w-fit min-w-48 max-w-xs px-3 py-2 border border-gray-300 rounded-md shadow-sm bg-white text-sm focus:outline-none focus:ring-2 focus:ring-cyan-600 focus:border-cyan-600 disabled:bg-gray-50 disabled:text-gray-500" data-belief-id="{{.Slug}}" data-belief-type="Belief" data-pane-id="{{.PaneID}}" aria-labelledby="{{.LabelID}}">{{end}}` +

		`{{define "placeholderOption"}}<option value=""{{if .IsSelected}} selected{{end}} aria-label="{{.AriaLabel}}">{{.Text}}</option>{{end}}` +

		`{{define "valueOption"}}<option value="{{.Slug}}"{{if .IsSelected}} selected{{end}} aria-label="{{.Name}}">{{.Name}}</option>{{end}}` +

		`{{define "helpText"}}<div id="{{.HelpID}}" class="sr-only">Select your belief level. This choice affects the content you see and helps personalize your experience.</div>{{end}}`,
))

// Data structs for template execution
type (
	beliefWrapperData     struct{ Slug, ClassNames string }
	beliefLabelData       struct{ LabelID, SelectID, LabelText string }
	beliefSelectData      struct{ SelectID, Slug, PaneID, LabelID string }
	placeholderOptionData struct {
		Text, AriaLabel string
		IsSelected      bool
	}
)
type valueOptionData struct {
	Slug, Name string
	IsSelected bool
}
type helpTextData struct{ HelpID string }

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

	selectID := "belief-select-" + slug
	labelID := "belief-label-" + slug
	helpID := "belief-help-" + slug

	var buf bytes.Buffer

	// Render each part of the component using secure templates
	executeBeliefTemplate(&buf, "wrapper", beliefWrapperData{Slug: slug, ClassNames: classNames})

	labelText := extra
	if labelText == "" {
		labelText = placeholder
	}
	executeBeliefTemplate(&buf, "label", beliefLabelData{LabelID: labelID, SelectID: selectID, LabelText: labelText})

	executeBeliefTemplate(&buf, "selectTag", beliefSelectData{SelectID: selectID, Slug: slug, PaneID: ctx.ContainingPaneID, LabelID: labelID})

	// Render placeholder option
	if selectedValue == "" {
		executeBeliefTemplate(&buf, "placeholderOption", placeholderOptionData{Text: placeholder, AriaLabel: "No selection made", IsSelected: true})
	} else {
		executeBeliefTemplate(&buf, "placeholderOption", placeholderOptionData{Text: placeholder, AriaLabel: "Reset to no selection", IsSelected: false})
	}

	// Render actual value options
	for _, option := range actualOptions {
		executeBeliefTemplate(&buf, "valueOption", valueOptionData{
			Slug:       option.Slug,
			Name:       option.Name,
			IsSelected: option.Slug == selectedValue,
		})
	}

	buf.WriteString(`</select>`)

	executeBeliefTemplate(&buf, "helpText", helpTextData{HelpID: helpID})

	buf.WriteString(`</div>`)

	return buf.String()
}

// executeBeliefTemplate is a helper to render a named template and handle errors
func executeBeliefTemplate(buf *bytes.Buffer, name string, data interface{}) {
	err := beliefWidgetTmpl.ExecuteTemplate(buf, name, data)
	if err != nil {
		log.Printf("ERROR: Failed to execute belief widget template '%s': %v", name, err)
		buf.WriteString("<!-- template error -->")
	}
}

// The helper functions below do not generate HTML and remain unchanged.

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
