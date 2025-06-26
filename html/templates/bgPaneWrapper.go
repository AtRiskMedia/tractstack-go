// Package templates provides BgPaneWrapper.astro rendering functionality
package templates

import (
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// BgPaneWrapperRenderer handles BgPaneWrapper.astro rendering logic
type BgPaneWrapperRenderer struct {
	ctx *models.RenderContext
}

// NewBgPaneWrapperRenderer creates a new background pane wrapper renderer
func NewBgPaneWrapperRenderer(ctx *models.RenderContext) *BgPaneWrapperRenderer {
	return &BgPaneWrapperRenderer{ctx: ctx}
}

// Render implements the BgPaneWrapper.astro rendering logic exactly
func (bpwr *BgPaneWrapperRenderer) Render(nodeID string) string {
	nodeData := bpwr.getNodeData(nodeID)
	if nodeData == nil {
		return `<div class="bg-error">Missing background node</div>`
	}

	// Check if this is a background image node or visual break
	// Matches BgPaneWrapper.astro: node && (isBgImageNode(node) || isArtpackImageNode(node))
	if bpwr.isBgImageNode(nodeData) || bpwr.isArtpackImageNode(nodeData) {
		return bpwr.renderBgImage(nodeData)
	} else {
		return bpwr.renderBgVisualBreak(nodeData)
	}
}

// isBgImageNode checks if node is a background image node
func (bpwr *BgPaneWrapperRenderer) isBgImageNode(nodeData *models.NodeRenderData) bool {
	return nodeData.BgImageData != nil && nodeData.BgImageData.Type == "background-image"
}

// isArtpackImageNode checks if node is an artpack image node
func (bpwr *BgPaneWrapperRenderer) isArtpackImageNode(nodeData *models.NodeRenderData) bool {
	return nodeData.BgImageData != nil && nodeData.BgImageData.Type == "artpack-image"
}

// renderBgImage renders background image - matches <BgImage payload={node as BgImageNode} />
func (bpwr *BgPaneWrapperRenderer) renderBgImage(nodeData *models.NodeRenderData) string {
	if nodeData.BgImageData == nil {
		return `<div class="bg-error">Missing image data</div>`
	}

	var html strings.Builder

	// Build background image container
	html.WriteString(`<div class="bg-image-container"`)

	// Add positioning based on BgImageData
	position := nodeData.BgImageData.Position

	// Apply positioning classes
	switch position {
	case "background":
		html.WriteString(` style="position: absolute; top: 0; left: 0; width: 100%; height: 100%; z-index: 0;"`)
	case "left", "right":
		// These are handled in Markdown.astro with flex layout
		html.WriteString(fmt.Sprintf(` data-position="%s"`, position))
	case "leftBleed", "rightBleed":
		// These are handled in Pane.astro with flex layout
		html.WriteString(fmt.Sprintf(` data-position="%s"`, position))
	}

	html.WriteString(`>`)

	// Render the actual image
	if nodeData.ImageURL != nil && *nodeData.ImageURL != "" {
		html.WriteString(`<img`)
		html.WriteString(fmt.Sprintf(` src="%s"`, *nodeData.ImageURL))

		if nodeData.SrcSet != nil && *nodeData.SrcSet != "" {
			html.WriteString(fmt.Sprintf(` srcset="%s"`, *nodeData.SrcSet))
		}

		html.WriteString(` class="w-full h-full object-cover"`)

		altText := "background image"
		if nodeData.AltText != nil && *nodeData.AltText != "" {
			altText = *nodeData.AltText
		}
		html.WriteString(fmt.Sprintf(` alt="%s"`, altText))

		html.WriteString(` />`)
	}

	html.WriteString(`</div>`)
	return html.String()
}

// renderBgVisualBreak renders visual break - matches <BgVisualBreak payload={node as VisualBreakNode} />
func (bpwr *BgPaneWrapperRenderer) renderBgVisualBreak(nodeData *models.NodeRenderData) string {
	var html strings.Builder

	// Create a visual break element
	html.WriteString(`<div class="visual-break"`)

	// Add any element CSS
	if nodeData.ElementCSS != nil && *nodeData.ElementCSS != "" {
		html.WriteString(fmt.Sprintf(` class="%s"`, *nodeData.ElementCSS))
	}

	html.WriteString(`>`)

	// Visual break content - can be expanded based on actual VisualBreak patterns
	html.WriteString(`<div class="visual-break-content"></div>`)

	html.WriteString(`</div>`)
	return html.String()
}

// getNodeData retrieves node data from real context
func (bpwr *BgPaneWrapperRenderer) getNodeData(nodeID string) *models.NodeRenderData {
	if bpwr.ctx.AllNodes == nil {
		return nil
	}

	return bpwr.ctx.AllNodes[nodeID]
}
