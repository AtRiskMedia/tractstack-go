// Package templates provides IdentifyAs widget implementation
package templates

import (
	"bytes"
	"encoding/json"
	"html/template"
	"log"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
)

var identifyAsTmpl = template.Must(template.New("identifyAs").Parse(
	`{{define "widgetWrapper"}}<div class="{{.ClassNames}}" data-belief="{{.Slug}}" data-pane-id="{{.PaneID}}">{{if .Extra}}<span class="mr-2">{{.Extra}}</span>{{end}}<div class="flex flex-wrap gap-2">{{end}}` +

		`{{define "button"}}
		<div class="block mt-3 w-fit">
			<button
				type="button"
				id="{{.ID}}"
				class="{{.Classes}} rounded-md px-3 py-2 text-lg text-black shadow-sm ring-1 ring-inset"
				hx-post="/api/v1/state"
				hx-trigger="click"
				hx-swap="none"
				hx-vals="{{.HxVals}}"
				hx-preserve="true"
			>
				<div class="flex items-center">
					<span
						aria-label="Color swatch for belief"
						class="motion-safe:animate-pulse {{.IndicatorColor}} inline-block h-2 w-2 flex-shrink-0 rounded-full"
					></span>
					<span class="ml-3 block whitespace-normal text-left w-fit">{{.Title}}</span>
				</div>
			</button>
		</div>{{end}}`,
))

type widgetWrapperData struct {
	ClassNames string
	Slug       string
	PaneID     string
	Extra      string
}

type buttonData struct {
	ID             string
	Classes        string
	HxVals         string
	IndicatorColor string
	Title          string
}

// RenderIdentifyAs renders button group components for exclusive selection widgets
func RenderIdentifyAs(ctx *rendering.RenderContext, classNames, slug, targets, extra string) string {
	targetsList := parseTargets(targets)
	if len(targetsList) == 0 {
		return ""
	}

	userBeliefs := getUserBeliefs(ctx)
	currentBelief := getCurrentBeliefState(userBeliefs, slug)

	selectedTarget := getSelectedTarget(currentBelief)
	isOtherSelected := isOtherTargetSelected(currentBelief, targetsList)

	var buf bytes.Buffer

	// Render the main widget wrapper securely
	wrapperData := widgetWrapperData{
		ClassNames: classNames,
		Slug:       slug,
		PaneID:     ctx.ContainingPaneID,
	}
	// Only set Extra if it's not the "noprompt" sentinel value
	if extra != "" {
		wrapperData.Extra = extra
	}
	executeTemplate(&buf, "widgetWrapper", wrapperData)

	// Generate a button for each target
	for _, target := range targetsList {
		buf.WriteString(renderIdentifyAsButton(slug, target, selectedTarget, isOtherSelected, extra == "", ctx))
	}

	buf.WriteString(`</div></div>`)

	return buf.String()
}

// renderIdentifyAsButton generates a single button for the IdentifyAs widget
func renderIdentifyAsButton(beliefSlug, target, selectedTarget string, isOtherSelected bool, noprompt bool, ctx *rendering.RenderContext) string {
	isSelected := target == selectedTarget

	// Create JSON for hx-vals using proper marshaling
	hxValsMap := map[string]string{
		"beliefId":     beliefSlug,
		"beliefType":   "Belief",
		"beliefObject": target,
		"paneId":       ctx.ContainingPaneID,
	}
	// The template will handle HTML attribute escaping of the resulting JSON string.
	hxValsBytes, _ := json.Marshal(hxValsMap)

	data := buttonData{
		ID:             "identifyas-" + beliefSlug + "-" + sanitizeID(target),
		Classes:        getIdentifyAsButtonClasses(isSelected, isOtherSelected),
		HxVals:         string(hxValsBytes),
		IndicatorColor: getIdentifyAsIndicatorColor(isSelected, isOtherSelected),
		Title:          getButtonTitle(target, noprompt),
	}

	var buf bytes.Buffer
	executeTemplate(&buf, "button", data)
	return buf.String()
}

// executeTemplate is a helper to render a named template and handle errors
func executeTemplate(buf *bytes.Buffer, name string, data interface{}) {
	err := identifyAsTmpl.ExecuteTemplate(buf, name, data)
	if err != nil {
		log.Printf("ERROR: Failed to execute identifyAs template '%s': %v", name, err)
		buf.WriteString("<!-- template error -->")
	}
}

// The helper functions below do not generate HTML and remain unchanged.

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
	for _, target := range targets {
		if target == selectedTarget {
			return false
		}
	}
	return true
}

func getButtonTitle(target string, noprompt bool) string {
	if noprompt {
		return "Tell me more!"
	}
	return target
}

func sanitizeID(target string) string {
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
		return "bg-lime-400"
	}
	if isOtherSelected {
		return "bg-gray-500"
	}
	return "bg-orange-500"
}
