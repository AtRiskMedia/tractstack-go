// Package templates provides Toggle widget implementation
package templates

import (
	"bytes"
	"html/template"
	"log"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
)

var toggleWidgetTmpl = template.Must(template.New("toggleWidget").Parse(
	`{{define "wrapper"}}<div class="{{.ClassNames}} mt-6" data-belief="{{.Slug}}" data-pane-id="{{.PaneID}}">{{end}}` +

		`{{define "toggle"}}
    <div class="flex items-center">
        <input type="checkbox"
               id="{{.CheckboxID}}"
               name="beliefValue"
               {{if .IsEnabled}}checked{{end}}
               class="sr-only peer"
               data-belief-id="{{.Slug}}"
               data-belief-type="Belief"
               data-pane-id="{{.PaneID}}"
               role="switch"
               aria-checked="{{.IsEnabled}}"
               aria-labelledby="{{.LabelID}}"
               aria-describedby="{{.HelpID}}">

        <label for="{{.CheckboxID}}"
               class="relative inline-flex items-center cursor-pointer"
               id="{{.LabelID}}">
            <div class="w-11 h-6 bg-gray-200 peer-focus:outline-none peer-focus:ring-4 peer-focus:ring-cyan-300 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-cyan-600">
            </div>
            <span class="ml-3 text-sm text-gray-900">{{.Prompt}}</span>
        </label>
    </div>{{end}}` +

		`{{define "helpText"}}
    <div id="{{.HelpID}}" class="sr-only">
        Toggle switch. Currently {{.StateText}}. Press space to toggle.
    </div>{{end}}`,
))

// Data structs for template execution
type (
	toggleWrapperData struct{ ClassNames, Slug, PaneID string }
	toggleData        struct {
		CheckboxID, LabelID, HelpID, Slug, PaneID, Prompt string
		IsEnabled                                         bool
	}
)

// Renamed to avoid redeclaration error
type toggleHelpTextData struct{ HelpID, StateText string }

// RenderToggle renders an accessible native HTML checkbox toggle for binary belief toggles
func RenderToggle(ctx *rendering.RenderContext, classNames, slug, prompt string) string {
	userBeliefs := getUserBeliefs(ctx)
	currentBelief := getCurrentBeliefState(userBeliefs, slug)
	isEnabled := getToggleState(currentBelief)

	checkboxID := "toggle-checkbox-" + slug
	labelID := "toggle-label-" + slug
	helpID := "toggle-help-" + slug

	var buf bytes.Buffer

	// Render each part of the component using secure templates
	executeToggleTemplate(&buf, "wrapper", toggleWrapperData{ClassNames: classNames, Slug: slug, PaneID: ctx.ContainingPaneID})

	executeToggleTemplate(&buf, "toggle", toggleData{
		CheckboxID: checkboxID,
		LabelID:    labelID,
		HelpID:     helpID,
		Slug:       slug,
		PaneID:     ctx.ContainingPaneID,
		Prompt:     prompt,
		IsEnabled:  isEnabled,
	})

	// Use the correctly named struct with the correct field
	executeToggleTemplate(&buf, "helpText", toggleHelpTextData{HelpID: helpID, StateText: getToggleStateText(isEnabled)})

	buf.WriteString(`</div>`)

	return buf.String()
}

// executeToggleTemplate is a helper to render a named template and handle errors
func executeToggleTemplate(buf *bytes.Buffer, name string, data any) {
	err := toggleWidgetTmpl.ExecuteTemplate(buf, name, data)
	if err != nil {
		log.Printf("ERROR: Failed to execute toggle widget template '%s': %v", name, err)
		buf.WriteString("<!-- template error -->")
	}
}

// Helper functions do not generate HTML and remain unchanged

func getToggleState(currentBelief *BeliefState) bool {
	if currentBelief == nil {
		return false
	}
	switch currentBelief.Verb {
	case "BELIEVES_YES", "ENABLE", "ACTIVATE", "TOGGLE_ON":
		return true
	default:
		return false
	}
}

func getToggleStateText(isEnabled bool) string {
	if isEnabled {
		return "enabled"
	}
	return "disabled"
}
