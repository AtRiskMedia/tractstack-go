// Package html provides CSS class extraction for pre-computed node styles
package html

import (
	"github.com/AtRiskMedia/tractstack-go/models"
)

// CSSProcessorImpl handles extraction of pre-computed CSS classes from nodes
type CSSProcessorImpl struct {
	ctx *models.RenderContext
}

// NewCSSProcessorImpl creates a new CSS processor
func NewCSSProcessorImpl(ctx *models.RenderContext) *CSSProcessorImpl {
	return &CSSProcessorImpl{ctx: ctx}
}

// GetNodeClasses extracts CSS classes for a node, matching getCtx().getNodeClasses() behavior
// Returns elementCss if available, otherwise returns the defaultClasses parameter
func (cp *CSSProcessorImpl) GetNodeClasses(nodeID string, defaultClasses string) string {
	nodeData := cp.getNodeRenderData(nodeID)
	if nodeData != nil && nodeData.ElementCSS != nil && *nodeData.ElementCSS != "" {
		return *nodeData.ElementCSS
	}

	return defaultClasses
}

// GetNodeStringStyles extracts inline styles for a node, matching getCtx().getNodeStringStyles() behavior
// Currently focuses on background color styles from pane nodes
func (cp *CSSProcessorImpl) GetNodeStringStyles(nodeID string) string {
	nodeData := cp.getNodeRenderData(nodeID)
	if nodeData == nil {
		return ""
	}

	// Handle Pane background color styles
	if nodeData.NodeType == "Pane" && nodeData.PaneData != nil && nodeData.PaneData.BgColour != nil {
		return "background-color: " + *nodeData.PaneData.BgColour
	}

	return ""
}

// ExtractParentCSSClasses extracts parentCss array from optionsPayload nodes
// Returns array of CSS class strings for nested wrapper elements
func (cp *CSSProcessorImpl) ExtractParentCSSClasses(optionsPayload map[string]any) []string {
	var parentCSSClasses []string

	if nodes, ok := optionsPayload["nodes"].([]any); ok {
		for _, nodeInterface := range nodes {
			if node, ok := nodeInterface.(map[string]any); ok {
				if parentCSS, ok := node["parentCss"].([]any); ok {
					for _, cssInterface := range parentCSS {
						if cssString, ok := cssInterface.(string); ok {
							parentCSSClasses = append(parentCSSClasses, cssString)
						}
					}
				}
			}
		}
	}

	return parentCSSClasses
}

// Helper function to get node render data - will be connected to cache in Stage 4
func (cp *CSSProcessorImpl) getNodeRenderData(nodeID string) *models.NodeRenderData {
	// For Stage 2, return basic data structure
	// This will be replaced with actual cache integration in Stage 4
	if cp.ctx.AllNodes == nil {
		return nil
	}

	// Placeholder - will be replaced with actual node data extraction
	return &models.NodeRenderData{
		ID:       nodeID,
		NodeType: "EmptyNode",
		Children: []string{},
	}
}
