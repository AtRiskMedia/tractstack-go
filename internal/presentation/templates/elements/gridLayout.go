// Package templates provides GridLayoutNode rendering functionality
package templates

import (
	"html"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
)

// GridLayoutRenderer handles GridLayout.astro rendering logic
type GridLayoutRenderer struct {
	ctx          *rendering.RenderContext
	nodeRenderer NodeRenderer
}

// NewGridLayoutRenderer creates a new grid layout renderer
func NewGridLayoutRenderer(ctx *rendering.RenderContext, nodeRenderer NodeRenderer) *GridLayoutRenderer {
	return &GridLayoutRenderer{
		ctx:          ctx,
		nodeRenderer: nodeRenderer,
	}
}

// Render implements the GridLayout.astro rendering logic
func (gr *GridLayoutRenderer) Render(nodeID string, depth int) string {
	nodeData := gr.getNodeData(nodeID)
	if nodeData == nil {
		return `<div></div>`
	}

	responsiveClasses := gr.buildResponsiveClass(nodeData)
	parentCSS := nodeData.ParentCSS
	if parentCSS == nil {
		parentCSS = []string{}
	}

	if len(parentCSS) > 0 && depth < len(parentCSS) {
		var sb strings.Builder
		currentClass := parentCSS[depth]
		if depth == 0 {
			currentClass = strings.TrimSpace(currentClass + " " + responsiveClasses)
		}

		sb.WriteString(`<div class="`)
		sb.WriteString(html.EscapeString(currentClass))
		sb.WriteString(`"`)

		if depth == 0 {
			sb.WriteString(` style="position: relative; z-index: 10;"`)
		}

		sb.WriteString(`>`)
		sb.WriteString(gr.Render(nodeID, depth+1))
		sb.WriteString(`</div>`)
		return sb.String()
	}

	var sb strings.Builder
	finalClasses := ""
	if nodeData.GridCSS != "" {
		finalClasses = nodeData.GridCSS
	}
	if len(parentCSS) == 0 {
		finalClasses = strings.TrimSpace(finalClasses + " " + responsiveClasses)
	}

	sb.WriteString(`<div class="`)
	sb.WriteString(html.EscapeString(finalClasses))
	sb.WriteString(`" style="position: relative; z-index: 10;">`)

	childNodeIDs := gr.nodeRenderer.GetChildNodeIDs(nodeID)
	for _, childID := range childNodeIDs {
		sb.WriteString(gr.nodeRenderer.RenderNode(childID))
	}

	sb.WriteString(`</div>`)
	return sb.String()
}

func (gr *GridLayoutRenderer) buildResponsiveClass(nodeData *rendering.NodeRenderData) string {
	if nodeData == nil {
		return ""
	}

	var classes []string
	if nodeData.HiddenViewportMobile {
		classes = append(classes, "hidden")
	} else {
		classes = append(classes, "block")
	}

	if nodeData.HiddenViewportTablet {
		classes = append(classes, "md:hidden")
	} else if nodeData.HiddenViewportMobile {
		classes = append(classes, "md:block")
	}

	if nodeData.HiddenViewportDesktop {
		classes = append(classes, "xl:hidden")
	} else if nodeData.HiddenViewportTablet {
		classes = append(classes, "xl:block")
	}

	return strings.Join(classes, " ")
}

func (gr *GridLayoutRenderer) getNodeData(nodeID string) *rendering.NodeRenderData {
	if gr.ctx.AllNodes == nil {
		return nil
	}
	return gr.ctx.AllNodes[nodeID]
}
