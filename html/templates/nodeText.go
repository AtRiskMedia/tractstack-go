// Package templates provides NodeText.astro rendering functionality
package templates

import (
	"html"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// NodeTextRenderer handles NodeText.astro rendering logic
type NodeTextRenderer struct {
	ctx *models.RenderContext
}

// NewNodeTextRenderer creates a new node text renderer
func NewNodeTextRenderer(ctx *models.RenderContext) *NodeTextRenderer {
	return &NodeTextRenderer{ctx: ctx}
}

// Render implements the NodeText.astro rendering logic with spacing exactly
func (ntr *NodeTextRenderer) Render(nodeID string) string {
	nodeData := ntr.getNodeData(nodeID)
	if nodeData == nil {
		return ""
	}

	// Get parent node to check if it's a link - matches NodeText.astro exactly
	parentData := ntr.getParentNodeData(nodeData.ParentID)
	isLink := parentData != nil &&
		parentData.TagName != nil &&
		(*parentData.TagName == "a" || *parentData.TagName == "button")

	// Handle text content exactly like NodeText.astro
	var content string
	if nodeData.Copy != nil {
		trimmedCopy := strings.TrimSpace(*nodeData.Copy)
		if trimmedCopy == "" {
			// Empty or whitespace-only content gets non-breaking space + space
			content = "\u00A0 "
		} else {
			content = html.EscapeString(*nodeData.Copy)
		}
	} else {
		content = ""
	}

	// Add trailing space logic exactly like NodeText.astro: {isLink ? `` : " "}
	if !isLink {
		content += " "
	}

	return content
}

// getNodeData retrieves node data - FIXED TO USE REAL DATA
func (ntr *NodeTextRenderer) getNodeData(nodeID string) *models.NodeRenderData {
	if ntr.ctx.AllNodes == nil {
		return nil
	}

	// Use real data from context
	return ntr.ctx.AllNodes[nodeID]
}

// getParentNodeData retrieves parent node data - FIXED TO USE REAL DATA
func (ntr *NodeTextRenderer) getParentNodeData(parentID string) *models.NodeRenderData {
	if parentID == "" || ntr.ctx.AllNodes == nil {
		return nil
	}

	// Use real data from context
	return ntr.ctx.AllNodes[parentID]
}
