package templates

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/lisp"
)

var nodeActionButtonTmpl = template.Must(template.New("nodeActionButton").Parse(
	`<button class="{{.Class}}" hx-post="/api/v1/state" hx-trigger="click" hx-swap="none" hx-vals='{{.HxVals}}'>`,
))

type nodeActionButtonData struct {
	Class  string
	HxVals template.HTML
}

type NodeButtonRenderer struct {
	ctx          *rendering.RenderContext
	nodeRenderer NodeRenderer
}

func NewNodeButtonRenderer(ctx *rendering.RenderContext, nodeRenderer NodeRenderer) *NodeButtonRenderer {
	return &NodeButtonRenderer{
		ctx:          ctx,
		nodeRenderer: nodeRenderer,
	}
}

func (nbr *NodeButtonRenderer) Render(nodeID string) string {
	nodeData := nbr.getNodeData(nodeID)
	if nodeData == nil {
		return `<button>missing button</button>`
	}

	var callbackPayloadStr string
	if nodeData.CustomData != nil {
		if callbackPayload, exists := nodeData.CustomData["callbackPayload"]; exists {
			if payload, ok := callbackPayload.(string); ok && payload != "" {
				callbackPayloadStr = payload
			}
		}
	}

	// Use the new centralized parser. A homeSlug is not needed as nodeButton only handles actions.
	parsedAction := lisp.Parse(callbackPayloadStr, nbr.ctx.ContainingPaneID, "")

	if !parsedAction.IsValid || parsedAction.HxVals == nil {
		log.Printf("WARN: nodeButton.go failed to get valid HxVals from parser for payload: %q", callbackPayloadStr)
		// Render a disabled button to prevent errors
		return fmt.Sprintf(`<button class="%s" disabled>`, nbr.getClasses(nodeData))
	}

	hxValsBytes, err := json.Marshal(parsedAction.HxVals)
	if err != nil {
		log.Printf("ERROR: Failed to marshal hx-vals for nodeButton: %v", err)
		return `<!-- error marshalling node button -->`
	}

	var htmlBuilder strings.Builder
	buttonData := nodeActionButtonData{
		Class:  nbr.getClasses(nodeData),
		HxVals: template.HTML(hxValsBytes),
	}

	err = nodeActionButtonTmpl.Execute(&htmlBuilder, buttonData)
	if err != nil {
		log.Printf("ERROR: Failed to execute nodeActionButton template for nodeID %s: %v", nodeID, err)
		return `<!-- error rendering action button -->`
	}

	childNodeIDs := nbr.nodeRenderer.GetChildNodeIDs(nodeID)
	if len(childNodeIDs) > 0 {
		htmlBuilder.WriteString(`<span class="whitespace-nowrap">`)
		for _, childID := range childNodeIDs {
			htmlBuilder.WriteString(nbr.nodeRenderer.RenderNode(childID))
		}
		htmlBuilder.WriteString(`</span>`)
	}

	htmlBuilder.WriteString(`</button>`)

	if nbr.checkNeedsTrailingSpace(nodeID) {
		htmlBuilder.WriteString(" ")
	}

	return htmlBuilder.String()
}

func (nbr *NodeButtonRenderer) getClasses(nodeData *rendering.NodeRenderData) string {
	var cssClasses strings.Builder
	if nodeData.ElementCSS != nil && *nodeData.ElementCSS != "" {
		cssClasses.WriteString(*nodeData.ElementCSS)
	}
	cssClasses.WriteString(" whitespace-nowrap")
	return strings.TrimSpace(cssClasses.String())
}

func (nbr *NodeButtonRenderer) checkNeedsTrailingSpace(nodeID string) bool {
	nodeData := nbr.getNodeData(nodeID)
	if nodeData == nil || nodeData.ParentID == "" {
		return false
	}
	parentID := nodeData.ParentID
	childNodeIDs := nbr.nodeRenderer.GetChildNodeIDs(parentID)
	currentIndex := -1
	for i, childID := range childNodeIDs {
		if childID == nodeID {
			currentIndex = i
			break
		}
	}
	if currentIndex == -1 || currentIndex >= len(childNodeIDs)-1 {
		return false
	}
	nextNodeID := childNodeIDs[currentIndex+1]
	nextNodeData := nbr.getNodeData(nextNodeID)
	if nextNodeData == nil {
		return false
	}
	if nextNodeData.TagName != nil && *nextNodeData.TagName == "text" {
		if nextNodeData.Copy != nil {
			text := *nextNodeData.Copy
			return !strings.HasPrefix(text, ".") && !strings.HasPrefix(text, ",") && !strings.HasPrefix(text, ";") && !strings.HasPrefix(text, ":")
		}
	}
	return false
}

func (nbr *NodeButtonRenderer) getNodeData(nodeID string) *rendering.NodeRenderData {
	if nbr.ctx.AllNodes == nil {
		return nil
	}
	return nbr.ctx.AllNodes[nodeID]
}
