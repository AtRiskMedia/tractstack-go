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

	// Check if optionsPayload exists and has nodes
	if paneNode.OptionsPayload == nil {
		return nodesData, parentChildMap, nil
	}

	// Extract nodes array from optionsPayload
	nodesInterface, exists := paneNode.OptionsPayload["nodes"]
	if !exists {
		return nodesData, parentChildMap, nil
	}

	// Convert to array of maps
	nodesArray, ok := nodesInterface.([]any)
	if !ok {
		return nodesData, parentChildMap, fmt.Errorf("nodes is not an array")
	}

	// Parse each node
	for _, nodeInterface := range nodesArray {
		nodeMap, ok := nodeInterface.(map[string]any)
		if !ok {
			continue
		}

		nodeData, err := parseNodeFromMap(nodeMap)
		if err != nil {
			continue // Skip invalid nodes rather than failing entirely
		}

		if nodeData.ID != "" {
			nodesData[nodeData.ID] = nodeData

			// Build parent-child relationships
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

	// Extract required fields
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

	// Extract optional fields
	if tagName, ok := nodeMap["tagName"].(string); ok {
		nodeData.TagName = &tagName
	}

	if copy, ok := nodeMap["copy"].(string); ok {
		nodeData.Copy = &copy
	}

	if elementCSS, ok := nodeMap["elementCss"].(string); ok {
		nodeData.ElementCSS = &elementCSS
	}

	if parentID, ok := nodeMap["parentId"].(string); ok {
		nodeData.ParentID = parentID
	}

	// Handle ParentCSS array
	if parentCSS, ok := nodeMap["parentCss"].([]any); ok {
		cssStrings := make([]string, 0, len(parentCSS))
		for _, css := range parentCSS {
			if cssStr, ok := css.(string); ok {
				cssStrings = append(cssStrings, cssStr)
			}
		}
		nodeData.ParentCSS = cssStrings
	}

	// Handle image-related fields
	if src, ok := nodeMap["src"].(string); ok {
		nodeData.ImageURL = &src
	}

	if srcSet, ok := nodeMap["srcSet"].(string); ok {
		nodeData.SrcSet = &srcSet
	}

	if alt, ok := nodeMap["alt"].(string); ok {
		nodeData.AltText = &alt
	}

	// Handle link fields
	if href, ok := nodeMap["href"].(string); ok {
		nodeData.Href = &href
	}

	if target, ok := nodeMap["target"].(string); ok {
		nodeData.Target = &target
	}

	// Handle codeHookParams array
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

	// Handle BgPane specific fields
	if nodeData.NodeType == "BgPane" {
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

	return nodeData, nil
}
