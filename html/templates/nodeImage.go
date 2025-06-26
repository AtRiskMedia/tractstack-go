// Package templates provides NodeImg.astro rendering functionality
package templates

import (
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// NodeImgRenderer handles NodeImg.astro rendering logic
type NodeImgRenderer struct {
	ctx *models.RenderContext
}

// NewNodeImgRenderer creates a new node image renderer
func NewNodeImgRenderer(ctx *models.RenderContext) *NodeImgRenderer {
	return &NodeImgRenderer{ctx: ctx}
}

// Render implements the NodeImg.astro rendering logic exactly
func (nir *NodeImgRenderer) Render(nodeID string) string {
	nodeData := nir.getNodeData(nodeID)
	if nodeData == nil {
		return `<img alt="missing image" />`
	}

	var html strings.Builder
	html.WriteString(`<img`)

	// Add src attribute
	if nodeData.ImageURL != nil && *nodeData.ImageURL != "" {
		html.WriteString(fmt.Sprintf(` src="%s"`, *nodeData.ImageURL))
	}

	// Add srcSet attribute if available - matches NodeImg.astro: {...node.srcSet ? { srcSet: node.srcSet } : {}}
	if nodeData.SrcSet != nil && *nodeData.SrcSet != "" {
		html.WriteString(fmt.Sprintf(` srcset="%s"`, *nodeData.SrcSet))
	}

	// Add CSS classes - matches NodeImg.astro: class={getCtx().getNodeClasses(nodeId, `auto`)}
	cssClasses := ""
	if nodeData.ElementCSS != nil && *nodeData.ElementCSS != "" {
		cssClasses = *nodeData.ElementCSS
	} else {
		cssClasses = "auto" // Default fallback
	}
	html.WriteString(fmt.Sprintf(` class="%s"`, cssClasses))

	// Add alt attribute - matches NodeImg.astro: alt={node.alt}
	altText := "image"
	if nodeData.AltText != nil && *nodeData.AltText != "" {
		altText = *nodeData.AltText
	}
	html.WriteString(fmt.Sprintf(` alt="%s"`, altText))

	html.WriteString(` />`)
	return html.String()
}

// getNodeData retrieves node data from real context
func (nir *NodeImgRenderer) getNodeData(nodeID string) *models.NodeRenderData {
	if nir.ctx.AllNodes == nil {
		return nil
	}

	return nir.ctx.AllNodes[nodeID]
}
