// Package templates provides Markdown.astro rendering functionality
package templates

import (
	"html"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
)

// MarkdownRenderer handles Markdown.astro rendering logic
type MarkdownRenderer struct {
	ctx          *rendering.RenderContext
	nodeRenderer NodeRenderer
}

// NodeRenderer interface for child node rendering
type NodeRenderer interface {
	RenderNode(nodeID string) string
	GetChildNodeIDs(nodeID string) []string
}

// NewMarkdownRenderer creates a new markdown renderer
func NewMarkdownRenderer(ctx *rendering.RenderContext, nodeRenderer NodeRenderer) *MarkdownRenderer {
	return &MarkdownRenderer{
		ctx:          ctx,
		nodeRenderer: nodeRenderer,
	}
}

// Render implements the COMPLETE Markdown.astro rendering logic with bgNode detection and flex layout
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

	// Check for positioned background image in parent pane
	bgNode := mr.getBackgroundNodeFromParent(nodeID)
	useFlexLayout := bgNode != nil && (bgNode.Position == "left" || bgNode.Position == "right")

	// Recursive parentCSS wrapping - matches Markdown.astro exactly
	if len(parentCSS) > 0 && depth < len(parentCSS) {
		var sb strings.Builder
		sb.WriteString(`<div class="`)
		sb.WriteString(html.EscapeString(parentCSS[depth]))
		sb.WriteString(`"`)

		// Add style for depth 0 - matches: style={depth === 0 ? "position: relative; z-index: 10;" : ""}
		if depth == 0 {
			sb.WriteString(` style="position: relative; z-index: 10;"`)
		}

		sb.WriteString(`>`)

		// Recursive call - matches Astro.self pattern: <Astro.self nodeId={nodeId} depth={depth + 1} />
		sb.WriteString(mr.Render(nodeID, depth+1))

		sb.WriteString(`</div>`)
		return sb.String()
	}

	// Main content rendering with flex layout detection
	var sb strings.Builder
	sb.WriteString(`<div style="position: relative; z-index: 10;">`)

	if useFlexLayout {
		// Create flex layout for left/right positioned images
		flexDirection := "flex-col md:flex-row"
		if bgNode.Position == "right" {
			flexDirection = "flex-col md:flex-row-reverse"
		}

		sb.WriteString(`<div class="flex flex-nowrap justify-center items-center gap-6 md:gap-10 xl:gap-12 `)
		sb.WriteString(flexDirection)
		sb.WriteString(`">`)

		// Image side
		imageSizeClass := mr.getSizeClasses(bgNode.Size, "image")
		sb.WriteString(`<div class="relative overflow-hidden `)
		sb.WriteString(imageSizeClass)
		sb.WriteString(`">`)
		sb.WriteString(mr.nodeRenderer.RenderNode(bgNode.ID))
		sb.WriteString(`</div>`)

		// Content side
		contentSizeClass := mr.getSizeClasses(bgNode.Size, "content")
		sb.WriteString(`<div class="`)
		sb.WriteString(contentSizeClass)
		sb.WriteString(`">`)

		// Render content children (excluding BgPane)
		contentChildren := mr.getContentChildren(nodeID)
		for _, childID := range contentChildren {
			sb.WriteString(mr.nodeRenderer.RenderNode(childID))
		}

		sb.WriteString(`</div>`)
		sb.WriteString(`</div>`)
	} else {
		// Normal rendering - render all direct children
		childNodeIDs := mr.nodeRenderer.GetChildNodeIDs(nodeID)
		for _, childID := range childNodeIDs {
			sb.WriteString(mr.nodeRenderer.RenderNode(childID))
		}
	}

	sb.WriteString(`</div>`)
	return sb.String()
}

// getBackgroundNodeFromParent checks parent pane for positioned background image
func (mr *MarkdownRenderer) getBackgroundNodeFromParent(nodeID string) *rendering.BackgroundNode {
	nodeData := mr.getNodeData(nodeID)
	if nodeData == nil || nodeData.ParentID == "" {
		return nil
	}

	parentID := nodeData.ParentID
	parentData := mr.getNodeData(parentID)
	if parentData == nil || parentData.NodeType != "Pane" {
		return nil
	}

	// Get parent pane's children to find BgPane
	childNodeIDs := mr.nodeRenderer.GetChildNodeIDs(parentID)
	for _, childID := range childNodeIDs {
		childData := mr.getNodeData(childID)
		if childData != nil && childData.NodeType == "BgPane" &&
			childData.BgImageData != nil &&
			(childData.BgImageData.Type == "background-image" || childData.BgImageData.Type == "artpack-image") &&
			(childData.BgImageData.Position == "left" || childData.BgImageData.Position == "right") {

			return &rendering.BackgroundNode{
				ID:       childData.ID,
				Position: childData.BgImageData.Position,
				Size:     childData.BgImageData.Size,
			}
		}
	}
	return nil
}

// getContentChildren returns child node IDs excluding any from the parent that would be handled by flex layout
func (mr *MarkdownRenderer) getContentChildren(nodeID string) []string {
	// For markdown nodes, just return direct children since BgPane is handled separately
	return mr.nodeRenderer.GetChildNodeIDs(nodeID)
}

// getSizeClasses returns responsive size classes for flex layout
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

// getNodeData retrieves node data from context
func (mr *MarkdownRenderer) getNodeData(nodeID string) *rendering.NodeRenderData {
	if mr.ctx.AllNodes == nil {
		return nil
	}
	return mr.ctx.AllNodes[nodeID]
}
