// Package html provides node rendering functionality for nodes-compositor
package html

import (
	"fmt"
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
		// console.log equivalent for debugging
		fmt.Printf("Node.astro miss on %s\n", nodeType)
		return nr.renderEmptyNode()
	}
}

// GetChildNodeIDs returns child node IDs for a given parent
func (nr *NodeRendererImpl) GetChildNodeIDs(parentID string) []string {
	if nr.ctx.ParentNodes == nil {
		return []string{}
	}

	children, exists := nr.ctx.ParentNodes[parentID]
	if !exists {
		return []string{}
	}

	return children
}

// Core rendering methods
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

// Placeholder methods
func (nr *NodeRendererImpl) renderStoryFragment(nodeID string) string {
	return nr.renderEmptyNode()
}

func (nr *NodeRendererImpl) renderWidget(nodeID string, hookData map[string]interface{}) string {
	// Placeholder for widget rendering
	return `<!-- Widget placeholder -->`
}

func (nr *NodeRendererImpl) renderEmptyNode() string {
	return `<div></div>`
}

// Helper methods
func (nr *NodeRendererImpl) parseCodeHook(nodeData *models.NodeRenderData) map[string]interface{} {
	// Placeholder for code hook parsing - matches Node.astro parseCodeHook
	return nil
}

func (nr *NodeRendererImpl) getNodeRenderData(nodeID string) *models.NodeRenderData {
	if nr.ctx.AllNodes == nil {
		return nil
	}
	return nr.ctx.AllNodes[nodeID]
}

// PaneRenderer handles Pane.astro rendering logic - COMPLETE REWRITE
type PaneRenderer struct {
	ctx          *models.RenderContext
	cssProcessor *CSSProcessorImpl
	nodeRenderer NodeRenderer
}

// Render implements the COMPLETE Pane.astro rendering logic
func (pr *PaneRenderer) Render(nodeID string) string {
	nodeData := pr.getNodeData(nodeID)
	if nodeData == nil || nodeData.PaneData == nil {
		return `<div></div>`
	}

	paneData := nodeData.PaneData
	beliefs := pr.getPaneBeliefs(nodeID)
	isDecorative := paneData.IsDecorative

	// Get slug for inner div - matches Pane.astro: const slug = getCtx().getPaneSlug(nodeId)
	slug := pr.getPaneSlug(nodeID)

	// Build wrapper classes - matches Pane.astro: `grid ${getCtx().getNodeClasses(nodeId, 'auto')}`
	wrapperClasses := fmt.Sprintf("grid %s", pr.cssProcessor.GetNodeClasses(nodeID, "auto"))

	// Content classes and styles - matches Pane.astro exactly
	contentClasses := "relative w-full h-auto justify-self-start"
	contentStyles := pr.cssProcessor.GetNodeStringStyles(nodeID) + "; grid-area: 1/1/1/1; position: relative; z-index: 1"

	// Get background node - matches Pane.astro background node detection
	bgNode := pr.getBackgroundNode(nodeID)

	// Determine layout type - matches Pane.astro conditional logic
	useFlexLayout := bgNode != nil && (bgNode.Position == "leftBleed" || bgNode.Position == "rightBleed")
	deferFlexLayout := bgNode != nil && (bgNode.Position == "left" || bgNode.Position == "right")

	// Build the pane HTML - matches Pane.astro structure exactly
	var html strings.Builder

	// Opening div with pane ID - matches: <div id={`pane-${nodeId}`} class={isDecorative ? `` : `pane`} style="position: relative;">
	html.WriteString(fmt.Sprintf(`<div id="pane-%s"`, nodeID))

	if isDecorative {
		html.WriteString(` class=""`)
	} else {
		html.WriteString(` class="pane"`)
	}
	html.WriteString(` style="position: relative;">`)

	// Add Filter component for beliefs (placeholder for now)
	if len(beliefs) > 0 {
		html.WriteString(`<!-- Filter component placeholder -->`)
	}

	// Handle CodeHook payload (placeholder for now)
	codeHookPayload := pr.getCodeHookPayload(nodeID)
	if codeHookPayload != nil {
		html.WriteString(fmt.Sprintf(`<div id="%s" style="%s">`, slug, contentStyles))
		html.WriteString(`<!-- CodeHook component placeholder -->`)
		html.WriteString(`</div>`)
	} else if useFlexLayout {
		// useFlexLayout - matches Pane.astro flex layout for leftBleed/rightBleed
		flexDirection := "flex-col md:flex-row"
		if bgNode.Position == "rightBleed" {
			flexDirection = "flex-col md:flex-row-reverse"
		}

		html.WriteString(fmt.Sprintf(`<div id="%s" class="flex flex-nowrap %s %s">`,
			slug, flexDirection, pr.cssProcessor.GetNodeClasses(nodeID, "auto")))

		// Image side
		imageSizeClass := pr.getSizeClasses(bgNode.Size, "image")
		html.WriteString(fmt.Sprintf(`<div class="relative overflow-hidden %s">`, imageSizeClass))

		// Render only BgPane children
		bgChildrenIDs := pr.getBgPaneChildren(nodeID)
		for _, childID := range bgChildrenIDs {
			html.WriteString(pr.nodeRenderer.RenderNode(childID))
		}
		html.WriteString(`</div>`)

		// Content side
		contentSizeClass := pr.getSizeClasses(bgNode.Size, "content")
		html.WriteString(fmt.Sprintf(`<div class="%s %s" style="%s">`,
			contentClasses, contentSizeClass, pr.cssProcessor.GetNodeStringStyles(nodeID)))

		// Render non-BgPane children
		contentChildrenIDs := pr.getNonBgPaneChildren(nodeID)
		for _, childID := range contentChildrenIDs {
			html.WriteString(pr.nodeRenderer.RenderNode(childID))
		}
		html.WriteString(`</div>`)

		html.WriteString(`</div>`)
	} else if deferFlexLayout {
		// deferFlexLayout - matches Pane.astro deferred flex layout for left/right
		html.WriteString(fmt.Sprintf(`<div id="%s" class="%s">`, slug, wrapperClasses))
		html.WriteString(fmt.Sprintf(`<div class="%s" style="%s">`, contentClasses, contentStyles))

		// Render non-BgPane children (BgPane handled in Markdown.astro)
		contentChildrenIDs := pr.getNonBgPaneChildren(nodeID)
		for _, childID := range contentChildrenIDs {
			html.WriteString(pr.nodeRenderer.RenderNode(childID))
		}
		html.WriteString(`</div>`)
		html.WriteString(`</div>`)
	} else {
		// Standard layout - matches Pane.astro default case
		html.WriteString(fmt.Sprintf(`<div id="%s" class="%s">`, slug, wrapperClasses))
		html.WriteString(fmt.Sprintf(`<div class="%s" style="%s">`, contentClasses, contentStyles))

		// Render all children
		childNodeIDs := pr.nodeRenderer.GetChildNodeIDs(nodeID)
		for _, childID := range childNodeIDs {
			html.WriteString(pr.nodeRenderer.RenderNode(childID))
		}
		html.WriteString(`</div>`)
		html.WriteString(`</div>`)
	}

	html.WriteString(`</div>`)
	return html.String()
}

// Helper methods for Pane rendering

func (pr *PaneRenderer) getPaneSlug(nodeID string) string {
	// Placeholder - implement slug extraction logic
	// This should match getCtx().getPaneSlug(nodeId) from Astro
	nodeData := pr.getNodeData(nodeID)
	if nodeData != nil && nodeData.PaneData != nil && nodeData.PaneData.Slug != "" {
		return nodeData.PaneData.Slug
	}
	return fmt.Sprintf("pane-%s", nodeID)
}

func (pr *PaneRenderer) getBackgroundNode(nodeID string) *models.BackgroundNode {
	childNodeIDs := pr.nodeRenderer.GetChildNodeIDs(nodeID)

	for _, childID := range childNodeIDs {
		childData := pr.getNodeData(childID)
		if childData != nil && childData.NodeType == "BgPane" &&
			childData.BgImageData != nil &&
			(childData.BgImageData.Type == "background-image" || childData.BgImageData.Type == "artpack-image") {

			return &models.BackgroundNode{
				ID:       childData.ID,
				Position: childData.BgImageData.Position,
				Size:     childData.BgImageData.Size,
			}
		}
	}
	return nil
}

func (pr *PaneRenderer) getSizeClasses(size string, side string) string {
	// Matches Pane.astro getSizeClasses exactly
	switch size {
	case "narrow":
		if side == "image" {
			return "w-full md:w-1/3"
		}
		return "w-full md:w-2/3"
	case "wide":
		if side == "image" {
			return "w-full md:w-2/3"
		}
		return "w-full md:w-1/3"
	default: // "equal"
		return "w-full md:w-1/2"
	}
}

func (pr *PaneRenderer) getBgPaneChildren(nodeID string) []string {
	childNodeIDs := pr.nodeRenderer.GetChildNodeIDs(nodeID)
	var bgChildren []string

	for _, childID := range childNodeIDs {
		childData := pr.getNodeData(childID)
		if childData != nil && childData.NodeType == "BgPane" {
			bgChildren = append(bgChildren, childID)
		}
	}
	return bgChildren
}

func (pr *PaneRenderer) getNonBgPaneChildren(nodeID string) []string {
	childNodeIDs := pr.nodeRenderer.GetChildNodeIDs(nodeID)
	var contentChildren []string

	for _, childID := range childNodeIDs {
		childData := pr.getNodeData(childID)
		if childData == nil || childData.NodeType != "BgPane" {
			contentChildren = append(contentChildren, childID)
		}
	}
	return contentChildren
}

func (pr *PaneRenderer) getCodeHookPayload(nodeID string) map[string]interface{} {
	// Placeholder for CodeHook detection
	return nil
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

func (pr *PaneRenderer) getNodeData(nodeID string) *models.NodeRenderData {
	if pr.ctx.AllNodes == nil {
		return nil
	}
	return pr.ctx.AllNodes[nodeID]
}
