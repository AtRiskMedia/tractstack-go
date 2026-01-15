package templates

import (
	"encoding/json"
	"fmt"
	"html/template"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/lisp"
	"github.com/microcosm-cc/bluemonday"
)

var playIconSVG = `<svg viewBox="0 0 459 459" xmlns="http://www.w3.org/2000/svg" preserveAspectRatio="xMidYMid meet" class="inline-block" style="height:1em; margin-left: 0.25em;"><path d="M229.5,0C102.751,0,0,102.751,0,229.5S102.751,459,229.5,459S459,356.249,459,229.5S356.249,0,229.5,0z M310.292,239.651 l-111.764,76.084c-3.761,2.56-8.63,2.831-12.652,0.704c-4.022-2.128-6.538-6.305-6.538-10.855V153.416 c0-4.55,2.516-8.727,6.538-10.855c4.022-2.127,8.891-1.857,12.652,0.704l111.764,76.084c3.359,2.287,5.37,6.087,5.37,10.151 C315.662,233.564,313.652,237.364,310.292,239.651z" fill="currentColor"></path></svg>`

// HastRenderer handles the rendering of an HTML Abstract Syntax Tree (HAST) into a string.
type HastRenderer struct {
	ctx *rendering.RenderContext
}

// Render traverses the HAST and produces the final sanitized HTML string.
func (hr *HastRenderer) Render(ast *rendering.HTMLAST) string {
	var sb strings.Builder

	if ast.CSS != "" && (hr.ctx == nil || !hr.ctx.IsEditorPreview) {
		sb.WriteString("<style>")
		sb.WriteString(ast.CSS)
		sb.WriteString("</style>")
	}

	var bodySb strings.Builder
	for _, node := range ast.Tree {
		hr.renderNode(&bodySb, node)
	}

	p := bluemonday.UGCPolicy()
	p.AllowDataURIImages()
	p.AllowRelativeURLs(true)
	p.AllowAttrs("class", "style", "id", "data-callback", "data-bunny-payload", "data-external").Globally()

	// Feature Parity: Allow HTMX and interactive attributes
	p.AllowAttrs("hx-post", "hx-trigger", "hx-swap", "hx-vals", "onclick", "target", "rel").Globally()
	p.AllowElements("button", "svg", "path") // Allow SVG for icons
	p.AllowAttrs("viewBox", "xmlns", "preserveAspectRatio", "d", "fill").OnElements("svg", "path")

	if hr.ctx != nil && hr.ctx.IsEditorPreview {
		p.AllowAttrs("data-ast-id", "contenteditable", "data-file-id", "data-collection", "data-image").Globally()
	}

	sanitizedBody := p.Sanitize(bodySb.String())
	sb.WriteString(sanitizedBody)

	return sb.String()
}

func isEditableTag(tag string) bool {
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6", "p", "li":
		return true
	default:
		return false
	}
}

func (hr *HastRenderer) renderNode(sb *strings.Builder, node rendering.HTMLASTNode) {
	if node.Tag == "text" {
		sb.WriteString(template.HTMLEscapeString(node.Text))
		return
	}

	sb.WriteString("<")
	sb.WriteString(node.Tag)

	// Determine if we should apply interactive logic (Not in Editor Preview)
	shouldApplyInteractions := hr.ctx == nil || !hr.ctx.IsEditorPreview

	// Helpers for processing
	var buttonParsedAction *lisp.ParsedAction
	isExternalLink := false
	hasTarget := false

	if _, ok := node.Attrs["target"]; ok {
		hasTarget = true
	}

	for k, v := range node.Attrs {
		sb.WriteString(" ")
		sb.WriteString(k)
		sb.WriteString("=\"")
		sb.WriteString(template.HTMLEscapeString(v))
		sb.WriteString("\"")
	}

	// Logic for Interactive Elements
	if shouldApplyInteractions {
		paneID := ""
		if hr.ctx != nil {
			paneID = hr.ctx.ContainingPaneID
		}

		switch node.Tag {
		case "button":
			if payload, ok := node.Attrs["data-callback"]; ok && payload != "" {
				parsed := lisp.Parse(payload, paneID, "")
				buttonParsedAction = parsed // Save for icon rendering later

				if parsed.IsValid {
					if parsed.IsClientSideEvent {
						payloadBytes, _ := json.Marshal(parsed.ClientSidePayload)
						onclickVal := fmt.Sprintf(`document.dispatchEvent(new CustomEvent('%s', { bubbles: true, detail: %s }))`, parsed.ClientSideEventName, string(payloadBytes))
						fmt.Fprintf(sb, ` onclick="%s"`, template.HTMLEscapeString(onclickVal))
					} else if parsed.HxVals != nil {
						hxValsBytes, _ := json.Marshal(parsed.HxVals)
						sb.WriteString(` hx-post="/api/v1/state" hx-trigger="click" hx-swap="none"`)
						fmt.Fprintf(sb, ` hx-vals='%s'`, string(hxValsBytes))
					}
				}
			}
		case "a":
			// External Link Attributes
			if val, ok := node.Attrs["data-external"]; ok && val == "true" {
				isExternalLink = true
				if !hasTarget {
					sb.WriteString(` target="_blank"`)
				}
			}

			// Click Tracking Attributes
			if payload, ok := node.Attrs["data-callback"]; ok && payload != "" {
				vals := map[string]string{
					"beliefId":     paneID,
					"beliefType":   "Pane",
					"beliefValue":  "CLICKED",
					"beliefObject": payload,
				}
				valsBytes, _ := json.Marshal(vals)
				sb.WriteString(` hx-post="/api/v1/state" hx-trigger="mousedown" hx-swap="none"`)
				fmt.Fprintf(sb, ` hx-vals='%s'`, string(valsBytes))
			}
		}
	}

	// Editor Preview Attributes
	if !shouldApplyInteractions && node.ID != "" {
		sb.WriteString(" data-ast-id=\"")
		sb.WriteString(node.ID)
		sb.WriteString("\"")

		if isEditableTag(node.Tag) {
			sb.WriteString(" contenteditable=\"true\"")
		}
		if node.Tag == "button" {
			sb.WriteString(" disabled=\"true\"")
		}
		if node.Tag == "a" {
			sb.WriteString(" onclick=\"return false;\"")
			sb.WriteString(" style=\"pointer-events: none;\"")
		}
	}

	sb.WriteString(">")

	for _, child := range node.Children {
		hr.renderNode(sb, child)
	}

	// Content Injection for Feature Parity
	if shouldApplyInteractions {
		// External Link Icon
		if node.Tag == "a" && isExternalLink {
			sb.WriteString(`<span class="ml-1" aria-label="external link">↗</span>`)
		}
		// Video Play Icon
		if node.Tag == "button" && buttonParsedAction != nil && buttonParsedAction.IsClientSideEvent && buttonParsedAction.ClientSideEventName == "update-video" {
			sb.WriteString(playIconSVG)
		}
	}

	if !isVoidTag(node.Tag) {
		sb.WriteString("</")
		sb.WriteString(node.Tag)
		sb.WriteString(">")
	}
}

func isVoidTag(tag string) bool {
	switch tag {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr":
		return true
	default:
		return false
	}
}
