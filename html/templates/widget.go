// Package templates provides Widget.astro rendering functionality
package templates

import (
	"bytes"
	"html/template"
	"log"

	"github.com/AtRiskMedia/tractstack-go/cache"
	templates "github.com/AtRiskMedia/tractstack-go/html/templates/widgets"
	"github.com/AtRiskMedia/tractstack-go/models"
)

var widgetTmpl = template.Must(template.New("mainWidget").Parse(
	`{{define "unknown"}}<div class="{{.ClassNames}}">unknown widget: {{.Hook}}</div>{{end}}` +
		`{{define "youtube"}}<div class="{{.ClassNames}}"><div>YouTube Widget: {{.Value1}} - {{.Value2}}</div></div>{{end}}` +
		`{{define "bunny"}}<div class="{{.ClassNames}}"><div>Bunny Video Widget: {{.Value1}} - {{.Value2}}</div></div>{{end}}` +
		`{{define "signup"}}<div class="{{.ClassNames}}"><div>SignUp Widget: {{.Persona}} - {{.Prompt}} (consent: {{.ClarifyConsent}})</div></div>{{end}}` +
		`{{define "resource"}}<div class="{{.ClassNames}}"><div><strong>Resource Template (not yet implemented):</strong> {{.Value1}}, {{.Value2}}</div></div>{{end}}`,
))

type widgetData struct {
	ClassNames     string
	Hook           string
	Value1         string
	Value2         string
	Persona        string
	Prompt         string
	ClarifyConsent bool
}

// WidgetRenderer handles Widget.astro rendering logic - dispatcher for all widget types
type WidgetRenderer struct {
	ctx *models.RenderContext
}

// NewWidgetRenderer creates a new widget renderer
func NewWidgetRenderer(ctx *models.RenderContext) *WidgetRenderer {
	return &WidgetRenderer{ctx: ctx}
}

// Render implements the Widget.astro rendering logic - exact dispatcher pattern
func (wr *WidgetRenderer) Render(nodeID string, hook *models.CodeHook) string {
	if hook == nil {
		return `<div>widget error: no hook</div>`
	}

	classNames := wr.getNodeClasses(nodeID)

	// Dispatch to specific widget renderers exactly like Widget.astro switch pattern
	switch hook.Hook {
	case "youtube":
		return wr.renderYouTube(nodeID, classNames, hook)
	case "bunny":
		return wr.renderBunny(nodeID, classNames, hook)
	case "signup":
		return wr.renderSignUp(nodeID, classNames, hook)
	case "belief":
		return templates.RenderBelief(wr.ctx, classNames, *hook.Value1, *hook.Value2, hook.Value3)
	case "identifyAs":
		return templates.RenderIdentifyAs(wr.ctx, classNames, *hook.Value1, *hook.Value2, hook.Value3)
	case "toggle":
		return templates.RenderToggle(wr.ctx, classNames, *hook.Value1, *hook.Value2)
	case "resource":
		return wr.renderResource(nodeID, classNames, hook)
	default:
		data := widgetData{ClassNames: classNames, Hook: hook.Hook}
		var buf bytes.Buffer
		err := widgetTmpl.ExecuteTemplate(&buf, "unknown", data)
		if err != nil {
			log.Printf("ERROR: Failed to execute unknown widget template: %v", err)
			return "<!-- template error -->"
		}
		return buf.String()
	}
}

// renderYouTube matches Widget.astro youtube condition exactly
func (wr *WidgetRenderer) renderYouTube(nodeID, classNames string, hook *models.CodeHook) string {
	if hook.Value1 != nil && hook.Value2 != nil && *hook.Value1 != "" && *hook.Value2 != "" {
		data := widgetData{ClassNames: classNames, Value1: *hook.Value1, Value2: *hook.Value2}
		var buf bytes.Buffer
		err := widgetTmpl.ExecuteTemplate(&buf, "youtube", data)
		if err != nil {
			log.Printf("ERROR: Failed to execute youtube widget template: %v", err)
			return "<!-- template error -->"
		}
		return buf.String()
	}
	return ""
}

// renderBunny matches Widget.astro bunny condition exactly
func (wr *WidgetRenderer) renderBunny(nodeID, classNames string, hook *models.CodeHook) string {
	if hook.Value1 != nil && hook.Value2 != nil && *hook.Value1 != "" && *hook.Value2 != "" {
		data := widgetData{ClassNames: classNames, Value1: *hook.Value1, Value2: *hook.Value2}
		var buf bytes.Buffer
		err := widgetTmpl.ExecuteTemplate(&buf, "bunny", data)
		if err != nil {
			log.Printf("ERROR: Failed to execute bunny widget template: %v", err)
			return "<!-- template error -->"
		}
		return buf.String()
	}
	return ""
}

// renderSignUp matches Widget.astro signup condition exactly
func (wr *WidgetRenderer) renderSignUp(nodeID, classNames string, hook *models.CodeHook) string {
	if hook.Value1 != nil && *hook.Value1 != "" {
		persona := *hook.Value1
		prompt := "Keep in touch!"
		if hook.Value2 != nil && *hook.Value2 != "" {
			prompt = *hook.Value2
		}
		clarifyConsent := hook.Value3 == "true"

		data := widgetData{
			ClassNames:     classNames,
			Persona:        persona,
			Prompt:         prompt,
			ClarifyConsent: clarifyConsent,
		}
		var buf bytes.Buffer
		err := widgetTmpl.ExecuteTemplate(&buf, "signup", data)
		if err != nil {
			log.Printf("ERROR: Failed to execute signup widget template: %v", err)
			return "<!-- template error -->"
		}
		return buf.String()
	}
	return ""
}

// renderResource matches Widget.astro resource condition exactly
func (wr *WidgetRenderer) renderResource(nodeID, classNames string, hook *models.CodeHook) string {
	if hook.Value1 != nil && *hook.Value1 != "" {
		value2 := ""
		if hook.Value2 != nil {
			value2 = *hook.Value2
		}
		data := widgetData{ClassNames: classNames, Value1: *hook.Value1, Value2: value2}
		var buf bytes.Buffer
		err := widgetTmpl.ExecuteTemplate(&buf, "resource", data)
		if err != nil {
			log.Printf("ERROR: Failed to execute resource widget template: %v", err)
			return "<!-- template error -->"
		}
		return buf.String()
	}
	return ""
}

// getNodeClasses retrieves CSS classes for widget - direct elementCss access like other renderers
func (wr *WidgetRenderer) getNodeClasses(nodeID string) string {
	nodeData := wr.getNodeData(nodeID)
	if nodeData != nil && nodeData.ElementCSS != nil && *nodeData.ElementCSS != "" {
		return *nodeData.ElementCSS
	}
	return "auto" // Default fallback
}

// getNodeData retrieves node data from real context - matches other renderer patterns
func (wr *WidgetRenderer) getNodeData(nodeID string) *models.NodeRenderData {
	if wr.ctx.AllNodes == nil {
		return nil
	}
	return wr.ctx.AllNodes[nodeID]
}

func (wr *WidgetRenderer) getUserBeliefs() map[string][]string {
	if wr.ctx.SessionID == "" || wr.ctx.StoryfragmentID == "" {
		return nil
	}

	sessionContext, exists := cache.GetGlobalManager().GetSessionBeliefContext(
		wr.ctx.TenantID,
		wr.ctx.SessionID,
		wr.ctx.StoryfragmentID,
	)
	if !exists {
		return nil
	}

	return sessionContext.UserBeliefs
}
