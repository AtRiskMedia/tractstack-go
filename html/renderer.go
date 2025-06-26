// Package html provides node rendering functionality for nodes-compositor
package html

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// CSSProcessor interface for dependency injection
type CSSProcessor interface {
	GetNodeClasses(nodeID string, defaultClasses string) string
	GetNodeStringStyles(nodeID string) string
	ExtractParentCSSClasses(optionsPayload map[string]interface{}) []string
}

// NodeRenderer interface for child node rendering
type NodeRenderer interface {
	RenderNode(nodeID string) string
	GetChildNodeIDs(nodeID string) []string
}

// NodeRendererImpl handles the core Node.astro switch logic
type NodeRendererImpl struct {
	ctx          *models.RenderContext
	cssProcessor *CSSProcessorImpl
}

// NewNodeRenderer creates a new node renderer with context
func NewNodeRenderer(ctx *models.RenderContext) *NodeRendererImpl {
	renderer := &NodeRendererImpl{ctx: ctx}
	renderer.cssProcessor = NewCSSProcessorImpl(ctx)
	return renderer
}

// RenderNode renders a node by ID, implementing Node.astro switch logic
func (nr *NodeRendererImpl) RenderNode(nodeID string) string {
	if nodeID == "" {
		return nr.renderEmptyNode()
	}

	// Get node data (simplified for Stage 1)
	nodeData := nr.getNodeRenderData(nodeID)
	if nodeData == nil {
		return nr.renderEmptyNode()
	}

	// Determine node type for switching - matches Node.astro logic
	nodeType := nodeData.NodeType
	if nodeData.TagName != nil {
		nodeType = *nodeData.TagName
	}

	// Handle code nodes with widget hooks first (matches Node.astro special case)
	if nodeType == "code" {
		hookData := nr.parseCodeHook(nodeData)
		if hookData != nil {
			return nr.renderWidget(nodeID, hookData)
		}
		return nr.renderEmptyNode()
	}

	// Main switch statement matching Node.astro getElement function
	switch nodeType {
	case "Pane":
		return nr.renderPane(nodeID)
	case "StoryFragment":
		return nr.renderStoryFragment(nodeID)
	case "Markdown":
		return nr.renderMarkdown(nodeID)
	case "BgPane":
		return nr.renderBgPaneWrapper(nodeID)
	case "TagElement":
		return nr.renderTagElement(nodeID)
	// Basic HTML tag elements
	case "h2", "h3", "h4", "p", "em", "strong", "li", "ol", "ul", "aside":
		return nr.renderNodeBasicTag(nodeID)
	case "text":
		return nr.renderNodeText(nodeID)
	case "img":
		return nr.renderNodeImg(nodeID)
	case "button":
		return nr.renderNodeButton(nodeID)
	case "a":
		return nr.renderNodeA(nodeID)
	case "impression":
		return nr.renderEmptyNode()
	default:
		// Log miss like Node.astro does
		fmt.Printf("Node.go miss on %s\n", nodeType)
		return nr.renderEmptyNode()
	}
}

// parseCodeHook implements Node.astro parseCodeHook function logic
func (nr *NodeRendererImpl) parseCodeHook(nodeData *models.NodeRenderData) *models.CodeHook {
	if nodeData.Copy == nil {
		return nil
	}

	// Match Node.astro regexp patterns
	regexpHook := regexp.MustCompile(`^(identifyAs|youtube|bunny|bunnyContext|toggle|resource|belief|signup)\((.*)\)$`)
	hookMatch := regexpHook.FindStringSubmatch(*nodeData.Copy)

	if len(hookMatch) < 3 {
		return nil
	}

	hook := hookMatch[1]
	paramString := hookMatch[2]

	// Parse parameters using similar logic to Node.astro
	regexpValues := regexp.MustCompile(`((?:[^\\|]+|\\\|?)+)`)
	paramMatches := regexpValues.FindAllString(paramString, -1)

	var value1, value2 *string
	var value3 string

	if len(paramMatches) > 0 {
		value1 = &paramMatches[0]
	}
	if len(paramMatches) > 1 {
		value2 = &paramMatches[1]
	}
	if len(paramMatches) > 2 {
		value3 = paramMatches[2]
	}

	return &models.CodeHook{
		Hook:   hook,
		Value1: value1,
		Value2: value2,
		Value3: value3,
	}
}

// getNodeRenderData retrieves and prepares node data for rendering
func (nr *NodeRendererImpl) getNodeRenderData(nodeID string) *models.NodeRenderData {
	// For Stage 1, return minimal data to allow compilation
	// This will be expanded in Stage 2 when we integrate with cache system
	if nr.ctx.AllNodes == nil {
		return nil
	}

	// Placeholder implementation - will be replaced with actual cache integration
	return &models.NodeRenderData{
		ID:       nodeID,
		NodeType: "EmptyNode", // Default for Stage 1
		Children: []string{},
	}
}

// getChildNodeIDs returns child node IDs for a given parent
func (nr *NodeRendererImpl) getChildNodeIDs(nodeID string) []string {
	if nr.ctx.ParentNodes == nil {
		return []string{}
	}

	children, exists := nr.ctx.ParentNodes[nodeID]
	if !exists {
		return []string{}
	}

	return children
}

// GetChildNodeIDs is the public interface method
func (nr *NodeRendererImpl) GetChildNodeIDs(nodeID string) []string {
	return nr.getChildNodeIDs(nodeID)
}

// Stage 1 placeholder implementations - will be expanded in later stages

func (nr *NodeRendererImpl) renderEmptyNode() string {
	return `<div></div>`
}

func (nr *NodeRendererImpl) renderPane(nodeID string) string {
	// For Stage 2, create pane renderer on demand to avoid circular imports
	paneRenderer := &PaneRenderer{
		ctx:          nr.ctx,
		cssProcessor: nr.cssProcessor, // CSSProcessor implements CSSProcessor interface
		nodeRenderer: nr,              // NodeRenderer implements NodeRenderer interface
	}
	return paneRenderer.Render(nodeID)
}

func (nr *NodeRendererImpl) renderStoryFragment(nodeID string) string {
	return nr.renderEmptyNode() // Stage 1 placeholder
}

func (nr *NodeRendererImpl) renderMarkdown(nodeID string) string {
	return nr.renderEmptyNode() // Stage 1 placeholder
}

func (nr *NodeRendererImpl) renderBgPaneWrapper(nodeID string) string {
	return nr.renderEmptyNode() // Stage 1 placeholder
}

func (nr *NodeRendererImpl) renderTagElement(nodeID string) string {
	return nr.renderEmptyNode() // Stage 1 placeholder
}

func (nr *NodeRendererImpl) renderNodeBasicTag(nodeID string) string {
	return nr.renderEmptyNode() // Stage 1 placeholder
}

func (nr *NodeRendererImpl) renderNodeText(nodeID string) string {
	return nr.renderEmptyNode() // Stage 1 placeholder
}

func (nr *NodeRendererImpl) renderNodeImg(nodeID string) string {
	return nr.renderEmptyNode() // Stage 1 placeholder
}

func (nr *NodeRendererImpl) renderNodeButton(nodeID string) string {
	return nr.renderEmptyNode() // Stage 1 placeholder
}

func (nr *NodeRendererImpl) renderNodeA(nodeID string) string {
	return nr.renderEmptyNode() // Stage 1 placeholder
}

func (nr *NodeRendererImpl) renderWidget(nodeID string, hook *models.CodeHook) string {
	return nr.renderEmptyNode() // Stage 1 placeholder
}

// PaneRenderer handles Pane.astro rendering logic
type PaneRenderer struct {
	ctx          *models.RenderContext
	cssProcessor CSSProcessor // Interface
	nodeRenderer NodeRenderer // Interface
}

// Render implements the Pane.astro rendering logic
func (pr *PaneRenderer) Render(nodeID string) string {
	nodeData := pr.getNodeData(nodeID)
	if nodeData == nil || nodeData.PaneData == nil {
		return `<div></div>` // EmptyNode fallback
	}

	paneData := nodeData.PaneData
	beliefs := pr.getPaneBeliefs(nodeID)
	isDecorative := paneData.IsDecorative

	// Build wrapper classes - matches Pane.astro: `grid ${getCtx().getNodeClasses(nodeId, 'auto')}`
	wrapperClasses := fmt.Sprintf("grid %s", pr.cssProcessor.GetNodeClasses(nodeID, "auto"))

	// Content classes and styles - matches Pane.astro
	contentClasses := "relative w-full h-auto justify-self-start"
	contentStyles := pr.cssProcessor.GetNodeStringStyles(nodeID) + "; grid-area: 1/1/1/1; position: relative; z-index: 1"

	// Get child nodes
	childNodeIDs := pr.nodeRenderer.GetChildNodeIDs(nodeID)

	// Build the pane HTML
	var html strings.Builder

	// Opening div with pane ID
	html.WriteString(fmt.Sprintf(`<div id="pane-%s" class="`, nodeID))

	if isDecorative {
		html.WriteString(wrapperClasses)
	} else {
		// Non-decorative panes get additional wrapper
		html.WriteString(wrapperClasses)
	}

	html.WriteString(`"`)

	// Add inline styles if any
	nodeStyles := pr.cssProcessor.GetNodeStringStyles(nodeID)
	if nodeStyles != "" {
		html.WriteString(` style="`)
		html.WriteString(nodeStyles)
		html.WriteString(`"`)
	}

	html.WriteString(`>`)

	// Handle belief-based visibility
	if !pr.checkBeliefVisibility(beliefs) {
		html.WriteString(`</div>`)
		return html.String() // Return empty pane if beliefs don't match
	}

	// Standard content wrapper
	if !isDecorative {
		html.WriteString(fmt.Sprintf(`<div class="%s" style="%s">`, contentClasses, contentStyles))
	}

	// Render all child nodes
	for _, childID := range childNodeIDs {
		html.WriteString(pr.nodeRenderer.RenderNode(childID))
	}

	if !isDecorative {
		html.WriteString(`</div>`)
	}

	// Add Filter component for belief handling (placeholder for Stage 2)
	if len(beliefs) > 0 {
		html.WriteString(`<!-- Filter component placeholder -->`)
	}

	html.WriteString(`</div>`)
	return html.String()
}

func (pr *PaneRenderer) getPaneBeliefs(nodeID string) map[string]interface{} {
	nodeData := pr.getNodeData(nodeID)
	if nodeData == nil || nodeData.PaneData == nil {
		return make(map[string]interface{})
	}

	beliefs := make(map[string]interface{})

	// Combine held and withheld beliefs
	if nodeData.PaneData.HeldBeliefs != nil {
		for k, v := range nodeData.PaneData.HeldBeliefs {
			beliefs[k] = v
		}
	}
	if nodeData.PaneData.WithheldBeliefs != nil {
		for k, v := range nodeData.PaneData.WithheldBeliefs {
			beliefs[k] = v
		}
	}

	return beliefs
}

func (pr *PaneRenderer) checkBeliefVisibility(beliefs map[string]interface{}) bool {
	// For Stage 2, always return true - implement belief checking in Stage 4
	// This will check user state against held/withheld beliefs
	return true
}

func (pr *PaneRenderer) getNodeData(nodeID string) *models.NodeRenderData {
	// For Stage 2, return basic structure - will connect to cache in Stage 4
	if pr.ctx.AllNodes == nil {
		return nil
	}

	// Placeholder implementation
	return &models.NodeRenderData{
		ID:       nodeID,
		NodeType: "Pane",
		PaneData: &models.PaneRenderData{
			Title:        "Test Pane",
			Slug:         "test-pane",
			IsDecorative: false,
		},
		Children: []string{},
	}
}
