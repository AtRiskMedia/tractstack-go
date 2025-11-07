// Package templates provides CSS class extraction for pre-computed node styles
package templates

import (
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
)

// CSSProcessorImpl handles extraction of pre-computed CSS classes from nodes
type CSSProcessorImpl struct {
	ctx *rendering.RenderContext
}

// NewCSSProcessorImpl creates a new CSS processor
func NewCSSProcessorImpl(ctx *rendering.RenderContext) *CSSProcessorImpl {
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

// GetNodeGridCSS extracts and joins responsive classes from the node's custom data
func (cp *CSSProcessorImpl) GetNodeGridCSS(nodeID string) string {
	nodeData := cp.getNodeRenderData(nodeID)
	if nodeData == nil || nodeData.CustomData == nil {
		return ""
	}

	gridClassesData, ok := nodeData.CustomData["gridClasses"]
	if !ok {
		return ""
	}

	// The frontend stores grid classes as a map with breakpoint keys
	payload, ok := gridClassesData.(map[string]any)
	if !ok {
		return ""
	}

	// This logic duplicates the pattern from ExtractParentCSSClasses, but for a single map
	mobileClasses := getClassesFromBreakpoint(payload, "mobile")
	tabletClasses := getClassesFromBreakpoint(payload, "tablet")
	desktopClasses := getClassesFromBreakpoint(payload, "desktop")

	var allClassLists [][]string
	if len(mobileClasses) > 0 {
		allClassLists = append(allClassLists, mobileClasses)
	}
	if len(tabletClasses) > 0 {
		allClassLists = append(allClassLists, tabletClasses)
	}
	if len(desktopClasses) > 0 {
		allClassLists = append(allClassLists, desktopClasses)
	}

	var finalClasses []string
	for _, classList := range allClassLists {
		finalClasses = append(finalClasses, classList...)
	}

	return strings.Join(finalClasses, " ")
}

// getClassesFromBreakpoint safely extracts a list of class strings from a breakpoint map
func getClassesFromBreakpoint(classes map[string]any, breakpoint string) []string {
	var classList []string
	if bpClasses, ok := classes[breakpoint].(map[string]any); ok {
		for classStr := range bpClasses {
			classList = append(classList, classStr)
		}
	}
	return classList
}

// getNodeRenderData retrieves node data
func (cp *CSSProcessorImpl) getNodeRenderData(nodeID string) *rendering.NodeRenderData {
	if cp.ctx.AllNodes == nil {
		return nil
	}

	nodeInterface, exists := cp.ctx.AllNodes[nodeID]
	if !exists {
		return nil
	}

	// nodeInterface is already *rendering.NodeRenderData based on the map type
	return nodeInterface
}
