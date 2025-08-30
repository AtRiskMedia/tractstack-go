package templates

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/url"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/shapes"
)

type BgPaneWrapperRenderer struct {
	ctx *rendering.RenderContext
}

func NewBgPaneWrapperRenderer(ctx *rendering.RenderContext) *BgPaneWrapperRenderer {
	return &BgPaneWrapperRenderer{ctx: ctx}
}

type bgImageTemplateData struct {
	Src             string
	SrcSet          string
	ObjectFit       string
	ResponsiveClass string
	AltText         string
}

type bgDivTemplateData struct {
	ResponsiveClass string
	BackgroundURL   template.CSS
	ObjectFit       string
	AltText         string
}

type visualBreakTemplateData struct {
	HasShape        bool
	ShapeName       string
	ViewportKey     string
	ID              string
	SvgFill         string
	ResponsiveClass string
	ViewBoxWidth    int
	ViewBoxHeight   int
	Path            template.HTML
}

var (
	bgImageTemplate = template.Must(template.New("bgImage").Parse(`<img src="{{.Src}}"{{if .SrcSet}} srcset="{{.SrcSet}}"{{end}} class="w-full h-full object-{{.ObjectFit}} {{.ResponsiveClass}}" alt="{{.AltText}}" />`))

	bgDivTemplate = template.Must(template.New("bgDiv").Parse(`<div class="w-full h-full absolute top-0 left-0 {{.ResponsiveClass}}" style="{{.BackgroundURL}}background-size:{{.ObjectFit}};background-repeat:no-repeat;background-position:center;z-index:0" role="img" aria-label="{{.AltText}}"></div>`))

	visualBreakTemplate = template.Must(template.New("visualBreak").Parse(`{{if .HasShape}}<div class="grid {{.ResponsiveClass}}" style="fill: {{.SvgFill}};">
  <svg id="svg__{{.ID}}" 
       data-name="svg__{{.ShapeName}}--{{.ViewportKey}}" 
       xmlns="http://www.w3.org/2000/svg" 
       viewBox="0 0 {{.ViewBoxWidth}} {{.ViewBoxHeight}}" 
       class="svg svg__{{.ShapeName}} svg__{{.ShapeName}}--{{.ViewportKey}}">
    <desc>decorative background</desc>
    <g>
      <path d="{{.Path}}" />
    </g>
  </svg>
</div>{{else}}<div class="visual-break-placeholder">Shape not found: {{.ShapeName}}</div>{{end}}`))
)

func (bpwr *BgPaneWrapperRenderer) Render(nodeID string) string {
	nodeData := bpwr.getNodeData(nodeID)
	if nodeData == nil {
		return `<div class="bg-error">Missing background node</div>`
	}

	if bpwr.isBgImageNode(nodeData) || bpwr.isArtpackImageNode(nodeData) {
		return bpwr.renderBgImage(nodeData)
	} else {
		return bpwr.renderBgVisualBreak(nodeData)
	}
}

func (bpwr *BgPaneWrapperRenderer) isBgImageNode(nodeData *rendering.NodeRenderData) bool {
	return nodeData.BgImageData != nil && nodeData.BgImageData.Type == "background-image"
}

func (bpwr *BgPaneWrapperRenderer) isArtpackImageNode(nodeData *rendering.NodeRenderData) bool {
	return nodeData.BgImageData != nil && nodeData.BgImageData.Type == "artpack-image"
}

func (bpwr *BgPaneWrapperRenderer) renderBgImage(nodeData *rendering.NodeRenderData) string {
	if nodeData.BgImageData == nil {
		return `<div class="bg-error">Missing image data</div>`
	}

	bgData := nodeData.BgImageData
	isFlexImage := bgData.Position == "left" || bgData.Position == "right"
	responsiveClass := bpwr.buildResponsiveClass(nodeData)

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

	objectFit := "cover"

	if isFlexImage {
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
		backgroundCSS := ""
		if src != "" {
			if sanitizedURL := bpwr.sanitizeBackgroundURL(src); sanitizedURL != "" {
				backgroundCSS = fmt.Sprintf("background-image:url(%s);", sanitizedURL)
			}
		}

		templateData := bgDivTemplateData{
			ResponsiveClass: responsiveClass,
			BackgroundURL:   template.CSS(backgroundCSS),
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

func (bpwr *BgPaneWrapperRenderer) sanitizeBackgroundURL(rawURL string) string {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		log.Printf("Invalid background URL: %s, error: %v", rawURL, err)
		return ""
	}

	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "" && scheme != "http" && scheme != "https" {
		log.Printf("Blocked unsafe URL scheme: %s", scheme)
		return ""
	}

	return rawURL
}

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

func (bpwr *BgPaneWrapperRenderer) renderBgVisualBreak(nodeData *rendering.NodeRenderData) string {
	if nodeData.VisualBreakData == nil {
		return `<div class="bg-error">Missing visual break data</div>`
	}

	var html strings.Builder
	viewports := []string{"mobile", "tablet", "desktop"}

	for _, viewport := range viewports {
		vbData := nodeData.VisualBreakData.GetViewportData(viewport)
		if vbData == nil {
			continue
		}

		shapeName := vbData.Collection + vbData.Image
		shape, exists := shapes.GetShape(shapeName)
		if !exists {
			continue
		}

		templateData := visualBreakTemplateData{
			HasShape:        true,
			ShapeName:       shapeName,
			ViewportKey:     viewport,
			ID:              fmt.Sprintf("%s-%s", viewport, shapeName),
			SvgFill:         vbData.SvgFill,
			ResponsiveClass: bpwr.getViewportClass(viewport),
			ViewBoxWidth:    shape.ViewBox[0],
			ViewBoxHeight:   shape.ViewBox[1],
			Path:            template.HTML(shape.Path),
		}

		var buf bytes.Buffer
		if err := visualBreakTemplate.Execute(&buf, templateData); err != nil {
			log.Printf("Error executing visual break template: %v", err)
			continue
		}
		html.WriteString(buf.String())
	}

	if html.Len() == 0 {
		return `<div class="visual-break-placeholder">No valid visual break shapes found</div>`
	}

	return html.String()
}

func (bpwr *BgPaneWrapperRenderer) getViewportClass(viewport string) string {
	switch viewport {
	case "mobile":
		return "md:hidden"
	case "tablet":
		return "hidden md:block xl:hidden"
	case "desktop":
		return "hidden xl:block"
	default:
		return ""
	}
}

func (bpwr *BgPaneWrapperRenderer) getNodeData(nodeID string) *rendering.NodeRenderData {
	if bpwr.ctx.AllNodes == nil {
		return nil
	}
	return bpwr.ctx.AllNodes[nodeID]
}
