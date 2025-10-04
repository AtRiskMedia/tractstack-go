package templates

import (
	"encoding/json"
	"html/template"
	"log"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
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

	var htmlBuilder strings.Builder

	hxValsMap := parseActionLispToGoPayload(callbackPayloadStr)
	if hxValsMap == nil {
		log.Printf("ERROR: Failed to parse action Lisp for state change button: %q", callbackPayloadStr)
		return `<!-- error parsing action button -->`
	}

	hxValsBytes, err := json.Marshal(hxValsMap)
	if err != nil {
		log.Printf("ERROR: Failed to marshal hx-vals for action button: %v", err)
		return `<!-- error marshalling action button -->`
	}

	var cssClasses strings.Builder
	if nodeData.ElementCSS != nil && *nodeData.ElementCSS != "" {
		cssClasses.WriteString(*nodeData.ElementCSS)
	}
	cssClasses.WriteString(" whitespace-nowrap")

	buttonData := nodeActionButtonData{
		Class:  strings.TrimSpace(cssClasses.String()),
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

func parseActionLispToGoPayload(payload string) map[string]string {
	trimmed := strings.Trim(payload, "() ")
	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return nil
	}

	command := parts[0]
	innerPayload := strings.Trim(trimmed[len(command):], "() ")
	params := strings.Fields(innerPayload)

	if len(params) < 2 {
		return nil
	}

	beliefId := params[0]
	value := params[1]

	switch command {
	case "declare":
		return map[string]string{
			"beliefId":    beliefId,
			"beliefType":  "Belief",
			"beliefValue": value,
		}
	case "identifyAs":
		return map[string]string{
			"beliefId":     beliefId,
			"beliefType":   "Belief",
			"beliefVerb":   "IDENTIFY_AS",
			"beliefObject": value,
		}
	}

	return nil
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
