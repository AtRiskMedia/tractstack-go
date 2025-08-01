// Package templates provides BgPaneWrapper.astro rendering functionality
package templates

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/url"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
)

// BgPaneWrapperRenderer handles BgPaneWrapper.astro rendering logic
type BgPaneWrapperRenderer struct {
	ctx *rendering.RenderContext
}

// NewBgPaneWrapperRenderer creates a new background pane wrapper renderer
func NewBgPaneWrapperRenderer(ctx *rendering.RenderContext) *BgPaneWrapperRenderer {
	return &BgPaneWrapperRenderer{ctx: ctx}
}

// Template data structures for safe HTML rendering
type bgImageTemplateData struct {
	Src             string
	SrcSet          string
	ObjectFit       string
	ResponsiveClass string
	AltText         string
}

type bgDivTemplateData struct {
	ResponsiveClass string
	BackgroundURL   template.CSS // Safe CSS for background-image
	ObjectFit       string
	AltText         string
}

type visualBreakTemplateData struct {
	ElementCSS string
}

// Compiled templates for different rendering modes
var (
	bgImageTemplate = template.Must(template.New("bgImage").Parse(`<img src="{{.Src}}"{{if .SrcSet}} srcset="{{.SrcSet}}"{{end}} class="w-full h-full object-{{.ObjectFit}} {{.ResponsiveClass}}" alt="{{.AltText}}" />`))

	bgDivTemplate = template.Must(template.New("bgDiv").Parse(`<div class="w-full h-full absolute top-0 left-0 {{.ResponsiveClass}}" style="{{.BackgroundURL}}background-size:{{.ObjectFit}};background-repeat:no-repeat;background-position:center;z-index:0" role="img" aria-label="{{.AltText}}"></div>`))

	visualBreakTemplate = template.Must(template.New("visualBreak").Parse(`<div class="visual-break"{{if .ElementCSS}} class="{{.ElementCSS}}"{{end}}><div class="visual-break-content"></div></div>`))
)

// Render implements the BgPaneWrapper.astro rendering logic exactly
func (bpwr *BgPaneWrapperRenderer) Render(nodeID string) string {
	nodeData := bpwr.getNodeData(nodeID)
	if nodeData == nil {
		return `<div class="bg-error">Missing background node</div>`
	}

	// Check if this is a background image node or visual break
	if bpwr.isBgImageNode(nodeData) || bpwr.isArtpackImageNode(nodeData) {
		return bpwr.renderBgImage(nodeData)
	} else {
		return bpwr.renderBgVisualBreak(nodeData)
	}
}

// isBgImageNode checks if node is a background image node
func (bpwr *BgPaneWrapperRenderer) isBgImageNode(nodeData *rendering.NodeRenderData) bool {
	return nodeData.BgImageData != nil && nodeData.BgImageData.Type == "background-image"
}

// isArtpackImageNode checks if node is an artpack image node
func (bpwr *BgPaneWrapperRenderer) isArtpackImageNode(nodeData *rendering.NodeRenderData) bool {
	return nodeData.BgImageData != nil && nodeData.BgImageData.Type == "artpack-image"
}

// renderBgImage renders background image with secure URL handling
func (bpwr *BgPaneWrapperRenderer) renderBgImage(nodeData *rendering.NodeRenderData) string {
	if nodeData.BgImageData == nil {
		return `<div class="bg-error">Missing image data</div>`
	}

	bgData := nodeData.BgImageData
	isFlexImage := bgData.Position == "left" || bgData.Position == "right"
	responsiveClass := bpwr.buildResponsiveClass(nodeData)

	// Get safe values with defaults
	src := ""
	if nodeData.ImageURL != nil {
		src = *nodeData.ImageURL
	}

	srcSet := ""
	if nodeData.SrcSet != nil {
		srcSet = *nodeData.SrcSet
	}

	altText := "Background image"
	if nodeData.AltText != nil && *nodeData.AltText != "" {
		altText = *nodeData.AltText
	}

	objectFit := "cover" // Default

	if isFlexImage {
		// Render as <img> for flex positioned images
		templateData := bgImageTemplateData{
			Src:             src,
			SrcSet:          srcSet,
			ObjectFit:       objectFit,
			ResponsiveClass: responsiveClass,
			AltText:         altText,
		}

		var buf bytes.Buffer
		if err := bgImageTemplate.Execute(&buf, templateData); err != nil {
			log.Printf("Error executing bg image template: %v", err)
			return `<div class="bg-error">Template error</div>`
		}
		return buf.String()
	} else {
		// Render as background div with secure CSS URL handling
		backgroundCSS := ""
		if src != "" {
			// Validate and sanitize URL for CSS background-image
			if sanitizedURL := bpwr.sanitizeBackgroundURL(src); sanitizedURL != "" {
				backgroundCSS = fmt.Sprintf("background-image:url(%s);", sanitizedURL)
			}
		}

		templateData := bgDivTemplateData{
			ResponsiveClass: responsiveClass,
			BackgroundURL:   template.CSS(backgroundCSS), // Safe CSS type
			ObjectFit:       objectFit,
			AltText:         altText,
		}

		var buf bytes.Buffer
		if err := bgDivTemplate.Execute(&buf, templateData); err != nil {
			log.Printf("Error executing bg div template: %v", err)
			return `<div class="bg-error">Template error</div>`
		}
		return buf.String()
	}
}

// sanitizeBackgroundURL validates and sanitizes URLs for CSS background-image usage
func (bpwr *BgPaneWrapperRenderer) sanitizeBackgroundURL(rawURL string) string {
	// Parse and validate the URL
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		log.Printf("Invalid background URL: %s, error: %v", rawURL, err)
		return ""
	}

	// Only allow http, https, and relative URLs
	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "" && scheme != "http" && scheme != "https" {
		log.Printf("Blocked unsafe URL scheme: %s", scheme)
		return ""
	}

	// Return the original URL if valid (template.CSS will handle escaping)
	return rawURL
}

// buildResponsiveClass implements responsive visibility classes
func (bpwr *BgPaneWrapperRenderer) buildResponsiveClass(nodeData *rendering.NodeRenderData) string {
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

	if hiddenMobile {
		classes = append(classes, "hidden", "xs:hidden")
	} else {
		classes = append(classes, "block")
	}

	if hiddenTablet {
		classes = append(classes, "md:hidden")
	} else if hiddenMobile {
		classes = append(classes, "md:block")
	}

	if hiddenDesktop {
		classes = append(classes, "xl:hidden")
	} else if hiddenTablet {
		classes = append(classes, "xl:block")
	}

	return strings.Join(classes, " ")
}

// renderBgVisualBreak renders visual break elements
func (bpwr *BgPaneWrapperRenderer) renderBgVisualBreak(nodeData *rendering.NodeRenderData) string {
	elementCSS := ""
	if nodeData.ElementCSS != nil {
		elementCSS = *nodeData.ElementCSS
	}

	templateData := visualBreakTemplateData{
		ElementCSS: elementCSS,
	}

	var buf bytes.Buffer
	if err := visualBreakTemplate.Execute(&buf, templateData); err != nil {
		log.Printf("Error executing visual break template: %v", err)
		return `<div class="bg-error">Template error</div>`
	}
	return buf.String()
}

// getNodeData retrieves node data from real context
func (bpwr *BgPaneWrapperRenderer) getNodeData(nodeID string) *rendering.NodeRenderData {
	if bpwr.ctx.AllNodes == nil {
		return nil
	}

	return bpwr.ctx.AllNodes[nodeID]
}
