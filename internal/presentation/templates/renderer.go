// Package templates provides node rendering functionality for nodes-compositor
package templates

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"

	templates "github.com/AtRiskMedia/tractstack-go/internal/presentation/templates/elements"
)

var paneTemplates = template.Must(template.New("paneRenderer").Parse(
	`{{define "paneWrapper"}}<div id="pane-{{.ID}}" class="{{.Class}}" style="position: relative;">{{end}}` +
		`{{define "contentDiv"}}<div id="{{.ID}}" class="{{.Class}}" style="{{.Style}}">{{end}}` +
		`{{define "flexLayoutDiv"}}<div id="{{.ID}}" class="flex flex-nowrap {{.FlexDirection}} {{.Classes}}">{{end}}` +
		`{{define "imageSideDiv"}}<div class="relative overflow-hidden {{.Class}}">{{end}}` +
		`{{define "contentSideDiv"}}<div class="{{.ContentClasses}} {{.SizeClass}}" style="{{.Style}}">{{end}}`,
))

type paneWrapperData struct {
	ID    string
	Class string
}
type contentDivData struct {
	ID    string
	Class string
	Style template.CSS
}
type flexLayoutDivData struct {
	ID            string
	FlexDirection string
	Classes       string
}
type imageSideDivData struct {
	Class string
}
type contentSideDivData struct {
	ContentClasses string
	SizeClass      string
	Style          template.CSS
}

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
	ctx          *rendering.RenderContext
	cssProcessor *CSSProcessorImpl
}

// NewNodeRenderer creates a new node renderer with context
func NewNodeRenderer(ctx *rendering.RenderContext) *NodeRendererImpl {
	renderer := &NodeRendererImpl{ctx: ctx}
	renderer.cssProcessor = NewCSSProcessorImpl(ctx)
	return renderer
}

// RenderNode renders a node by ID, implementing Node.astro switch logic
func (nr *NodeRendererImpl) RenderNode(nodeID string) string {
	if nodeID == "" {
		return nr.renderEmptyNode()
	}

	nodeData := nr.getNodeRenderData(nodeID)
	if nodeData == nil {
		return nr.renderEmptyNode()
	}

	nodeType := nodeData.NodeType
	if nodeData.TagName != nil {
		nodeType = *nodeData.TagName
	}

	if nodeType == "code" {
		hookData := nr.parseCodeHook(nodeData)
		if hookData != nil {
			return nr.renderWidget(nodeID, hookData)
		}
		return nr.renderEmptyNode()
	}

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

func (nr *NodeRendererImpl) renderEmptyNode() string {
	return `<div></div>`
}

func (nr *NodeRendererImpl) getNodeRenderData(nodeID string) *rendering.NodeRenderData {
	if nr.ctx.AllNodes == nil {
		return nil
	}
	return nr.ctx.AllNodes[nodeID]
}

type PaneRenderer struct {
	ctx          *rendering.RenderContext
	cssProcessor *CSSProcessorImpl
	nodeRenderer NodeRenderer
}

func (pr *PaneRenderer) Render(nodeID string) string {
	nodeData := pr.getNodeData(nodeID)
	if nodeData == nil || nodeData.PaneData == nil {
		return `<div></div>`
	}

	paneData := nodeData.PaneData
	heldBeliefs, withheldBeliefs := pr.getPaneBeliefs(nodeID)
	isDecorative := paneData.IsDecorative

	slug := pr.getPaneSlug(nodeID)

	wrapperClasses := "grid " + pr.cssProcessor.GetNodeClasses(nodeID, "auto")

	contentClasses := "relative w-full h-auto justify-self-start"
	contentStyles := pr.cssProcessor.GetNodeStringStyles(nodeID) + "; grid-area: 1/1/1/1; position: relative; z-index: 1"

	bgNode := pr.getBackgroundNode(nodeID)

	useFlexLayout := bgNode != nil && (bgNode.Position == "leftBleed" || bgNode.Position == "rightBleed")
	deferFlexLayout := bgNode != nil && (bgNode.Position == "left" || bgNode.Position == "right")

	var html bytes.Buffer

	wrapperData := paneWrapperData{ID: nodeID}
	if isDecorative {
		wrapperData.Class = ""
	} else {
		wrapperData.Class = "pane"
	}
	pr.executeTemplate(&html, "paneWrapper", wrapperData)

	if len(heldBeliefs) > 0 || len(withheldBeliefs) > 0 {
		html.WriteString(`<!-- Filter component placeholder -->`)
	}

	codeHookPayload := pr.getCodeHookPayload(nodeID)
	if codeHookPayload != nil {
		pr.executeTemplate(&html, "contentDiv", contentDivData{ID: slug, Style: template.CSS(contentStyles)})
		html.WriteString(`<!-- CodeHook component placeholder --></div>`)
	} else if useFlexLayout {
		flexDirection := "flex-col md:flex-row"
		if bgNode.Position == "rightBleed" {
			flexDirection = "flex-col md:flex-row-reverse"
		}

		pr.executeTemplate(&html, "flexLayoutDiv", flexLayoutDivData{
			ID:            slug,
			FlexDirection: flexDirection,
			Classes:       pr.cssProcessor.GetNodeClasses(nodeID, "auto"),
		})

		imageSizeClass := pr.getSizeClasses(bgNode.Size, "image")
		pr.executeTemplate(&html, "imageSideDiv", imageSideDivData{Class: imageSizeClass})

		bgChildrenIDs := pr.getBgPaneChildren(nodeID)
		for _, childID := range bgChildrenIDs {
			html.WriteString(pr.nodeRenderer.RenderNode(childID))
		}
		html.WriteString(`</div>`)

		contentSizeClass := pr.getSizeClasses(bgNode.Size, "content")
		pr.executeTemplate(&html, "contentSideDiv", contentSideDivData{
			ContentClasses: contentClasses,
			SizeClass:      contentSizeClass,
			Style:          template.CSS(pr.cssProcessor.GetNodeStringStyles(nodeID)),
		})

		contentChildrenIDs := pr.getNonBgPaneChildren(nodeID)
		for _, childID := range contentChildrenIDs {
			html.WriteString(pr.nodeRenderer.RenderNode(childID))
		}
		html.WriteString(`</div>`)

		html.WriteString(`</div>`)
	} else if deferFlexLayout {
		pr.executeTemplate(&html, "contentDiv", contentDivData{ID: slug, Class: wrapperClasses})
		pr.executeTemplate(&html, "contentDiv", contentDivData{Class: contentClasses, Style: template.CSS(contentStyles)})

		contentChildrenIDs := pr.getNonBgPaneChildren(nodeID)
		for _, childID := range contentChildrenIDs {
			html.WriteString(pr.nodeRenderer.RenderNode(childID))
		}
		html.WriteString(`</div>`)
		html.WriteString(`</div>`)
	} else {
		pr.executeTemplate(&html, "contentDiv", contentDivData{ID: slug, Class: wrapperClasses})
		pr.executeTemplate(&html, "contentDiv", contentDivData{Class: contentClasses, Style: template.CSS(contentStyles)})

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

func (pr *PaneRenderer) executeTemplate(buf *bytes.Buffer, name string, data interface{}) {
	err := paneTemplates.ExecuteTemplate(buf, name, data)
	if err != nil {
		log.Printf("ERROR: Failed to execute pane template '%s': %v", name, err)
		buf.WriteString("<!-- template error -->")
	}
}

func (pr *PaneRenderer) getPaneSlug(nodeID string) string {
	nodeData := pr.getNodeData(nodeID)
	if nodeData != nil && nodeData.PaneData != nil && nodeData.PaneData.Slug != "" {
		return nodeData.PaneData.Slug
	}
	return fmt.Sprintf("pane-%s", nodeID)
}

func (pr *PaneRenderer) getBackgroundNode(nodeID string) *rendering.BackgroundNode {
	childNodeIDs := pr.nodeRenderer.GetChildNodeIDs(nodeID)

	for _, childID := range childNodeIDs {
		childData := pr.getNodeData(childID)
		if childData != nil && childData.NodeType == "BgPane" &&
			childData.BgImageData != nil &&
			(childData.BgImageData.Type == "background-image" || childData.BgImageData.Type == "artpack-image") {

			return &rendering.BackgroundNode{
				ID:       childData.ID,
				Position: childData.BgImageData.Position,
				Size:     childData.BgImageData.Size,
			}
		}
	}
	return nil
}

func (pr *PaneRenderer) getSizeClasses(size string, side string) string {
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
	return nil
}

func (pr *PaneRenderer) getPaneBeliefs(nodeID string) (map[string][]string, map[string][]string) {
	nodeData := pr.getNodeData(nodeID)
	if nodeData == nil || nodeData.PaneData == nil {
		return make(map[string][]string), make(map[string][]string)
	}

	heldBeliefs := make(map[string][]string)
	withheldBeliefs := make(map[string][]string)

	if nodeData.PaneData.HeldBeliefs != nil {
		for k, v := range nodeData.PaneData.HeldBeliefs {
			// Fix: Proper type assertion for interface{} to []string
			if strSlice, ok := v.([]string); ok {
				heldBeliefs[k] = strSlice
			} else if str, ok := v.(string); ok {
				heldBeliefs[k] = []string{str}
			}
		}
	}

	if nodeData.PaneData.WithheldBeliefs != nil {
		for k, v := range nodeData.PaneData.WithheldBeliefs {
			// Fix: Proper type assertion for interface{} to []string
			if strSlice, ok := v.([]string); ok {
				withheldBeliefs[k] = strSlice
			} else if str, ok := v.(string); ok {
				withheldBeliefs[k] = []string{str}
			}
		}
	}

	return heldBeliefs, withheldBeliefs
}

func (pr *PaneRenderer) getNodeData(nodeID string) *rendering.NodeRenderData {
	if pr.ctx.AllNodes == nil {
		return nil
	}
	return pr.ctx.AllNodes[nodeID]
}

func (nr *NodeRendererImpl) parseCodeHook(nodeData *rendering.NodeRenderData) *rendering.CodeHook {
	if nodeData == nil {
		return nil
	}

	copyText := ""
	if nodeData.Copy != nil {
		copyText = *nodeData.Copy
	}

	hook := extractWidgetTypeFromCopy(copyText)
	if hook == "" {
		return nil
	}

	var params []string
	if nodeData.CustomData != nil {
		if codeHookParams, exists := nodeData.CustomData["codeHookParams"]; exists {
			if paramsSlice, ok := codeHookParams.([]string); ok {
				params = paramsSlice
			}
		}
	}

	codeHook := &rendering.CodeHook{
		Hook: hook,
	}

	if len(params) > 0 && params[0] != "" {
		codeHook.Value1 = &params[0]
	}

	if len(params) > 1 && params[1] != "" {
		codeHook.Value2 = &params[1]
	}

	if len(params) > 2 {
		codeHook.Value3 = params[2]
	}

	return codeHook
}

func (nr *NodeRendererImpl) renderWidget(nodeID string, hook *rendering.CodeHook) string {
	if hook == nil {
		return `<!-- Widget error: no hook data -->`
	}
	widgetRenderer := templates.NewWidgetRenderer(nr.ctx)
	return widgetRenderer.Render(nodeID, hook)
}

func extractWidgetTypeFromCopy(copyText string) string {
	if copyText == "" {
		return ""
	}

	parenIndex := strings.Index(copyText, "(")
	if parenIndex == -1 {
		return ""
	}

	return copyText[:parenIndex]
}
