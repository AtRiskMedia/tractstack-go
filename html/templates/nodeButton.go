// Package templates provides NodeButton.astro rendering functionality
package templates

import (
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// NodeButtonRenderer handles NodeButton.astro rendering logic
type NodeButtonRenderer struct {
	ctx          *models.RenderContext
	nodeRenderer NodeRenderer
}

// NewNodeButtonRenderer creates a new node button renderer
func NewNodeButtonRenderer(ctx *models.RenderContext, nodeRenderer NodeRenderer) *NodeButtonRenderer {
	return &NodeButtonRenderer{
		ctx:          ctx,
		nodeRenderer: nodeRenderer,
	}
}

// Render implements the NodeButton.astro rendering logic - basic version
func (nbr *NodeButtonRenderer) Render(nodeID string) string {
	nodeData := nbr.getNodeData(nodeID)
	if nodeData == nil {
		return `<button>missing button</button>`
	}

	var html strings.Builder

	// Opening <button> tag
	html.WriteString(`<button`)

	// Add CSS classes - matches NodeButton.astro: className={`${node.elementCss || ""} whitespace-nowrap`}
	var cssClasses strings.Builder
	if nodeData.ElementCSS != nil && *nodeData.ElementCSS != "" {
		cssClasses.WriteString(*nodeData.ElementCSS)
	}
	cssClasses.WriteString(" whitespace-nowrap")
	html.WriteString(fmt.Sprintf(` class="%s"`, strings.TrimSpace(cssClasses.String())))

	// Basic onclick for now - advanced ButtonIsland logic can be added later
	html.WriteString(` onclick="return false;"`)

	html.WriteString(`>`)

	// Render all child nodes
	childNodeIDs := nbr.nodeRenderer.GetChildNodeIDs(nodeID)
	for _, childID := range childNodeIDs {
		html.WriteString(nbr.nodeRenderer.RenderNode(childID))
	}

	// Closing </button> tag
	html.WriteString(`</button>`)

	// Add trailing space logic - matches NodeButton.astro: {needsTrailingSpace && " "}
	needsTrailingSpace := nbr.checkNeedsTrailingSpace(nodeID)
	if needsTrailingSpace {
		html.WriteString(" ")
	}

	return html.String()
}

// checkNeedsTrailingSpace implements the NodeButton.astro trailing space logic
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
	// Matches: nextNode.tagName === "text" && !(...nextNode.copy?.startsWith...)
	if nextNodeData.TagName != nil && *nextNodeData.TagName == "text" && nextNodeData.Copy != nil {
		copy := *nextNodeData.Copy
		return !(strings.HasPrefix(copy, ".") ||
			strings.HasPrefix(copy, ",") ||
			strings.HasPrefix(copy, ";") ||
			strings.HasPrefix(copy, ":"))
	}

	return false
}

// getNodeData retrieves node data from real context
func (nbr *NodeButtonRenderer) getNodeData(nodeID string) *models.NodeRenderData {
	if nbr.ctx.AllNodes == nil {
		return nil
	}

	return nbr.ctx.AllNodes[nodeID]
}
