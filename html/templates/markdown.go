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

// Render implements the Markdown.astro rendering logic with parentCSS layering
func (mr *MarkdownRenderer) Render(nodeID string, depth int) string {
	nodeData := mr.getNodeData(nodeID)
	if nodeData == nil {
		return `<div></div>`
	}

	parentCSS := nodeData.ParentCSS
	if parentCSS == nil {
		parentCSS = []string{}
	}

	// Check for positioned background image
	bgNode := mr.getPositionedBackgroundNode(nodeID)

	// Build content nodes - everything except BgPane
	contentNodes := mr.nodeRenderer.GetChildNodeIDs(nodeID)

	// Recursive parentCSS wrapping - matches Markdown.astro exactly
	if len(parentCSS) > 0 && depth < len(parentCSS) {
		var html strings.Builder
		html.WriteString(fmt.Sprintf(`<div class="%s"`, parentCSS[depth]))

		if depth == 0 {
			html.WriteString(` style="position: relative; z-index: 10;"`)
		}

		html.WriteString(`>`)

		// Recursive call - matches Astro.self pattern
		html.WriteString(mr.Render(nodeID, depth+1))

		html.WriteString(`</div>`)
		return html.String()
	}

	// Main content rendering
	var html strings.Builder
	html.WriteString(`<div style="position: relative; z-index: 10;">`)

	// Check if we should use flex layout for positioned background
	useFlexLayout := bgNode != nil && (bgNode.Position == "left" || bgNode.Position == "right")

	if useFlexLayout {
		// Flex layout with positioned background image
		flexDirection := "flex-col md:flex-row"
		if bgNode.Position == "right" {
			flexDirection = "flex-col md:flex-row-reverse"
		}

		html.WriteString(fmt.Sprintf(`<div class="flex flex-nowrap justify-center items-center gap-6 md:gap-10 xl:gap-12 %s">`, flexDirection))

		// Image Side - MUST USE RESPONSIVE MODIFIERS like Astro
		imageSizeClass := mr.getSizeClasses(bgNode.Size, "image")
		html.WriteString(fmt.Sprintf(`<div class="relative overflow-hidden %s">`, imageSizeClass))
		html.WriteString(mr.nodeRenderer.RenderNode(bgNode.ID))
		html.WriteString(`</div>`)

		// Content Side - MUST USE RESPONSIVE MODIFIERS like Astro
		contentSizeClass := mr.getSizeClasses(bgNode.Size, "content")
		html.WriteString(fmt.Sprintf(`<div class="%s">`, contentSizeClass))
		for _, childID := range contentNodes {
			html.WriteString(mr.nodeRenderer.RenderNode(childID))
		}
		html.WriteString(`</div>`)

		html.WriteString(`</div>`)
	} else {
		// Standard layout - render content nodes directly
		for _, childID := range contentNodes {
			html.WriteString(mr.nodeRenderer.RenderNode(childID))
		}
	}

	html.WriteString(`</div>`)
	return html.String()
}

// getSizeClasses helper function matching Markdown.astro exactly
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

	parentPaneID := nodeData.ParentID
	if parentPaneID == "" {
		return nil
	}

	// Get child nodes of parent pane
	childNodeIDs := mr.nodeRenderer.GetChildNodeIDs(parentPaneID)

	// Look for positioned background image
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

// getNodeData retrieves node data - FIXED TO USE REAL DATA
func (mr *MarkdownRenderer) getNodeData(nodeID string) *models.NodeRenderData {
	if mr.ctx.AllNodes == nil {
		return nil
	}

	// Use real data from context
	return mr.ctx.AllNodes[nodeID]
}
