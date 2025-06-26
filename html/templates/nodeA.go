// Package templates provides NodeA.astro rendering functionality
package templates

import (
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// NodeARenderer handles NodeA.astro rendering logic
type NodeARenderer struct {
	ctx          *models.RenderContext
	nodeRenderer NodeRenderer
}

// NewNodeARenderer creates a new node link renderer
func NewNodeARenderer(ctx *models.RenderContext, nodeRenderer NodeRenderer) *NodeARenderer {
	return &NodeARenderer{
		ctx:          ctx,
		nodeRenderer: nodeRenderer,
	}
}

// Render implements the NodeA.astro rendering logic exactly
func (nar *NodeARenderer) Render(nodeID string) string {
	nodeData := nar.getNodeData(nodeID)
	if nodeData == nil {
		return `<a href="#">missing link</a>`
	}

	var html strings.Builder

	// Opening <a> tag
	html.WriteString(`<a`)

	// Add href attribute
	href := "#"
	if nodeData.Href != nil && *nodeData.Href != "" {
		href = *nodeData.Href
	}
	html.WriteString(fmt.Sprintf(` href="%s"`, href))

	// Add target attribute if specified
	if nodeData.Target != nil && *nodeData.Target != "" {
		html.WriteString(fmt.Sprintf(` target="%s"`, *nodeData.Target))
	}

	// Add CSS classes
	if nodeData.ElementCSS != nil && *nodeData.ElementCSS != "" {
		html.WriteString(fmt.Sprintf(` class="%s"`, *nodeData.ElementCSS))
	}

	html.WriteString(`>`)

	// Render all child nodes
	childNodeIDs := nar.nodeRenderer.GetChildNodeIDs(nodeID)
	for _, childID := range childNodeIDs {
		html.WriteString(nar.nodeRenderer.RenderNode(childID))
	}

	// Closing </a> tag
	html.WriteString(`</a>`)

	return html.String()
}

// getNodeData retrieves node data from real context
func (nar *NodeARenderer) getNodeData(nodeID string) *models.NodeRenderData {
	if nar.ctx.AllNodes == nil {
		return nil
	}

	return nar.ctx.AllNodes[nodeID]
}
