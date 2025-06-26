// Package templates provides NodeBasicTag.astro rendering functionality
package templates

import (
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// NodeBasicTagRenderer handles NodeBasicTag.astro rendering logic
type NodeBasicTagRenderer struct {
	ctx          *models.RenderContext
	nodeRenderer NodeRenderer
}

// NewNodeBasicTagRenderer creates a new node basic tag renderer
func NewNodeBasicTagRenderer(ctx *models.RenderContext, nodeRenderer NodeRenderer) *NodeBasicTagRenderer {
	return &NodeBasicTagRenderer{
		ctx:          ctx,
		nodeRenderer: nodeRenderer,
	}
}

// Render implements the NodeBasicTag.astro rendering logic with elementCss exactly
func (nbtr *NodeBasicTagRenderer) Render(nodeID string) string {
	nodeData := nbtr.getNodeData(nodeID)
	if nodeData == nil {
		return `<div></div>`
	}

	// Get tag name - matches NodeBasicTag.astro: const Tag = node.tagName || EmptyNode;
	tagName := "div" // Default fallback
	if nodeData.TagName != nil {
		tagName = *nodeData.TagName
	}

	// Get CSS classes - matches NodeBasicTag.astro: class={node.elementCss || ``}
	cssClasses := ""
	if nodeData.ElementCSS != nil {
		cssClasses = *nodeData.ElementCSS
	}

	// Get child nodes - matches NodeBasicTag.astro child iteration
	childNodeIDs := nbtr.nodeRenderer.GetChildNodeIDs(nodeID)

	// Build the HTML exactly like NodeBasicTag.astro
	var html strings.Builder

	// Opening tag with CSS classes
	if cssClasses != "" {
		html.WriteString(fmt.Sprintf(`<%s class="%s">`, tagName, cssClasses))
	} else {
		html.WriteString(fmt.Sprintf(`<%s>`, tagName))
	}

	// Render all child nodes - matches getCtx().getChildNodeIDs(nodeId).map((id) => <Node nodeId={id} />)
	for _, childID := range childNodeIDs {
		html.WriteString(nbtr.nodeRenderer.RenderNode(childID))
	}

	// Closing tag
	html.WriteString(fmt.Sprintf(`</%s>`, tagName))

	return html.String()
}

// getNodeData retrieves node data - FIXED TO USE REAL DATA
func (nbtr *NodeBasicTagRenderer) getNodeData(nodeID string) *models.NodeRenderData {
	if nbtr.ctx.AllNodes == nil {
		return nil
	}

	// Use real data from context
	return nbtr.ctx.AllNodes[nodeID]
}
