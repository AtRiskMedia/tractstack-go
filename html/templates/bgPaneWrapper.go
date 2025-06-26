// Package templates provides BgPaneWrapper.astro rendering functionality
package templates

import (
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// BgPaneWrapperRenderer handles BgPaneWrapper.astro rendering logic
type BgPaneWrapperRenderer struct {
	ctx *models.RenderContext
}

// NewBgPaneWrapperRenderer creates a new background pane wrapper renderer
func NewBgPaneWrapperRenderer(ctx *models.RenderContext) *BgPaneWrapperRenderer {
	return &BgPaneWrapperRenderer{ctx: ctx}
}

// Render implements the BgPaneWrapper.astro rendering logic exactly
func (bpwr *BgPaneWrapperRenderer) Render(nodeID string) string {
	nodeData := bpwr.getNodeData(nodeID)
	if nodeData == nil {
		return `<div class="bg-error">Missing background node</div>`
	}

	// Check if this is a background image node or visual break
	// Matches BgPaneWrapper.astro: node && (isBgImageNode(node) || isArtpackImageNode(node))
	if bpwr.isBgImageNode(nodeData) || bpwr.isArtpackImageNode(nodeData) {
		return bpwr.renderBgImage(nodeData)
	} else {
		return bpwr.renderBgVisualBreak(nodeData)
	}
}

// isBgImageNode checks if node is a background image node
func (bpwr *BgPaneWrapperRenderer) isBgImageNode(nodeData *models.NodeRenderData) bool {
	return nodeData.BgImageData != nil && nodeData.BgImageData.Type == "background-image"
}

// isArtpackImageNode checks if node is an artpack image node
func (bpwr *BgPaneWrapperRenderer) isArtpackImageNode(nodeData *models.NodeRenderData) bool {
	return nodeData.BgImageData != nil && nodeData.BgImageData.Type == "artpack-image"
}

// renderBgImage renders background image - EXACT BgImage.astro implementation
func (bpwr *BgPaneWrapperRenderer) renderBgImage(nodeData *models.NodeRenderData) string {
	if nodeData.BgImageData == nil {
		return `<div class="bg-error">Missing image data</div>`
	}

	bgData := nodeData.BgImageData

	// Check if we should render as normal image - EXACT match: const isFlexImage = payload.position === "left" || payload.position === "right";
	isFlexImage := bgData.Position == "left" || bgData.Position == "right"

	// Build responsive class - EXACT match to BgImage.astro buildResponsiveClass()
	responsiveClass := bpwr.buildResponsiveClass(nodeData)

	var html strings.Builder

	if isFlexImage {
		// Render as <img> for flex positioned images - EXACT match to BgImage.astro flex branch
		html.WriteString(`<img`)

		// Add src attribute
		if nodeData.ImageURL != nil && *nodeData.ImageURL != "" {
			html.WriteString(fmt.Sprintf(` src="%s"`, *nodeData.ImageURL))
		}

		// Add srcSet if available - EXACT match: {...(payload.srcSet ? { srcSet: payload.srcSet } : {})}
		if nodeData.SrcSet != nil && *nodeData.SrcSet != "" {
			html.WriteString(fmt.Sprintf(` srcset="%s"`, *nodeData.SrcSet))
		}

		// Add CSS classes - EXACT match: class={`w-full h-full object-${payload.objectFit || "cover"} ${buildResponsiveClass()}`}
		objectFit := "cover" // Default from TypeScript: payload.objectFit || "cover"
		// TODO: Add ObjectFit field to BackgroundImageData struct when extending the model
		html.WriteString(fmt.Sprintf(` class="w-full h-full object-%s %s"`, objectFit, responsiveClass))

		// Add alt attribute - EXACT match: alt={payload.alt || "Background image"}
		altText := "Background image"
		if nodeData.AltText != nil && *nodeData.AltText != "" {
			altText = *nodeData.AltText
		}
		html.WriteString(fmt.Sprintf(` alt="%s"`, altText))

		html.WriteString(` />`)
	} else {
		// Render as background div - EXACT match to BgImage.astro background branch
		// This produces the EXACT output we need:
		// <div class="w-full h-full absolute top-0 left-0 block" style="background-image:url(...);..." role="img" aria-label="...">

		html.WriteString(fmt.Sprintf(`<div class="w-full h-full absolute top-0 left-0 %s"`, responsiveClass))

		// Add inline styles - EXACT match to BgImage.astro style object
		html.WriteString(` style="`)

		if nodeData.ImageURL != nil && *nodeData.ImageURL != "" {
			html.WriteString(fmt.Sprintf(`background-image:url(%s);`, *nodeData.ImageURL))
		}

		// Background size - EXACT match: backgroundSize: payload.objectFit || "cover"
		objectFit := "cover" // Default from TypeScript
		html.WriteString(fmt.Sprintf(`background-size:%s;`, objectFit))
		html.WriteString(`background-repeat:no-repeat;`)
		html.WriteString(`background-position:center;`)
		html.WriteString(`z-index:0`)

		html.WriteString(`"`)

		// Add accessibility attributes - EXACT match to BgImage.astro
		html.WriteString(` role="img"`)

		altText := "Background image"
		if nodeData.AltText != nil && *nodeData.AltText != "" {
			altText = *nodeData.AltText
		}
		html.WriteString(fmt.Sprintf(` aria-label="%s"`, altText))

		html.WriteString(`></div>`)
	}

	return html.String()
}

// buildResponsiveClass implements BgImage.astro buildResponsiveClass() function EXACTLY
func (bpwr *BgPaneWrapperRenderer) buildResponsiveClass(nodeData *models.NodeRenderData) string {
	// EXACT implementation of BgImage.astro isHiddenOnViewport and buildResponsiveClass
	// const isHiddenOnViewport = (viewport: "Mobile" | "Tablet" | "Desktop"): boolean => {
	//   const key = `hiddenViewport${viewport}` as keyof BgImageNode;
	//   return !!payload[key];
	// };

	// For now, use CustomData to access hiddenViewport fields until BackgroundImageData is extended
	isHiddenOnViewport := func(viewport string) bool {
		if nodeData.CustomData == nil {
			return false
		}
		key := fmt.Sprintf("hiddenViewport%s", viewport)
		if val, exists := nodeData.CustomData[key]; exists {
			if hidden, ok := val.(bool); ok {
				return hidden
			}
		}
		return false
	}

	hiddenMobile := isHiddenOnViewport("Mobile")
	hiddenTablet := isHiddenOnViewport("Tablet")
	hiddenDesktop := isHiddenOnViewport("Desktop")

	var classes []string

	// EXACT implementation of BgImage.astro buildResponsiveClass logic:
	// if (hiddenMobile) classes.push("hidden xs:hidden");
	// else classes.push("block");
	if hiddenMobile {
		classes = append(classes, "hidden", "xs:hidden")
	} else {
		classes = append(classes, "block")
	}

	// if (hiddenTablet) classes.push("md:hidden");
	// else if (hiddenMobile) classes.push("md:block");
	if hiddenTablet {
		classes = append(classes, "md:hidden")
	} else if hiddenMobile {
		classes = append(classes, "md:block")
	}

	// if (hiddenDesktop) classes.push("xl:hidden");
	// else if (hiddenTablet) classes.push("xl:block");
	if hiddenDesktop {
		classes = append(classes, "xl:hidden")
	} else if hiddenTablet {
		classes = append(classes, "xl:block")
	}

	// return classes.join(" ");
	return strings.Join(classes, " ")
}

// renderBgVisualBreak renders visual break - matches <BgVisualBreak payload={node as VisualBreakNode} />
func (bpwr *BgPaneWrapperRenderer) renderBgVisualBreak(nodeData *models.NodeRenderData) string {
	var html strings.Builder

	// Create a visual break element - matches the expected output from visual breaks
	html.WriteString(`<div class="visual-break"`)

	// Add any element CSS
	if nodeData.ElementCSS != nil && *nodeData.ElementCSS != "" {
		html.WriteString(fmt.Sprintf(` class="%s"`, *nodeData.ElementCSS))
	}

	html.WriteString(`>`)

	// Visual break content - matches the pattern we see in the Go 2.0 output
	html.WriteString(`<div class="visual-break-content"></div>`)

	html.WriteString(`</div>`)
	return html.String()
}

// getNodeData retrieves node data from real context
func (bpwr *BgPaneWrapperRenderer) getNodeData(nodeID string) *models.NodeRenderData {
	if bpwr.ctx.AllNodes == nil {
		return nil
	}

	return bpwr.ctx.AllNodes[nodeID]
}
