// Package templates provides NodeBasicTag.astro rendering functionality
package templates

import (
	"html/template"
	"log"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
)

var (
	// classAttrTmpl is a small, secure template just for rendering the class attribute.
	// This prevents attribute injection from user-provided CSS.
	classAttrTmpl = template.Must(template.New("classAttr").Parse(` class="{{.}}"`))

	// allowedTags is a security allowlist. Only tags in this map are allowed to be
	// rendered. This is crucial to prevent arbitrary HTML injection (e.g., <script>).
	allowedTags = map[string]struct{}{
		"div":     {},
		"p":       {},
		"span":    {},
		"section": {},
		"article": {},
		"header":  {},
		"footer":  {},
		"h1":      {},
		"h2":      {},
		"h3":      {},
		"h4":      {},
		"h5":      {},
		"h6":      {},
		"ul":      {},
		"ol":      {},
		"li":      {},
		"strong":  {},
		"em":      {},
		"b":       {},
		"i":       {},
	}
)

// NodeBasicTagRenderer handles NodeBasicTag.astro rendering logic
type NodeBasicTagRenderer struct {
	ctx          *rendering.RenderContext
	nodeRenderer NodeRenderer
}

// NewNodeBasicTagRenderer creates a new node basic tag renderer
func NewNodeBasicTagRenderer(ctx *rendering.RenderContext, nodeRenderer NodeRenderer) *NodeBasicTagRenderer {
	return &NodeBasicTagRenderer{
		ctx:          ctx,
		nodeRenderer: nodeRenderer,
	}
}

// Render implements the NodeBasicTag.astro rendering logic with elementCss exactly
func (nbtr *NodeBasicTagRenderer) Render(nodeID string) string {
	nodeData := nbtr.getNodeData(nodeID)
	if nodeData == nil {
		return `<div></div>`
	}

	// SECURITY: Validate the user-provided tag name against an allowlist.
	// Default to "div" if the tag is not in the list. This prevents HTML injection.
	safeTag := "div" // Default fallback
	if nodeData.TagName != nil {
		if _, ok := allowedTags[*nodeData.TagName]; ok {
			safeTag = *nodeData.TagName
		}
	}

	// Get CSS classes
	cssClasses := ""
	if nodeData.ElementCSS != nil {
		cssClasses = *nodeData.ElementCSS
	}

	var html strings.Builder

	// Manually write the validated, safe opening tag.
	html.WriteString("<" + safeTag)

	// If CSS classes exist, render them using the secure template.
	// This replaces the insecure fmt.Sprintf() for the class attribute.
	if cssClasses != "" {
		err := classAttrTmpl.Execute(&html, cssClasses)
		if err != nil {
			log.Printf("ERROR: Failed to execute classAttr template for nodeID %s: %v", nodeID, err)
			// Don't return here, just log the error and continue rendering the tag without classes.
		}
	}

	html.WriteString(">")

	// Render all child nodes
	childNodeIDs := nbtr.nodeRenderer.GetChildNodeIDs(nodeID)
	for _, childID := range childNodeIDs {
		html.WriteString(nbtr.nodeRenderer.RenderNode(childID))
	}

	// Closing tag, using the same validated safe tag.
	// This replaces the final insecure fmt.Sprintf().
	html.WriteString("</" + safeTag + ">")

	return html.String()
}

// getNodeData retrieves node data
func (nbtr *NodeBasicTagRenderer) getNodeData(nodeID string) *rendering.NodeRenderData {
	if nbtr.ctx.AllNodes == nil {
		return nil
	}
	return nbtr.ctx.AllNodes[nodeID]
}
