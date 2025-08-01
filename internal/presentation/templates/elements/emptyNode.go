// Package templates provides node template rendering functions
package templates

import "github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"

// EmptyNodeRenderer renders empty placeholder nodes
type EmptyNodeRenderer struct {
	ctx *rendering.RenderContext
}

// NewEmptyNodeRenderer creates a new empty node renderer
func NewEmptyNodeRenderer(ctx *rendering.RenderContext) *EmptyNodeRenderer {
	return &EmptyNodeRenderer{ctx: ctx}
}

// Render returns an empty div element, matching EmptyNode.astro behavior
func (enr *EmptyNodeRenderer) Render(nodeID string) string {
	return `<div></div>`
}

// RenderEmpty is a static method for quick empty node rendering
func RenderEmpty() string {
	return `<div></div>`
}
