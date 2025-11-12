// Package templates provides NodeBasicTag.astro rendering functionality
package templates

import (
	"encoding/json"
	"html/template"
	"log"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
	widgets "github.com/AtRiskMedia/tractstack-go/internal/presentation/templates/elements/widgets"
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

	// Handle Word Carousel Payload
	var carouselScript string
	var carouselWordsJSON string
	var carouselSpeed float64
	isCarousel := false

	if nodeData.CustomData != nil {
		if payload, ok := nodeData.CustomData["wordCarouselPayload"].(map[string]any); ok {
			if words, ok := payload["words"].([]any); ok {
				var strWords []string
				for _, w := range words {
					if s, ok := w.(string); ok {
						strWords = append(strWords, s)
					}
				}
				if len(strWords) > 0 {
					// Serialize words for data attribute
					if bytes, err := json.Marshal(strWords); err == nil {
						carouselWordsJSON = string(bytes)
						isCarousel = true
					}
				}
			}
			if speed, ok := payload["speed"].(float64); ok {
				carouselSpeed = speed
			} else {
				carouselSpeed = 2.0 // default
			}

			if isCarousel {
				// Generate the script to be appended later
				carouselScript = widgets.RenderWordCarousel(nodeID, carouselSpeed)
			}
		}
	}

	var html strings.Builder

	// Manually write the validated, safe opening tag.
	html.WriteString("<" + safeTag)
	// Ensure ID is present for carousel targeting
	html.WriteString(` id="` + nodeID + `"`)

	// If CSS classes exist, render them using the secure template.
	// This replaces the insecure fmt.Sprintf() for the class attribute.
	if cssClasses != "" {
		err := classAttrTmpl.Execute(&html, cssClasses)
		if err != nil {
			log.Printf("ERROR: Failed to execute classAttr template for nodeID %s: %v", nodeID, err)
		}
	}

	// Inject Carousel Data Attributes
	if isCarousel {
		html.WriteString(` data-word-carousel-words='` + carouselWordsJSON + `'`)
	}

	html.WriteString(">")

	// Render all child nodes
	childNodeIDs := nbtr.nodeRenderer.GetChildNodeIDs(nodeID)
	for _, childID := range childNodeIDs {
		html.WriteString(nbtr.nodeRenderer.RenderNode(childID))
	}

	// Closing tag, using the same validated safe tag.
	html.WriteString("</" + safeTag + ">")

	// Append the carousel script if active
	if isCarousel {
		html.WriteString(carouselScript)
	}

	return html.String()
}

// getNodeData retrieves node data
func (nbtr *NodeBasicTagRenderer) getNodeData(nodeID string) *rendering.NodeRenderData {
	if nbtr.ctx.AllNodes == nil {
		return nil
	}
	return nbtr.ctx.AllNodes[nodeID]
}
