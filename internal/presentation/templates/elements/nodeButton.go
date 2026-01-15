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

var playIconSVG = `<svg viewBox="0 0 459 459" xmlns="http://www.w3.org/2000/svg" preserveAspectRatio="xMidYMid meet" class="inline-block" style="height:1em; margin-left: 0.25em;"><path d="M229.5,0C102.751,0,0,102.751,0,229.5S102.751,459,229.5,459S459,356.249,459,229.5S356.249,0,229.5,0z M310.292,239.651 l-111.764,76.084c-3.761,2.56-8.63,2.831-12.652,0.704c-4.022-2.128-6.538-6.305-6.538-10.855V153.416 c0-4.55,2.516-8.727,6.538-10.855c4.022-2.127,8.891-1.857,12.652,0.704l111.764,76.084c3.359,2.287,5.37,6.087,5.37,10.151 C315.662,233.564,313.652,237.364,310.292,239.651z" fill="currentColor"></path></svg>`

type nodeActionButtonData struct {
	Class  string
	HxVals template.HTML
}

// NodeButtonRenderer handles the rendering of interactive button nodes, including Lisp action parsing.
type NodeButtonRenderer struct {
	ctx          *rendering.RenderContext
	nodeRenderer NodeRenderer
}

// NewNodeButtonRenderer creates a new renderer for button nodes.
func NewNodeButtonRenderer(ctx *rendering.RenderContext, nodeRenderer NodeRenderer) *NodeButtonRenderer {
	return &NodeButtonRenderer{
		ctx:          ctx,
		nodeRenderer: nodeRenderer,
	}
}

// Render generates the HTML for a button node, handling client-side events or HTMX attributes.
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

	parsedAction := lisp.Parse(callbackPayloadStr, nbr.ctx.ContainingPaneID, "")

	if !parsedAction.IsValid {
		log.Printf("WARN: nodeButton.go failed to get valid action from parser for payload: %q", callbackPayloadStr)
		return fmt.Sprintf(`<button class="%s" disabled></button>`, nbr.getClasses(nodeData))
	}

	var htmlBuilder strings.Builder

	if parsedAction.IsClientSideEvent {
		payloadBytes, err := json.Marshal(parsedAction.ClientSidePayload)
		if err != nil {
			log.Printf("ERROR: Failed to marshal client-side payload for nodeButton: %v", err)
			return ``
		}

		onclickVal := fmt.Sprintf(`document.dispatchEvent(new CustomEvent('%s', { bubbles: true, detail: %s }))`,
			parsedAction.ClientSideEventName,
			string(payloadBytes),
		)

		htmlBuilder.WriteString(fmt.Sprintf(
			`<button class="%s" onclick="%s">`,
			nbr.getClasses(nodeData),
			template.HTMLEscapeString(onclickVal),
		))

	} else {
		if parsedAction.HxVals == nil {
			log.Printf("WARN: nodeButton.go got a valid server-side action but HxVals was nil for payload: %q", callbackPayloadStr)
			return fmt.Sprintf(`<button class="%s" disabled></button>`, nbr.getClasses(nodeData))
		}

		hxValsBytes, err := json.Marshal(parsedAction.HxVals)
		if err != nil {
			log.Printf("ERROR: Failed to marshal hx-vals for nodeButton: %v", err)
			return ``
		}

		buttonData := nodeActionButtonData{
			Class:  nbr.getClasses(nodeData),
			HxVals: template.HTML(hxValsBytes),
		}

		err = nodeActionButtonTmpl.Execute(&htmlBuilder, buttonData)
		if err != nil {
			log.Printf("ERROR: Failed to execute nodeActionButton template for nodeID %s: %v", nodeID, err)
			return ``
		}
	}

	childNodeIDs := nbr.nodeRenderer.GetChildNodeIDs(nodeID)
	if len(childNodeIDs) > 0 {
		// htmlBuilder.WriteString(`<span class="whitespace-nowrap">`)
		for _, childID := range childNodeIDs {
			htmlBuilder.WriteString(nbr.nodeRenderer.RenderNode(childID))
		}
		// htmlBuilder.WriteString(`</span>`)
	}

	if parsedAction.IsClientSideEvent && parsedAction.ClientSideEventName == "update-video" {
		htmlBuilder.WriteString(playIconSVG)
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
	// cssClasses.WriteString(" whitespace-nowrap")
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
