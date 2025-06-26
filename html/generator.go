// Package html provides the main HTML generator entry point
package html

import (
	"github.com/AtRiskMedia/tractstack-go/models"
)

// Generator is the main entry point for HTML generation, matching AstroNodesRenderer.astro
type Generator struct {
	ctx      *models.RenderContext
	renderer *NodeRendererImpl
}

// NewGenerator creates a new HTML generator
func NewGenerator(ctx *models.RenderContext) *Generator {
	return &Generator{
		ctx:      ctx,
		renderer: NewNodeRenderer(ctx),
	}
}

// Render generates HTML for a given node ID, matching AstroNodesRenderer.astro behavior
// If no ID provided, uses the root node ID from context
func (g *Generator) Render(id string) string {
	rootID := id
	if rootID == "" {
		rootID = g.getRootNodeID()
	}

	return g.renderer.RenderNode(rootID)
}

// RenderPaneFragment renders a pane fragment by ID - main entry point for /api/v1/fragments/panes/{id}
func (g *Generator) RenderPaneFragment(paneID string) string {
	// This is the primary function for the Stage 4 API endpoint
	return g.renderer.RenderNode(paneID)
}

// getRootNodeID gets the root node ID from context, defaulting to empty string
func (g *Generator) getRootNodeID() string {
	// For Stage 2, return empty string - will integrate with actual context in Stage 4
	return ""
}
