// Package templates provides TagElement.astro rendering functionality
package templates

import (
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
)

// TagElementRenderer handles TagElement.astro rendering logic
type TagElementRenderer struct {
	ctx          *rendering.RenderContext
	nodeRenderer NodeRenderer
}

// NewTagElementRenderer creates a new tag element renderer
func NewTagElementRenderer(ctx *rendering.RenderContext, nodeRenderer NodeRenderer) *TagElementRenderer {
	return &TagElementRenderer{
		ctx:          ctx,
		nodeRenderer: nodeRenderer,
	}
}

// Render implements the TagElement.astro rendering logic - child iteration exactly
func (ter *TagElementRenderer) Render(nodeID string) string {
	// Get child node IDs exactly like TagElement.astro
	childNodeIDs := ter.nodeRenderer.GetChildNodeIDs(nodeID)

	// Render all child nodes - matches getCtx().getChildNodeIDs(nodeId).map((id: string) => <Node nodeId={id} />)
	var html strings.Builder

	for _, childID := range childNodeIDs {
		html.WriteString(ter.nodeRenderer.RenderNode(childID))
	}

	return html.String()
}
