// Package templates provides NodeText.astro rendering functionality
package templates

import (
	"html/template"
	"log"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
)

// textEscaper is a pre-parsed template used for securely escaping text content.
// This replaces the direct call to html.EscapeString() for consistency across the project.
var textEscaper = template.Must(template.New("textEscaper").Parse("{{.}}"))

// NodeTextRenderer handles NodeText.astro rendering logic
type NodeTextRenderer struct {
	ctx *rendering.RenderContext
}

// NewNodeTextRenderer creates a new node text renderer
func NewNodeTextRenderer(ctx *rendering.RenderContext) *NodeTextRenderer {
	return &NodeTextRenderer{ctx: ctx}
}

// Render implements the EXACT NodeText.astro rendering logic with enhanced spacing
func (ntr *NodeTextRenderer) Render(nodeID string) string {
	nodeData := ntr.getNodeData(nodeID)
	if nodeData == nil {
		return ""
	}

	// Get parent node to check if it's a link - EXACT match NodeText.astro
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
			// Use the pre-parsed template to securely escape the text content for consistency.
			var escapedContentBuilder strings.Builder
			err := textEscaper.Execute(&escapedContentBuilder, *nodeData.Copy)
			if err != nil {
				log.Printf("ERROR: Failed to escape text content for nodeID %s: %v", nodeID, err)
				return "<!-- error escaping text -->"
			}
			content = escapedContentBuilder.String()
		}
	} else {
		content = ""
	}

	// Check if this text should have   around it (for button spacing)
	// Based on the expected output:    <a class="..."> ... </a>
	needsLeadingNbsp := ntr.shouldAddLeadingNbsp(nodeID)
	needsTrailingNbsp := ntr.shouldAddTrailingNbsp(nodeID)

	var result strings.Builder

	// Add leading   if needed
	if needsLeadingNbsp {
		result.WriteString(" ")
	}

	// Add the main content
	result.WriteString(content)

	// Add trailing   if needed, otherwise regular space for non-links
	if needsTrailingNbsp {
		result.WriteString(" ")
	} else if !isLink {
		// EXACT match NodeText.astro: {isLink ? `` : " "}
		result.WriteString(" ")
	}

	return result.String()
}

// shouldAddLeadingNbsp checks if this text node should have leading
func (ntr *NodeTextRenderer) shouldAddLeadingNbsp(nodeID string) bool {
	nodeData := ntr.getNodeData(nodeID)
	if nodeData == nil || nodeData.ParentID == "" {
		return false
	}

	// Check if the previous sibling is a button/link and this is spacing text
	parentID := nodeData.ParentID
	childNodeIDs := ntr.getChildNodeIDs(parentID)

	currentIndex := -1
	for i, childID := range childNodeIDs {
		if childID == nodeID {
			currentIndex = i
			break
		}
	}

	if currentIndex <= 0 {
		return false
	}

	// Check previous sibling
	prevNodeID := childNodeIDs[currentIndex-1]
	prevNodeData := ntr.getNodeData(prevNodeID)
	if prevNodeData != nil && prevNodeData.TagName != nil {
		prevTag := *prevNodeData.TagName
		if (prevTag == "a" || prevTag == "button") && ntr.isSpacingText(nodeData) {
			return true
		}
	}

	return false
}

// shouldAddTrailingNbsp checks if this text node should have trailing
func (ntr *NodeTextRenderer) shouldAddTrailingNbsp(nodeID string) bool {
	nodeData := ntr.getNodeData(nodeID)
	if nodeData == nil || nodeData.ParentID == "" {
		return false
	}

	// Check if the next sibling is a button/link and this is spacing text
	parentID := nodeData.ParentID
	childNodeIDs := ntr.getChildNodeIDs(parentID)

	currentIndex := -1
	for i, childID := range childNodeIDs {
		if childID == nodeID {
			currentIndex = i
			break
		}
	}

	if currentIndex == -1 || currentIndex >= len(childNodeIDs)-1 {
		return false
	}

	// Check next sibling
	nextNodeID := childNodeIDs[currentIndex+1]
	nextNodeData := ntr.getNodeData(nextNodeID)
	if nextNodeData != nil && nextNodeData.TagName != nil {
		nextTag := *nextNodeData.TagName
		if (nextTag == "a" || nextTag == "button") && ntr.isSpacingText(nodeData) {
			return true
		}
	}

	return false
}

// isSpacingText checks if this is a spacing text node (contains only whitespace/ )
func (ntr *NodeTextRenderer) isSpacingText(nodeData *rendering.NodeRenderData) bool {
	if nodeData.Copy == nil {
		return false
	}

	text := strings.TrimSpace(*nodeData.Copy)
	// Check if it's empty, contains only spaces, or is already
	return text == "" || text == " " || text == " " || text == "\u00A0"
}

// Helper methods

func (ntr *NodeTextRenderer) getChildNodeIDs(parentID string) []string {
	if ntr.ctx.ParentNodes == nil {
		return []string{}
	}
	if children, exists := ntr.ctx.ParentNodes[parentID]; exists {
		return children
	}
	return []string{}
}

func (ntr *NodeTextRenderer) getNodeData(nodeID string) *rendering.NodeRenderData {
	if ntr.ctx.AllNodes == nil {
		return nil
	}
	return ntr.ctx.AllNodes[nodeID]
}

func (ntr *NodeTextRenderer) getParentNodeData(parentID string) *rendering.NodeRenderData {
	if parentID == "" || ntr.ctx.AllNodes == nil {
		return nil
	}
	return ntr.ctx.AllNodes[parentID]
}
