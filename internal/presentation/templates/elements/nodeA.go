// Package templates provides NodeA.astro rendering functionality
package templates

import (
	"html/template"
	"log"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
)

// nodeATmpl is a pre-parsed and secure template for the opening <a> tag with HTMX attributes.
// Follows the exact pattern from unset-button.go and belief widgets
var nodeATmpl = template.Must(template.New("nodeA").Parse(
	`<a href="{{.Href}}"{{if .Target}} target="{{.Target}}"{{end}}{{if .Class}} class="{{.Class}}"{{end}}{{if .CallbackPayload}} hx-post="/api/v1/state" hx-trigger="mousedown" hx-swap="none" hx-vals='{"beliefId":"{{.PaneID}}","beliefType":"Pane","beliefValue":"CLICKED","beliefObject":"{{.CallbackPayload}}"}'{{end}}>`,
))

// nodeAData holds the data for the nodeA template.
type nodeAData struct {
	Href            string
	Target          string
	Class           string
	CallbackPayload string
	PaneID          string
}

// NodeARenderer handles NodeA.astro rendering logic
type NodeARenderer struct {
	ctx          *rendering.RenderContext
	nodeRenderer NodeRenderer
}

// NewNodeARenderer creates a new node link renderer
func NewNodeARenderer(ctx *rendering.RenderContext, nodeRenderer NodeRenderer) *NodeARenderer {
	return &NodeARenderer{
		ctx:          ctx,
		nodeRenderer: nodeRenderer,
	}
}

// Render implements the EXACT NodeA.astro rendering logic with HTMX CLICKED event support
func (nar *NodeARenderer) Render(nodeID string) string {
	nodeData := nar.getNodeData(nodeID)
	if nodeData == nil {
		return `<a href="#">missing link</a>`
	}

	var html strings.Builder

	// Prepare the data for the template
	data := nodeAData{
		Href: "#", // Default href
	}
	if nodeData.Href != nil && *nodeData.Href != "" {
		data.Href = *nodeData.Href
	}
	if nodeData.Target != nil && *nodeData.Target != "" {
		data.Target = *nodeData.Target
	}
	if nodeData.ElementCSS != nil && *nodeData.ElementCSS != "" {
		data.Class = *nodeData.ElementCSS
	}

	// Extract callback payload from CustomData for action links
	if nodeData.CustomData != nil {
		if callbackPayload, exists := nodeData.CustomData["callbackPayload"]; exists {
			if payloadStr, ok := callbackPayload.(string); ok && payloadStr != "" {
				data.CallbackPayload = payloadStr
			}
		}
	}

	// Get pane ID from context for action links
	if nar.ctx.ContainingPaneID != "" {
		data.PaneID = nar.ctx.ContainingPaneID
	}

	// Use the pre-parsed template to safely render the opening <a> tag.
	err := nodeATmpl.Execute(&html, data)
	if err != nil {
		log.Printf("ERROR: Failed to execute nodeA template for nodeID %s: %v", nodeID, err)
		return `<!-- error rendering link -->`
	}

	// Render all child nodes with <span class="whitespace-nowrap"> wrapper
	// This matches the expected output: <span class="whitespace-nowrap">See Pricing</span>
	childNodeIDs := nar.nodeRenderer.GetChildNodeIDs(nodeID)
	if len(childNodeIDs) > 0 {
		html.WriteString(` <span class="whitespace-nowrap">`)
		for _, childID := range childNodeIDs {
			html.WriteString(nar.nodeRenderer.RenderNode(childID))
		}
		html.WriteString(`</span>`)
	}

	// Closing </a> tag
	html.WriteString(`</a>`)

	return html.String()
}

// getNodeData retrieves node data from real context
func (nar *NodeARenderer) getNodeData(nodeID string) *rendering.NodeRenderData {
	if nar.ctx.AllNodes == nil {
		return nil
	}
	return nar.ctx.AllNodes[nodeID]
}
