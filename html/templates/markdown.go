// Package templates provides Markdown.astro rendering functionality
package templates

import (
	"html"
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

	// Recursive parentCSS wrapping - matches Markdown.astro exactly
	// {parentCss.length > 0 && depth < parentCss.length ? (...) : (...)}
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

	// Main content rendering - render the markdown node's direct children
	var sb strings.Builder
	sb.WriteString(`<div style="position: relative; z-index: 10;">`)

	// Get direct children of this markdown node
	childNodeIDs := mr.nodeRenderer.GetChildNodeIDs(nodeID)
	for _, childID := range childNodeIDs {
		sb.WriteString(mr.nodeRenderer.RenderNode(childID))
	}

	sb.WriteString(`</div>`)
	return sb.String()
}

// getNodeData retrieves node data from real context
func (mr *MarkdownRenderer) getNodeData(nodeID string) *models.NodeRenderData {
	if mr.ctx.AllNodes == nil {
		return nil
	}

	return mr.ctx.AllNodes[nodeID]
}
