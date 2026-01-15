// Package templates provides node rendering functionality for nodes-compositor
package templates

import (
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
)

// ExtractNodesFromPane parses the optionsPayload.nodes array and builds data structures
func ExtractNodesFromPane(paneNode *content.PaneNode) (map[string]*rendering.NodeRenderData, map[string][]string, error) {
	nodesData := make(map[string]*rendering.NodeRenderData)
	parentChildMap := make(map[string][]string)

	if paneNode.OptionsPayload == nil {
		return nodesData, parentChildMap, nil
	}

	nodesInterface, exists := paneNode.OptionsPayload["nodes"]
	if !exists {
		return nodesData, parentChildMap, nil
	}

	nodesArray, ok := nodesInterface.([]any)
	if !ok {
		return nodesData, parentChildMap, fmt.Errorf("nodes is not an array")
	}

	for _, nodeInterface := range nodesArray {
		nodeMap, ok := nodeInterface.(map[string]any)
		if !ok {
			continue
		}

		nodeData, err := parseNodeFromMap(nodeMap)
		if err != nil {
			continue
		}

		if nodeData.ID != "" {
			nodesData[nodeData.ID] = nodeData

			if nodeData.ParentID != "" {
				if parentChildMap[nodeData.ParentID] == nil {
					parentChildMap[nodeData.ParentID] = make([]string, 0)
				}
				parentChildMap[nodeData.ParentID] = append(parentChildMap[nodeData.ParentID], nodeData.ID)
			}
		}
	}

	return nodesData, parentChildMap, nil
}

// parseNodeFromMap converts a map[string]any to NodeRenderData
func parseNodeFromMap(nodeMap map[string]any) (*rendering.NodeRenderData, error) {
	nodeData := &rendering.NodeRenderData{}

	if id, ok := nodeMap["id"].(string); ok {
		nodeData.ID = id
	} else {
		return nil, fmt.Errorf("missing or invalid node id")
	}

	if nodeType, ok := nodeMap["nodeType"].(string); ok {
		nodeData.NodeType = nodeType
	} else {
		return nil, fmt.Errorf("missing or invalid nodeType")
	}

	if tagName, ok := nodeMap["tagName"].(string); ok {
		nodeData.TagName = &tagName
	}

	if copyVal, ok := nodeMap["copy"].(string); ok {
		nodeData.Copy = &copyVal
	}

	if elementCSS, ok := nodeMap["elementCss"].(string); ok {
		nodeData.ElementCSS = &elementCSS
	}

	if gridCSS, ok := nodeMap["gridCss"].(string); ok {
		nodeData.GridCSS = gridCSS
	}

	if parentID, ok := nodeMap["parentId"].(string); ok {
		nodeData.ParentID = parentID
	}

	if parentCSS, ok := nodeMap["parentCss"].([]any); ok {
		cssStrings := make([]string, 0, len(parentCSS))
		for _, css := range parentCSS {
			if cssStr, ok := css.(string); ok {
				cssStrings = append(cssStrings, cssStr)
			}
		}
		nodeData.ParentCSS = cssStrings
	}

	// Parse responsive visibility flags for all applicable nodes
	if hidden, ok := nodeMap["hiddenViewportMobile"].(bool); ok {
		nodeData.HiddenViewportMobile = hidden
	}
	if hidden, ok := nodeMap["hiddenViewportTablet"].(bool); ok {
		nodeData.HiddenViewportTablet = hidden
	}
	if hidden, ok := nodeMap["hiddenViewportDesktop"].(bool); ok {
		nodeData.HiddenViewportDesktop = hidden
	}

	if buttonPayload, ok := nodeMap["buttonPayload"].(map[string]any); ok {
		if callbackPayload, ok := buttonPayload["callbackPayload"].(string); ok && callbackPayload != "" {
			if nodeData.CustomData == nil {
				nodeData.CustomData = make(map[string]any)
			}
			nodeData.CustomData["callbackPayload"] = callbackPayload
		}
		if isExternalURL, ok := buttonPayload["isExternalUrl"].(bool); ok && isExternalURL {
			if nodeData.CustomData == nil {
				nodeData.CustomData = make(map[string]any)
			}
			nodeData.CustomData["isExternalUrl"] = isExternalURL
		}
	}

	if src, ok := nodeMap["src"].(string); ok {
		nodeData.ImageURL = &src
	}

	if srcSet, ok := nodeMap["srcSet"].(string); ok {
		nodeData.SrcSet = &srcSet
	}

	if alt, ok := nodeMap["alt"].(string); ok {
		nodeData.AltText = &alt
	}

	if href, ok := nodeMap["href"].(string); ok {
		nodeData.Href = &href
	}

	if target, ok := nodeMap["target"].(string); ok {
		nodeData.Target = &target
	}

	if codeHookParams, ok := nodeMap["codeHookParams"].([]any); ok {
		params := make([]string, 0, len(codeHookParams))
		for _, param := range codeHookParams {
			if paramStr, ok := param.(string); ok {
				params = append(params, paramStr)
			}
		}
		if nodeData.CustomData == nil {
			nodeData.CustomData = make(map[string]any)
		}
		nodeData.CustomData["codeHookParams"] = params
	}

	if wordCarouselPayload, ok := nodeMap["wordCarouselPayload"].(map[string]any); ok {
		if nodeData.CustomData == nil {
			nodeData.CustomData = make(map[string]any)
		}
		nodeData.CustomData["wordCarouselPayload"] = wordCarouselPayload
	}

	if nodeData.NodeType == "BgPane" {
		if nodeType, ok := nodeMap["type"].(string); ok && nodeType == "visual-break" {
			visualBreakNode := &rendering.VisualBreakNode{}

			if breakDesktop, ok := nodeMap["breakDesktop"].(map[string]any); ok {
				visualBreakNode.BreakDesktop = &rendering.VisualBreakData{
					Collection: getStringValue(breakDesktop, "collection"),
					Image:      getStringValue(breakDesktop, "image"),
					SvgFill:    getStringValue(breakDesktop, "svgFill"),
				}
			}

			if breakTablet, ok := nodeMap["breakTablet"].(map[string]any); ok {
				visualBreakNode.BreakTablet = &rendering.VisualBreakData{
					Collection: getStringValue(breakTablet, "collection"),
					Image:      getStringValue(breakTablet, "image"),
					SvgFill:    getStringValue(breakTablet, "svgFill"),
				}
			}

			if breakMobile, ok := nodeMap["breakMobile"].(map[string]any); ok {
				visualBreakNode.BreakMobile = &rendering.VisualBreakData{
					Collection: getStringValue(breakMobile, "collection"),
					Image:      getStringValue(breakMobile, "image"),
					SvgFill:    getStringValue(breakMobile, "svgFill"),
				}
			}
			// Note: hiddenViewport flags are parsed globally above now
			nodeData.VisualBreakData = visualBreakNode
		} else {
			bgImageData := &rendering.BackgroundImageData{}

			if nodeType, ok := nodeMap["type"].(string); ok {
				bgImageData.Type = nodeType
			}

			if position, ok := nodeMap["position"].(string); ok {
				bgImageData.Position = position
			}

			if size, ok := nodeMap["size"].(string); ok {
				bgImageData.Size = size
			}

			nodeData.BgImageData = bgImageData
		}
	}

	return nodeData, nil
}

func getStringValue(m map[string]any, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}
