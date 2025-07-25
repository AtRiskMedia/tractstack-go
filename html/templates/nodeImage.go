// Package templates provides NodeImg.astro rendering functionality
package templates

import (
	"html/template"
	"log"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// nodeImageTmpl is a secure, pre-parsed template for the <img> tag.
// It uses Go's html/template to prevent XSS by automatically escaping all attributes.
// Optional attributes (src, srcset) are only rendered if they have a value.
var nodeImageTmpl = template.Must(template.New("nodeImage").Parse(
	`<img{{if .Src}} src="{{.Src}}"{{end}}{{if .SrcSet}} srcset="{{.SrcSet}}"{{end}} class="{{.Class}}" alt="{{.Alt}}" />`,
))

// nodeImageData holds the data for the nodeImage template.
type nodeImageData struct {
	Src    string
	SrcSet string
	Class  string
	Alt    string
}

// NodeImgRenderer handles NodeImg.astro rendering logic
type NodeImgRenderer struct {
	ctx *models.RenderContext
}

// NewNodeImgRenderer creates a new node image renderer
func NewNodeImgRenderer(ctx *models.RenderContext) *NodeImgRenderer {
	return &NodeImgRenderer{ctx: ctx}
}

// Render implements the NodeImg.astro rendering logic exactly
func (nir *NodeImgRenderer) Render(nodeID string) string {
	nodeData := nir.getNodeData(nodeID)
	if nodeData == nil {
		return `<img alt="missing image" />`
	}

	// Prepare the data for the template, applying logic from the original file.
	data := nodeImageData{
		// Default values matching NodeImg.astro logic
		Class: "auto",
		Alt:   "image",
	}

	if nodeData.ImageURL != nil && *nodeData.ImageURL != "" {
		data.Src = *nodeData.ImageURL
	}
	if nodeData.SrcSet != nil && *nodeData.SrcSet != "" {
		data.SrcSet = *nodeData.SrcSet
	}
	if nodeData.ElementCSS != nil && *nodeData.ElementCSS != "" {
		data.Class = *nodeData.ElementCSS // Override default
	}
	if nodeData.AltText != nil && *nodeData.AltText != "" {
		data.Alt = *nodeData.AltText // Override default
	}

	var html strings.Builder
	// Use the pre-parsed template to safely render the <img> tag.
	// This replaces the four insecure fmt.Sprintf() calls.
	err := nodeImageTmpl.Execute(&html, data)
	if err != nil {
		log.Printf("ERROR: Failed to execute nodeImage template for nodeID %s: %v", nodeID, err)
		return `<!-- error rendering image -->`
	}
	return html.String()
}

// getNodeData retrieves node data from real context
func (nir *NodeImgRenderer) getNodeData(nodeID string) *models.NodeRenderData {
	if nir.ctx.AllNodes == nil {
		return nil
	}
	return nir.ctx.AllNodes[nodeID]
}
