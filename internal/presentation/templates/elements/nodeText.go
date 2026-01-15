// Package templates provides NodeText rendering functionality
package templates

import (
	"bytes"
	"html/template"
	"log"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
)

var textEscaper = template.Must(template.New("textEscaper").Parse("{{.}}"))

// NodeTextRenderer handles the rendering and escaping of text content nodes.
type NodeTextRenderer struct {
	ctx *rendering.RenderContext
}

// NewNodeTextRenderer creates a new renderer for text nodes.
func NewNodeTextRenderer(ctx *rendering.RenderContext) *NodeTextRenderer {
	return &NodeTextRenderer{ctx: ctx}
}

// Render generates the safe HTML content for a text node.
func (ntr *NodeTextRenderer) Render(nodeID string) string {
	nodeData := ntr.getNodeData(nodeID)
	if nodeData == nil {
		log.Printf("Warning: Node data not found for ID %s in NodeTextRenderer", nodeID)
		return ""
	}

	var content string
	if nodeData.Copy != nil {
		copyContent := *nodeData.Copy

		switch copyContent {
		case " ":
			content = " " // Render literal space
		case "":
			content = "&nbsp;" // Render non-breaking space for empty strings
		default:
			// Escape other content
			var buf bytes.Buffer
			data := map[string]string{"Content": copyContent} // Template data needs a map key
			if err := textEscaper.Execute(&buf, data["Content"]); err != nil {
				log.Printf("Error escaping text node %s: %v", nodeID, err)
				content = template.HTMLEscapeString(copyContent) // Fallback
			} else {
				content = buf.String()
			}
		}
	} else {
		content = "&nbsp;"
	}

	return content
}

// getNodeData retrieves node data from the context.
func (ntr *NodeTextRenderer) getNodeData(nodeID string) *rendering.NodeRenderData {
	if ntr.ctx.AllNodes == nil {
		return nil
	}
	if data, exists := ntr.ctx.AllNodes[nodeID]; exists {
		return data
	}
	return nil
}
