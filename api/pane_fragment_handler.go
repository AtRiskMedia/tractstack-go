// Package api provides HTTP handlers for fragment rendering endpoints
package api

import (
	"fmt"
	"net/http"
	"strings"

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
	paneService := content.NewPaneService(ctx, nil)
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

	// Find root nodes (nodes whose parentId equals the pane ID)
	rootNodeIDs := findRootNodes(paneID, nodesData)
	if len(rootNodeIDs) == 0 {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, `<div class="pane-empty">No content nodes found</div>`)
		return
	}

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

	// Build pane wrapper and render all root nodes
	var htmlBuilder strings.Builder

	// Pane wrapper opening
	htmlBuilder.WriteString(fmt.Sprintf(`<div id="pane-%s" class="grid auto">`, paneID))

	// Render each root node
	for _, rootNodeID := range rootNodeIDs {
		nodeHTML := generator.Render(rootNodeID)
		htmlBuilder.WriteString(nodeHTML)
	}

	// Pane wrapper closing
	htmlBuilder.WriteString(`</div>`)

	// Return HTML response
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, htmlBuilder.String())
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

	// Extract markdownBody for Markdown nodes
	if markdownBody, ok := nodeMap["markdownBody"].(string); ok {
		nodeData.MarkdownBody = &markdownBody
	}

	// Extract href for link nodes
	if href, ok := nodeMap["href"].(string); ok {
		nodeData.Href = &href
	}

	// Extract target for link nodes
	if target, ok := nodeMap["target"].(string); ok {
		nodeData.Target = &target
	}

	// Extract altText for image nodes
	if altText, ok := nodeMap["altText"].(string); ok {
		nodeData.AltText = &altText
	}

	// Extract imageUrl for image nodes
	if imageURL, ok := nodeMap["imageUrl"].(string); ok {
		nodeData.ImageURL = &imageURL
	}

	// Extract parentCss array
	if parentCSSInterface, ok := nodeMap["parentCss"]; ok {
		if parentCSSArray, ok := parentCSSInterface.([]any); ok {
			parentCSS := make([]string, 0, len(parentCSSArray))
			for _, item := range parentCSSArray {
				if cssString, ok := item.(string); ok {
					parentCSS = append(parentCSS, cssString)
				}
			}
			nodeData.ParentCSS = parentCSS
		}
	}

	// Initialize empty children slice (will be populated by parent-child map)
	nodeData.Children = make([]string, 0)

	return nodeData, nil
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
