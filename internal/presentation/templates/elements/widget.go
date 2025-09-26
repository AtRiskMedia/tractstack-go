// Package templates provides Widget.astro rendering functionality
package templates

import (
	"bytes"
	"html/template"
	"log"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
	templates "github.com/AtRiskMedia/tractstack-go/internal/presentation/templates/elements/widgets"
)

// liteYouTubeTmpl renders a high-performance, lazy-loaded YouTube embed.
var liteYouTubeTmpl = template.Must(template.New("liteYouTube").Parse(
	`
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/lite-youtube-embed@0.3.2/src/lite-yt-embed.css" />
<lite-youtube videoid="{{.Value1}}" playlabel="{{.Value2}}"></lite-youtube>
<script type="module" src="https://cdn.jsdelivr.net/npm/lite-youtube-embed@0.3.2/src/lite-yt-embed.js"></script>
`))

// widgetTmpl contains templates for other placeholder widgets.
var widgetTmpl = template.Must(template.New("mainWidget").Parse(
	`{{define "unknown"}}<div class="{{.ClassNames}}">unknown widget: {{.Hook}}</div>{{end}}` +
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

// WidgetRenderer handles rendering for all widget types.
type WidgetRenderer struct {
	ctx *rendering.RenderContext
}

// NewWidgetRenderer creates a new widget renderer.
func NewWidgetRenderer(ctx *rendering.RenderContext) *WidgetRenderer {
	return &WidgetRenderer{ctx: ctx}
}

// Render dispatches a node to the appropriate widget rendering function.
func (wr *WidgetRenderer) Render(nodeID string, hook *rendering.CodeHook) string {
	if hook == nil {
		return `<div>widget error: no hook</div>`
	}

	classNames := wr.getNodeClasses(nodeID)

	switch hook.Hook {
	case "youtube":
		return wr.renderLiteYouTube(classNames, hook)
	case "bunny":
		return templates.RenderBunny(classNames, hook)
	case "signup":
		return wr.renderSignUp(classNames, hook)
	case "belief":
		return templates.RenderBelief(wr.ctx, classNames, *hook.Value1, *hook.Value2, hook.Value3)
	case "identifyAs":
		return templates.RenderIdentifyAs(wr.ctx, classNames, *hook.Value1, *hook.Value2, hook.Value3)
	case "toggle":
		return templates.RenderToggle(wr.ctx, classNames, *hook.Value1, *hook.Value2)
	case "resource":
		return wr.renderResource(classNames, hook)
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

// renderLiteYouTube renders a performant YouTube player.
func (wr *WidgetRenderer) renderLiteYouTube(classNames string, hook *rendering.CodeHook) string {
	if hook.Value1 != nil && hook.Value2 != nil && *hook.Value1 != "" && *hook.Value2 != "" {
		data := widgetData{Value1: *hook.Value1, Value2: *hook.Value2}
		var buf bytes.Buffer
		err := liteYouTubeTmpl.Execute(&buf, data)
		if err != nil {
			log.Printf("ERROR: Failed to execute lite youtube template: %v", err)
			return "<!-- template error -->"
		}
		return buf.String()
	}
	return ""
}

// renderSignUp renders a placeholder for a signup widget.
func (wr *WidgetRenderer) renderSignUp(classNames string, hook *rendering.CodeHook) string {
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

// renderResource renders a placeholder for a resource widget.
func (wr *WidgetRenderer) renderResource(classNames string, hook *rendering.CodeHook) string {
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

// getNodeClasses retrieves CSS classes for the widget node.
func (wr *WidgetRenderer) getNodeClasses(nodeID string) string {
	nodeData := wr.getNodeData(nodeID)
	if nodeData != nil && nodeData.ElementCSS != nil && *nodeData.ElementCSS != "" {
		return *nodeData.ElementCSS
	}
	return "auto" // Default fallback
}

// getNodeData retrieves node data from the render context.
func (wr *WidgetRenderer) getNodeData(nodeID string) *rendering.NodeRenderData {
	if wr.ctx.AllNodes == nil {
		return nil
	}
	return wr.ctx.AllNodes[nodeID]
}
