// Package templates provides Markdown.astro rendering functionality
package templates

import (
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// MarkdownRenderer handles Markdown.astro rendering logic
type MarkdownRenderer struct {
	ctx          *models.RenderContext
	nodeRenderer NodeRenderer
}

// NodeRenderer interface for child node rendering
type NodeRenderer interface {
	RenderNode(nodeID string) string
	GetChildNodeIDs(nodeID string) []string
}

// NewMarkdownRenderer creates a new markdown renderer
func NewMarkdownRenderer(ctx *models.RenderContext, nodeRenderer NodeRenderer) *MarkdownRenderer {
	return &MarkdownRenderer{
		ctx:          ctx,
		nodeRenderer: nodeRenderer,
	}
}

// Render implements the COMPLETE Markdown.astro rendering logic with parentCSS layering and background integration
func (mr *MarkdownRenderer) Render(nodeID string, depth int) string {
	nodeData := mr.getNodeData(nodeID)
	if nodeData == nil {
		return `<div></div>`
	}

	// Get parentCss - matches Markdown.astro: const parentCss = (node?.parentCss as string[]) || [];
	parentCSS := nodeData.ParentCSS
	if parentCSS == nil {
		parentCSS = []string{}
	}

	// Check for positioned background image - matches Markdown.astro background detection logic
	bgNode := mr.getPositionedBackgroundNode(nodeID)

	// Build content nodes - everything except BgPane - matches Markdown.astro
	contentNodes := mr.getContentNodeIDs(nodeID)

	// Recursive parentCSS wrapping - matches Markdown.astro exactly
	// {parentCss.length > 0 && depth < parentCss.length ? (...) : (...)}
	if len(parentCSS) > 0 && depth < len(parentCSS) {
		var html strings.Builder
		html.WriteString(fmt.Sprintf(`<div class="%s"`, parentCSS[depth]))

		// Add style for depth 0 - matches: style={depth === 0 ? "position: relative; z-index: 10;" : ""}
		if depth == 0 {
			html.WriteString(` style="position: relative; z-index: 10;"`)
		}

		html.WriteString(`>`)

		// Recursive call - matches Astro.self pattern: <Astro.self nodeId={nodeId} depth={depth + 1} />
		html.WriteString(mr.Render(nodeID, depth+1))

		html.WriteString(`</div>`)
		return html.String()
	}

	// Main content rendering - matches the else branch of Markdown.astro
	var html strings.Builder
	html.WriteString(`<div style="position: relative; z-index: 10;">`)

	// Check if we should use flex layout for positioned background - matches Markdown.astro
	// const useFlexLayout = bgNode && (bgNode.position === "left" || bgNode.position === "right");
	useFlexLayout := bgNode != nil && (bgNode.Position == "left" || bgNode.Position == "right")

	if useFlexLayout {
		// Flex layout with positioned background image - matches Markdown.astro useFlexLayout branch
		// const flexDirection = bgNode?.position === "right" ? "flex-col md:flex-row-reverse" : "flex-col md:flex-row";
		flexDirection := "flex-col md:flex-row"
		if bgNode.Position == "right" {
			flexDirection = "flex-col md:flex-row-reverse"
		}

		// Main flex container - matches: <div class={`flex flex-nowrap justify-center items-center gap-6 md:gap-10 xl:gap-12 ${flexDirection}`}>
		html.WriteString(fmt.Sprintf(`<div class="flex flex-nowrap justify-center items-center gap-6 md:gap-10 xl:gap-12 %s">`, flexDirection))

		// Image Side - MUST USE RESPONSIVE MODIFIERS like Astro
		// <div class={`relative overflow-hidden ${getSizeClasses(bgNode.size || "equal", "image")}`}>
		imageSizeClass := mr.getSizeClasses(bgNode.Size, "image")
		html.WriteString(fmt.Sprintf(`<div class="relative overflow-hidden %s">`, imageSizeClass))

		// Render the background node - matches: <Node nodeId={bgNode.id} />
		html.WriteString(mr.nodeRenderer.RenderNode(bgNode.ID))
		html.WriteString(`</div>`)

		// Content Side - MUST USE RESPONSIVE MODIFIERS like Astro
		// <div class={`${getSizeClasses(bgNode.size || "equal", "content")}`}>
		contentSizeClass := mr.getSizeClasses(bgNode.Size, "content")
		html.WriteString(fmt.Sprintf(`<div class="%s">`, contentSizeClass))

		// Render content nodes - matches: {contentNodes.map((x) => (<Node nodeId={x} />))}
		for _, childID := range contentNodes {
			html.WriteString(mr.nodeRenderer.RenderNode(childID))
		}
		html.WriteString(`</div>`)

		html.WriteString(`</div>`)
	} else {
		// Standard layout - render content nodes directly
		// matches: contentNodes.map((x) => <Node nodeId={x} />)
		for _, childID := range contentNodes {
			html.WriteString(mr.nodeRenderer.RenderNode(childID))
		}
	}

	html.WriteString(`</div>`)
	return html.String()
}

// getSizeClasses helper function matching Markdown.astro getSizeClasses exactly
func (mr *MarkdownRenderer) getSizeClasses(size string, side string) string {
	switch size {
	case "narrow":
		if side == "image" {
			return "w-full md:w-1/3"
		}
		return "w-full md:w-2/3"
	case "wide":
		if side == "image" {
			return "w-full md:w-2/3"
		}
		return "w-full md:w-1/3"
	default: // "equal"
		return "w-full md:w-1/2"
	}
}

// getPositionedBackgroundNode finds positioned background image like Markdown.astro
func (mr *MarkdownRenderer) getPositionedBackgroundNode(nodeID string) *models.BackgroundNode {
	nodeData := mr.getNodeData(nodeID)
	if nodeData == nil {
		return nil
	}

	// Get parent pane ID - matches: const parentPaneId = node.parentId;
	parentPaneID := nodeData.ParentID
	if parentPaneID == "" {
		return nil
	}

	// Get child nodes of parent pane - matches: const childNodeIds = getCtx().getChildNodeIDs(parentPaneId);
	childNodeIDs := mr.nodeRenderer.GetChildNodeIDs(parentPaneID)

	// Look for positioned background image - matches Markdown.astro's find logic:
	// return childNodeIds.map((id) => allNodes.get(id)).find((n) =>
	//   n?.nodeType === "BgPane" && "type" in n &&
	//   (n.type === "background-image" || n.type === "artpack-image") &&
	//   "position" in n && (n.position === "left" || n.position === "right")
	for _, childID := range childNodeIDs {
		childData := mr.getNodeData(childID)
		if childData != nil && childData.NodeType == "BgPane" &&
			childData.BgImageData != nil &&
			(childData.BgImageData.Type == "background-image" || childData.BgImageData.Type == "artpack-image") &&
			(childData.BgImageData.Position == "left" || childData.BgImageData.Position == "right") {

			return &models.BackgroundNode{
				ID:       childData.ID,
				Position: childData.BgImageData.Position,
				Size:     childData.BgImageData.Size,
			}
		}
	}

	return nil
}

// getContentNodeIDs builds content nodes - everything except BgPane
// matches: const contentNodes = getCtx().getChildNodeIDs(nodeId);
func (mr *MarkdownRenderer) getContentNodeIDs(nodeID string) []string {
	// For Markdown nodes, we simply return all child node IDs
	// The BgPane filtering happens in the parent Pane component, not here
	return mr.nodeRenderer.GetChildNodeIDs(nodeID)
}

// getNodeData retrieves node data - FIXED TO USE REAL DATA
func (mr *MarkdownRenderer) getNodeData(nodeID string) *models.NodeRenderData {
	if mr.ctx.AllNodes == nil {
		return nil
	}

	// Use real data from context
	return mr.ctx.AllNodes[nodeID]
}
