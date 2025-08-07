// Package templates provides NodeButton.astro rendering functionality
package templates

import (
	"html/template"
	"log"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
)

// nodeButtonTmpl is a pre-parsed template for rendering the button's opening tag.
// Using html/template automatically escapes the class attribute, preventing HTML injection.
var nodeButtonTmpl = template.Must(template.New("nodeButton").Parse(`<button class="{{.}}" onclick="return false;">`))

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

// Render implements the EXACT NodeButton.astro rendering logic
func (nbr *NodeButtonRenderer) Render(nodeID string) string {
	nodeData := nbr.getNodeData(nodeID)
	if nodeData == nil {
		return `<button>missing button</button>`
	}

	var html strings.Builder

	// Prepare CSS classes string
	// EXACT match: className={`${node.elementCss || ""} whitespace-nowrap`}
	var cssClasses strings.Builder
	if nodeData.ElementCSS != nil && *nodeData.ElementCSS != "" {
		cssClasses.WriteString(*nodeData.ElementCSS)
	}
	cssClasses.WriteString(" whitespace-nowrap")
	finalClasses := strings.TrimSpace(cssClasses.String())

	// Use the pre-parsed template to safely render the opening <button> tag.
	// This replaces the insecure fmt.Sprintf() call.
	err := nodeButtonTmpl.Execute(&html, finalClasses)
	if err != nil {
		log.Printf("ERROR: Failed to execute nodeButton template for nodeID %s: %v", nodeID, err)
		return `<!-- error rendering button -->`
	}

	// Render all child nodes with <span class="whitespace-nowrap"> wrapper
	// This matches the expected output: <span class="whitespace-nowrap">Talk to me!​​ </span>
	childNodeIDs := nbr.nodeRenderer.GetChildNodeIDs(nodeID)
	if len(childNodeIDs) > 0 {
		html.WriteString(`<span class="whitespace-nowrap">`)
		for _, childID := range childNodeIDs {
			html.WriteString(nbr.nodeRenderer.RenderNode(childID))
		}
		html.WriteString(`</span>`)
	}

	// Closing </button> tag
	html.WriteString(`</button>`)

	// Add trailing space logic - EXACT match: {needsTrailingSpace && " "}
	needsTrailingSpace := nbr.checkNeedsTrailingSpace(nodeID)
	if needsTrailingSpace {
		html.WriteString(" ")
	}

	return html.String()
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
			return !(strings.HasPrefix(text, ".") ||
				strings.HasPrefix(text, ",") ||
				strings.HasPrefix(text, ";") ||
				strings.HasPrefix(text, ":"))
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
