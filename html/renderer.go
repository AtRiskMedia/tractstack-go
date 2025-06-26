// Package html provides node rendering functionality for nodes-compositor
package html

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/html/templates"
	"github.com/AtRiskMedia/tractstack-go/models"
)

// CSSProcessor interface for dependency injection
type CSSProcessor interface {
	GetNodeClasses(nodeID string, defaultClasses string) string
	GetNodeStringStyles(nodeID string) string
	ExtractParentCSSClasses(optionsPayload map[string]any) []string
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

	// Get node data - FIXED TO USE REAL DATA
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

	// Match Node.astro regexp patterns exactly
	regexpHook := regexp.MustCompile(`^(identifyAs|youtube|bunny|bunnyContext|toggle|resource|belief|signup)\(([^)]*)\)$`)
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

// getNodeRenderData retrieves and prepares node data for rendering - FIXED TO USE REAL DATA
func (nr *NodeRendererImpl) getNodeRenderData(nodeID string) *models.NodeRenderData {
	if nr.ctx.AllNodes == nil {
		return nil
	}

	// Use real data from context
	nodeData := nr.ctx.AllNodes[nodeID]
	if nodeData == nil {
		return nil
	}

	// Populate children from parent-child map
	if nr.ctx.ParentNodes != nil {
		if children, exists := nr.ctx.ParentNodes[nodeID]; exists {
			nodeData.Children = children
		} else {
			nodeData.Children = []string{}
		}
	}

	return nodeData
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

// Rendering implementations

func (nr *NodeRendererImpl) renderEmptyNode() string {
	return `<div></div>`
}

func (nr *NodeRendererImpl) renderPane(nodeID string) string {
	paneRenderer := &PaneRenderer{
		ctx:          nr.ctx,
		cssProcessor: nr.cssProcessor,
		nodeRenderer: nr,
	}
	return paneRenderer.Render(nodeID)
}

func (nr *NodeRendererImpl) renderMarkdown(nodeID string) string {
	markdownRenderer := templates.NewMarkdownRenderer(nr.ctx, nr)
	return markdownRenderer.Render(nodeID, 0)
}

func (nr *NodeRendererImpl) renderTagElement(nodeID string) string {
	tagElementRenderer := templates.NewTagElementRenderer(nr.ctx, nr)
	return tagElementRenderer.Render(nodeID)
}

func (nr *NodeRendererImpl) renderNodeBasicTag(nodeID string) string {
	nodeBasicTagRenderer := templates.NewNodeBasicTagRenderer(nr.ctx, nr)
	return nodeBasicTagRenderer.Render(nodeID)
}

func (nr *NodeRendererImpl) renderNodeText(nodeID string) string {
	nodeTextRenderer := templates.NewNodeTextRenderer(nr.ctx)
	return nodeTextRenderer.Render(nodeID)
}

func (nr *NodeRendererImpl) renderWidget(nodeID string, hook *models.CodeHook) string {
	widgetRenderer := templates.NewWidgetRenderer(nr.ctx)
	return widgetRenderer.Render(nodeID, hook)
}

// NEW: Missing node type implementations
func (nr *NodeRendererImpl) renderNodeImg(nodeID string) string {
	nodeImgRenderer := templates.NewNodeImgRenderer(nr.ctx)
	return nodeImgRenderer.Render(nodeID)
}

func (nr *NodeRendererImpl) renderNodeButton(nodeID string) string {
	nodeButtonRenderer := templates.NewNodeButtonRenderer(nr.ctx, nr)
	return nodeButtonRenderer.Render(nodeID)
}

func (nr *NodeRendererImpl) renderNodeA(nodeID string) string {
	nodeARenderer := templates.NewNodeARenderer(nr.ctx, nr)
	return nodeARenderer.Render(nodeID)
}

func (nr *NodeRendererImpl) renderBgPaneWrapper(nodeID string) string {
	bgPaneWrapperRenderer := templates.NewBgPaneWrapperRenderer(nr.ctx)
	return bgPaneWrapperRenderer.Render(nodeID)
}

// Placeholders - not yet implemented
func (nr *NodeRendererImpl) renderStoryFragment(nodeID string) string {
	return nr.renderEmptyNode()
}

// PaneRenderer handles Pane.astro rendering logic
type PaneRenderer struct {
	ctx          *models.RenderContext
	cssProcessor *CSSProcessorImpl
	nodeRenderer NodeRenderer
}

// Render implements the Pane.astro rendering logic
func (pr *PaneRenderer) Render(nodeID string) string {
	nodeData := pr.getNodeData(nodeID)
	if nodeData == nil || nodeData.PaneData == nil {
		return `<div></div>`
	}

	paneData := nodeData.PaneData
	beliefs := pr.getPaneBeliefs(nodeID)
	isDecorative := paneData.IsDecorative

	// Build wrapper classes
	wrapperClasses := fmt.Sprintf("grid %s", pr.cssProcessor.GetNodeClasses(nodeID, "auto"))

	// Content classes and styles
	contentClasses := "relative w-full h-auto justify-self-start"
	contentStyles := pr.cssProcessor.GetNodeStringStyles(nodeID) + "; grid-area: 1/1/1/1; position: relative; z-index: 1"

	// Get child nodes
	childNodeIDs := pr.nodeRenderer.GetChildNodeIDs(nodeID)

	// Build the pane HTML
	var html strings.Builder

	// Opening div with pane ID
	html.WriteString(fmt.Sprintf(`<div id="pane-%s" class="`, nodeID))
	html.WriteString(wrapperClasses)
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
		return html.String()
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

	// Add Filter component for belief handling (placeholder)
	if len(beliefs) > 0 {
		html.WriteString(`<!-- Filter component placeholder -->`)
	}

	html.WriteString(`</div>`)
	return html.String()
}

func (pr *PaneRenderer) getPaneBeliefs(nodeID string) map[string]any {
	nodeData := pr.getNodeData(nodeID)
	if nodeData == nil || nodeData.PaneData == nil {
		return make(map[string]any)
	}

	beliefs := make(map[string]any)

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

func (pr *PaneRenderer) checkBeliefVisibility(beliefs map[string]any) bool {
	// For now, always return true - implement belief checking later
	return true
}

// getNodeData retrieves node data for the pane - FIXED TO USE REAL DATA
func (pr *PaneRenderer) getNodeData(nodeID string) *models.NodeRenderData {
	if pr.ctx.AllNodes == nil {
		return nil
	}

	// Use real data from context
	return pr.ctx.AllNodes[nodeID]
}
