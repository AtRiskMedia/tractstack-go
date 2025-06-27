// Package api provides HTTP handlers for fragment rendering endpoints
package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/html"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/models/content"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/gin-gonic/gin"
)

// GetPaneFragmentHandler handles GET /api/v1/fragments/panes/{id}
// Returns rendered HTML for a specific pane using cache-first architecture
func GetPaneFragmentHandler(c *gin.Context) {
	paneID := c.Param("id")
	if paneID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pane ID is required"})
		return
	}

	// Extract tenant context from request using existing pattern
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Activate tenant if needed
	if ctx.Status == "inactive" {
		if err := tenant.ActivateTenant(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("tenant activation failed: %v", err)})
			return
		}
	}

	fmt.Printf("DEBUG: TenantID=%s, PaneID=%s\n", ctx.TenantID, paneID)

	// Use cache-first pane service with global cache manager - SAME AS WORKING HANDLERS
	paneService := content.NewPaneService(ctx, cache.GetGlobalManager())
	paneNode, err := paneService.GetByID(paneID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if paneNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pane not found"})
		return
	}

	fmt.Printf("DEBUG: Found pane: %s\n", paneNode.Title)

	// Extract and parse nodes from optionsPayload
	nodesData, parentChildMap, err := extractNodesFromPane(paneNode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to parse pane nodes: %v", err)})
		return
	}

	// Add the pane itself to the nodes data structure
	paneNodeData := &models.NodeRenderData{
		ID:       paneID,
		NodeType: "Pane",
		PaneData: &models.PaneRenderData{
			Title:        paneNode.Title,
			Slug:         paneNode.Slug,
			IsDecorative: paneNode.IsDecorative,
			BgColour:     extractBgColour(paneNode),
		},
	}
	nodesData[paneID] = paneNodeData

	// TODO: Extract user state from cookies/headers for belief-based variants
	// For now, use default variant
	userID := extractUserIDFromRequest(c)

	// Create render context with real data
	renderCtx := &models.RenderContext{
		AllNodes:    nodesData,
		ParentNodes: parentChildMap,
		TenantID:    ctx.TenantID,
		UserID:      userID,
	}

	// Create HTML generator
	generator := html.NewGenerator(renderCtx)

	// Use our PaneRenderer to render the complete pane structure
	// This will call our PaneRenderer.Render() which implements the complete Pane.astro logic
	paneHTML := generator.Render(paneID)

	// Return HTML response
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, paneHTML)
}

// extractUserIDFromRequest extracts user ID from request context, cookies, or headers
func extractUserIDFromRequest(c *gin.Context) string {
	// Try to get user ID from context first
	if userID, exists := c.Get("userID"); exists {
		if uid, ok := userID.(string); ok {
			return uid
		}
	}

	// Try to extract from cookies
	if cookie, err := c.Cookie("user_id"); err == nil && cookie != "" {
		return cookie
	}

	// Try to extract from headers
	if authHeader := c.GetHeader("Authorization"); authHeader != "" {
		// Simple bearer token extraction - adjust based on your auth system
		if strings.HasPrefix(authHeader, "Bearer ") {
			return strings.TrimPrefix(authHeader, "Bearer ")
		}
	}

	// Return empty string if no user ID found
	return ""
}

// Helper function to extract background color from pane options
func extractBgColour(paneNode *models.PaneNode) *string {
	if paneNode.OptionsPayload == nil {
		return nil
	}

	if bgColour, exists := paneNode.OptionsPayload["bgColour"]; exists {
		if bgColourStr, ok := bgColour.(string); ok {
			return &bgColourStr
		}
	}

	return nil
}

// findRootNodes finds nodes that are direct children of the pane
func findRootNodes(paneID string, nodesData map[string]*models.NodeRenderData) []string {
	var rootNodes []string

	for nodeID, nodeData := range nodesData {
		if nodeData.ParentID == paneID {
			rootNodes = append(rootNodes, nodeID)
		}
	}

	return rootNodes
}

// extractNodesFromPane parses the optionsPayload.nodes array and builds data structures
func extractNodesFromPane(paneNode *models.PaneNode) (map[string]*models.NodeRenderData, map[string][]string, error) {
	nodesData := make(map[string]*models.NodeRenderData)
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
func parseNodeFromMap(nodeMap map[string]any) (*models.NodeRenderData, error) {
	nodeData := &models.NodeRenderData{}

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
	if parentCSS, ok := nodeMap["parentCss"].([]interface{}); ok {
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

	// Handle BgPane specific fields
	if nodeData.NodeType == "BgPane" {
		bgImageData := &models.BackgroundImageData{}

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

		// Store additional fields in CustomData for viewport visibility
		nodeData.CustomData = make(map[string]any)
		if collection, ok := nodeMap["collection"]; ok {
			nodeData.CustomData["collection"] = collection
		}
		if image, ok := nodeMap["image"]; ok {
			nodeData.CustomData["image"] = image
		}
		if objectFit, ok := nodeMap["objectFit"]; ok {
			nodeData.CustomData["objectFit"] = objectFit
		}
		// Copy other fields that might be needed
		for key, value := range nodeMap {
			if key != "id" && key != "nodeType" && key != "type" && key != "position" && key != "size" {
				nodeData.CustomData[key] = value
			}
		}
	}

	return nodeData, nil
}
