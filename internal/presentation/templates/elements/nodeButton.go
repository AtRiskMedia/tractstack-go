// Package templates provides NodeButton.astro rendering functionality
package templates

import (
	"html/template"
	"log"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
)

// nodeButtonTmpl is a pre-parsed template for rendering the button's opening tag with HTMX attributes.
// Follows the exact pattern from unset-button.go and belief widgets
var nodeButtonTmpl = template.Must(template.New("nodeButton").Parse(`<button class="{{.Class}}"{{if .CallbackPayload}} hx-post="/api/v1/state" hx-trigger="click" hx-swap="none" hx-vals='{"beliefId":"{{.PaneID}}","beliefType":"Pane","beliefValue":"CLICKED","beliefObject":"{{.CallbackPayload}}"}'{{end}}>`))

// nodeButtonData holds the data for the nodeButton template
type nodeButtonData struct {
	Class           string
	CallbackPayload string
	PaneID          string
}

// NodeButtonRenderer handles NodeButton.astro rendering logic
type NodeButtonRenderer struct {
	ctx          *rendering.RenderContext
	nodeRenderer NodeRenderer
}

// NewNodeButtonRenderer creates a new node button renderer
func NewNodeButtonRenderer(ctx *rendering.RenderContext, nodeRenderer NodeRenderer) *NodeButtonRenderer {
	return &NodeButtonRenderer{
		ctx:          ctx,
		nodeRenderer: nodeRenderer,
	}
}

// Render implements the EXACT NodeButton.astro rendering logic with HTMX CLICKED event support
func (nbr *NodeButtonRenderer) Render(nodeID string) string {
	nodeData := nbr.getNodeData(nodeID)
	if nodeData == nil {
		return `<button>missing button</button>`
	}

	var htmlBuilder strings.Builder

	// Prepare template data
	data := nodeButtonData{}

	// Prepare CSS classes string
	// EXACT match: className={`${node.elementCss || ""} whitespace-nowrap`}
	var cssClasses strings.Builder
	if nodeData.ElementCSS != nil && *nodeData.ElementCSS != "" {
		cssClasses.WriteString(*nodeData.ElementCSS)
	}
	cssClasses.WriteString(" whitespace-nowrap")
	data.Class = strings.TrimSpace(cssClasses.String())

	// Extract callback payload from CustomData
	if nodeData.CustomData != nil {
		if callbackPayload, exists := nodeData.CustomData["callbackPayload"]; exists {
			if payloadStr, ok := callbackPayload.(string); ok && payloadStr != "" {
				data.CallbackPayload = payloadStr
			}
		}
	}

	// Get pane ID from context
	if nbr.ctx.ContainingPaneID != "" {
		data.PaneID = nbr.ctx.ContainingPaneID
	}

	// Use the pre-parsed template to safely render the opening <button> tag
	err := nodeButtonTmpl.Execute(&htmlBuilder, data)
	if err != nil {
		log.Printf("ERROR: Failed to execute nodeButton template for nodeID %s: %v", nodeID, err)
		return `<!-- error rendering button -->`
	}

	// Render all child nodes with <span class="whitespace-nowrap"> wrapper
	// This matches the expected output: <span class="whitespace-nowrap">Talk to me!​​ </span>
	childNodeIDs := nbr.nodeRenderer.GetChildNodeIDs(nodeID)
	if len(childNodeIDs) > 0 {
		htmlBuilder.WriteString(`<span class="whitespace-nowrap">`)
		for _, childID := range childNodeIDs {
			htmlBuilder.WriteString(nbr.nodeRenderer.RenderNode(childID))
		}
		htmlBuilder.WriteString(`</span>`)
	}

	// Closing </button> tag
	htmlBuilder.WriteString(`</button>`)

	// Add trailing space logic - EXACT match: {needsTrailingSpace && " "}
	needsTrailingSpace := nbr.checkNeedsTrailingSpace(nodeID)
	if needsTrailingSpace {
		htmlBuilder.WriteString(" ")
	}

	return htmlBuilder.String()
}

// checkNeedsTrailingSpace implements the EXACT NodeButton.astro trailing space logic
func (nbr *NodeButtonRenderer) checkNeedsTrailingSpace(nodeID string) bool {
	nodeData := nbr.getNodeData(nodeID)
	if nodeData == nil || nodeData.ParentID == "" {
		return false
	}

	// Get sibling nodes to find the next node
	parentID := nodeData.ParentID
	childNodeIDs := nbr.nodeRenderer.GetChildNodeIDs(parentID)

	currentIndex := -1
	for i, childID := range childNodeIDs {
		if childID == nodeID {
			currentIndex = i
			break
		}
	}

	// If this is the last child or we couldn't find it, no trailing space
	if currentIndex == -1 || currentIndex >= len(childNodeIDs)-1 {
		return false
	}

	// Get the next node
	nextNodeID := childNodeIDs[currentIndex+1]
	nextNodeData := nbr.getNodeData(nextNodeID)
	if nextNodeData == nil {
		return false
	}

	// Check if next node is text and doesn't start with punctuation
	// EXACT match: nextNode.tagName === "text" && !(...nextNode.copy?.startsWith...)
	if nextNodeData.TagName != nil && *nextNodeData.TagName == "text" {
		if nextNodeData.Copy != nil {
			text := *nextNodeData.Copy
			return !strings.HasPrefix(text, ".") &&
				!strings.HasPrefix(text, ",") &&
				!strings.HasPrefix(text, ";") &&
				!strings.HasPrefix(text, ":")
		}
	}

	return false
}

// getNodeData retrieves node data from real context
func (nbr *NodeButtonRenderer) getNodeData(nodeID string) *rendering.NodeRenderData {
	if nbr.ctx.AllNodes == nil {
		return nil
	}
	return nbr.ctx.AllNodes[nodeID]
}
